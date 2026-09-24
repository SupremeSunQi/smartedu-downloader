package download

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"smartedu-downloader/internal/resource"
	"smartedu-downloader/internal/storage"
)

var (
	ErrManagerClosed          = errors.New("download manager is shutting down")
	ErrTaskNotFound           = errors.New("download task was not found")
	ErrInvalidSource          = errors.New("download source is invalid")
	ErrDuplicateNeedsDecision = errors.New("destination file already exists")
	ErrDuplicateSkipped       = errors.New("destination file was skipped")
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Session interface {
	Token() (string, bool)
	Clear()
}

type Journal interface {
	Save([]storage.TaskRecord) error
}

type ManagerConfig struct {
	DownloadDir     string
	Concurrency     int
	DuplicatePolicy storage.DuplicatePolicy
}

type managedTask struct {
	task          Task
	source        resource.PDFSource
	finalPath     string
	partPath      string
	cancel        context.CancelFunc
	done          chan struct{}
	cleanupErr    error
	desiredStatus TaskStatus
	removePartial bool
}

type Manager struct {
	mutex     sync.RWMutex
	persistMu sync.Mutex
	client    HTTPClient
	session   Session
	journal   Journal
	config    ManagerConfig
	onEvent   func(TaskEvent)
	tasks     map[string]*managedTask
	queue     chan string
	ctx       context.Context
	cancel    context.CancelFunc
	workers   sync.WaitGroup
	accepting bool
}

func NewManager(client HTTPClient, session Session, journal Journal, config ManagerConfig, onEvent func(TaskEvent)) (*Manager, error) {
	if client == nil || session == nil || journal == nil {
		return nil, errors.New("download manager dependencies are required")
	}
	if config.Concurrency < 1 || config.Concurrency > 5 || strings.TrimSpace(config.DownloadDir) == "" {
		return nil, errors.New("download manager configuration is invalid")
	}
	if err := os.MkdirAll(config.DownloadDir, 0o700); err != nil {
		return nil, fmt.Errorf("create download directory: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	manager := &Manager{
		client: client, session: session, journal: journal, config: config, onEvent: onEvent,
		tasks: make(map[string]*managedTask), queue: make(chan string, 256), ctx: ctx, cancel: cancel, accepting: true,
	}
	for range config.Concurrency {
		manager.workers.Add(1)
		go manager.worker()
	}
	return manager, nil
}

func (manager *Manager) Enqueue(source resource.PDFSource) (Task, error) {
	if source.BookID == "" || source.Filename == "" || source.ExpectedSize <= 0 || len(source.Mirrors) == 0 {
		return Task{}, ErrInvalidSource
	}
	filename := filepath.Base(source.Filename)
	if filename != source.Filename || filename == "." {
		return Task{}, ErrInvalidSource
	}
	finalPath := filepath.Join(manager.config.DownloadDir, filename)
	resolvedFilename, resolvedPath, err := manager.resolveDuplicate(filename, finalPath)
	if err != nil {
		return Task{}, err
	}
	source.Filename = resolvedFilename
	partPath := resolvedPath + ".part"
	partialBytes := int64(0)
	if info, statErr := os.Stat(partPath); statErr == nil {
		partialBytes = info.Size()
	} else if !os.IsNotExist(statErr) {
		return Task{}, fmt.Errorf("inspect partial download: %w", statErr)
	}

	now := time.Now().UTC()
	entry := &managedTask{
		task:   Task{ID: newTaskID(), BookID: source.BookID, Filename: resolvedFilename, Status: StatusQueued, BytesDone: partialBytes, TotalBytes: source.ExpectedSize, CreatedAt: now, UpdatedAt: now},
		source: source, finalPath: resolvedPath, partPath: partPath, done: make(chan struct{}),
	}
	manager.mutex.Lock()
	if !manager.accepting {
		manager.mutex.Unlock()
		return Task{}, ErrManagerClosed
	}
	manager.tasks[entry.task.ID] = entry
	task := entry.task
	manager.mutex.Unlock()
	manager.persist()
	manager.publish(task)
	manager.queueTask(task.ID)
	return task, nil
}

func (manager *Manager) Task(id string) (Task, bool) {
	manager.mutex.RLock()
	defer manager.mutex.RUnlock()
	entry, ok := manager.tasks[id]
	if !ok {
		return Task{}, false
	}
	return entry.task, true
}

func (manager *Manager) Tasks() []Task {
	manager.mutex.RLock()
	result := make([]Task, 0, len(manager.tasks))
	for _, entry := range manager.tasks {
		result = append(result, entry.task)
	}
	manager.mutex.RUnlock()
	sort.SliceStable(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result
}

func (manager *Manager) Pause(id string) error {
	return manager.interrupt(id, StatusPaused, false)
}

func (manager *Manager) Cancel(id string, removePartial bool) error {
	return manager.interrupt(id, StatusCanceled, removePartial)
}

func (manager *Manager) interrupt(id string, status TaskStatus, removePartial bool) error {
	manager.mutex.Lock()
	entry, ok := manager.tasks[id]
	if !ok {
		manager.mutex.Unlock()
		return ErrTaskNotFound
	}
	if terminal(entry.task.Status) {
		canceled := entry.task.Status == StatusCanceled
		partPath := entry.partPath
		manager.mutex.Unlock()
		if canceled && removePartial {
			if err := os.Remove(partPath); err != nil && !os.IsNotExist(err) {
				return err
			}
			if err := manager.persist(); err != nil {
				return err
			}
			manager.mutex.Lock()
			if manager.tasks[id] == entry {
				delete(manager.tasks, id)
			}
			manager.mutex.Unlock()
		}
		return nil
	}
	entry.desiredStatus = status
	entry.removePartial = removePartial
	cancel := entry.cancel
	done := entry.done
	active := entry.task.Status == StatusDownloading
	if !active {
		entry.task.Status = status
		entry.task.UpdatedAt = time.Now().UTC()
	}
	task := entry.task
	partPath := entry.partPath
	manager.mutex.Unlock()
	if cancel != nil {
		cancel()
	}
	if active && status == StatusCanceled && removePartial && done != nil {
		<-done
		manager.mutex.RLock()
		cleanupErr := entry.cleanupErr
		finalStatus := entry.task.Status
		manager.mutex.RUnlock()
		if cleanupErr != nil {
			return cleanupErr
		}
		if finalStatus != StatusCanceled && finalStatus != StatusCompleted && finalStatus != StatusFailed {
			return errors.New("download completed before cancellation could finish")
		}
		if err := os.Remove(partPath); err != nil && !os.IsNotExist(err) {
			manager.mutex.Lock()
			entry.cleanupErr = err
			entry.task.Error = "未完成文件清理失败，请重试"
			task = entry.task
			manager.mutex.Unlock()
			manager.publish(task)
			return err
		}
		publishTask := false
		manager.mutex.Lock()
		if manager.tasks[id] == entry {
			entry.task.Status = StatusCanceled
			entry.task.Error = ""
			entry.task.UpdatedAt = time.Now().UTC()
			task = entry.task
			publishTask = true
		}
		manager.mutex.Unlock()
		if publishTask {
			manager.publish(task)
		}
		if err := manager.persist(); err != nil {
			return err
		}
		manager.mutex.Lock()
		if manager.tasks[id] == entry {
			delete(manager.tasks, id)
		}
		manager.mutex.Unlock()
		return nil
	}
	if !active {
		if status == StatusCanceled && removePartial {
			if err := os.Remove(partPath); err != nil && !os.IsNotExist(err) {
				manager.mutex.Lock()
				entry.task.Error = "未完成文件清理失败，请重试"
				task = entry.task
				manager.mutex.Unlock()
				manager.publish(task)
				return err
			}
			if err := manager.persist(); err != nil {
				return err
			}
			manager.mutex.Lock()
			if manager.tasks[id] == entry {
				delete(manager.tasks, id)
			}
			manager.mutex.Unlock()
		} else {
			manager.persist()
		}
		manager.publish(task)
	}
	return nil
}

func (manager *Manager) Resume(id string) error {
	manager.mutex.Lock()
	entry, ok := manager.tasks[id]
	if !ok {
		manager.mutex.Unlock()
		return ErrTaskNotFound
	}
	if entry.task.Status != StatusPaused && entry.task.Status != StatusWaitingAuth {
		manager.mutex.Unlock()
		return nil
	}
	entry.desiredStatus = ""
	entry.removePartial = false
	entry.task.Status = StatusQueued
	entry.task.Error = ""
	entry.task.UpdatedAt = time.Now().UTC()
	task := entry.task
	manager.mutex.Unlock()
	manager.persist()
	manager.publish(task)
	manager.queueTask(id)
	return nil
}

func (manager *Manager) Retry(id string) error {
	manager.mutex.RLock()
	entry, ok := manager.tasks[id]
	status := TaskStatus("")
	if ok {
		status = entry.task.Status
	}
	manager.mutex.RUnlock()
	if !ok {
		return ErrTaskNotFound
	}
	if status != StatusFailed {
		return nil
	}
	manager.mutex.Lock()
	entry.desiredStatus = ""
	entry.task.Status = StatusQueued
	entry.task.Error = ""
	entry.task.UpdatedAt = time.Now().UTC()
	task := entry.task
	manager.mutex.Unlock()
	manager.persist()
	manager.publish(task)
	manager.queueTask(id)
	return nil
}

func (manager *Manager) Shutdown(ctx context.Context) error {
	manager.mutex.Lock()
	manager.accepting = false
	for _, entry := range manager.tasks {
		if terminal(entry.task.Status) {
			continue
		}
		entry.desiredStatus = StatusPaused
		if entry.task.Status != StatusDownloading {
			entry.task.Status = StatusPaused
			entry.task.UpdatedAt = time.Now().UTC()
		}
		if entry.cancel != nil {
			entry.cancel()
		}
	}
	manager.mutex.Unlock()
	manager.cancel()

	done := make(chan struct{})
	go func() {
		manager.workers.Wait()
		close(done)
	}()
	select {
	case <-done:
		manager.persist()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (manager *Manager) worker() {
	defer manager.workers.Done()
	for {
		select {
		case <-manager.ctx.Done():
			return
		case id := <-manager.queue:
			manager.process(id)
		}
	}
}

func (manager *Manager) queueTask(id string) {
	select {
	case manager.queue <- id:
	case <-manager.ctx.Done():
	}
}

func (manager *Manager) resolveDuplicate(filename, finalPath string) (string, string, error) {
	if _, err := os.Stat(finalPath); os.IsNotExist(err) {
		return filename, finalPath, nil
	} else if err != nil {
		return "", "", fmt.Errorf("inspect destination: %w", err)
	}
	switch manager.config.DuplicatePolicy {
	case storage.DuplicateAsk:
		return "", "", ErrDuplicateNeedsDecision
	case storage.DuplicateSkip:
		return "", "", ErrDuplicateSkipped
	case storage.DuplicateOverwrite:
		return filename, finalPath, nil
	case storage.DuplicateRename:
		extension := filepath.Ext(filename)
		base := strings.TrimSuffix(filename, extension)
		for index := 2; index < 10_000; index++ {
			candidate := fmt.Sprintf("%s (%d)%s", base, index, extension)
			path := filepath.Join(manager.config.DownloadDir, candidate)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				return candidate, path, nil
			}
		}
		return "", "", errors.New("could not allocate a duplicate filename")
	default:
		return "", "", errors.New("invalid duplicate policy")
	}
}

func (manager *Manager) persist() error {
	manager.persistMu.Lock()
	defer manager.persistMu.Unlock()
	tasks := manager.Tasks()
	records := make([]storage.TaskRecord, 0, len(tasks))
	ownedIDs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ownedIDs = append(ownedIDs, task.ID)
		if terminal(task.Status) {
			continue
		}
		records = append(records, storage.TaskRecord{
			ID: task.ID, BookID: task.BookID, Filename: task.Filename, Status: string(task.Status),
			PartialBytes: task.BytesDone, ExpectedSize: task.TotalBytes, CreatedAt: task.CreatedAt,
			UpdatedAt: task.UpdatedAt, FailureCode: task.Error, MirrorAttempt: task.MirrorAttempt,
		})
	}
	if journal, ok := manager.journal.(interface {
		Reconcile([]string, []storage.TaskRecord) error
	}); ok {
		return journal.Reconcile(ownedIDs, records)
	}
	return manager.journal.Save(records)
}

func (manager *Manager) publish(task Task) {
	if manager.onEvent != nil {
		manager.onEvent(TaskEvent{Task: task})
	}
}

func terminal(status TaskStatus) bool {
	return status == StatusCompleted || status == StatusCanceled
}

func newTaskID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err == nil {
		return hex.EncodeToString(buffer)
	}
	return fmt.Sprintf("task-%d", time.Now().UnixNano())
}
