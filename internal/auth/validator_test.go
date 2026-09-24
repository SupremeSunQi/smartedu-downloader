package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func protectedPDFServer(t *testing.T, validToken string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("accessToken") != validToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Range") != "bytes=0-4" {
			t.Errorf("Range = %q, want bytes=0-4", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Range", "bytes 0-4/100")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("%PDF-"))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestValidatorRejectsSuccessfulNonPDFResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html"))
	}))
	t.Cleanup(server.Close)

	err := NewValidator(server.Client()).Validate(t.Context(), "token", server.URL+"/book.pdf")
	if err != ErrInvalidPDFResponse {
		t.Fatalf("Validate error = %v, want ErrInvalidPDFResponse", err)
	}
}

func TestValidatorPreservesExistingMirrorQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("v") != "1" || r.URL.Query().Get("accessToken") != "token" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("%PDF-"))
	}))
	t.Cleanup(server.Close)

	if err := NewValidator(server.Client()).Validate(t.Context(), "token", server.URL+"/book.pdf?v=1"); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}
