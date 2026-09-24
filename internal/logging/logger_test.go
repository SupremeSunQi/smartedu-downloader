package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingWriterKeepsFilesInsideLogDirectory(t *testing.T) {
	directory := t.TempDir()
	writer, err := newRotatingWriter(directory, 32, 2)
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if _, err := writer.Write([]byte(strings.Repeat("x", 20))); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"app.log", "app.log.1", "app.log.2"} {
		path := filepath.Join(directory, name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected rotated file %q: %v", name, err)
		}
	}
}

func TestLoggerRedactsMessageAndAttributesBeforeWriting(t *testing.T) {
	directory := t.TempDir()
	logger, closeLogger, err := NewRotatingLogger(directory)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info(`request accessToken=message-secret`, "authorization", "attribute-secret")
	if err := closeLogger.Close(); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(filepath.Join(directory, "app.log"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	if strings.Contains(text, "message-secret") || strings.Contains(text, "attribute-secret") {
		t.Fatalf("logger retained a secret: %s", text)
	}
}
