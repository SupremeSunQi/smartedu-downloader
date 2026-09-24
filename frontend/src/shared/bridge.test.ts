import {afterEach, expect, it, vi} from 'vitest';
import {desktopBridge} from './bridge';

afterEach(() => {
  delete (window as unknown as {go?: unknown}).go;
});

it('notifies the app when a native request reports an expired session', async () => {
  const listener = vi.fn();
  const unsubscribe = desktopBridge.onSessionExpired?.(listener);
  (window as unknown as {go: unknown}).go = {main: {App: {
    QueryCatalog: vi.fn().mockRejectedValue({code: 'AUTH_REQUIRED', message: '请先登录'}),
  }}};

  await expect(desktopBridge.queryCatalog({search: '', stageId: '', subjectId: '', editionId: '', gradeId: '', volumeId: ''})).rejects.toMatchObject({code: 'AUTH_REQUIRED'});

  expect(listener).toHaveBeenCalledOnce();
  unsubscribe?.();
});
