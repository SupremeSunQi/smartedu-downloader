package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestSyncFetchesDeclaredPartsAndAtomicallyCachesNormalizedCatalog(t *testing.T) {
	server := newCatalogServer(t, 42, [][]remoteResource{
		{catalogResource("book-2", "二年级数学下册", "数学", "二年级", "下册")},
		{catalogResource("book-1", "一年级语文上册", "语文", "一年级", "上册")},
	})
	cacheDir := t.TempDir()
	service := NewService(server.Client(), server.URL, cacheDir, testServerPolicy(server))

	snapshot, err := service.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}
	if snapshot.Version != 42 {
		t.Fatalf("Version = %d, want 42", snapshot.Version)
	}
	if len(snapshot.Textbooks) != 2 {
		t.Fatalf("Textbooks length = %d, want 2", len(snapshot.Textbooks))
	}
	if snapshot.Textbooks[0].ID != "book-1" || snapshot.Textbooks[1].ID != "book-2" {
		t.Fatalf("textbooks are not sorted by title: %#v", snapshot.Textbooks)
	}
	if snapshot.Textbooks[0].Subject != "语文" || snapshot.Textbooks[0].SubjectID != "subject-语文" {
		t.Fatalf("subject was not normalized: %#v", snapshot.Textbooks[0])
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "catalog.json")); err != nil {
		t.Fatalf("catalog cache not written: %v", err)
	}
}

func TestSyncRejectsPartOutsideAllowlist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case TagsPath:
			writeJSON(t, w, map[string]any{"tag_path": "root", "hierarchies": []any{}})
		case VersionPath:
			writeJSON(t, w, map[string]any{"module_version": 1, "urls": "https://example.invalid/part.json"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	service := NewService(server.Client(), server.URL, t.TempDir(), testServerPolicy(server))

	if _, err := service.Sync(context.Background()); err == nil {
		t.Fatal("Sync accepted a part URL outside the allowlist")
	}
}

func TestSyncFailureLeavesPreviousCacheReadable(t *testing.T) {
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "offline", http.StatusServiceUnavailable)
			return
		}
		switch r.URL.Path {
		case TagsPath:
			writeJSON(t, w, map[string]any{"tag_path": "root", "hierarchies": []any{}})
		case VersionPath:
			writeJSON(t, w, map[string]any{"module_version": 41, "urls": serverURL(r) + "/part.json"})
		case "/part.json":
			writeJSON(t, w, []remoteResource{catalogResource("book-1", "一年级语文上册", "语文", "一年级", "上册")})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	cacheDir := t.TempDir()
	service := NewService(server.Client(), server.URL, cacheDir, testServerPolicy(server))
	if _, err := service.Sync(context.Background()); err != nil {
		t.Fatalf("initial Sync returned error: %v", err)
	}

	fail = true
	if _, err := service.Sync(context.Background()); err == nil {
		t.Fatal("Sync succeeded while server was failing")
	}
	cached, err := service.LoadCached()
	if err != nil {
		t.Fatalf("LoadCached returned error: %v", err)
	}
	if cached.Version != 41 || len(cached.Textbooks) != 1 {
		t.Fatalf("previous cache was not preserved: %#v", cached)
	}
}

func TestLoadCachedQuarantinesInvalidJSON(t *testing.T) {
	cacheDir := t.TempDir()
	cachePath := filepath.Join(cacheDir, "catalog.json")
	if err := os.WriteFile(cachePath, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService(http.DefaultClient, ProductionBase, cacheDir, ProductionURLPolicy)

	if _, err := service.LoadCached(); err == nil {
		t.Fatal("LoadCached accepted corrupt JSON")
	}
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("corrupt cache remained at active path: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(cacheDir, "catalog.corrupt-*.json"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("corrupt cache was not quarantined: matches=%v err=%v", matches, err)
	}
}

func newCatalogServer(t *testing.T, version int64, parts [][]remoteResource) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case TagsPath:
			writeJSON(t, w, map[string]any{"tag_path": "root", "hierarchies": []any{}})
		case VersionPath:
			urls := make([]string, len(parts))
			for index := range parts {
				urls[index] = server.URL + "/part-" + string(rune('a'+index)) + ".json"
			}
			writeJSON(t, w, map[string]any{"module_version": version, "urls": joinURLs(urls)})
		default:
			for index, resources := range parts {
				if r.URL.Path == "/part-"+string(rune('a'+index))+".json" {
					writeJSON(t, w, resources)
					return
				}
			}
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func testServerPolicy(server *httptest.Server) URLPolicy {
	parsed, _ := url.Parse(server.URL)
	return func(candidate *url.URL) bool { return candidate.Host == parsed.Host }
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}

func joinURLs(urls []string) string {
	result := ""
	for index, value := range urls {
		if index > 0 {
			result += ","
		}
		result += value
	}
	return result
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode fixture response: %v", err)
	}
}

func catalogResource(id, title, subject, grade, volume string) remoteResource {
	return remoteResource{
		ID:    id,
		Title: title,
		CustomProperties: remoteCustomProperties{
			Size:       1024,
			Thumbnails: []string{"https://r1-ndr.ykt.cbern.com.cn/cover.png"},
		},
		ProviderList: []remoteProvider{{Name: "人民教育出版社"}},
		TagList: []remoteTag{
			{ID: "stage-primary", Name: "小学", DimensionID: dimensionStage},
			{ID: "subject-" + subject, Name: subject, DimensionID: dimensionSubject},
			{ID: "edition-a", Name: "统编版", DimensionID: dimensionEdition},
			{ID: "grade-" + grade, Name: grade, DimensionID: dimensionGrade},
			{ID: "volume-" + volume, Name: volume, DimensionID: dimensionVolume},
		},
	}
}
