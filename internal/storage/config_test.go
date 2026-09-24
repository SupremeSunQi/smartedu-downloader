package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveConfigRejectsDownloadDirectoryInsideCache(t *testing.T) {
	layout, err := ResolveLayout(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	err = SaveConfig(layout, Config{
		DownloadDir:         layout.CacheDir,
		ConcurrentDownloads: 2,
		DuplicatePolicy:     DuplicateAsk,
	})
	if !errors.Is(err, ErrUnsafeDownloadDirectory) {
		t.Fatalf("SaveConfig error = %v, want ErrUnsafeDownloadDirectory", err)
	}
}

func TestSaveAndLoadConfigRoundTrip(t *testing.T) {
	layout, err := ResolveLayout(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureWritable(layout); err != nil {
		t.Fatal(err)
	}

	want := Config{
		DataRoot:            layout.Root,
		DownloadDir:         filepath.Join(t.TempDir(), "books"),
		ConcurrentDownloads: 3,
		DuplicatePolicy:     DuplicateRename,
	}
	if err := SaveConfig(layout, want); err != nil {
		t.Fatalf("SaveConfig returned error: %v", err)
	}
	got, err := LoadConfig(layout)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if got != want {
		t.Fatalf("LoadConfig = %#v, want %#v", got, want)
	}
	if _, err := os.Stat(layout.ConfigFile + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary config file remained: %v", err)
	}
}

func TestSaveConfigRejectsOutOfRangeConcurrency(t *testing.T) {
	layout, err := ResolveLayout(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, concurrency := range []int{0, 6} {
		err := SaveConfig(layout, Config{
			DownloadDir:         layout.DownloadDir,
			ConcurrentDownloads: concurrency,
			DuplicatePolicy:     DuplicateAsk,
		})
		if !errors.Is(err, ErrInvalidConcurrency) {
			t.Errorf("concurrency %d error = %v, want ErrInvalidConcurrency", concurrency, err)
		}
	}
}

func TestLoadConfigReturnsDefaultsWhenMissing(t *testing.T) {
	layout, err := ResolveLayout(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(layout)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if got.ConcurrentDownloads != 2 || got.DuplicatePolicy != DuplicateAsk {
		t.Fatalf("unexpected defaults: %#v", got)
	}
	if got.DataRoot != layout.Root || got.DownloadDir != layout.DownloadDir {
		t.Fatalf("default paths do not match layout: %#v", got)
	}
}
