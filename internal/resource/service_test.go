package resource

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"smartedu-downloader/internal/catalog"
)

func TestResolveSelectsSourcePDFAndOrdersOfficialMirrors(t *testing.T) {
	service, closeServer := detailFixtureService(t, detailDocument{
		ID: "book-1",
		Items: []detailItem{
			{Format: "jpg", FileFlag: "thumbnail", Storages: []string{"https://example.invalid/cover.jpg"}},
			{Format: "pdf", IsSourceFile: true, FileFlag: "source", Size: 46402014, MD5: "7efc5785d97f606cdd3d844e7bdce285"},
		},
	})
	defer closeServer()

	source, err := service.Resolve(context.Background(), catalog.Textbook{ID: "book-1", Title: "数学 一年级"})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if source.Filename != "数学 一年级.pdf" {
		t.Fatalf("Filename = %q", source.Filename)
	}
	if source.ExpectedSize != 46402014 || source.MD5 != "7efc5785d97f606cdd3d844e7bdce285" {
		t.Fatalf("source metadata was not normalized: %#v", source)
	}
	if len(source.Mirrors) != 3 {
		t.Fatalf("Mirrors length = %d, want 3", len(source.Mirrors))
	}
}

func TestResolveRejectsNonAllowlistedMirror(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeDetailJSON(t, w, detailDocument{ID: "book-1", Items: []detailItem{{
			Format: "pdf", IsSourceFile: true, FileFlag: "source", Size: 10,
			MD5: "d41d8cd98f00b204e9800998ecf8427e", Storages: []string{"https://example.invalid/book.pdf"},
		}}})
	}))
	defer server.Close()
	service := NewService(server.Client(), server.URL, t.TempDir(), testURLPolicy(server))

	if _, err := service.Resolve(context.Background(), catalog.Textbook{ID: "book-1", Title: "语文"}); err == nil {
		t.Fatal("Resolve accepted a mirror outside the allowlist")
	}
}

func TestResolveUsesValidCachedDetailWhenNetworkFails(t *testing.T) {
	fail := false
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "offline", http.StatusServiceUnavailable)
			return
		}
		mirrors := []string{server.URL + "/r1.pdf", server.URL + "/r2.pdf", server.URL + "/r3.pdf"}
		writeDetailJSON(t, w, detailDocument{ID: "book-1", Items: []detailItem{{
			Format: "pdf", IsSourceFile: true, FileFlag: "source", Size: 12,
			MD5: "d41d8cd98f00b204e9800998ecf8427e", Storages: mirrors,
		}}})
	}))
	defer server.Close()
	service := NewService(server.Client(), server.URL, t.TempDir(), testURLPolicy(server))
	book := catalog.Textbook{ID: "book-1", Title: "语文"}

	first, err := service.Resolve(context.Background(), book)
	if err != nil {
		t.Fatalf("first Resolve returned error: %v", err)
	}
	fail = true
	second, err := service.Resolve(context.Background(), book)
	if err != nil {
		t.Fatalf("cached Resolve returned error: %v", err)
	}
	if second.ExpectedSize != first.ExpectedSize || len(second.Mirrors) != len(first.Mirrors) {
		t.Fatalf("cached source differs: first=%#v second=%#v", first, second)
	}
}

func detailFixtureService(t *testing.T, document detailDocument) (*Service, func()) {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for index := range document.Items {
			if document.Items[index].Format == "pdf" && document.Items[index].IsSourceFile {
				document.Items[index].Storages = []string{server.URL + "/r1.pdf", server.URL + "/r2.pdf", server.URL + "/r3.pdf"}
			}
		}
		writeDetailJSON(t, w, document)
	}))
	return NewService(server.Client(), server.URL, t.TempDir(), testURLPolicy(server)), server.Close
}

func testURLPolicy(server *httptest.Server) URLPolicy {
	parsed, _ := url.Parse(server.URL)
	return func(candidate *url.URL) bool { return candidate.Host == parsed.Host }
}

func writeDetailJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}

func md5String(value string) string {
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}
