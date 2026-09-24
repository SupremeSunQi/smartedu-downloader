package main

import (
	"context"
	"errors"
	"strings"
	"sync"

	"smartedu-downloader/internal/auth"
	"smartedu-downloader/internal/browserauth"
	"smartedu-downloader/internal/catalog"
	"smartedu-downloader/internal/download"
	"smartedu-downloader/internal/resource"
	"smartedu-downloader/internal/storage"
)

const (
	validationBookID    = "bdc00134-465d-454b-a541-dcd0cec4d86e"
	validationBookTitle = "智慧教育教材"
)

type UIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	cause   error
}

func (err *UIError) Error() string { return err.Code + ": " + err.Message }
func (err *UIError) Unwrap() error { return err.cause }

type sessionService interface {
	Status() auth.Status
	SetValidated(context.Context, string, resource.PDFSource) error
	Clear()
}

type catalogService interface {
	Sync(context.Context) (catalog.Snapshot, error)
	LoadCached() (catalog.Snapshot, error)
}

type resourceService interface {
	Resolve(context.Context, catalog.Textbook) (resource.PDFSource, error)
}

type downloadService interface {
	Enqueue(resource.PDFSource) (download.Task, error)
	Pause(string) error
	Resume(string) error
	Cancel(string, bool) error
	Retry(string) error
	Tasks() []download.Task
}

type journalService interface {
	LoadForRecovery() ([]storage.TaskRecord, error)
	Save([]storage.TaskRecord) error
	Remove([]string) error
	Flush() error
}

type browserSessionService interface {
	Candidates(context.Context) ([]browserauth.Candidate, error)
}

type shutdownCoordinator interface {
	RequestShutdown(context.Context) error
}

type AppDependencies struct {
	Layout          storage.Layout
	Config          storage.Config
	Session         sessionService
	Catalog         catalogService
	Resource        resourceService
	Downloads       downloadService
	Journal         journalService
	Coordinator     shutdownCoordinator
	ChooseDirectory func(context.Context, string) (string, error)
	OpenDirectory   func(string) error
	BrowserSessions browserSessionService
	OpenLoginPage   func() error
	Quit            func(context.Context)
}

type App struct {
	mutex           sync.RWMutex
	ctx             context.Context
	layout          storage.Layout
	config          storage.Config
	session         sessionService
	catalog         catalogService
	resource        resourceService
	downloads       downloadService
	journal         journalService
	coordinator     shutdownCoordinator
	chooseDirectory func(context.Context, string) (string, error)
	openDirectory   func(string) error
	browserSessions browserSessionService
	openLoginPage   func() error
	quit            func(context.Context)
	snapshot        catalog.Snapshot
	shuttingDown    bool
}

type PathState struct {
	DataRoot    string `json:"dataRoot"`
	DownloadDir string `json:"downloadDir"`
	CacheDir    string `json:"cacheDir"`
	LogsDir     string `json:"logsDir"`
	WebViewDir  string `json:"webviewDir"`
}

type BootstrapState struct {
	Session     auth.Status          `json:"session"`
	Catalog     catalog.Snapshot     `json:"catalog"`
	Config      storage.Config       `json:"config"`
	Tasks       []download.Task      `json:"tasks"`
	Recoverable []storage.TaskRecord `json:"recoverable"`
	Paths       PathState            `json:"paths"`
}

func NewApp(dependencies AppDependencies) *App {
	return &App{
		ctx: context.Background(), layout: dependencies.Layout, config: dependencies.Config,
		session: dependencies.Session, catalog: dependencies.Catalog, resource: dependencies.Resource,
		downloads: dependencies.Downloads, journal: dependencies.Journal, coordinator: dependencies.Coordinator,
		chooseDirectory: dependencies.ChooseDirectory, openDirectory: dependencies.OpenDirectory,
		browserSessions: dependencies.BrowserSessions, openLoginPage: dependencies.OpenLoginPage, quit: dependencies.Quit,
	}
}

func (app *App) Startup(ctx context.Context) {
	app.mutex.Lock()
	app.ctx = ctx
	app.mutex.Unlock()
}

