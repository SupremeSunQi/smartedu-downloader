package download

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	appLogging "smartedu-downloader/internal/logging"
)

var errUnauthorized = errors.New("download authorization expired")

type transientError struct{ err error }

func (err *transientError) Error() string { return err.err.Error() }
func (err *transientError) Unwrap() error { return err.err }

type validationError struct{ err error }

func (err *validationError) Error() string { return err.err.Error() }
func (err *validationError) Unwrap() error { return err.err }

func (manager *Manager) process(id string) {
	manager.mutex.Lock()
	entry, ok := manager.tasks[id]
	if !ok || entry.task.Status != StatusQueued {
		manager.mutex.Unlock()
		return
	}
	token, authenticated := manager.session.Token()
	if !authenticated {
		manager.mutex.Unlock()
		manager.pauseAllForAuth()
		return
	}
	ctx, cancel := context.WithCancel(manager.ctx)
	entry.cancel = cancel
	entry.done = make(chan struct{})
	entry.cleanupErr = nil
	entry.task.Status = StatusDownloading
	entry.task.Error = ""
	entry.task.UpdatedAt = time.Now().UTC()
	task := entry.task
	manager.mutex.Unlock()
	manager.persist()
	manager.publish(task)

	err := manager.download(ctx, token, entry)
	cancel()
	if errors.Is(err, errUnauthorized) {
		manager.mutex.RLock()
		interrupted := entry.desiredStatus == StatusPaused || entry.desiredStatus == StatusCanceled
		manager.mutex.RUnlock()
		if interrupted {
			manager.finishInterrupted(entry)
			return
		}
		manager.pauseAllForAuth()
		manager.mutex.RLock()
		interrupted = entry.desiredStatus == StatusPaused || entry.desiredStatus == StatusCanceled
		manager.mutex.RUnlock()
		if interrupted {
			manager.finishInterrupted(entry)
		}
		return
	}
	if errors.Is(err, context.Canceled) {
		manager.finishInterrupted(entry)
		return
	}
	if err != nil {
		manager.finishFailed(entry, err)
		return
	}
	manager.finishCompleted(entry)
}

