package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Service struct {
	client   HTTPClient
	baseURL  string
	cacheDir string
	policy   URLPolicy
	now      func() time.Time
}

func NewService(client HTTPClient, baseURL, cacheDir string, policy URLPolicy) *Service {
	return &Service{client: client, baseURL: strings.TrimRight(baseURL, "/"), cacheDir: cacheDir, policy: policy, now: time.Now}
}

type remoteResource struct {
	ID               string                 `json:"id"`
	Title            string                 `json:"title"`
	GlobalTitle      map[string]string      `json:"global_title"`
	CustomProperties remoteCustomProperties `json:"custom_properties"`
	ProviderList     []remoteProvider       `json:"provider_list"`
	TagList          []remoteTag            `json:"tag_list"`
}

type remoteCustomProperties struct {
	Size       int64    `json:"size"`
	Thumbnails []string `json:"thumbnails"`
}

type remoteProvider struct {
	Name string `json:"name"`
}

type remoteTag struct {
	ID          string `json:"tag_id"`
	Name        string `json:"tag_name"`
	DimensionID string `json:"tag_dimension_id"`
}

func (service *Service) Sync(ctx context.Context) (Snapshot, error) {
	if service.client == nil || service.policy == nil {
		return Snapshot{}, errors.New("catalog service is not configured")
	}
	tagsURL, err := resolveEndpoint(service.baseURL, TagsPath)
	if err != nil {
		return Snapshot{}, err
	}
	var tags json.RawMessage
	if err := fetchJSON(ctx, service.client, tagsURL, maxTagsBytes, &tags); err != nil {
		return Snapshot{}, fmt.Errorf("sync catalog tags: %w", err)
	}
	if len(tags) == 0 || string(tags) == "null" {
		return Snapshot{}, errors.New("catalog tags are empty")
	}

	versionURL, err := resolveEndpoint(service.baseURL, VersionPath)
	if err != nil {
		return Snapshot{}, err
	}
	var version versionDocument
	if err := fetchJSON(ctx, service.client, versionURL, maxVersionBytes, &version); err != nil {
		return Snapshot{}, fmt.Errorf("sync catalog version: %w", err)
	}
	if version.ModuleVersion <= 0 {
		return Snapshot{}, errors.New("catalog version is invalid")
	}
	partURLs := splitPartURLs(version.URLs)
	if len(partURLs) == 0 {
		return Snapshot{}, errors.New("catalog version contains no parts")
	}

	resources := make([]remoteResource, 0, 1024)
	for _, address := range partURLs {
		parsed, parseErr := url.Parse(address)
		if parseErr != nil || !service.policy(parsed) {
			return Snapshot{}, fmt.Errorf("catalog part URL is not allowed: %q", address)
		}
		var part []remoteResource
		if err := fetchJSON(ctx, service.client, parsed.String(), maxPartBytes, &part); err != nil {
			return Snapshot{}, fmt.Errorf("sync catalog part: %w", err)
		}
		resources = append(resources, part...)
	}

	books := make([]Textbook, 0, len(resources))
	seen := make(map[string]struct{}, len(resources))
	for _, resource := range resources {
		book, normalizeErr := normalizeResource(resource)
		if normalizeErr != nil {
			return Snapshot{}, normalizeErr
		}
		if _, exists := seen[book.ID]; exists {
			return Snapshot{}, fmt.Errorf("duplicate textbook ID %q", book.ID)
		}
		seen[book.ID] = struct{}{}
		books = append(books, book)
	}
	sort.SliceStable(books, func(i, j int) bool {
		if books[i].Title == books[j].Title {
			return books[i].ID < books[j].ID
		}
		return books[i].Title < books[j].Title
	})

	snapshot := Snapshot{
		Version:    version.ModuleVersion,
		SyncedAt:   service.now().UTC(),
		Textbooks:  books,
		Filters:    buildFilters(books),
		SourceTags: append([]byte(nil), tags...),
	}
	if err := service.save(snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (service *Service) LoadCached() (Snapshot, error) {
	path := service.cachePath()
	payload, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read catalog cache: %w", err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil || snapshot.Version <= 0 || snapshot.Textbooks == nil {
		quarantineErr := service.quarantine(path)
		return Snapshot{}, errors.Join(errors.New("catalog cache is corrupt"), err, quarantineErr)
	}
	return snapshot, nil
}

func (service *Service) save(snapshot Snapshot) (err error) {
	if err := os.MkdirAll(service.cacheDir, 0o700); err != nil {
		return fmt.Errorf("create catalog cache directory: %w", err)
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode catalog cache: %w", err)
	}
	temporary := service.cachePath() + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create catalog cache: %w", err)
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
		return fmt.Errorf("write catalog cache: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync catalog cache: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close catalog cache: %w", err)
	}
	if err := os.Rename(temporary, service.cachePath()); err != nil {
		return fmt.Errorf("replace catalog cache: %w", err)
	}
	return nil
}

