import {render, screen, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {useState} from 'react';
import {expect, it, vi} from 'vitest';
import type {AppConfig, Bridge, PathState} from '../shared/types';
import {SettingsDialog} from './SettingsDialog';

function bridgeStub(): Bridge {
  return {
    bootstrap: vi.fn(), detectBrowserSession: vi.fn(), openBrowserLogin: vi.fn(), submitToken: vi.fn(), logout: vi.fn(), syncCatalog: vi.fn(), queryCatalog: vi.fn(),
    enqueueDownloads: vi.fn(), pauseDownload: vi.fn(), resumeDownload: vi.fn(), cancelDownload: vi.fn(),
    retryDownload: vi.fn(), updateSettings: vi.fn(), clearCache: vi.fn(), clearIncomplete: vi.fn(),
    exportDiagnostics: vi.fn(), shutdown: vi.fn(), onDownloadUpdate: vi.fn(() => () => {}),
  };
}

const config: AppConfig = {dataRoot: 'D:\\App\\.smartedu-data', downloadDir: 'D:\\App\\downloads', concurrentDownloads: 2, duplicatePolicy: 'ask'};
const paths: PathState = {
  dataRoot: 'D:\\App\\.smartedu-data', downloadDir: 'D:\\App\\downloads', cacheDir: 'D:\\App\\.smartedu-data\\cache',
  logsDir: 'D:\\App\\.smartedu-data\\logs', webviewDir: 'D:\\App\\.smartedu-data\\webview',
};

it('does not expose disconnect or exit commands in settings', () => {
  render(<SettingsDialog bridge={bridgeStub()} config={config} paths={paths} activeTaskCount={1} onClose={vi.fn()} onSaved={vi.fn()} onLogout={vi.fn()} />);

  expect(screen.queryByRole('button', {name: '断开本程序登录'})).not.toBeInTheDocument();
  expect(screen.queryByRole('button', {name: '退出程序'})).not.toBeInTheDocument();
});

it('uses the top-right close control and Escape, with save as the only footer action', async () => {
  const user = userEvent.setup();
  const onClose = vi.fn();
  render(<SettingsDialog bridge={bridgeStub()} config={config} paths={paths} activeTaskCount={0} onClose={onClose} onSaved={vi.fn()} onLogout={vi.fn()} />);

  const dialog = screen.getByRole('dialog', {name: '设置'});
  expect(dialog.querySelector('.settings-footer')).not.toBeNull();
  expect(within(dialog).queryByRole('button', {name: '关闭'})).not.toBeInTheDocument();
  expect(within(dialog).getByRole('button', {name: '关闭设置'})).toBeVisible();
  expect(within(dialog).getByRole('button', {name: '保存设置'})).toBeVisible();
  expect(dialog.querySelector('.settings-restart-hint')).toHaveTextContent('重启程序后生效');
  expect(dialog.querySelector('.settings-restart-hint')).toHaveAttribute('title', '下载目录、同时下载数量和同名文件处理会在重启程序后生效');
  expect(dialog.querySelector('.settings-note--single-line')).toBeNull();

  await user.click(within(dialog).getByRole('button', {name: '关闭设置'}));
  expect(onClose).toHaveBeenCalledOnce();
  onClose.mockClear();

  await user.keyboard('{Escape}');
  expect(onClose).toHaveBeenCalledOnce();
});

it('keeps the active download path visible and explains that saved download settings need a restart', async () => {
  const user = userEvent.setup();
  const bridge = bridgeStub();
  const nextConfig = {...config, downloadDir: 'D:\\Next\\downloads', concurrentDownloads: 4};
  bridge.chooseDownloadDirectory = vi.fn().mockResolvedValue(nextConfig.downloadDir);
  vi.mocked(bridge.updateSettings).mockResolvedValue(nextConfig);
  render(<SettingsDialog bridge={bridge} config={config} paths={paths} activeTaskCount={0} onClose={vi.fn()} onSaved={vi.fn()} onLogout={vi.fn()} />);

  await user.click(screen.getByRole('button', {name: '选择下载目录'}));
  await user.clear(screen.getByRole('spinbutton', {name: '同时下载数量'}));
  await user.type(screen.getByRole('spinbutton', {name: '同时下载数量'}), '4');
  await user.click(screen.getByRole('button', {name: '保存设置'}));

  expect(await screen.findByRole('status')).toHaveTextContent('重启程序后生效');
  expect(screen.getByText(paths.downloadDir)).toBeVisible();
});

it('exports diagnostics to the active download directory', async () => {
  const user = userEvent.setup();
  const bridge = bridgeStub();
  vi.mocked(bridge.exportDiagnostics).mockResolvedValue(`${paths.downloadDir}\\SmartEduDownloader-diagnostics.zip`);
  render(<SettingsDialog
    bridge={bridge}
    config={{...config, downloadDir: 'D:\\Next\\downloads'}}
    paths={paths}
    activeTaskCount={0}
    onClose={vi.fn()}
    onSaved={vi.fn()}
    onLogout={vi.fn()}
  />);

  await user.click(screen.getByRole('button', {name: '导出诊断信息'}));

  expect(bridge.exportDiagnostics).toHaveBeenCalledWith(paths.downloadDir);
});

it('uses a compact custom arrow wrapper for duplicate-file handling', () => {
  render(<SettingsDialog bridge={bridgeStub()} config={config} paths={paths} activeTaskCount={0} onClose={vi.fn()} onSaved={vi.fn()} onLogout={vi.fn()} />);

  expect(screen.getByRole('combobox', {name: '同名文件处理'}).closest('.select-control')).not.toBeNull();
  expect(screen.getByRole('dialog', {name: '设置'}).querySelector('.settings-section--download')).not.toBeNull();
  expect(screen.getByRole('dialog', {name: '设置'}).querySelector('.settings-section--storage')).not.toBeNull();
  expect(screen.getByRole('dialog', {name: '设置'}).querySelector('.settings-restart-hint')).not.toBeNull();
  expect(screen.getByRole('dialog', {name: '设置'}).querySelectorAll('.settings-control')).toHaveLength(3);
  expect(screen.getByRole('heading', {name: '本机数据'}).querySelector('.lucide-hard-drive')).not.toBeNull();
  expect(screen.getByRole('button', {name: '打开下载目录'}).querySelector('.lucide-folder-open')).not.toBeNull();
  expect(screen.getByRole('button', {name: '删除未完成文件'})).toHaveClass('danger-text');
});

it('opens and selects a styled duplicate-file option', async () => {
  const user = userEvent.setup();
  render(<SettingsDialog bridge={bridgeStub()} config={config} paths={paths} activeTaskCount={0} onClose={vi.fn()} onSaved={vi.fn()} onLogout={vi.fn()} />);

  await user.click(screen.getByRole('combobox', {name: '同名文件处理'}));

  expect(screen.getByRole('listbox', {name: '同名文件处理'})).toBeVisible();
  expect(screen.getByRole('option', {name: '自动重命名'})).toBeVisible();
  await user.click(screen.getByRole('option', {name: '自动重命名'}));

  expect(screen.getByRole('combobox', {name: '同名文件处理'})).toHaveTextContent('自动重命名');
  expect(screen.queryByRole('listbox', {name: '同名文件处理'})).not.toBeInTheDocument();
});

it('keeps Escape inside the duplicate-file menu and supports arrow navigation', async () => {
  const user = userEvent.setup();
  const onClose = vi.fn();
  render(<SettingsDialog bridge={bridgeStub()} config={config} paths={paths} activeTaskCount={0} onClose={onClose} onSaved={vi.fn()} onLogout={vi.fn()} />);

  await user.click(screen.getByRole('combobox', {name: '同名文件处理'}));
  await user.keyboard('{Escape}');

  expect(screen.queryByRole('listbox', {name: '同名文件处理'})).not.toBeInTheDocument();
  expect(onClose).not.toHaveBeenCalled();

  await user.click(screen.getByRole('combobox', {name: '同名文件处理'}));
  await user.keyboard('{ArrowDown}');
  await user.keyboard('{Enter}');

  expect(screen.getByRole('combobox', {name: '同名文件处理'})).toHaveTextContent('自动重命名');
  expect(onClose).not.toHaveBeenCalled();
});

it('keeps keyboard focus inside settings and restores it to the opener on close', async () => {
  const user = userEvent.setup();
  function Harness() {
    const [open, setOpen] = useState(false);
    return <><button type="button" onClick={() => setOpen(true)}>打开设置弹窗</button>{open && <SettingsDialog bridge={bridgeStub()} config={config} paths={paths} activeTaskCount={0} onClose={() => setOpen(false)} onSaved={vi.fn()} onLogout={vi.fn()} />}</>;
  }
  render(<Harness />);

  const opener = screen.getByRole('button', {name: '打开设置弹窗'});
  await user.click(opener);
  const dialog = screen.getByRole('dialog', {name: '设置'});
  expect(dialog).toHaveFocus();
  expect(opener).toHaveAttribute('inert');
  await user.tab({shift: true});
  expect(screen.getByRole('button', {name: '保存设置'})).toHaveFocus();
  await user.keyboard('{Escape}');
  expect(screen.queryByRole('dialog', {name: '设置'})).not.toBeInTheDocument();
  expect(opener).toHaveFocus();
  expect(opener).not.toHaveAttribute('inert');
});

it('keeps focus on a nested confirmation until it closes', async () => {
  const user = userEvent.setup();
  render(<SettingsDialog bridge={bridgeStub()} config={config} paths={paths} activeTaskCount={0} onClose={vi.fn()} onSaved={vi.fn()} onLogout={vi.fn()} />);

  const settingsDialog = screen.getByRole('dialog', {name: '设置'});
  const clearButton = screen.getByRole('button', {name: '清理目录缓存'});
  await user.click(clearButton);
  expect(screen.getByRole('dialog', {name: '清理目录缓存？'})).toHaveFocus();
  expect(settingsDialog).toHaveAttribute('inert');
  await user.keyboard('{Escape}');
  expect(screen.queryByRole('dialog', {name: '清理目录缓存？'})).not.toBeInTheDocument();
  expect(clearButton).toHaveFocus();
});
