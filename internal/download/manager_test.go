package download

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"smartedu-downloader/internal/resource"
	"smartedu-downloader/internal/storage"
)

func TestWorkerResumesPartFileAndAtomicallyCompletesVerifiedPDF(t *testing.T) {
	pdf := append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte("x"), 4096)...)
	var mutex sync.Mutex
	lastRange := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutex.Lock()
		lastRange = r.Header.Get("Range")
		mutex.Unlock()
		if r.URL.Query().Get("accessToken") != "valid-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Range", "bytes 1024-")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(pdf[1024:])
	}))
	t.Cleanup(server.Close)

	directory := t.TempDir()
	partPath := filepath.Join(directory, "语文.pdf.part")
	if err := os.WriteFile(partPath, pdf[:1024], 0o600); err != nil {
		t.Fatal(err)
	}
	manager := newTestManager(t, server.Client(), newFakeSession("valid-token"), directory, 1)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	task, err := manager.Enqueue(pdfSource("book-1", "语文.pdf", server.URL+"/book.pdf", pdf))
	if err != nil {
		t.Fatal(err)
	}
	completed := waitForStatus(t, manager, task.ID, StatusCompleted)
	if completed.BytesDone != int64(len(pdf)) {
		t.Fatalf("BytesDone = %d, want %d", completed.BytesDone, len(pdf))
	}
	if _, err := os.Stat(filepath.Join(directory, "语文.pdf")); err != nil {
		t.Fatalf("completed PDF missing: %v", err)
	}
	if _, err := os.Stat(partPath); !os.IsNotExist(err) {
		t.Fatalf("part file remained: %v", err)
	}
	mutex.Lock()
	gotRange := lastRange
	mutex.Unlock()
	if gotRange != "bytes=1024-" {
		t.Fatalf("Range = %q, want bytes=1024-", gotRange)
	}
}

func TestNetworkErrorsNeverExposeAccessTokenInTaskOrJournal(t *testing.T) {
	const token = "sensitive-token"
	journal := &memoryJournal{}
	manager, err := NewManager(echoURLClient{}, newFakeSession(token), journal, ManagerConfig{
		DownloadDir: t.TempDir(), Concurrency: 1, DuplicatePolicy: storage.DuplicateOverwrite,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Shutdown(t.Context())
	payload := []byte("%PDF-1.7\ncontent")
	task, err := manager.Enqueue(pdfSource("book", "教材.pdf", "https://example.invalid/book.pdf", payload))
	if err != nil {
		t.Fatal(err)
	}
	failed := waitForStatus(t, manager, task.ID, StatusFailed)
	if strings.Contains(failed.Error, token) {
		t.Fatalf("task error exposed access token: %s", failed.Error)
	}
	journal.mutex.Lock()
	records := append([]storage.TaskRecord(nil), journal.records...)
	journal.mutex.Unlock()
	for _, record := range records {
		if strings.Contains(record.FailureCode, token) {
			t.Fatalf("journal error exposed access token: %s", record.FailureCode)
		}
	}
}

type echoURLClient struct{}

func (echoURLClient) Do(request *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("network error for %s", request.URL.String())
}

func TestWorkerRestartsWhenServerIgnoresRange(t *testing.T) {
	pdf := append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte("y"), 2048)...)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pdf)
	}))
	t.Cleanup(server.Close)
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "数学.pdf.part"), pdf[:100], 0o600); err != nil {
		t.Fatal(err)
	}
	manager := newTestManager(t, server.Client(), newFakeSession("token"), directory, 1)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	task, err := manager.Enqueue(pdfSource("book-2", "数学.pdf", server.URL, pdf))
	if err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, manager, task.ID, StatusCompleted)
	payload, err := os.ReadFile(filepath.Join(directory, "数学.pdf"))
	if err != nil || !bytes.Equal(payload, pdf) {
		t.Fatalf("completed payload differs: err=%v length=%d", err, len(payload))
	}
}

func TestUnauthorizedResponsePausesEveryTaskForReauthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "expired", http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)
	session := newFakeSession("expired")
	manager := newTestManager(t, server.Client(), session, t.TempDir(), 1)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	pdf := []byte("%PDF-test")
	first, _ := manager.Enqueue(pdfSource("first", "first.pdf", server.URL, pdf))
	second, _ := manager.Enqueue(pdfSource("second", "second.pdf", server.URL, pdf))
	waitForStatus(t, manager, first.ID, StatusWaitingAuth)
	waitForStatus(t, manager, second.ID, StatusWaitingAuth)
	if _, ok := session.Token(); ok {
		t.Fatal("expired session token was not cleared")
	}
}