func (manager *Manager) download(ctx context.Context, token string, entry *managedTask) error {
	if info, err := os.Stat(entry.partPath); err == nil && info.Size() == entry.source.ExpectedSize {
		if verifyErr := verifyPartial(entry.partPath, entry.source.ExpectedSize, entry.source.MD5); verifyErr == nil {
			if renameErr := os.Rename(entry.partPath, entry.finalPath); renameErr != nil {
				return fmt.Errorf("complete recovered download: %w", renameErr)
			}
			return nil
		}
		if removeErr := os.Remove(entry.partPath); removeErr != nil {
			return fmt.Errorf("remove invalid recovered download: %w", removeErr)
		}
		manager.updateProgress(entry, 0, 0, true)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect recovered download: %w", err)
	}

	var lastErr error
	for mirrorIndex, mirror := range entry.source.Mirrors {
		manager.setMirrorAttempt(entry, mirrorIndex+1)
		for retry := 0; retry < 3; retry++ {
			lastErr = manager.downloadAttempt(ctx, token, mirror, entry)
			if lastErr == nil || errors.Is(lastErr, errUnauthorized) || errors.Is(lastErr, context.Canceled) {
				return lastErr
			}
			var transient *transientError
			if !errors.As(lastErr, &transient) {
				break
			}
			if retry < 2 {
				delay := []time.Duration{500 * time.Millisecond, time.Second}[retry]
				select {
				case <-time.After(delay):
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
		var invalid *validationError
		if errors.As(lastErr, &invalid) {
			if err := os.Remove(entry.partPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove invalid partial download: %w", err)
			}
			manager.updateProgress(entry, 0, 0, true)
		}
	}
	return fmt.Errorf("all download mirrors failed: %w", lastErr)
}

func (manager *Manager) downloadAttempt(ctx context.Context, token, mirror string, entry *managedTask) error {
	offset := int64(0)
	if info, err := os.Stat(entry.partPath); err == nil {
		offset = info.Size()
		if offset > entry.source.ExpectedSize {
			offset = 0
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect partial file: %w", err)
	}
	parsed, err := url.Parse(mirror)
	if err != nil {
		return fmt.Errorf("parse download mirror: %w", err)
	}
	query := parsed.Query()
	query.Set("accessToken", token)
	parsed.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}
	if offset > 0 {
		request.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	response, err := manager.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &transientError{err: fmt.Errorf("download request failed: %w", err)}
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return errUnauthorized
	}
	if response.StatusCode >= 500 {
		return &transientError{err: fmt.Errorf("mirror returned HTTP %d", response.StatusCode)}
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("mirror returned HTTP %d", response.StatusCode)
	}

	flags := os.O_CREATE | os.O_WRONLY
	start := offset
	if offset > 0 && response.StatusCode == http.StatusPartialContent {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
		start = 0
	}
	file, err := os.OpenFile(entry.partPath, flags, 0o600)
	if err != nil {
		return fmt.Errorf("open partial file: %w", err)
	}
	progress := &progressWriter{destination: file, manager: manager, entry: entry, written: start, started: time.Now()}
	_, copyErr := io.CopyBuffer(progress, response.Body, make([]byte, 128*1024))
	syncErr := file.Sync()
	closeErr := file.Close()
	if copyErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("write partial file: %w", copyErr)
	}
	if syncErr != nil {
		return fmt.Errorf("sync partial file: %w", syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close partial file: %w", closeErr)
	}
	if err := verifyPartial(entry.partPath, entry.source.ExpectedSize, entry.source.MD5); err != nil {
		return &validationError{err: err}
	}
	if err := os.Rename(entry.partPath, entry.finalPath); err != nil {
		return fmt.Errorf("complete download: %w", err)
	}
	return nil
}

type progressWriter struct {
	destination io.Writer
	manager     *Manager
	entry       *managedTask
	written     int64
	started     time.Time
	lastEvent   time.Time
}

func (writer *progressWriter) Write(payload []byte) (int, error) {
	written, err := writer.destination.Write(payload)
	writer.written += int64(written)
	now := time.Now()
	emit := writer.lastEvent.IsZero() || now.Sub(writer.lastEvent) >= 250*time.Millisecond
	writer.manager.updateProgress(writer.entry, writer.written, now.Sub(writer.started), emit)
	if emit {
		writer.lastEvent = now
	}
	return written, err
}

func verifyPartial(path string, expectedSize int64, expectedMD5 string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open completed partial file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat completed partial file: %w", err)
	}
	if info.Size() != expectedSize {
		return fmt.Errorf("download size mismatch: got %d, want %d", info.Size(), expectedSize)
	}
	prefix := make([]byte, 5)
	if _, err := io.ReadFull(file, prefix); err != nil || !strings.HasPrefix(string(prefix), "%PDF-") {
		return errors.New("download is not a PDF")
	}
	if expectedMD5 != "" {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("seek completed partial file: %w", err)
		}
		hash := md5.New()
		if _, err := io.Copy(hash, file); err != nil {
			return fmt.Errorf("hash completed partial file: %w", err)
		}
		if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), expectedMD5) {
			return errors.New("download checksum mismatch")
		}
	}
	return nil
}

func (manager *Manager) updateProgress(entry *managedTask, bytes int64, elapsed time.Duration, emit bool) {
	manager.mutex.Lock()
	entry.task.BytesDone = bytes
	if elapsed > 0 {
		entry.task.SpeedBytes = int64(float64(bytes) / elapsed.Seconds())
		if entry.task.SpeedBytes > 0 && entry.task.TotalBytes > bytes {
			entry.task.ETASeconds = (entry.task.TotalBytes - bytes) / entry.task.SpeedBytes
		}
	}
	entry.task.UpdatedAt = time.Now().UTC()
	task := entry.task
	manager.mutex.Unlock()
	if emit {
		manager.publish(task)
	}
}

func (manager *Manager) setMirrorAttempt(entry *managedTask, attempt int) {
	manager.mutex.Lock()
	entry.task.MirrorAttempt = attempt
	entry.task.UpdatedAt = time.Now().UTC()
	task := entry.task
	manager.mutex.Unlock()
	manager.publish(task)
}

