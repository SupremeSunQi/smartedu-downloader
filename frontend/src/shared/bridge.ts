import type {Bridge, CatalogQuery, DownloadTask} from './types';

type RuntimeWindow = Window & {
  go?: {main?: {App?: Record<string, (...args: unknown[]) => Promise<unknown>>}};
  runtime?: {
    EventsOn?: (event: string, callback: (payload: unknown) => void) => () => void;
  };
};

function method(name: string) {
  return (...args: unknown[]) => {
    const target = (window as RuntimeWindow).go?.main?.App?.[name];
    if (!target) return Promise.reject(new Error(`桌面接口 ${name} 尚未就绪`));
    return target(...args).catch((reason) => {
      if (reason && typeof reason === 'object' && (reason as {code?: string}).code === 'AUTH_REQUIRED') {
        window.dispatchEvent(new Event('smartedu:session-expired'));
      }
      throw reason;
    });
  };
}

export const desktopBridge: Bridge = {
  bootstrap: method('Bootstrap') as Bridge['bootstrap'],
  detectBrowserSession: method('DetectBrowserSession') as Bridge['detectBrowserSession'],
  openBrowserLogin: method('OpenBrowserLogin') as Bridge['openBrowserLogin'],
  submitToken: method('SubmitToken') as Bridge['submitToken'],
  logout: method('Logout') as Bridge['logout'],
  syncCatalog: method('SyncCatalog') as Bridge['syncCatalog'],
  queryCatalog: method('QueryCatalog') as (query: CatalogQuery) => ReturnType<Bridge['queryCatalog']>,
  enqueueDownloads: method('EnqueueDownloads') as Bridge['enqueueDownloads'],
  pauseDownload: method('PauseDownload') as Bridge['pauseDownload'],
  resumeDownload: method('ResumeDownload') as Bridge['resumeDownload'],
  cancelDownload: method('CancelDownload') as Bridge['cancelDownload'],
  retryDownload: method('RetryDownload') as Bridge['retryDownload'],
  updateSettings: method('UpdateSettings') as Bridge['updateSettings'],
  clearCache: method('ClearCache') as Bridge['clearCache'],
  clearIncomplete: method('ClearIncomplete') as Bridge['clearIncomplete'],
  exportDiagnostics: method('ExportDiagnostics') as Bridge['exportDiagnostics'],
  chooseDownloadDirectory: method('ChooseDownloadDirectory') as NonNullable<Bridge['chooseDownloadDirectory']>,
  openDownloadFolder: method('OpenDownloadFolder') as NonNullable<Bridge['openDownloadFolder']>,
  shutdown: method('Shutdown') as Bridge['shutdown'],
  onDownloadUpdate(listener) {
    return (window as RuntimeWindow).runtime?.EventsOn?.('download:update', (payload) => listener(payload as DownloadTask)) ?? (() => {});
  },
  onCloseRequested(listener) {
    return (window as RuntimeWindow).runtime?.EventsOn?.('app:close-requested', listener) ?? (() => {});
  },
  onSessionExpired(listener) {
    const cancelNative = (window as RuntimeWindow).runtime?.EventsOn?.('session:expired', listener) ?? (() => {});
    window.addEventListener('smartedu:session-expired', listener);
    return () => {
      cancelNative();
      window.removeEventListener('smartedu:session-expired', listener);
    };
  },
};