func TestCancelRemovesPartialWhenRequested(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("%PDF-"))
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	directory := t.TempDir()
	journal := &memoryJournal{}
	manager, err := NewManager(server.Client(), newFakeSession("token"), journal, ManagerConfig{
		DownloadDir: directory, Concurrency: 1, DuplicatePolicy: storage.DuplicateOverwrite,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	pdf := append([]byte("%PDF-"), bytes.Repeat([]byte("z"), 1000)...)
	task, _ := manager.Enqueue(pdfSource("book", "cancel.pdf", server.URL, pdf))
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("download did not start")
	}
	if err := manager.Cancel(task.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, ok := manager.Task(task.ID); ok {
		t.Fatal("cancel returned before removing the task record")
	}
	if _, err := os.Stat(filepath.Join(directory, "cancel.pdf.part")); !os.IsNotExist(err) {
		t.Fatalf("partial file remained after cancellation: %v", err)
	}
	journal.mutex.Lock()
	defer journal.mutex.Unlock()
	for _, record := range journal.records {
		if record.ID == task.ID {
			t.Fatalf("canceled task remained in journal: %#v", record)
		}
	}
}

func TestCanceledTaskCanRemoveAnEarlierRetainedPartial(t *testing.T) {
	directory := t.TempDir()
	manager := newTestManager(t, http.DefaultClient, newFakeSession(""), directory, 1)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	partPath := filepath.Join(directory, "canceled.pdf.part")
	if err := os.WriteFile(partPath, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager.mutex.Lock()
	manager.tasks["old-canceled"] = &managedTask{
		task:     Task{ID: "old-canceled", Filename: "canceled.pdf", Status: StatusCanceled},
		partPath: partPath,
	}
	manager.mutex.Unlock()

	if err := manager.Cancel("old-canceled", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(partPath); !os.IsNotExist(err) {
		t.Fatalf("retained partial file still exists: %v", err)
	}
	if _, ok := manager.Task("old-canceled"); ok {
		t.Fatal("old canceled task record remained in manager")
	}
}

func TestCancelUnblocksWhenWorkerCompletes(t *testing.T) {
	manager := newTestManager(t, http.DefaultClient, newFakeSession("token"), t.TempDir(), 1)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	entry := &managedTask{task: Task{ID: "completed-race", Status: StatusDownloading}, done: make(chan struct{})}
	manager.mutex.Lock()
	manager.tasks[entry.task.ID] = entry
	manager.mutex.Unlock()

	cancelStarted := make(chan struct{})
	var cancelOnce sync.Once
	entry.cancel = func() { cancelOnce.Do(func() { close(cancelStarted) }) }
	result := make(chan error, 1)
	go func() { result <- manager.Cancel(entry.task.ID, true) }()
	<-cancelStarted
	manager.finishCompleted(entry)
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("Cancel returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Cancel remained blocked after worker completion")
	}
}

func TestCancelUnblocksWhenWorkerFails(t *testing.T) {
	manager := newTestManager(t, http.DefaultClient, newFakeSession("token"), t.TempDir(), 1)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	entry := &managedTask{task: Task{ID: "failed-race", Status: StatusDownloading}, done: make(chan struct{})}
	manager.mutex.Lock()
	manager.tasks[entry.task.ID] = entry
	manager.mutex.Unlock()

	cancelStarted := make(chan struct{})
	var cancelOnce sync.Once
	entry.cancel = func() { cancelOnce.Do(func() { close(cancelStarted) }) }
	result := make(chan error, 1)
	go func() { result <- manager.Cancel(entry.task.ID, true) }()
	<-cancelStarted
	manager.finishFailed(entry, errors.New("test failure"))
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("Cancel returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Cancel remained blocked after worker failure")
	}
}

func TestCancelRemovesPartialWhenWorkerFails(t *testing.T) {
	directory := t.TempDir()
	journal := &memoryJournal{}
	manager, err := NewManager(http.DefaultClient, newFakeSession("token"), journal, ManagerConfig{
		DownloadDir: directory, Concurrency: 1, DuplicatePolicy: storage.DuplicateOverwrite,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	partPath := filepath.Join(directory, "failed-cancel.pdf.part")
	if err := os.WriteFile(partPath, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	entry := &managedTask{
		task:     Task{ID: "failed-cancel-race", Filename: "failed-cancel.pdf", Status: StatusDownloading},
		partPath: partPath,
		done:     make(chan struct{}),
	}
	manager.mutex.Lock()
	manager.tasks[entry.task.ID] = entry
	manager.mutex.Unlock()

	cancelStarted := make(chan struct{})
	var cancelOnce sync.Once
	entry.cancel = func() { cancelOnce.Do(func() { close(cancelStarted) }) }
	result := make(chan error, 1)
	go func() { result <- manager.Cancel(entry.task.ID, true) }()
	<-cancelStarted
	manager.finishFailed(entry, errors.New("test failure"))
	if err := <-result; err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}
	if _, err := os.Stat(partPath); !os.IsNotExist(err) {
		t.Fatalf("partial file remained after cancellation: %v", err)
	}
	if _, ok := manager.Task(entry.task.ID); ok {
		t.Fatal("failed cancellation task remained in manager")
	}
	journal.mutex.Lock()
	defer journal.mutex.Unlock()
	for _, record := range journal.records {
		if record.ID == entry.task.ID {
			t.Fatalf("failed cancellation task remained in journal: %#v", record)
		}
	}
}

func TestCancelUnblocksWhenWorkerReportsExpiredSession(t *testing.T) {
	directory := t.TempDir()
	manager := newTestManager(t, http.DefaultClient, newFakeSession("token"), directory, 1)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	partPath := filepath.Join(directory, "auth-race.pdf.part")
	if err := os.WriteFile(partPath, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	entry := &managedTask{
		task:     Task{ID: "auth-race", Filename: "auth-race.pdf", Status: StatusDownloading},
		partPath: partPath,
		done:     make(chan struct{}),
	}
	manager.mutex.Lock()
	manager.tasks[entry.task.ID] = entry
	manager.mutex.Unlock()

	cancelStarted := make(chan struct{})
	var cancelOnce sync.Once
	entry.cancel = func() { cancelOnce.Do(func() { close(cancelStarted) }) }
	result := make(chan error, 1)
	go func() { result <- manager.Cancel(entry.task.ID, true) }()
	<-cancelStarted
	manager.pauseAllForAuth()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("Cancel returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Cancel remained blocked after session expiry")
	}
	if _, err := os.Stat(partPath); !os.IsNotExist(err) {
		t.Fatalf("partial file remained after cancellation: %v", err)
	}
	if _, ok := manager.Task(entry.task.ID); ok {
		t.Fatal("auth-race cancellation task remained in manager")
	}
}

type fakeSession struct {
	mutex sync.RWMutex
	token string
}

func newFakeSession(token string) *fakeSession { return &fakeSession{token: token} }
func (session *fakeSession) Token() (string, bool) {
	session.mutex.RLock()
	defer session.mutex.RUnlock()
	return session.token, session.token != ""
}
func (session *fakeSession) Clear() {
	session.mutex.Lock()
	session.token = ""
	session.mutex.Unlock()
}

type memoryJournal struct {
	mutex   sync.Mutex
	records []storage.TaskRecord
}

func TestEnqueuePreservesRecoverableRecordsOwnedByAnEarlierProcess(t *testing.T) {
	directory := t.TempDir()
	journal := &memoryJournal{records: []storage.TaskRecord{{ID: "recovery-task", BookID: "old-book", Filename: "旧任务.pdf", Status: "paused"}}}
	manager, err := NewManager(http.DefaultClient, newFakeSession(""), journal, ManagerConfig{
		DownloadDir: directory, Concurrency: 1, DuplicatePolicy: storage.DuplicateOverwrite,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Shutdown(t.Context())
	payload := []byte("%PDF-1.7\nnew")
	if _, err := manager.Enqueue(pdfSource("new-book", "新任务.pdf", "https://example.invalid/new.pdf", payload)); err != nil {
		t.Fatal(err)
	}

	journal.mutex.Lock()
	records := append([]storage.TaskRecord(nil), journal.records...)
	journal.mutex.Unlock()
	foundRecovery := false
	for _, record := range records {
		if record.ID == "recovery-task" {
			foundRecovery = true
		}
	}
	if !foundRecovery {
		t.Fatalf("enqueue replaced the earlier recovery record: %#v", records)
	}
}

func (journal *memoryJournal) Save(records []storage.TaskRecord) error {
	journal.mutex.Lock()
	journal.records = append([]storage.TaskRecord(nil), records...)
	journal.mutex.Unlock()
	return nil
}

func (journal *memoryJournal) Reconcile(ownedIDs []string, records []storage.TaskRecord) error {
	journal.mutex.Lock()
	defer journal.mutex.Unlock()
	owned := make(map[string]struct{}, len(ownedIDs))
	for _, id := range ownedIDs {
		owned[id] = struct{}{}
	}
	merged := make([]storage.TaskRecord, 0, len(journal.records)+len(records))
	for _, record := range journal.records {
		if _, replace := owned[record.ID]; !replace {
			merged = append(merged, record)
		}
	}
	journal.records = append(merged, records...)
	return nil
}

func newTestManager(t *testing.T, client HTTPClient, session Session, directory string, concurrency int) *Manager {
	t.Helper()
	manager, err := NewManager(client, session, &memoryJournal{}, ManagerConfig{
		DownloadDir: directory, Concurrency: concurrency, DuplicatePolicy: storage.DuplicateOverwrite,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func pdfSource(id, filename, mirror string, payload []byte) resource.PDFSource {
	sum := md5.Sum(payload)
	return resource.PDFSource{BookID: id, Filename: filename, ExpectedSize: int64(len(payload)), MD5: hex.EncodeToString(sum[:]), Mirrors: []string{mirror}}
}

func waitForStatus(t *testing.T, manager *Manager, id string, status TaskStatus) Task {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		task, ok := manager.Task(id)
		if ok && task.Status == status {
			return task
		}
		time.Sleep(10 * time.Millisecond)
	}
	task, _ := manager.Task(id)
	t.Fatalf("task %s status = %s, want %s; error=%s", id, task.Status, status, task.Error)
	return Task{}
}
