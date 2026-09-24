package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"smartedu-downloader/internal/auth"
	"smartedu-downloader/internal/browserauth"
	"smartedu-downloader/internal/catalog"
	"smartedu-downloader/internal/download"
	"smartedu-downloader/internal/lifecycle"
	appLogging "smartedu-downloader/internal/logging"
	"smartedu-downloader/internal/resource"
	"smartedu-downloader/internal/storage"
)

const applicationID = "SmartEduDownloader-5f30b350-7d57-47db-8f24-99d6fe0eb9d3"

const officialLoginURL = "https://auth.smartedu.cn/uias/login"

func main() {
	if err := run(os.Args[1:]); err != nil {
		showFatalError("程序启动失败：\n" + err.Error())
		os.Exit(1)
	}
}

func run(arguments []string) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("定位程序文件: %w", err)
	}
	executableDir := filepath.Dir(executable)
	smoke, dataRoot := parseArguments(arguments)
	if smoke {
		return runSmokeMode(executableDir, dataRoot)
	}
	return runDesktop(executableDir, dataRoot)
}

func runDesktop(executableDir, dataRoot string) error {
	layout, err := storage.ResolveLayout(executableDir, dataRoot)
	if err != nil {
		return err
	}
	if err := storage.EnsureWritable(layout); err != nil {
		return fmt.Errorf("程序目录不可写，请将程序移动到有写入权限的文件夹: %w", err)
	}
	config, err := storage.LoadConfig(layout)
	if err != nil {
		return err
	}
	logger, logCloser, err := appLogging.NewRotatingLogger(layout.LogsDir)
	if err != nil {
		return err
	}
	defer logCloser.Close()
	slog.SetDefault(logger)

	window := &windowRuntime{}
	releaseInstance, err := lifecycle.AcquireSingleInstance(applicationID, window.Activate)
	if errors.Is(err, lifecycle.ErrAlreadyRunning) {
		return nil
	}
	if err != nil {
		return err
	}
	defer releaseInstance()

	metadataClient, downloadClient := httpClients()
	catalogService := catalog.NewService(metadataClient, catalog.ProductionBase, layout.CacheDir, catalog.ProductionURLPolicy)
	resourceService := resource.NewService(metadataClient, catalog.ProductionBase, layout.DetailsDir, resource.ProductionURLPolicy)
	session := auth.NewSession(auth.NewValidator(metadataClient))
	journal := storage.NewTaskJournal(layout.TasksDir)

	var desktopApp *App
	manager, err := download.NewManager(downloadClient, session, journal, download.ManagerConfig{
		DownloadDir: config.DownloadDir, Concurrency: config.ConcurrentDownloads, DuplicatePolicy: config.DuplicatePolicy,
	}, func(event download.TaskEvent) {
		ctx, ready := window.Context()
		if !ready {
			return
		}
		wailsruntime.EventsEmit(ctx, "download:update", event.Task)
		if event.Task.Status == download.StatusWaitingAuth {
			wailsruntime.EventsEmit(ctx, "session:expired")
		}
	})
	if err != nil {
		return err
	}
	coordinator := lifecycle.NewCoordinator(manager, session, journal)
	localAppData := os.Getenv("LOCALAPPDATA")
	if strings.TrimSpace(localAppData) == "" {
		localAppData, err = os.UserConfigDir()
		if err != nil {
			return fmt.Errorf("定位浏览器数据目录: %w", err)
		}
	}
	browserReader := browserauth.NewReader(browserauth.DefaultWindowsOptions(localAppData))
	desktopApp = NewApp(AppDependencies{
		Layout: layout, Config: config, Session: session, Catalog: catalogService, Resource: resourceService,
		Downloads: manager, Journal: journal, Coordinator: coordinator, BrowserSessions: browserReader,
		ChooseDirectory: func(ctx context.Context, current string) (string, error) {
			return wailsruntime.OpenDirectoryDialog(ctx, wailsruntime.OpenDialogOptions{Title: "选择教材下载目录", DefaultDirectory: current})
		},
		OpenDirectory: openDirectoryInShell,
		OpenLoginPage: func() error { return openURLInShell(officialLoginURL) },
		Quit:          wailsruntime.Quit,
	})

	messages := windows.DefaultMessages()
	messages.Webview2NotInstalled = "缺少 Microsoft Edge WebView2 运行时"
	messages.MissingRequirements = "缺少运行组件"
	messages.DownloadPage = "本程序需要 Microsoft Edge WebView2 运行时。请安装后重新打开程序。最低版本："
	messages.ContactAdmin = "本程序需要 Microsoft Edge WebView2 运行时，请联系管理员安装。"

	err = wails.Run(&options.App{
		Title: "智教教材下载器", Width: 1280, Height: 800, MinWidth: 1024, MinHeight: 680,
		BackgroundColour: options.NewRGB(246, 249, 254),
		AssetServer:      &assetserver.Options{Assets: loadAssets()},
		Bind:             []interface{}{desktopApp},
		OnStartup: func(ctx context.Context) {
			desktopApp.Startup(ctx)
			window.SetContext(ctx)
		},
		OnBeforeClose: func(ctx context.Context) bool {
			if desktopApp.ShutdownInProgress() {
				return false
			}
			if desktopApp.HasActiveDownloads() {
				wailsruntime.EventsEmit(ctx, "app:close-requested")
				return true
			}
			_ = desktopApp.PrepareShutdown()
			return false
		},
		OnShutdown: func(context.Context) {
			_ = desktopApp.PrepareShutdown()
			window.Clear()
		},
		ErrorFormatter: formatUIError,
		DragAndDrop:    &options.DragAndDrop{DisableWebViewDrop: true},
		Windows: &windows.Options{
			WebviewUserDataPath: layout.WebViewDir, Theme: windows.SystemDefault, Messages: messages,
			DisablePinchZoom: true, IsZoomControlEnabled: false, ResizeDebounceMS: 16,
		},
	})
	if err != nil {
		_ = desktopApp.PrepareShutdown()
		return err
	}
	return nil
}

