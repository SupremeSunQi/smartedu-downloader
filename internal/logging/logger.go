package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

const (
	defaultMaxLogBytes = 2 << 20
	defaultLogBackups  = 2
)

type rotatingWriter struct {
	mutex    sync.Mutex
	dir      string
	path     string
	file     *os.File
	size     int64
	maxBytes int64
	backups  int
}

func newRotatingWriter(directory string, maxBytes int64, backups int) (*rotatingWriter, error) {
	if maxBytes <= 0 || backups < 0 {
		return nil, fmt.Errorf("invalid log rotation settings")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	path := filepath.Join(directory, "app.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open application log: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("stat application log: %w", err)
	}
	return &rotatingWriter{dir: directory, path: path, file: file, size: info.Size(), maxBytes: maxBytes, backups: backups}, nil
}

func (writer *rotatingWriter) Write(payload []byte) (int, error) {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()
	if writer.file == nil {
		return 0, os.ErrClosed
	}
	if writer.size > 0 && writer.size+int64(len(payload)) > writer.maxBytes {
		if err := writer.rotate(); err != nil {
			return 0, err
		}
	}
	written, err := writer.file.Write(payload)
	writer.size += int64(written)
	return written, err
}

func (writer *rotatingWriter) Close() error {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()
	if writer.file == nil {
		return nil
	}
	err := writer.file.Close()
	writer.file = nil
	return err
}

func (writer *rotatingWriter) rotate() error {
	if err := writer.file.Sync(); err != nil {
		return fmt.Errorf("sync application log: %w", err)
	}
	if err := writer.file.Close(); err != nil {
		return fmt.Errorf("close application log: %w", err)
	}
	writer.file = nil

	if writer.backups > 0 {
		oldest := filepath.Join(writer.dir, fmt.Sprintf("app.log.%d", writer.backups))
		if err := os.Remove(oldest); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove oldest log: %w", err)
		}
		for index := writer.backups - 1; index >= 1; index-- {
			from := filepath.Join(writer.dir, fmt.Sprintf("app.log.%d", index))
			to := filepath.Join(writer.dir, fmt.Sprintf("app.log.%d", index+1))
			if err := os.Rename(from, to); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("rotate log backup: %w", err)
			}
		}
		if err := os.Rename(writer.path, filepath.Join(writer.dir, "app.log.1")); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("rotate application log: %w", err)
		}
	} else if err := os.Remove(writer.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove application log: %w", err)
	}

	file, err := os.OpenFile(writer.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open new application log: %w", err)
	}
	writer.file = file
	writer.size = 0
	return nil
}

type redactingWriter struct {
	destination io.Writer
}

func (writer redactingWriter) Write(payload []byte) (int, error) {
	redacted := []byte(Redact(string(payload)))
	if _, err := writer.destination.Write(redacted); err != nil {
		return 0, err
	}
	return len(payload), nil
}

func NewRotatingLogger(directory string) (*slog.Logger, io.Closer, error) {
	writer, err := newRotatingWriter(directory, defaultMaxLogBytes, defaultLogBackups)
	if err != nil {
		return nil, nil, err
	}
	handler := slog.NewTextHandler(redactingWriter{destination: writer}, &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(&contextHandler{Handler: handler}), writer, nil
}

type contextHandler struct {
	slog.Handler
}

func (handler *contextHandler) Handle(ctx context.Context, record slog.Record) error {
	return handler.Handler.Handle(ctx, record)
}
