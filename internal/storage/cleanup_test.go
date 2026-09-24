package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClearCacheNeverRemovesAnythingOutsideCacheRoot(t *testing.T) {
	layout, err := ResolveLayout(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureWritable(layout); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "keep.txt")
	if err := os.WriteFile(outside, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.CacheDir, "remove.json"), []byte("cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ClearCache(layout); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file was removed: %v", err)
	}
	entries, err := os.ReadDir(layout.CacheDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("cache was not cleared: entries=%v err=%v", entries, err)
	}
}

func TestClearIncompleteDeletesOnlyRecordedPartFiles(t *testing.T) {
	downloads := t.TempDir()
	recorded := filepath.Join(downloads, "book.pdf.part")
	unrecorded := filepath.Join(downloads, "keep.pdf.part")
	for _, path := range []string{recorded, unrecorded} {
		if err := os.WriteFile(path, []byte("partial"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	records := []TaskRecord{{ID: "one", Filename: "book.pdf", Status: "paused"}, {ID: "unsafe", Filename: `..\outside.pdf`, Status: "paused"}}
	if err := ClearIncomplete(downloads, records); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(recorded); !os.IsNotExist(err) {
		t.Fatalf("recorded part remained: %v", err)
	}
	if _, err := os.Stat(unrecorded); err != nil {
		t.Fatalf("unrecorded part was removed: %v", err)
	}
}