func (service *Service) quarantine(path string) error {
	stamp := service.now().UTC().Format("20060102T150405.000000000")
	destination := filepath.Join(service.cacheDir, "catalog.corrupt-"+stamp+".json")
	if err := os.Rename(path, destination); err != nil {
		return fmt.Errorf("quarantine catalog cache: %w", err)
	}
	return nil
}

func (service *Service) cachePath() string {
	return filepath.Join(service.cacheDir, "catalog.json")
}

func normalizeResource(resource remoteResource) (Textbook, error) {
	resource.ID = strings.TrimSpace(resource.ID)
	title := strings.TrimSpace(resource.Title)
	if title == "" {
		title = strings.TrimSpace(resource.GlobalTitle["zh-CN"])
	}
	if resource.ID == "" || title == "" || resource.CustomProperties.Size < 0 {
		return Textbook{}, errors.New("catalog resource is missing required fields")
	}
	book := Textbook{ID: resource.ID, Title: title, Size: resource.CustomProperties.Size}
	if len(resource.ProviderList) > 0 {
		book.Provider = strings.TrimSpace(resource.ProviderList[0].Name)
	}
	if len(resource.CustomProperties.Thumbnails) > 0 {
		book.ThumbnailURL = strings.TrimSpace(resource.CustomProperties.Thumbnails[0])
	}
	for _, tag := range resource.TagList {
		switch tag.DimensionID {
		case dimensionStage:
			book.StageID, book.Stage = tag.ID, tag.Name
		case dimensionSubject:
			book.SubjectID, book.Subject = tag.ID, tag.Name
		case dimensionEdition:
			book.EditionID, book.Edition = tag.ID, tag.Name
		case dimensionGrade:
			book.GradeID, book.Grade = tag.ID, tag.Name
		case dimensionVolume:
			book.VolumeID, book.Volume = tag.ID, tag.Name
		}
	}
	return book, nil
}

func buildFilters(books []Textbook) Filters {
	return Filters{
		Stages:   collectOptions(books, func(book Textbook) (string, string) { return book.StageID, book.Stage }),
		Subjects: collectOptions(books, func(book Textbook) (string, string) { return book.SubjectID, book.Subject }),
		Editions: collectOptions(books, func(book Textbook) (string, string) { return book.EditionID, book.Edition }),
		Grades:   collectOptions(books, func(book Textbook) (string, string) { return book.GradeID, book.Grade }),
		Volumes:  collectOptions(books, func(book Textbook) (string, string) { return book.VolumeID, book.Volume }),
	}
}

func collectOptions(books []Textbook, value func(Textbook) (string, string)) []TagOption {
	unique := make(map[string]string)
	for _, book := range books {
		id, name := value(book)
		if id != "" && name != "" {
			unique[id] = name
		}
	}
	options := make([]TagOption, 0, len(unique))
	for id, name := range unique {
		options = append(options, TagOption{ID: id, Name: name})
	}
	sort.SliceStable(options, func(i, j int) bool { return options[i].Name < options[j].Name })
	return options
}

func splitPartURLs(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