func (app *App) Bootstrap() (BootstrapState, error) {
	if err := app.validateDependencies(); err != nil {
		return BootstrapState{}, err
	}
	status := app.session.Status()
	state := BootstrapState{
		Session: status,
		Config:  app.currentConfig(),
		Tasks:   app.downloads.Tasks(),
		Paths: PathState{
			DataRoot: app.layout.Root, DownloadDir: app.currentConfig().DownloadDir,
			CacheDir: app.layout.CacheDir, LogsDir: app.layout.LogsDir, WebViewDir: app.layout.WebViewDir,
		},
	}
	records, err := app.journal.LoadForRecovery()
	if err != nil {
		return BootstrapState{}, uiError("TASK_RECOVERY_FAILED", "无法读取未完成任务", err)
	}
	state.Recoverable = records
	if status.Authenticated {
		snapshot, cacheErr := app.catalog.LoadCached()
		if cacheErr == nil {
			app.setSnapshot(snapshot)
			state.Catalog = snapshot
		}
	}
	return state, nil
}

func (app *App) DetectBrowserSession() (auth.Status, error) {
	if app.session == nil || app.resource == nil || app.browserSessions == nil {
		return auth.Status{}, uiError("AUTH_BROWSER_STORAGE", "浏览器登录检测暂不可用", nil)
	}
	candidates, err := app.browserSessions.Candidates(app.context())
	if err != nil {
		switch {
		case errors.Is(err, browserauth.ErrExpired):
			return auth.Status{}, uiError("AUTH_BROWSER_SESSION_EXPIRED", "浏览器中的登录状态已失效，请重新登录", nil)
		case errors.Is(err, browserauth.ErrIncompatibleStorage), errors.Is(err, browserauth.ErrStorage):
			return auth.Status{}, uiError("AUTH_BROWSER_STORAGE", "浏览器登录数据格式暂不兼容，请更新本程序", nil)
		default:
			return auth.Status{}, uiError("AUTH_BROWSER_STORAGE", "浏览器登录数据暂时不可读取，请重试", nil)
		}
	}
	if len(candidates) == 0 {
		return auth.Status{}, uiError("AUTH_BROWSER_SESSION_MISSING", "尚未检测到登录状态，请先前往网页登录", nil)
	}
	source, err := app.resource.Resolve(app.context(), catalog.Textbook{ID: validationBookID, Title: validationBookTitle})
	if err != nil {
		return auth.Status{}, uiError("AUTH_CHECK_UNAVAILABLE", "暂时无法连接认证资源", err)
	}
	var unexpectedError error
	for _, candidate := range candidates {
		if err := app.session.SetValidated(app.context(), candidate.Token, source); err != nil {
			if errors.Is(err, auth.ErrInvalidToken) || errors.Is(err, auth.ErrInvalidPDFResponse) {
				continue
			}
			unexpectedError = err
			continue
		}
		return app.session.Status(), nil
	}
	if unexpectedError != nil {
		return auth.Status{}, uiError("AUTH_CHECK_UNAVAILABLE", "暂时无法完成登录状态验证，请重试", unexpectedError)
	}
	return auth.Status{}, uiError("AUTH_BROWSER_SESSION_EXPIRED", "浏览器中的登录状态已失效，请重新登录", nil)
}

func (app *App) SubmitToken(token string) (auth.Status, error) {
	if app.session == nil || app.resource == nil {
		return auth.Status{}, uiError("AUTH_CHECK_UNAVAILABLE", "暂时无法完成登录状态验证，请重试", nil)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return auth.Status{}, uiError("AUTH_TOKEN_INVALID", "请输入有效的 Token", nil)
	}
	source, err := app.resource.Resolve(app.context(), catalog.Textbook{ID: validationBookID, Title: validationBookTitle})
	if err != nil {
		return auth.Status{}, uiError("AUTH_CHECK_UNAVAILABLE", "暂时无法连接认证资源", err)
	}
	if err := app.session.SetValidated(app.context(), token, source); err != nil {
		if errors.Is(err, auth.ErrInvalidToken) || errors.Is(err, auth.ErrInvalidPDFResponse) {
			return auth.Status{}, uiError("AUTH_TOKEN_INVALID", "Token 无效或已过期，请重新获取", nil)
		}
		return auth.Status{}, uiError("AUTH_CHECK_UNAVAILABLE", "暂时无法完成登录状态验证，请重试", err)
	}
	return app.session.Status(), nil
}

