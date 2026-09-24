package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	ProductionBase = "https://bdcs-file-2.ykt.cbern.com.cn/zxx_secondary"
	TagsPath       = "/ndrs/tags/tch_material_tag.json"
	VersionPath    = "/ndrs/resources/tch_material/version/data_version.json"

	maxTagsBytes    = 8 << 20
	maxVersionBytes = 1 << 20
	maxPartBytes    = 96 << 20
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
	case "bdcs-file-1.ykt.cbern.com.cn", "bdcs-file-2.ykt.cbern.com.cn":
		return true
	default:
		return false
	}
}

type versionDocument struct {
	Module        string `json:"module"`
	ModuleVersion int64  `json:"module_version"`
	URLs          string `json:"urls"`
}

func fetchJSON(ctx context.Context, client HTTPClient, address string, limit int64, destination any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return fmt.Errorf("create catalog request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("fetch catalog data: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("catalog endpoint returned HTTP %d", response.StatusCode)
	}

	limited := io.LimitReader(response.Body, limit+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read catalog response: %w", err)
	}
	if int64(len(payload)) > limit {
		return errors.New("catalog response exceeds size limit")
	}
	if err := json.Unmarshal(payload, destination); err != nil {
		return fmt.Errorf("decode catalog response: %w", err)
	}
	return nil
}

func resolveEndpoint(base, endpoint string) (string, error) {
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse catalog base URL: %w", err)
	}
	if strings.HasSuffix(baseURL.Path, "/zxx_secondary") && strings.HasPrefix(endpoint, "/") {
		baseURL.Path += endpoint
	} else {
		baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/" + strings.TrimLeft(endpoint, "/")
	}
	return baseURL.String(), nil
}
