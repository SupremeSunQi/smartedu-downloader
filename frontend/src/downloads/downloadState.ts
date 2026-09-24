import type {DownloadTask} from '../shared/types';

export function mergeTask(tasks: DownloadTask[], update: DownloadTask): DownloadTask[] {
  const index = tasks.findIndex((task) => task.id === update.id);
  if (index < 0) return [update, ...tasks];
  const next = [...tasks];
  next[index] = update;
  return next;
}

export function taskProgress(task: DownloadTask): number {
  if (task.status === 'completed') return 100;
  if (task.totalBytes <= 0) return 0;
  return Math.max(0, Math.min(100, Math.round(task.bytesDone / task.totalBytes * 100)));
}

export function taskDisplayName(filename: string): string {
  return filename.replace(/\.pdf$/i, '');
}

export function formatRate(bytes: number): string {
  if (bytes <= 0) return '';
  return `${formatAmount(bytes)}/s`;
}

export function formatAmount(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  if (bytes < 1024) return `${Math.round(bytes)} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

export function formatETA(seconds: number): string {
  if (seconds <= 0) return '';
  if (seconds < 60) return `约 ${Math.ceil(seconds)} 秒`;
  if (seconds < 3600) return `约 ${Math.ceil(seconds / 60)} 分钟`;
  return `约 ${(seconds / 3600).toFixed(1)} 小时`;
}