func (app *App) OpenBrowserLogin() error {
	if app.openLoginPage == nil {
		return uiError("AUTH_BROWSER_LAUNCH_FAILED", "无法打开默认浏览器，请手动访问 auth.smartedu.cn", nil)
	}
	if err := app.openLoginPage(); err != nil {
		return uiError("AUTH_BROWSER_LAUNCH_FAILED", "无法打开默认浏览器，请手动访问 auth.smartedu.cn", err)
	}
	return nil
}

func (app *App) Logout() error {
	for _, task := range app.downloads.Tasks() {
		switch task.Status {
		case download.StatusQueued, download.StatusResolving, download.StatusDownloading:
			_ = app.downloads.Pause(task.ID)
		}
	}
	app.session.Clear()
	app.setSnapshot(catalog.Snapshot{})
	return nil
}

func (app *App) SyncCatalog() (catalog.Snapshot, error) {
	if err := app.requireAuthentication(); err != nil {
		return catalog.Snapshot{}, err
	}
	snapshot, err := app.catalog.Sync(app.context())
	if err != nil {
		return catalog.Snapshot{}, uiError("CATALOG_SYNC_FAILED", "教材目录同步失败", err)
	}
	app.setSnapshot(snapshot)
	return snapshot, nil
}

func (app *App) QueryCatalog(query catalog.Query) ([]catalog.Textbook, error) {
	if err := app.requireAuthentication(); err != nil {
		return nil, err
	}
	if len(query.Search) > 200 {
		return nil, uiError("INVALID_QUERY", "搜索内容过长", nil)
	}
	snapshot, err := app.catalogSnapshot()
	if err != nil {
		return nil, err
	}
	return catalog.Filter(snapshot, query), nil
}

