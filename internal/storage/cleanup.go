package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ClearCache(layout Layout) error {
	if !IsWithin(layout.Root, layout.CacheDir) || filepath.Clean(layout.CacheDir) == filepath.Clean(layout.Root) {
		return errors.New("cache directory is outside the data root")
	}
	entries, err := os.ReadDir(layout.CacheDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read cache directory: %w", err)
	}
	var cleanupErrors []error
	for _, entry := range entries {
		path := filepath.Join(layout.CacheDir, entry.Name())
		if !IsWithin(layout.CacheDir, path) || filepath.Clean(path) == filepath.Clean(layout.CacheDir) {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("refuse unsafe cache path %q", path))
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove cache item %q: %w", entry.Name(), err))
		}
	}
	return errors.Join(cleanupErrors...)
}

func ClearIncomplete(downloadDir string, records []TaskRecord) error {
	downloadRoot, err := filepath.Abs(filepath.Clean(downloadDir))
	if err != nil {
		return fmt.Errorf("resolve download directory: %w", err)
	}
	var cleanupErrors []error
	for _, record := range records {
		if record.Status == "completed" || strings.TrimSpace(record.Filename) == "" || filepath.Base(record.Filename) != record.Filename {
			continue
		}
		path := filepath.Join(downloadRoot, record.Filename+".part")
		if !IsWithin(downloadRoot, path) || filepath.Clean(path) == downloadRoot {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove incomplete file %q: %w", record.Filename, err))
		}
	}
	return errors.Join(cleanupErrors...)
}
