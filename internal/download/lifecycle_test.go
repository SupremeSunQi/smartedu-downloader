package download

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInvalidPDFNeverCreatesCompletedFile(t *testing.T) {
	wanted := []byte("%PDF-valid-payload")
	invalid := bytes.Repeat([]byte("x"), len(wanted))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(invalid)
	}))
	t.Cleanup(server.Close)
	directory := t.TempDir()
	manager := newTestManager(t, server.Client(), newFakeSession("token"), directory, 1)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	task, err := manager.Enqueue(pdfSource("book", "invalid.pdf", server.URL, wanted))
	if err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, manager, task.ID, StatusFailed)
	if _, err := os.Stat(filepath.Join(directory, "invalid.pdf")); !os.IsNotExist(err) {
		t.Fatalf("invalid response created a completed PDF: %v", err)
	}
}

func TestShutdownCancelsActiveResponseAndLeavesTaskPaused(t *testing.T) {
	started := make(chan struct{})
	released := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("%PDF-"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
		close(released)
	}))
	t.Cleanup(server.Close)
	manager := newTestManager(t, server.Client(), newFakeSession("token"), t.TempDir(), 1)
	pdf := append([]byte("%PDF-"), bytes.Repeat([]byte("s"), 1024)...)
	task, err := manager.Enqueue(pdfSource("book", "shutdown.pdf", server.URL, pdf))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("download did not start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := manager.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("active HTTP response was not canceled")
	}
	got, _ := manager.Task(task.ID)
	if got.Status != StatusPaused {
		t.Fatalf("shutdown task status = %s, want paused", got.Status)
	}
}