func (app *App) EnqueueDownloads(bookIDs []string) ([]download.Task, error) {
	if err := app.requireAuthentication(); err != nil {
		return nil, err
	}
	if len(bookIDs) == 0 || len(bookIDs) > 100 {
		return nil, uiError("INVALID_SELECTION", "请选择 1 到 100 本教材", nil)
	}
	snapshot, err := app.catalogSnapshot()
	if err != nil {
		return nil, err
	}
	books := make(map[string]catalog.Textbook, len(snapshot.Textbooks))
	for _, book := range snapshot.Textbooks {
		books[book.ID] = book
	}
	selected := make([]catalog.Textbook, 0, len(bookIDs))
	seen := make(map[string]struct{}, len(bookIDs))
	for _, id := range bookIDs {
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		book, found := books[id]
		if !found {
			return nil, uiError("BOOK_NOT_FOUND", "所选教材不在当前目录中", nil)
		}
		seen[id] = struct{}{}
		selected = append(selected, book)
	}

	tasks := make([]download.Task, 0, len(selected))
	for _, book := range selected {
		source, resolveErr := app.resource.Resolve(app.context(), book)
		if resolveErr != nil {
			return tasks, uiError("RESOURCE_RESOLVE_FAILED", "无法获取教材下载地址", resolveErr)
		}
		task, enqueueErr := app.downloads.Enqueue(source)
		if enqueueErr != nil {
			return tasks, uiError("DOWNLOAD_ENQUEUE_FAILED", "无法加入下载队列", enqueueErr)
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func (app *App) PauseDownload(id string) error {
	return app.downloadCommand(id, app.downloads.Pause)
}

func (app *App) ResumeDownload(id string) error {
	if strings.TrimSpace(id) == "" {
		return uiError("INVALID_TASK", "下载任务 ID 为空", nil)
	}
	if err := app.downloads.Resume(id); err == nil {
		return nil
	} else if !errors.Is(err, download.ErrTaskNotFound) {
		return uiError("DOWNLOAD_COMMAND_FAILED", "下载操作失败", err)
	}

	records, err := app.journal.LoadForRecovery()
	if err != nil {
		return uiError("TASK_RECOVERY_FAILED", "无法读取未完成任务", err)
	}
	var record *storage.TaskRecord
	for index := range records {
		if records[index].ID == id {
			record = &records[index]
			break
		}
	}
	if record == nil {
		return uiError("DOWNLOAD_COMMAND_FAILED", "下载任务不存在", download.ErrTaskNotFound)
	}
	snapshot, err := app.catalogSnapshot()
	if err != nil {
		return err
	}
	var book *catalog.Textbook
	for index := range snapshot.Textbooks {
		if snapshot.Textbooks[index].ID == record.BookID {
			book = &snapshot.Textbooks[index]
			break
		}
	}
	if book == nil {
		return uiError("BOOK_NOT_FOUND", "未完成任务对应的教材已不在目录中", nil)
	}
	source, err := app.resource.Resolve(app.context(), *book)
	if err != nil {
		return uiError("RESOURCE_RESOLVE_FAILED", "无法重新获取教材下载地址", err)
	}
	source.Filename = record.Filename
	task, err := app.downloads.Enqueue(source)
	if err != nil {
		return uiError("DOWNLOAD_ENQUEUE_FAILED", "无法恢复下载任务", err)
	}
	if err := app.journal.Remove([]string{record.ID}); err != nil {
		_ = app.downloads.Cancel(task.ID, false)
		return uiError("TASK_RECOVERY_FAILED", "无法更新未完成任务记录", err)
	}
	return nil
}

func (app *App) CancelDownload(id string, removePartial bool) error {
	if strings.TrimSpace(id) == "" {
		return uiError("INVALID_TASK", "下载任务 ID 为空", nil)
	}
	if err := app.downloads.Cancel(id, removePartial); err == nil {
		return nil
	} else if !removePartial || !errors.Is(err, download.ErrTaskNotFound) {
		return uiError("DOWNLOAD_COMMAND_FAILED", "无法取消下载", err)
	}
	records, err := app.journal.LoadForRecovery()
	if err != nil {
		return uiError("TASK_RECOVERY_FAILED", "无法读取未完成任务", err)
	}
	var selected []storage.TaskRecord
	for _, record := range records {
		if record.ID == id {
			selected = append(selected, record)
		}
	}
	if len(selected) == 0 {
		return uiError("DOWNLOAD_COMMAND_FAILED", "下载任务不存在", download.ErrTaskNotFound)
	}
	if err := storage.ClearIncomplete(app.currentConfig().DownloadDir, selected); err != nil {
		return uiError("INCOMPLETE_CLEAR_FAILED", "未完成文件清理失败", err)
	}
	if err := app.journal.Remove([]string{id}); err != nil {
		return uiError("TASK_RECOVERY_FAILED", "无法更新未完成任务记录", err)
	}
	return nil
}

func (app *App) RetryDownload(id string) error {
	return app.downloadCommand(id, app.downloads.Retry)
}

func (app *App) UpdateSettings(config storage.Config) (storage.Config, error) {
	if err := storage.SaveConfig(app.layout, config); err != nil {
		return storage.Config{}, uiError("SETTINGS_INVALID", "设置无法保存", err)
	}
	saved, err := storage.LoadConfig(app.layout)
	if err != nil {
		return storage.Config{}, uiError("SETTINGS_LOAD_FAILED", "设置已保存但无法重新读取", err)
	}
	return saved, nil
}

func (app *App) ClearCache() error {
	if err := storage.ClearCache(app.layout); err != nil {
		return uiError("CACHE_CLEAR_FAILED", "缓存清理失败", err)
	}
	app.setSnapshot(catalog.Snapshot{})
	return nil
}

func (app *App) ClearIncomplete() error {
	managedIDs := make(map[string]struct{})
	var cancelErrors []error
	for _, task := range app.downloads.Tasks() {
		managedIDs[task.ID] = struct{}{}
		switch task.Status {
		case download.StatusCompleted:
			continue
		}
		if err := app.downloads.Cancel(task.ID, true); err != nil {
			cancelErrors = append(cancelErrors, err)
		}
	}
	records, err := app.journal.LoadForRecovery()
	if err != nil {
		return uiError("TASK_RECOVERY_FAILED", "无法读取未完成任务", err)
	}
	staleRecords := make([]storage.TaskRecord, 0, len(records))
	staleIDs := make([]string, 0, len(records))
	for _, record := range records {
		if _, managed := managedIDs[record.ID]; managed {
			continue
		}
		staleRecords = append(staleRecords, record)
		staleIDs = append(staleIDs, record.ID)
	}
	if err := storage.ClearIncomplete(app.currentConfig().DownloadDir, staleRecords); err != nil {
		return uiError("INCOMPLETE_CLEAR_FAILED", "未完成文件清理失败", err)
	}
	if err := app.journal.Remove(staleIDs); err != nil {
		return uiError("TASK_RECOVERY_FAILED", "无法更新未完成任务记录", err)
	}
	if err := errors.Join(cancelErrors...); err != nil {
		return uiError("INCOMPLETE_CLEAR_FAILED", "部分当前任务无法取消", err)
	}
	return nil
}

func (app *App) ExportDiagnostics(destination string) (string, error) {
	records, err := app.journal.LoadForRecovery()
	if err != nil {
		return "", uiError("DIAGNOSTICS_FAILED", "无法读取任务记录", err)
	}
	path, err := storage.ExportDiagnostics(app.layout, app.currentConfig(), records, destination)
	if err != nil {
		return "", uiError("DIAGNOSTICS_FAILED", "诊断信息导出失败", err)
	}
	return path, nil
}

func (app *App) ChooseDownloadDirectory(_ string) (string, error) {
	current := app.currentConfig().DownloadDir
	if app.chooseDirectory == nil {
		return "", uiError("DIALOG_UNAVAILABLE", "目录选择窗口不可用", nil)
	}
	selected, err := app.chooseDirectory(app.context(), current)
	if err != nil {
		return "", uiError("DIALOG_FAILED", "无法选择下载目录", err)
	}
	if strings.TrimSpace(selected) == "" {
		return current, nil
	}
	return selected, nil
}

func (app *App) OpenDownloadFolder() error {
	if app.openDirectory == nil {
		return uiError("OPEN_DIRECTORY_UNAVAILABLE", "无法打开下载目录", nil)
	}
	if err := app.openDirectory(app.currentConfig().DownloadDir); err != nil {
		return uiError("OPEN_DIRECTORY_FAILED", "无法打开下载目录", err)
	}
	return nil
}

func (app *App) HasActiveDownloads() bool {
	for _, task := range app.downloads.Tasks() {
		switch task.Status {
		case download.StatusQueued, download.StatusResolving, download.StatusDownloading, download.StatusPaused, download.StatusWaitingAuth:
			return true
		}
	}
	return false
}

func (app *App) PrepareShutdown() error {
	app.mutex.Lock()
	app.shuttingDown = true
	ctx := app.ctx
	app.mutex.Unlock()
	if app.coordinator == nil {
		return nil
	}
	if err := app.coordinator.RequestShutdown(ctx); err != nil {
		return uiError("SHUTDOWN_FAILED", "程序未能完全停止", err)
	}
	return nil
}

func (app *App) ShutdownInProgress() bool {
	app.mutex.RLock()
	defer app.mutex.RUnlock()
	return app.shuttingDown
}

func (app *App) Shutdown() error {
	err := app.PrepareShutdown()
	if app.quit != nil {
		app.quit(app.context())
	}
	return err
}

func (app *App) context() context.Context {
	app.mutex.RLock()
	defer app.mutex.RUnlock()
	return app.ctx
}

func (app *App) currentConfig() storage.Config {
	app.mutex.RLock()
	defer app.mutex.RUnlock()
	return app.config
}

func (app *App) setSnapshot(snapshot catalog.Snapshot) {
	app.mutex.Lock()
	app.snapshot = snapshot
	app.mutex.Unlock()
}

func (app *App) catalogSnapshot() (catalog.Snapshot, error) {
	app.mutex.RLock()
	snapshot := app.snapshot
	app.mutex.RUnlock()
	if snapshot.Version > 0 {
		return snapshot, nil
	}
	cached, err := app.catalog.LoadCached()
	if err != nil {
		return catalog.Snapshot{}, uiError("CATALOG_UNAVAILABLE", "本地没有可用的教材目录，请先同步", err)
	}
	app.setSnapshot(cached)
	return cached, nil
}

func (app *App) requireAuthentication() error {
	if app.session == nil || !app.session.Status().Authenticated {
		return uiError("AUTH_REQUIRED", "请先登录", nil)
	}
	return nil
}

func (app *App) downloadCommand(id string, command func(string) error) error {
	if strings.TrimSpace(id) == "" {
		return uiError("INVALID_TASK", "下载任务 ID 为空", nil)
	}
	if err := command(id); err != nil {
		return uiError("DOWNLOAD_COMMAND_FAILED", "下载操作失败", err)
	}
	return nil
}

func (app *App) validateDependencies() error {
	if app.session == nil || app.catalog == nil || app.resource == nil || app.downloads == nil || app.journal == nil {
		return uiError("APP_NOT_READY", "程序尚未初始化完成", nil)
	}
	return nil
}

func uiError(code, message string, cause error) *UIError {
	return &UIError{Code: code, Message: message, cause: cause}
}
