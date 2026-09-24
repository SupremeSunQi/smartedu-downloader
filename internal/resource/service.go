package resource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"smartedu-downloader/internal/catalog"
)

const maxDetailBytes = 16 << 20

var (
	contentIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	md5Pattern       = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type URLPolicy func(*url.URL) bool

func ProductionURLPolicy(candidate *url.URL) bool {
	if candidate == nil || candidate.Scheme != "https" {
		return false
	}
	switch strings.ToLower(candidate.Hostname()) {
	case "r1-ndr-private.ykt.cbern.com.cn", "r2-ndr-private.ykt.cbern.com.cn", "r3-ndr-private.ykt.cbern.com.cn":
		return true
	default:
		return false
	}
}

type Service struct {
	client   HTTPClient
	baseURL  string
	cacheDir string
	policy   URLPolicy
}

func NewService(client HTTPClient, baseURL, cacheDir string, policy URLPolicy) *Service {
	return &Service{client: client, baseURL: strings.TrimRight(baseURL, "/"), cacheDir: cacheDir, policy: policy}
}

type detailDocument struct {
	ID    string       `json:"id"`
	Items []detailItem `json:"ti_items"`
}

type detailItem struct {
	MD5          string   `json:"ti_md5"`
	Size         int64    `json:"ti_size"`
	Storage      string   `json:"ti_storage"`
	Storages     []string `json:"ti_storages"`
	FileFlag     string   `json:"ti_file_flag"`
	IsSourceFile bool     `json:"ti_is_source_file"`
	Format       string   `json:"ti_format"`
}

func (service *Service) Resolve(ctx context.Context, book catalog.Textbook) (PDFSource, error) {
	if service.client == nil || service.policy == nil {
		return PDFSource{}, errors.New("resource service is not configured")
	}
	if !contentIDPattern.MatchString(book.ID) {
		return PDFSource{}, errors.New("textbook ID is invalid")
	}

	document, payload, networkErr := service.fetch(ctx, book.ID)
	if networkErr == nil {
		source, err := service.selectSource(book, document)
		if err != nil {
			return PDFSource{}, err
		}
		if err := service.saveCache(book.ID, payload); err != nil {
			return PDFSource{}, err
		}
		return source, nil
	}

	cached, cacheErr := service.loadCache(book.ID)
	if cacheErr != nil {
		return PDFSource{}, errors.Join(networkErr, cacheErr)
	}
	source, parseErr := service.selectSource(book, cached)
	if parseErr != nil {
		return PDFSource{}, errors.Join(networkErr, parseErr)
	}
	return source, nil
}

func (service *Service) fetch(ctx context.Context, contentID string) (detailDocument, []byte, error) {
	base, err := url.Parse(service.baseURL)
	if err != nil {
		return detailDocument{}, nil, fmt.Errorf("parse detail base URL: %w", err)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/ndrv2/resources/tch_material/details/" + url.PathEscape(contentID) + ".json"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return detailDocument{}, nil, fmt.Errorf("create detail request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := service.client.Do(request)
	if err != nil {
		return detailDocument{}, nil, fmt.Errorf("fetch textbook detail: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return detailDocument{}, nil, fmt.Errorf("detail endpoint returned HTTP %d", response.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxDetailBytes+1))
	if err != nil {
		return detailDocument{}, nil, fmt.Errorf("read textbook detail: %w", err)
	}
	if len(payload) > maxDetailBytes {
		return detailDocument{}, nil, errors.New("textbook detail exceeds size limit")
	}
	var document detailDocument
	if err := json.Unmarshal(payload, &document); err != nil {
		return detailDocument{}, nil, fmt.Errorf("decode textbook detail: %w", err)
	}
	return document, payload, nil
}

func (service *Service) selectSource(book catalog.Textbook, document detailDocument) (PDFSource, error) {
	var selected *detailItem
	for index := range document.Items {
		item := &document.Items[index]
		if strings.EqualFold(item.Format, "pdf") && item.IsSourceFile && item.FileFlag == "source" {
			if selected != nil {
				return PDFSource{}, errors.New("textbook detail contains multiple source PDFs")
			}
			selected = item
		}
	}
	if selected == nil {
		return PDFSource{}, errors.New("textbook detail contains no source PDF")
	}
	if selected.Size <= 0 || !md5Pattern.MatchString(selected.MD5) {
		return PDFSource{}, errors.New("source PDF metadata is invalid")
	}

	mirrors := make([]string, 0, len(selected.Storages))
	seen := make(map[string]struct{}, len(selected.Storages))
	for _, value := range selected.Storages {
		parsed, err := url.Parse(strings.TrimSpace(value))
		if err != nil || !service.policy(parsed) {
			return PDFSource{}, fmt.Errorf("source PDF mirror is not allowed: %q", value)
		}
		normalized := parsed.String()
		if _, exists := seen[normalized]; !exists {
			seen[normalized] = struct{}{}
			mirrors = append(mirrors, normalized)
		}
	}
	if len(mirrors) == 0 {
		return PDFSource{}, errors.New("source PDF contains no mirrors")
	}
	return PDFSource{
		BookID:       book.ID,
		Filename:     SafeFilename(book.Title),
		ExpectedSize: selected.Size,
		MD5:          strings.ToLower(selected.MD5),
		Mirrors:      mirrors,
	}, nil
}

func (service *Service) saveCache(contentID string, payload []byte) (err error) {
	if err := os.MkdirAll(service.cacheDir, 0o700); err != nil {
		return fmt.Errorf("create detail cache directory: %w", err)
	}
	path := service.cachePath(contentID)
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create detail cache: %w", err)
	}
	defer func() {
		closeErr := file.Close()
		removeErr := os.Remove(temporary)
		if errors.Is(closeErr, os.ErrClosed) {
			closeErr = nil
		}
		if errors.Is(removeErr, os.ErrNotExist) {
			removeErr = nil
		}
		err = errors.Join(err, closeErr, removeErr)
	}()
	if _, err := file.Write(payload); err != nil {
		return fmt.Errorf("write detail cache: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync detail cache: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close detail cache: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("replace detail cache: %w", err)
	}
	return nil
}

func (service *Service) loadCache(contentID string) (detailDocument, error) {
	payload, err := os.ReadFile(service.cachePath(contentID))
	if err != nil {
		return detailDocument{}, fmt.Errorf("read cached textbook detail: %w", err)
	}
	var document detailDocument
	if err := json.Unmarshal(payload, &document); err != nil {
		return detailDocument{}, fmt.Errorf("decode cached textbook detail: %w", err)
	}
	return document, nil
}

func (service *Service) cachePath(contentID string) string {
	return filepath.Join(service.cacheDir, contentID+".json")
}
