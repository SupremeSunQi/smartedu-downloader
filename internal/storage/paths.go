package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Layout struct {
	Root        string
	CacheDir    string
	DetailsDir  string
	CoversDir   string
	LogsDir     string
	TasksDir    string
	WebViewDir  string
	DownloadDir string
	ConfigFile  string
}

func ResolveLayout(executableDir, override string) (Layout, error) {
	executableDir = strings.TrimSpace(executableDir)
	if executableDir == "" {
		return Layout{}, errors.New("executable directory is required")
	}

	executableDir, err := filepath.Abs(filepath.Clean(executableDir))
	if err != nil {
		return Layout{}, fmt.Errorf("resolve executable directory: %w", err)
	}

	root := filepath.Join(executableDir, ".smartedu-data")
	downloadDir := filepath.Join(executableDir, "downloads")
	if strings.TrimSpace(override) != "" {
		root, err = filepath.Abs(filepath.Clean(override))
		if err != nil {
			return Layout{}, fmt.Errorf("resolve data root: %w", err)
		}
		downloadDir = filepath.Join(root, "downloads")
	}

	cacheDir := filepath.Join(root, "cache")
	return Layout{
		Root:        root,
		CacheDir:    cacheDir,
		DetailsDir:  filepath.Join(cacheDir, "details"),
		CoversDir:   filepath.Join(cacheDir, "covers"),
		LogsDir:     filepath.Join(root, "logs"),
		TasksDir:    filepath.Join(root, "tasks"),
		WebViewDir:  filepath.Join(root, "webview"),
		DownloadDir: downloadDir,
		ConfigFile:  filepath.Join(root, "config.json"),
	}, nil
}

func IsWithin(root, candidate string) bool {
	rootAbs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return false
	}
	candidateAbs, err := filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func EnsureWritable(layout Layout) error {
	directories := []string{
		layout.Root,
		layout.CacheDir,
		layout.DetailsDir,
		layout.CoversDir,
		layout.LogsDir,
		layout.TasksDir,
		layout.WebViewDir,
		layout.DownloadDir,
	}
	for _, directory := range directories {
		if strings.TrimSpace(directory) == "" {
			return errors.New("layout contains an empty directory")
		}
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return fmt.Errorf("create directory %q: %w", directory, err)
		}
		if err := probeWritable(directory); err != nil {
			return fmt.Errorf("directory %q is not writable: %w", directory, err)
		}
	}
	return nil
}

func probeWritable(directory string) (err error) {
	probe, err := os.CreateTemp(directory, ".write-probe-*")
	if err != nil {
		return err
	}
	name := probe.Name()
	defer func() {
		closeErr := probe.Close()
		removeErr := os.Remove(name)
		err = errors.Join(err, ignoreAlreadyClosed(closeErr), ignoreNotExist(removeErr))
	}()

	if _, err = probe.Write([]byte("ok")); err != nil {
		return err
	}
	return probe.Sync()
}

func ignoreAlreadyClosed(err error) error {
	if errors.Is(err, os.ErrClosed) {
		return nil
	}
	return err
}

func ignoreNotExist(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

var _ io.Closer = (*os.File)(nil)
