package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"smartedu-downloader/internal/auth"
	"smartedu-downloader/internal/browserauth"
	"smartedu-downloader/internal/catalog"
	"smartedu-downloader/internal/download"
	"smartedu-downloader/internal/resource"
	"smartedu-downloader/internal/storage"
)

func TestBootstrapDoesNotExposeCatalogUntilAuthenticated(t *testing.T) {
	app := newTestApp(t, false)
	state, err := app.Bootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if state.Session.Authenticated {
		t.Fatal("anonymous bootstrap reported authenticated")
	}
	if len(state.Catalog.Textbooks) != 0 {
		t.Fatalf("anonymous bootstrap exposed %d textbooks", len(state.Catalog.Textbooks))
	}
}

func TestQueryCatalogReturnsBooksAfterValidatedToken(t *testing.T) {
	app := newTestApp(t, true)
	books, err := app.QueryCatalog(catalog.Query{Search: "语文"})
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].ID != "book-1" {
		t.Fatalf("QueryCatalog returned %#v", books)
	}
}

func TestQueryCatalogRejectsAnonymousCaller(t *testing.T) {
	app := newTestApp(t, false)
	_, err := app.QueryCatalog(catalog.Query{})
	var uiError *UIError
	if !errors.As(err, &uiError) || uiError.Code != "AUTH_REQUIRED" {
		t.Fatalf("QueryCatalog error = %v, want AUTH_REQUIRED", err)
	}
}

func TestDetectBrowserSessionSkipsInvalidCandidateAndAcceptsValidOne(t *testing.T) {
	app := newBrowserAuthTestApp(t, &fakeBrowserSessions{candidates: []browserauth.Candidate{
		{Browser: "Chrome", Profile: "Default", Token: "expired-candidate"},
		{Browser: "Edge", Profile: "Default", Token: "valid-candidate"},
	}})
	app.session = &selectiveSession{validToken: "valid-candidate"}

	status, err := app.DetectBrowserSession()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Authenticated {
		t.Fatal("DetectBrowserSession did not authenticate a valid candidate")
	}
}

func TestSubmitTokenTrimsAndValidatesToken(t *testing.T) {
	app := newTestApp(t, false)
	app.session = &selectiveSession{validToken: "accepted-token"}

	status, err := app.SubmitToken(" accepted-token ")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Authenticated {
		t.Fatal("SubmitToken did not authenticate a validated token")
	}
}

func TestSubmitTokenRejectsInvalidTokenWithoutEchoingIt(t *testing.T) {
	const submittedToken = "manual-token-must-not-appear-in-errors"
	app := newTestApp(t, false)
	app.session = &selectiveSession{validToken: "different-token"}

	_, err := app.SubmitToken(submittedToken)
	var visible *UIError
	if !errors.As(err, &visible) || visible.Code != "AUTH_TOKEN_INVALID" {
		t.Fatalf("SubmitToken error = %v, want AUTH_TOKEN_INVALID", err)
	}
	if strings.Contains(err.Error(), submittedToken) {
		t.Fatal("SubmitToken error exposed submitted token")
	}
}

func TestOpenBrowserLoginUsesInjectedFixedLauncher(t *testing.T) {
	app := newTestApp(t, false)
	called := false
	app.openLoginPage = func() error {
		called = true
		return nil
	}

	if err := app.OpenBrowserLogin(); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("OpenBrowserLogin did not call the injected launcher")
	}
}

func TestDetectBrowserSessionReportsMissingAndDoesNotEchoCandidateToken(t *testing.T) {
	app := newBrowserAuthTestApp(t, &fakeBrowserSessions{candidates: nil})
	_, err := app.DetectBrowserSession()
	var uiError *UIError
	if !errors.As(err, &uiError) || uiError.Code != "AUTH_BROWSER_SESSION_MISSING" {
		t.Fatalf("DetectBrowserSession error = %v, want AUTH_BROWSER_SESSION_MISSING", err)
	}
	if strings.Contains(err.Error(), "candidate-token") {
		t.Fatal("browser detection error exposed candidate token")
	}
}

func TestDetectBrowserSessionMapsExpiredBrowserStorage(t *testing.T) {
	app := newBrowserAuthTestApp(t, &fakeBrowserSessions{err: browserauth.ErrExpired})
	_, err := app.DetectBrowserSession()
	var uiError *UIError
	if !errors.As(err, &uiError) || uiError.Code != "AUTH_BROWSER_SESSION_EXPIRED" {
		t.Fatalf("DetectBrowserSession error = %v, want AUTH_BROWSER_SESSION_EXPIRED", err)
	}
}

