import {act, render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {expect, it, vi} from 'vitest';
import {App} from './App';
import {emptyCatalog, type Bridge} from './shared/types';

function unauthenticatedBridge(): Bridge {
  return {
    bootstrap: vi.fn().mockResolvedValue({
      session: {authenticated: false}, catalog: emptyCatalog,
      config: {dataRoot: 'data', downloadDir: 'downloads', concurrentDownloads: 2, duplicatePolicy: 'ask'},
      tasks: [], recoverable: [], paths: {dataRoot: 'data', downloadDir: 'downloads', cacheDir: 'cache', logsDir: 'logs', webviewDir: 'webview'},
    }),
    detectBrowserSession: vi.fn().mockResolvedValue({authenticated: false}), openBrowserLogin: vi.fn(), submitToken: vi.fn(), logout: vi.fn(), syncCatalog: vi.fn(), queryCatalog: vi.fn(), enqueueDownloads: vi.fn(),
    pauseDownload: vi.fn(), resumeDownload: vi.fn(), cancelDownload: vi.fn(), retryDownload: vi.fn(), updateSettings: vi.fn(),
    clearCache: vi.fn(), clearIncomplete: vi.fn(), exportDiagnostics: vi.fn(), shutdown: vi.fn(), onDownloadUpdate: vi.fn(() => () => {}),
  };
}

it('does not render the catalog before browser authentication succeeds', async () => {
  render(<App bridge={unauthenticatedBridge()} />);
  expect(await screen.findByRole('heading', {name: '需要登录 国家中小学智慧教育平台'})).toBeVisible();
  expect(screen.queryByRole('main', {name: '教材列表'})).not.toBeInTheDocument();
});

it('opens the download drawer from the catalog and still opens settings', async () => {
  const user = userEvent.setup();
  const bridge = unauthenticatedBridge();
  vi.mocked(bridge.bootstrap).mockResolvedValue({
    session: {authenticated: true}, catalog: {...emptyCatalog, version: 1},
    config: {dataRoot: 'data', downloadDir: 'downloads', concurrentDownloads: 2, duplicatePolicy: 'ask'},
    tasks: [], recoverable: [], paths: {dataRoot: 'data', downloadDir: 'downloads', cacheDir: 'cache', logsDir: 'logs', webviewDir: 'webview'},
  });
  vi.mocked(bridge.queryCatalog).mockResolvedValue([]);
  render(<App bridge={bridge} />);

  expect(await screen.findByRole('main', {name: '教材列表'})).toBeVisible();
  expect(screen.queryByRole('complementary', {name: '下载任务'})).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', {name: '查看下载任务'}));
  expect(await screen.findByRole('complementary', {name: '下载任务'})).toBeVisible();
  expect(screen.getByRole('button', {name: '关闭下载任务'})).toHaveFocus();
  expect(document.querySelector('.download-backdrop')).toBeInTheDocument();
  await user.click(screen.getByRole('button', {name: '关闭下载任务'}));
  expect(screen.queryByRole('complementary', {name: '下载任务'})).not.toBeInTheDocument();
  expect(screen.getByRole('button', {name: '查看下载任务'})).toHaveFocus();
  expect(document.querySelector('.download-backdrop')).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', {name: '查看下载任务'}));
  await user.click(document.querySelector('.download-backdrop') as Element);
  expect(screen.queryByRole('complementary', {name: '下载任务'})).not.toBeInTheDocument();
  screen.getByRole('button', {name: '打开设置'}).click();
  expect(await screen.findByRole('dialog', {name: '设置'})).toBeVisible();
});

it('confirms a native close request while downloads are active', async () => {
  const bridge = unauthenticatedBridge();
  let closeListener: (() => void) | undefined;
  bridge.onCloseRequested = vi.fn((listener) => { closeListener = listener; return () => {}; });
  vi.mocked(bridge.bootstrap).mockResolvedValue({
    session: {authenticated: true}, catalog: {...emptyCatalog, version: 1},
    config: {dataRoot: 'data', downloadDir: 'downloads', concurrentDownloads: 2, duplicatePolicy: 'ask'},
    tasks: [{id: 'active', bookId: 'book', filename: '教材.pdf', status: 'downloading', bytesDone: 1, totalBytes: 2, speedBytes: 1, etaSeconds: 1, mirrorAttempt: 1}],
    recoverable: [], paths: {dataRoot: 'data', downloadDir: 'downloads', cacheDir: 'cache', logsDir: 'logs', webviewDir: 'webview'},
  });
  vi.mocked(bridge.queryCatalog).mockResolvedValue([]);
  render(<App bridge={bridge} />);
  await screen.findByRole('main', {name: '教材列表'});

  act(() => closeListener?.());

  expect(screen.getByRole('dialog', {name: '退出并暂停下载？'})).toBeVisible();
});

it('returns to browser login when a download reports an expired session', async () => {
  const bridge = unauthenticatedBridge();
  let sessionExpired: (() => void) | undefined;
  bridge.onSessionExpired = vi.fn((listener) => { sessionExpired = listener; return () => {}; });
  vi.mocked(bridge.bootstrap).mockResolvedValue({
    session: {authenticated: true}, catalog: {...emptyCatalog, version: 1},
    config: {dataRoot: 'data', downloadDir: 'downloads', concurrentDownloads: 2, duplicatePolicy: 'ask'},
    tasks: [], recoverable: [{id: 'recoverable', bookId: 'book', filename: '教材.pdf', status: 'paused', partialBytes: 1, expectedSize: 2}],
    paths: {dataRoot: 'data', downloadDir: 'downloads', cacheDir: 'cache', logsDir: 'logs', webviewDir: 'webview'},
  });
  vi.mocked(bridge.queryCatalog).mockResolvedValue([]);
  render(<App bridge={bridge} />);
  expect(await screen.findByRole('dialog', {name: '发现未完成的下载'})).toBeVisible();

  act(() => sessionExpired?.());

  expect(await screen.findByRole('heading', {name: '需要登录 国家中小学智慧教育平台'})).toBeVisible();
  expect(screen.queryByRole('main', {name: '教材列表'})).not.toBeInTheDocument();
  expect(screen.queryByRole('dialog', {name: '发现未完成的下载'})).not.toBeInTheDocument();
});
