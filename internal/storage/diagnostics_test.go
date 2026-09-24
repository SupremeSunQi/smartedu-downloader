package storage

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportDiagnosticsRedactsLogsAndOmitsResourceURLs(t *testing.T) {
	layout, err := ResolveLayout(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureWritable(layout); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.LogsDir, "app.log"), []byte(`accessToken=topsecret {"password":"hidden"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "diagnostics.zip")
	path, err := ExportDiagnostics(layout, DefaultConfig(layout), []TaskRecord{{ID: "one", BookID: "book", Filename: "book.pdf", Status: "paused"}}, destination)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var combined strings.Builder
	for _, file := range reader.File {
		entry, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		payload, err := io.ReadAll(entry)
		entry.Close()
		if err != nil {
			t.Fatal(err)
		}
		combined.Write(payload)
	}
	text := strings.ToLower(combined.String())
	for _, forbidden := range []string{"topsecret", "hidden", "accesstoken=topsecret", "http://", "https://"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("diagnostics contain %q: %s", forbidden, combined.String())
		}
	}
	if !strings.Contains(combined.String(), "[REDACTED]") {
		t.Fatalf("diagnostics did not retain redaction marker: %s", combined.String())
	}
}