func (manager *Manager) finishInterrupted(entry *managedTask) {
	var status TaskStatus
	var remove bool
	var partPath string
	var removeErr error
	var task Task
	for {
		manager.mutex.Lock()
		status = entry.desiredStatus
		if status == "" {
			status = StatusPaused
		}
		remove = status == StatusCanceled && entry.removePartial
		partPath = entry.partPath
		manager.mutex.Unlock()

		removeErr = nil
		if remove {
			removeErr = os.Remove(partPath)
			if os.IsNotExist(removeErr) {
				removeErr = nil
			}
		}

		manager.mutex.Lock()
		latestStatus := entry.desiredStatus
		if latestStatus == "" {
			latestStatus = StatusPaused
		}
		latestRemove := latestStatus == StatusCanceled && entry.removePartial
		if latestStatus != status || latestRemove != remove {
			manager.mutex.Unlock()
			continue
		}
		entry.cancel = nil
		entry.task.Status = status
		entry.task.UpdatedAt = time.Now().UTC()
		if info, err := os.Stat(partPath); err == nil {
			entry.task.BytesDone = info.Size()
		} else if os.IsNotExist(err) {
			entry.task.BytesDone = 0
		}
		if removeErr != nil {
			entry.task.Error = "未完成文件清理失败，请重试"
		} else {
			entry.task.Error = ""
		}
		entry.cleanupErr = removeErr
		task = entry.task
		manager.mutex.Unlock()
		break
	}
	persistErr := manager.persist()
	if persistErr != nil {
		manager.mutex.Lock()
		entry.cleanupErr = persistErr
		manager.mutex.Unlock()
	}
	manager.publish(task)
	manager.mutex.Lock()
	if remove && removeErr == nil && persistErr == nil && manager.tasks[entry.task.ID] == entry {
		delete(manager.tasks, entry.task.ID)
	}
	closeDoneLocked(entry)
	manager.mutex.Unlock()
}

func (manager *Manager) finishFailed(entry *managedTask, err error) {
	manager.mutex.Lock()
	entry.cancel = nil
	entry.task.Status = StatusFailed
	entry.task.Error = appLogging.Redact(err.Error())
	entry.task.UpdatedAt = time.Now().UTC()
	task := entry.task
	manager.mutex.Unlock()
	manager.persist()
	manager.publish(task)
	manager.mutex.Lock()
	closeDoneLocked(entry)
	manager.mutex.Unlock()
}

func (manager *Manager) finishCompleted(entry *managedTask) {
	manager.mutex.Lock()
	entry.cancel = nil
	entry.task.Status = StatusCompleted
	entry.task.BytesDone = entry.task.TotalBytes
	entry.task.SpeedBytes = 0
	entry.task.ETASeconds = 0
	entry.task.UpdatedAt = time.Now().UTC()
	task := entry.task
	manager.mutex.Unlock()
	manager.persist()
	manager.publish(task)
	manager.mutex.Lock()
	closeDoneLocked(entry)
	manager.mutex.Unlock()
}

func (manager *Manager) pauseAllForAuth() {
	manager.session.Clear()
	manager.mutex.Lock()
	now := time.Now().UTC()
	tasks := make([]Task, 0, len(manager.tasks))
	interrupted := make([]*managedTask, 0)
	for _, entry := range manager.tasks {
		if terminal(entry.task.Status) || entry.task.Status == StatusFailed || entry.task.Status == StatusCanceled {
			continue
		}
		if entry.desiredStatus != "" {
			if entry.desiredStatus == StatusPaused || entry.desiredStatus == StatusCanceled {
				interrupted = append(interrupted, entry)
			}
			continue
		}
		entry.desiredStatus = StatusWaitingAuth
		entry.task.Status = StatusWaitingAuth
		entry.task.UpdatedAt = now
		if entry.cancel != nil {
			entry.cancel()
		}
		closeDoneLocked(entry)
		tasks = append(tasks, entry.task)
	}
	manager.mutex.Unlock()
	manager.persist()
	for _, task := range tasks {
		manager.publish(task)
	}
	for _, entry := range interrupted {
		manager.finishInterrupted(entry)
	}
}

func closeDoneLocked(entry *managedTask) {
	if entry.done == nil {
		return
	}
	close(entry.done)
	entry.done = nil
}
