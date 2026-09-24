package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type DuplicatePolicy string

const (
	DuplicateAsk       DuplicatePolicy = "ask"
	DuplicateRename    DuplicatePolicy = "rename"
	DuplicateOverwrite DuplicatePolicy = "overwrite"
	DuplicateSkip      DuplicatePolicy = "skip"
)

var (
	ErrUnsafeDownloadDirectory = errors.New("download directory must not be inside the cache")
	ErrInvalidConcurrency      = errors.New("concurrent downloads must be between 1 and 5")
	ErrInvalidDuplicatePolicy  = errors.New("invalid duplicate policy")
)

type Config struct {
	DataRoot            string          `json:"dataRoot"`
	DownloadDir         string          `json:"downloadDir"`
	ConcurrentDownloads int             `json:"concurrentDownloads"`
	DuplicatePolicy     DuplicatePolicy `json:"duplicatePolicy"`
}

func DefaultConfig(layout Layout) Config {
	return Config{
		DataRoot:            layout.Root,
		DownloadDir:         layout.DownloadDir,
		ConcurrentDownloads: 2,
		DuplicatePolicy:     DuplicateAsk,
	}
}

func LoadConfig(layout Layout) (Config, error) {
	file, err := os.Open(layout.ConfigFile)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultConfig(layout), nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("open config: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(io.LimitReader(file, 64*1024))
	decoder.DisallowUnknownFields()
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return Config{}, err
	}
	config, err = normalizeConfig(layout, config)
	if err != nil {
		return Config{}, err
	}
	return config, nil
}

func SaveConfig(layout Layout, config Config) (err error) {
	config, err = normalizeConfig(layout, config)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(layout.ConfigFile), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	temporary := layout.ConfigFile + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	defer func() {
		closeErr := file.Close()
		removeErr := os.Remove(temporary)
		err = errors.Join(err, ignoreAlreadyClosed(closeErr), ignoreNotExist(removeErr))
	}()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(config); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync config: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close config: %w", err)
	}
	if err := os.Rename(temporary, layout.ConfigFile); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

func normalizeConfig(layout Layout, config Config) (Config, error) {
	if config.ConcurrentDownloads < 1 || config.ConcurrentDownloads > 5 {
		return Config{}, ErrInvalidConcurrency
	}
	switch config.DuplicatePolicy {
	case DuplicateAsk, DuplicateRename, DuplicateOverwrite, DuplicateSkip:
	default:
		return Config{}, ErrInvalidDuplicatePolicy
	}

	if strings.TrimSpace(config.DataRoot) == "" {
		config.DataRoot = layout.Root
	}
	dataRoot, err := filepath.Abs(filepath.Clean(config.DataRoot))
	if err != nil {
		return Config{}, fmt.Errorf("resolve config data root: %w", err)
	}
	layoutRoot, err := filepath.Abs(filepath.Clean(layout.Root))
	if err != nil {
		return Config{}, fmt.Errorf("resolve layout data root: %w", err)
	}
	if !strings.EqualFold(dataRoot, layoutRoot) {
		return Config{}, errors.New("config data root does not match active layout")
	}
	config.DataRoot = layoutRoot

	if strings.TrimSpace(config.DownloadDir) == "" {
		config.DownloadDir = layout.DownloadDir
	}
	downloadDir, err := filepath.Abs(filepath.Clean(config.DownloadDir))
	if err != nil {
		return Config{}, fmt.Errorf("resolve download directory: %w", err)
	}
	if IsWithin(layout.CacheDir, downloadDir) {
		return Config{}, ErrUnsafeDownloadDirectory
	}
	config.DownloadDir = downloadDir
	return config, nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode trailing config data: %w", err)
	}
	return errors.New("config contains multiple JSON values")
}