func formatUIError(err error) any {
	var visible *UIError
	if errors.As(err, &visible) {
		return map[string]string{"code": visible.Code, "message": visible.Message}
	}
	return map[string]string{"code": "UNEXPECTED", "message": "操作失败，请稍后重试"}
}

func httpClients() (*http.Client, *http.Client) {
	transport := &http.Transport{
		Proxy:             http.ProxyFromEnvironment,
		DialContext:       (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2: true, MaxIdleConns: 20, MaxIdleConnsPerHost: 6,
		IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 30 * time.Second,
	}
	return &http.Client{Transport: transport, Timeout: 60 * time.Second}, &http.Client{Transport: transport.Clone()}
}

func parseArguments(arguments []string) (smoke bool, dataRoot string) {
	for index := 0; index < len(arguments); index++ {
		switch {
		case arguments[index] == "--smoke-test":
			smoke = true
		case strings.HasPrefix(arguments[index], "--data-root="):
			dataRoot = strings.TrimPrefix(arguments[index], "--data-root=")
		case arguments[index] == "--data-root" && index+1 < len(arguments):
			index++
			dataRoot = arguments[index]
		}
	}
	return smoke, dataRoot
}

func runSmokeMode(executableDir, dataRoot string) error {
	layout, err := storage.ResolveLayout(executableDir, dataRoot)
	if err != nil {
		return err
	}
	if err := storage.EnsureWritable(layout); err != nil {
		return err
	}
	idHash := sha256.Sum256([]byte(strings.ToLower(executableDir)))
	release, err := lifecycle.AcquireSingleInstance(fmt.Sprintf("%s-smoke-%x", applicationID, idHash[:8]), nil)
	if errors.Is(err, lifecycle.ErrAlreadyRunning) {
		return nil
	}
	if err != nil {
		return err
	}
	defer release()

	readyPath := filepath.Join(layout.Root, "smoke.ready")
	shutdownPath := filepath.Join(layout.Root, "smoke.shutdown")
	_ = os.Remove(shutdownPath)
	if err := os.WriteFile(readyPath, []byte("ready\n"), 0o600); err != nil {
		return err
	}
	defer os.Remove(readyPath)
	defer os.Remove(shutdownPath)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		if _, err := os.Stat(shutdownPath); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

type windowRuntime struct {
	mutex   sync.RWMutex
	ctx     context.Context
	pending bool
}

func (window *windowRuntime) SetContext(ctx context.Context) {
	window.mutex.Lock()
	window.ctx = ctx
	pending := window.pending
	window.pending = false
	window.mutex.Unlock()
	if pending {
		window.Activate()
	}
}

func (window *windowRuntime) Context() (context.Context, bool) {
	window.mutex.RLock()
	defer window.mutex.RUnlock()
	return window.ctx, window.ctx != nil
}

func (window *windowRuntime) Activate() {
	window.mutex.Lock()
	ctx := window.ctx
	if ctx == nil {
		window.pending = true
		window.mutex.Unlock()
		return
	}
	window.mutex.Unlock()
	wailsruntime.WindowUnminimise(ctx)
	wailsruntime.WindowShow(ctx)
}

func (window *windowRuntime) Clear() {
	window.mutex.Lock()
	window.ctx = nil
	window.pending = false
	window.mutex.Unlock()
}
