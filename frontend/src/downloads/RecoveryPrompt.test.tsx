import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {expect, it, vi} from 'vitest';
import type {Bridge, TaskRecord} from '../shared/types';
import {RecoveryPrompt} from './RecoveryPrompt';

function bridgeStub(): Bridge {
  return {
    bootstrap: vi.fn(), detectBrowserSession: vi.fn(), openBrowserLogin: vi.fn(), submitToken: vi.fn(), logout: vi.fn(), syncCatalog: vi.fn(), queryCatalog: vi.fn(),
    enqueueDownloads: vi.fn(), pauseDownload: vi.fn(), resumeDownload: vi.fn(), cancelDownload: vi.fn(),
    retryDownload: vi.fn(), updateSettings: vi.fn(), clearCache: vi.fn(), clearIncomplete: vi.fn(),
    exportDiagnostics: vi.fn(), shutdown: vi.fn(), onDownloadUpdate: vi.fn(() => () => {}),
  };
}

const records: TaskRecord[] = [
  {id: 'task-1', bookId: 'book-1', filename: '语文.pdf', status: 'paused', partialBytes: 100, expectedSize: 1000},
  {id: 'task-2', bookId: 'book-2', filename: '数学.pdf', status: 'paused', partialBytes: 200, expectedSize: 1000},
];

it('continues every recoverable task and closes the recovery prompt', async () => {
  const user = userEvent.setup();
  const bridge = bridgeStub();
  const onResolved = vi.fn();
  render(<RecoveryPrompt bridge={bridge} records={records} onResolved={onResolved} />);

  await user.click(screen.getByRole('button', {name: '继续全部'}));

  expect(bridge.resumeDownload).toHaveBeenCalledWith('task-1');
  expect(bridge.resumeDownload).toHaveBeenCalledWith('task-2');
  expect(onResolved).toHaveBeenCalledOnce();
});

it('removes an individually resumed task from the remaining list', async () => {
  const user = userEvent.setup();
  const bridge = bridgeStub();
  render(<RecoveryPrompt bridge={bridge} records={records} onResolved={vi.fn()} />);

  await user.click(screen.getByRole('button', {name: '逐项处理'}));
  await user.click(screen.getByRole('button', {name: '继续 语文.pdf'}));

  expect(screen.queryByText('语文.pdf')).not.toBeInTheDocument();
  expect(screen.getByText('数学.pdf')).toBeVisible();
});
