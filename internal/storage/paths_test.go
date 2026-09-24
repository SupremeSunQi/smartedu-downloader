package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveLayoutKeepsMutablePathsUnderSelectedRoot(t *testing.T) {
	root := t.TempDir()
	layout, err := ResolveLayout(t.TempDir(), root)
	if err != nil {
		t.Fatalf("ResolveLayout returned error: %v", err)
	}

	paths := []string{
		layout.CacheDir,
		layout.DetailsDir,
		layout.CoversDir,
		layout.LogsDir,
		layout.TasksDir,
		layout.WebViewDir,
		layout.ConfigFile,
	}
	for _, path := range paths {
		if !IsWithin(root, path) {
			t.Errorf("path escaped selected root: %s", path)
		}
	}

	wantDownloads := filepath.Join(root, "downloads")
	if layout.DownloadDir != wantDownloads {
		t.Fatalf("DownloadDir = %q, want %q", layout.DownloadDir, wantDownloads)
	}
}

func TestResolveLayoutDefaultsBesideExecutable(t *testing.T) {
	executableDir := t.TempDir()
	layout, err := ResolveLayout(executableDir, "")
	if err != nil {
		t.Fatalf("ResolveLayout returned error: %v", err)
	}

	if layout.Root != filepath.Join(executableDir, ".smartedu-data") {
		t.Fatalf("Root = %q", layout.Root)
	}
	if layout.DownloadDir != filepath.Join(executableDir, "downloads") {
		t.Fatalf("DownloadDir = %q", layout.DownloadDir)
	}
}

func TestIsWithinRejectsSiblingWithSharedPrefix(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "data")
	if IsWithin(parent, parent+"-other") {
		t.Fatal("shared path prefix must not count as containment")
	}
}

func TestEnsureWritableCreatesOnlyKnownDirectories(t *testing.T) {
	root := t.TempDir()
	layout, err := ResolveLayout(t.TempDir(), root)
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureWritable(layout); err != nil {
		t.Fatalf("EnsureWritable returned error: %v", err)
	}

	for _, dir := range []string{layout.Root, layout.CacheDir, layout.DetailsDir, layout.CoversDir, layout.LogsDir, layout.TasksDir, layout.WebViewDir, layout.DownloadDir} {
		info, statErr := os.Stat(dir)
		if statErr != nil || !info.IsDir() {
			t.Errorf("expected directory %q, stat error: %v", dir, statErr)
		}
	}
}
