package download

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestMirrorFailoverDoesNotRetryPermanentHTTPError(t *testing.T) {
	pdf := append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte("m"), 512)...)
	var rejectedCalls atomic.Int32
	rejected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rejectedCalls.Add(1)
		http.NotFound(w, r)
	}))
	t.Cleanup(rejected.Close)
	working := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pdf)
	}))
	t.Cleanup(working.Close)

	directory := t.TempDir()
	manager := newTestManager(t, working.Client(), newFakeSession("token"), directory, 1)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	source := pdfSource("book", "mirror.pdf", rejected.URL, pdf)
	source.Mirrors = append(source.Mirrors, working.URL)
	task, err := manager.Enqueue(source)
	if err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, manager, task.ID, StatusCompleted)
	if calls := rejectedCalls.Load(); calls != 1 {
		t.Fatalf("permanent HTTP error was requested %d times, want 1", calls)
	}
}

func TestCompleteValidPartIsPromotedWithoutNetworkRequest(t *testing.T) {
	pdf := append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte("r"), 512)...)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "must not be called", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	directory := t.TempDir()
	partPath := filepath.Join(directory, "recovered.pdf.part")
	if err := os.WriteFile(partPath, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	manager := newTestManager(t, server.Client(), newFakeSession("token"), directory, 1)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	task, err := manager.Enqueue(pdfSource("book", "recovered.pdf", server.URL, pdf))
	if err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, manager, task.ID, StatusCompleted)
	if calls := requests.Load(); calls != 0 {
		t.Fatalf("complete valid part caused %d network requests, want 0", calls)
	}
	if _, err := os.Stat(filepath.Join(directory, "recovered.pdf")); err != nil {
		t.Fatalf("recovered PDF missing: %v", err)
	}
}
