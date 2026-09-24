import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {expect, it, vi} from 'vitest';
import type {Bridge, DownloadTask} from '../shared/types';
import {DownloadPanel} from './DownloadPanel';

function bridgeStub(): Bridge {
  return {
    bootstrap: vi.fn(), detectBrowserSession: vi.fn(), openBrowserLogin: vi.fn(), submitToken: vi.fn(), logout: vi.fn(), syncCatalog: vi.fn(), queryCatalog: vi.fn(),
    enqueueDownloads: vi.fn(), pauseDownload: vi.fn(), resumeDownload: vi.fn(), cancelDownload: vi.fn(),
    retryDownload: vi.fn(), updateSettings: vi.fn(), clearCache: vi.fn(), clearIncomplete: vi.fn(),
    exportDiagnostics: vi.fn(), shutdown: vi.fn(), onDownloadUpdate: vi.fn(() => () => {}),
  };
}

const downloadingTask: DownloadTask = {
  id: 'downloading-id', bookId: 'math', filename: '数学一年级.pdf', status: 'downloading',
  bytesDone: 5_000_000, totalBytes: 10_000_000, speedBytes: 1_000_000, etaSeconds: 5, mirrorAttempt: 1,
};

const pausedTask: DownloadTask = {
  id: 'paused-id', bookId: 'chinese', filename: '语文一年级.pdf', status: 'paused',
  bytesDone: 2_000_000, totalBytes: 12_000_000, speedBytes: 0, etaSeconds: 0, mirrorAttempt: 1,
};

it('offers pause for downloading tasks and resume for paused tasks', async () => {
  const user = userEvent.setup();
  const bridge = bridgeStub();
  render(<DownloadPanel bridge={bridge} tasks={[downloadingTask, pausedTask]} />);

  await user.click(screen.getByRole('button', {name: '暂停 数学一年级'}));
  await user.click(screen.getByRole('button', {name: '继续 语文一年级'}));

  expect(bridge.pauseDownload).toHaveBeenCalledWith('downloading-id');
  expect(bridge.resumeDownload).toHaveBeenCalledWith('paused-id');
});

it('removes a canceled task and requests partial-file cleanup', async () => {
  const user = userEvent.setup();
  const bridge = bridgeStub();
  render(<DownloadPanel bridge={bridge} tasks={[pausedTask]} />);

  await user.click(screen.getByRole('button', {name: '取消 语文一年级'}));

  expect(bridge.cancelDownload).toHaveBeenCalledWith('paused-id', true);
  expect(screen.queryByText('语文一年级.pdf')).not.toBeInTheDocument();
});

it('offers cleanup for a previously canceled task', async () => {
  const user = userEvent.setup();
  const bridge = bridgeStub();
  render(<DownloadPanel bridge={bridge} tasks={[{...pausedTask, status: 'canceled'}]} />);

  await user.click(screen.getByRole('button', {name: '清理 语文一年级'}));

  expect(bridge.cancelDownload).toHaveBeenCalledWith('paused-id', true);
  expect(screen.queryByText('语文一年级.pdf')).not.toBeInTheDocument();
});

it('merges download events by task id without duplicating rows', async () => {
  const bridge = bridgeStub();
  let listener: ((task: DownloadTask) => void) | undefined;
  vi.mocked(bridge.onDownloadUpdate).mockImplementation((next) => { listener = next; return () => {}; });
  render(<DownloadPanel bridge={bridge} tasks={[downloadingTask]} />);

  listener?.({...downloadingTask, bytesDone: 8_000_000});

  expect(await screen.findAllByText('数学一年级.pdf')).toHaveLength(1);
  expect(screen.getByText('80%')).toBeVisible();
});

it('closes the open drawer on Escape', async () => {
  const user = userEvent.setup();
  const onClose = vi.fn();
  render(<DownloadPanel bridge={bridgeStub()} tasks={[]} onClose={onClose} />);

  expect(screen.getByRole('button', {name: '关闭下载任务'})).toHaveFocus();
  await user.keyboard('{Escape}');
  expect(onClose).toHaveBeenCalledOnce();
});