func TestOpenBrowserLoginMapsLauncherFailure(t *testing.T) {
	app := newTestApp(t, false)
	app.openLoginPage = func() error { return errors.New("shell failure") }
	if err := app.OpenBrowserLogin(); err == nil {
		t.Fatal("OpenBrowserLogin unexpectedly succeeded")
	} else {
		var uiError *UIError
		if !errors.As(err, &uiError) || uiError.Code != "AUTH_BROWSER_LAUNCH_FAILED" {
			t.Fatalf("OpenBrowserLogin error = %v, want AUTH_BROWSER_LAUNCH_FAILED", err)
		}
	}
}

func TestEnqueueDownloadsRejectsUnknownBookID(t *testing.T) {
	app := newTestApp(t, true)
	_, err := app.EnqueueDownloads([]string{"missing"})
	var uiError *UIError
	if !errors.As(err, &uiError) || uiError.Code != "BOOK_NOT_FOUND" {
		t.Fatalf("EnqueueDownloads error = %v, want BOOK_NOT_FOUND", err)
	}
}

func TestResumeDownloadRehydratesRecoverableTask(t *testing.T) {
	layout, err := storage.ResolveLayout(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	downloads := &fakeDownloads{resumeErr: download.ErrTaskNotFound}
	journal := &fakeAppJournal{records: []storage.TaskRecord{
		{ID: "old-task", BookID: "book-1", Filename: "语文一年级.pdf", Status: "paused"},
		{ID: "other-task", BookID: "book-2", Filename: "数学一年级.pdf", Status: "paused"},
	}}
	app := NewApp(AppDependencies{
		Layout: layout, Config: storage.DefaultConfig(layout), Session: &fakeAppSession{authenticated: true},
		Catalog:  &fakeCatalog{snapshot: catalog.Snapshot{Version: 1, Textbooks: []catalog.Textbook{{ID: "book-1", Title: "语文一年级"}}}},
		Resource: &fakeResource{}, Downloads: downloads,
		Journal:     journal,
		Coordinator: &fakeCoordinator{},
	})
	if err := app.ResumeDownload("old-task"); err != nil {
		t.Fatal(err)
	}
	if len(downloads.tasks) != 1 || downloads.tasks[0].BookID != "book-1" {
		t.Fatalf("rehydrated tasks = %#v", downloads.tasks)
	}
	if len(journal.records) != 1 || journal.records[0].ID != "other-task" {
		t.Fatalf("journal records after one recovery = %#v, want only other-task", journal.records)
	}
}

func TestCancelRecoverableTaskRemovesPartialAndJournalRecord(t *testing.T) {
	layout, err := storage.ResolveLayout(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := storage.DefaultConfig(layout)
	if err := os.MkdirAll(config.DownloadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	partPath := filepath.Join(config.DownloadDir, "语文一年级.pdf.part")
	if err := os.WriteFile(partPath, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	journal := &fakeAppJournal{records: []storage.TaskRecord{{ID: "old-task", BookID: "book-1", Filename: "语文一年级.pdf", Status: "paused"}}}
	app := NewApp(AppDependencies{
		Layout: layout, Config: config, Session: &fakeAppSession{authenticated: true},
		Catalog: &fakeCatalog{}, Resource: &fakeResource{}, Downloads: &fakeDownloads{cancelErr: download.ErrTaskNotFound},
		Journal: journal, Coordinator: &fakeCoordinator{},
	})
	if err := app.CancelDownload("old-task", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(partPath); !os.IsNotExist(err) {
		t.Fatalf("partial file still exists: %v", err)
	}
	if len(journal.records) != 0 {
		t.Fatalf("journal records = %#v, want empty", journal.records)
	}
}

func TestLogoutPausesActiveDownloadsBeforeClearingSession(t *testing.T) {
	app := newTestApp(t, true)
	downloads := app.downloads.(*fakeDownloads)
	downloads.tasks = []download.Task{
		{ID: "active", Status: download.StatusDownloading},
		{ID: "queued", Status: download.StatusQueued},
		{ID: "done", Status: download.StatusCompleted},
	}
	if err := app.Logout(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(downloads.pausedIDs, ",") != "active,queued" {
		t.Fatalf("paused IDs = %v, want active and queued", downloads.pausedIDs)
	}
	if app.session.Status().Authenticated {
		t.Fatal("session remained authenticated after logout")
	}
}

func TestClearIncompleteCancelsCurrentTasksAndDeletesOnlyOlderRecoveryFilesDirectly(t *testing.T) {
	layout, err := storage.ResolveLayout(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := storage.DefaultConfig(layout)
	if err := os.MkdirAll(config.DownloadDir, 0o700); err != nil {
		t.Fatal(err)
	}
	oldPart := filepath.Join(config.DownloadDir, "旧任务.pdf.part")
	currentPart := filepath.Join(config.DownloadDir, "当前任务.pdf.part")
	for _, path := range []string{oldPart, currentPart} {
		if err := os.WriteFile(path, []byte("partial"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	downloads := &fakeDownloads{tasks: []download.Task{{ID: "current", Filename: "当前任务.pdf", Status: download.StatusDownloading}}}
	journal := &fakeAppJournal{records: []storage.TaskRecord{
		{ID: "current", Filename: "当前任务.pdf", Status: "paused"},
		{ID: "older", Filename: "旧任务.pdf", Status: "paused"},
	}}
	app := NewApp(AppDependencies{
		Layout: layout, Config: config, Session: &fakeAppSession{}, Catalog: &fakeCatalog{}, Resource: &fakeResource{},
		Downloads: downloads, Journal: journal, Coordinator: &fakeCoordinator{},
	})
	if err := app.ClearIncomplete(); err != nil {
		t.Fatal(err)
	}
	if len(downloads.canceledIDs) != 1 || downloads.canceledIDs[0] != "current:true" {
		t.Fatalf("canceled IDs = %v, want current:true", downloads.canceledIDs)
	}
	if _, err := os.Stat(oldPart); !os.IsNotExist(err) {
		t.Fatalf("older partial file still exists: %v", err)
	}
	if _, err := os.Stat(currentPart); err != nil {
		t.Fatalf("current partial was removed directly while its worker may own it: %v", err)
	}
	if len(journal.records) != 1 || journal.records[0].ID != "current" {
		t.Fatalf("journal records = %#v, want current task left for manager cleanup", journal.records)
	}
}

func TestNativeDirectoryOperationsUseConfiguredDownloadDirectory(t *testing.T) {
	layout, err := storage.ResolveLayout(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := storage.DefaultConfig(layout)
	var chosenFrom, opened string
	app := NewApp(AppDependencies{
		Layout: layout, Config: config, Session: &fakeAppSession{}, Catalog: &fakeCatalog{}, Resource: &fakeResource{},
		Downloads: &fakeDownloads{}, Journal: &fakeAppJournal{}, Coordinator: &fakeCoordinator{},
		ChooseDirectory: func(_ context.Context, current string) (string, error) {
			chosenFrom = current
			return filepath.Join(current, "chosen"), nil
		},
		OpenDirectory: func(path string) error { opened = path; return nil },
	})
	selected, err := app.ChooseDownloadDirectory("")
	if err != nil {
		t.Fatal(err)
	}
	if chosenFrom != config.DownloadDir || selected != filepath.Join(config.DownloadDir, "chosen") {
		t.Fatalf("choose directory from %q returned %q", chosenFrom, selected)
	}
	if err := app.OpenDownloadFolder(); err != nil {
		t.Fatal(err)
	}
	if opened != config.DownloadDir {
		t.Fatalf("opened %q, want %q", opened, config.DownloadDir)
	}
}

func TestShutdownCoordinatesBeforeQuittingWindow(t *testing.T) {
	app := newTestApp(t, true)
	coordinator := app.coordinator.(*fakeCoordinator)
	app.quit = func(context.Context) {
		coordinator.quitAfter = coordinator.called && app.ShutdownInProgress()
	}
	if err := app.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if !coordinator.quitAfter {
		t.Fatal("window quit ran before shutdown coordination or close approval")
	}
}

func TestUpdateSettingsKeepsRuntimeDownloadConfigurationUntilRestart(t *testing.T) {
	app := newTestApp(t, true)
	runtimeConfig := app.currentConfig()
	futureConfig := runtimeConfig
	futureConfig.DownloadDir = filepath.Join(t.TempDir(), "next-downloads")
	futureConfig.ConcurrentDownloads = 4
	futureConfig.DuplicatePolicy = storage.DuplicateRename

	saved, err := app.UpdateSettings(futureConfig)
	if err != nil {
		t.Fatal(err)
	}
	if saved.DownloadDir != futureConfig.DownloadDir || saved.ConcurrentDownloads != 4 || saved.DuplicatePolicy != storage.DuplicateRename {
		t.Fatalf("saved config = %#v, want %#v", saved, futureConfig)
	}
	if active := app.currentConfig(); active != runtimeConfig {
		t.Fatalf("runtime config changed before restart: got %#v, want %#v", active, runtimeConfig)
	}
	persisted, err := storage.LoadConfig(app.layout)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.DownloadDir != futureConfig.DownloadDir || persisted.ConcurrentDownloads != 4 || persisted.DuplicatePolicy != storage.DuplicateRename {
		t.Fatalf("persisted config = %#v, want %#v", persisted, futureConfig)
	}
}

func newTestApp(t *testing.T, authenticated bool) *App {
	t.Helper()
	layout, err := storage.ResolveLayout(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := storage.DefaultConfig(layout)
	session := &fakeAppSession{authenticated: authenticated}
	snapshot := catalog.Snapshot{Version: 1, Textbooks: []catalog.Textbook{
		{ID: "book-1", Title: "语文一年级"},
		{ID: "book-2", Title: "数学二年级"},
	}}
	return NewApp(AppDependencies{
		Layout: layout, Config: config, Session: session,
		Catalog: &fakeCatalog{snapshot: snapshot}, Resource: &fakeResource{},
		Downloads: &fakeDownloads{}, Journal: &fakeAppJournal{}, Coordinator: &fakeCoordinator{},
	})
}

func newBrowserAuthTestApp(t *testing.T, sessions browserSessionService) *App {
	app := newTestApp(t, false)
	app.browserSessions = sessions
	return app
}

type fakeAppSession struct{ authenticated bool }

func (session *fakeAppSession) Status() auth.Status {
	return auth.Status{Authenticated: session.authenticated}
}
func (session *fakeAppSession) SetValidated(context.Context, string, resource.PDFSource) error {
	session.authenticated = true
	return nil
}
func (session *fakeAppSession) Clear() { session.authenticated = false }

type selectiveSession struct {
	validToken    string
	authenticated bool
}

func (session *selectiveSession) Status() auth.Status {
	return auth.Status{Authenticated: session.authenticated}
}
func (session *selectiveSession) SetValidated(_ context.Context, token string, _ resource.PDFSource) error {
	if token != session.validToken {
		return auth.ErrInvalidToken
	}
	session.authenticated = true
	return nil
}
func (session *selectiveSession) Clear() { session.authenticated = false }

type fakeBrowserSessions struct {
	candidates []browserauth.Candidate
	err        error
}

func (sessions *fakeBrowserSessions) Candidates(context.Context) ([]browserauth.Candidate, error) {
	return sessions.candidates, sessions.err
}

type fakeCatalog struct{ snapshot catalog.Snapshot }

func (service *fakeCatalog) Sync(context.Context) (catalog.Snapshot, error) {
	return service.snapshot, nil
}
func (service *fakeCatalog) LoadCached() (catalog.Snapshot, error) { return service.snapshot, nil }

type fakeResource struct{}

func (service *fakeResource) Resolve(_ context.Context, book catalog.Textbook) (resource.PDFSource, error) {
	return resource.PDFSource{BookID: book.ID, Filename: book.Title + ".pdf", ExpectedSize: 10, MD5: "d41d8cd98f00b204e9800998ecf8427e", Mirrors: []string{"https://allowed.invalid/book.pdf"}}, nil
}

type fakeDownloads struct {
	tasks       []download.Task
	resumeErr   error
	cancelErr   error
	pausedIDs   []string
	canceledIDs []string
}

func (downloads *fakeDownloads) Enqueue(source resource.PDFSource) (download.Task, error) {
	task := download.Task{ID: source.BookID, BookID: source.BookID, Filename: source.Filename, Status: download.StatusQueued}
	downloads.tasks = append(downloads.tasks, task)
	return task, nil
}
func (downloads *fakeDownloads) Pause(id string) error {
	downloads.pausedIDs = append(downloads.pausedIDs, id)
	return nil
}
func (downloads *fakeDownloads) Resume(string) error { return downloads.resumeErr }
func (downloads *fakeDownloads) Cancel(id string, removePartial bool) error {
	downloads.canceledIDs = append(downloads.canceledIDs, fmt.Sprintf("%s:%t", id, removePartial))
	return downloads.cancelErr
}
func (downloads *fakeDownloads) Retry(string) error { return nil }
func (downloads *fakeDownloads) Tasks() []download.Task {
	return append([]download.Task(nil), downloads.tasks...)
}

type fakeAppJournal struct{ records []storage.TaskRecord }

func (journal *fakeAppJournal) LoadForRecovery() ([]storage.TaskRecord, error) {
	return journal.records, nil
}
func (journal *fakeAppJournal) Save(records []storage.TaskRecord) error {
	journal.records = append([]storage.TaskRecord(nil), records...)
	return nil
}
func (journal *fakeAppJournal) Remove(ids []string) error {
	removed := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		removed[id] = struct{}{}
	}
	remaining := make([]storage.TaskRecord, 0, len(journal.records))
	for _, record := range journal.records {
		if _, ok := removed[record.ID]; !ok {
			remaining = append(remaining, record)
		}
	}
	journal.records = remaining
	return nil
}
func (journal *fakeAppJournal) Flush() error { return nil }

type fakeCoordinator struct {
	called    bool
	quitAfter bool
}

func (coordinator *fakeCoordinator) RequestShutdown(context.Context) error {
	coordinator.called = true
	return nil
}
