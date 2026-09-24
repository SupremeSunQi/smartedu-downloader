package storage

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	redactinglog "smartedu-downloader/internal/logging"
)

const maxDiagnosticLogBytes = 3 << 20

func ExportDiagnostics(layout Layout, config Config, records []TaskRecord, destination string) (result string, err error) {
	if strings.TrimSpace(destination) == "" {
		destination = filepath.Join(layout.Root, "diagnostics-"+time.Now().UTC().Format("20060102-150405")+".zip")
	} else if !strings.EqualFold(filepath.Ext(destination), ".zip") {
		destination = filepath.Join(destination, "SmartEduDownloader-diagnostics.zip")
	}
	destination, err = filepath.Abs(filepath.Clean(destination))
	if err != nil {
		return "", fmt.Errorf("resolve diagnostics destination: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return "", fmt.Errorf("create diagnostics directory: %w", err)
	}

	temporary := destination + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("create diagnostics archive: %w", err)
	}
	archive := zip.NewWriter(file)
	archiveClosed := false
	fileClosed := false
	defer func() {
		var archiveErr, closeErr error
		if !archiveClosed {
			archiveErr = archive.Close()
		}
		if !fileClosed {
			closeErr = file.Close()
		}
		removeErr := os.Remove(temporary)
		if errors.Is(closeErr, os.ErrClosed) {
			closeErr = nil
		}
		if errors.Is(removeErr, os.ErrNotExist) {
			removeErr = nil
		}
		err = errors.Join(err, archiveErr, closeErr, removeErr)
		if err != nil {
			result = ""
		}
	}()

	metadata := map[string]any{
		"application":  "SmartEduDownloader",
		"generatedAt":  time.Now().UTC(),
		"goVersion":    runtime.Version(),
		"os":           runtime.GOOS,
		"architecture": runtime.GOARCH,
	}
	if err := writeDiagnosticJSON(archive, "version.json", metadata); err != nil {
		return "", err
	}
	if err := writeDiagnosticJSON(archive, "config.json", config); err != nil {
		return "", err
	}
	if err := writeDiagnosticJSON(archive, "tasks.json", records); err != nil {
		return "", err
	}
	for _, name := range []string{"app.log", "app.log.1", "app.log.2"} {
		path := filepath.Join(layout.LogsDir, name)
		logFile, openErr := os.Open(path)
		if errors.Is(openErr, os.ErrNotExist) {
			continue
		}
		if openErr != nil {
			return "", fmt.Errorf("open diagnostic log %q: %w", name, openErr)
		}
		payload, readErr := io.ReadAll(io.LimitReader(logFile, maxDiagnosticLogBytes))
		closeLogErr := logFile.Close()
		if readErr != nil || closeLogErr != nil {
			return "", errors.Join(readErr, closeLogErr)
		}
		entry, createErr := archive.Create("logs/" + name)
		if createErr != nil {
			return "", fmt.Errorf("create diagnostic log entry: %w", createErr)
		}
		if _, writeErr := io.WriteString(entry, redactinglog.Redact(string(payload))); writeErr != nil {
			return "", fmt.Errorf("write diagnostic log entry: %w", writeErr)
		}
	}
	if err := archive.Close(); err != nil {
		return "", fmt.Errorf("close diagnostics archive: %w", err)
	}
	archiveClosed = true
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("sync diagnostics archive: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close diagnostics file: %w", err)
	}
	fileClosed = true
	if err := os.Rename(temporary, destination); err != nil {
		return "", fmt.Errorf("replace diagnostics archive: %w", err)
	}
	return destination, nil
}

func writeDiagnosticJSON(archive *zip.Writer, name string, value any) error {
	entry, err := archive.Create(name)
	if err != nil {
		return fmt.Errorf("create diagnostics entry %q: %w", name, err)
	}
	encoder := json.NewEncoder(entry)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write diagnostics entry %q: %w", name, err)
	}
	return nil
}
