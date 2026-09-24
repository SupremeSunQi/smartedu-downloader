import {CircleCheck, FolderOpen, Pause, Play, RotateCcw, Trash2, X} from 'lucide-react';
import type {Bridge, DownloadTask} from '../shared/types';
import {formatAmount, formatETA, formatRate, taskDisplayName, taskProgress} from './downloadState';

const statusLabels: Record<DownloadTask['status'], string> = {
  queued: '等待中', resolving: '获取资源', downloading: '下载中', paused: '已暂停',
  waiting_auth: '等待重新登录', completed: '已完成', failed: '失败', canceled: '已取消',
};

interface DownloadTaskRowProps {
  bridge: Bridge;
  task: DownloadTask;
  onError(message: string): void;
  onCanceled?(id: string): void;
}

export function DownloadTaskRow({bridge, task, onError, onCanceled}: DownloadTaskRowProps) {
  const name = taskDisplayName(task.filename);
  const progress = taskProgress(task);
  const command = async (operation: () => Promise<void>, message: string) => {
    try { await operation(); } catch { onError(message); }
  };
  return (
    <article className="download-task">
      <div className="task-title"><strong title={task.filename}>{task.filename}</strong><span>{progress}%</span></div>
      <div className="task-progress" role="progressbar" aria-label={`${name} 下载进度`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={progress}><span style={{width: `${progress}%`}} /></div>
      <div className="task-meta">
        <span>{statusLabels[task.status]}</span>
        {task.totalBytes > 0 && <span>{formatAmount(task.bytesDone)} / {formatAmount(task.totalBytes)}</span>}
        {task.speedBytes > 0 && <span>{formatRate(task.speedBytes)}</span>}
        {task.etaSeconds > 0 && <span>{formatETA(task.etaSeconds)}</span>}
        {task.mirrorAttempt > 1 && <span>线路 {task.mirrorAttempt}</span>}
      </div>
      {task.error && <p className="task-error">{task.error}</p>}
      <div className="task-actions">
        {task.status === 'downloading' && <button type="button" aria-label={`暂停 ${name}`} title="暂停" onClick={() => void command(() => bridge.pauseDownload(task.id), '无法暂停下载')}><Pause size={16} /></button>}
        {(task.status === 'paused' || task.status === 'waiting_auth') && <button type="button" aria-label={`继续 ${name}`} title="继续" onClick={() => void command(() => bridge.resumeDownload(task.id), '无法继续下载')}><Play size={16} /></button>}
        {task.status === 'failed' && <button type="button" aria-label={`重试 ${name}`} title="重试" onClick={() => void command(() => bridge.retryDownload(task.id), '无法重试下载')}><RotateCcw size={16} /></button>}
        {!['completed', 'canceled'].includes(task.status) && <button type="button" className="danger-icon" aria-label={`取消 ${name}`} title="取消并删除未完成文件" onClick={() => void command(async () => { await bridge.cancelDownload(task.id, true); onCanceled?.(task.id); }, '无法取消下载')}><X size={16} /></button>}
        {task.status === 'canceled' && <button type="button" className="danger-icon" aria-label={`清理 ${name}`} title="清理残留文件和记录" onClick={() => void command(async () => { await bridge.cancelDownload(task.id, true); onCanceled?.(task.id); }, '无法清理任务')}><Trash2 size={16} /></button>}
        {task.status === 'completed' && <>
          <CircleCheck className="task-complete" size={17} aria-label="下载完成" />
          <button type="button" aria-label={`打开 ${name} 所在文件夹`} title="打开文件夹" disabled={!bridge.openDownloadFolder} onClick={() => void command(() => bridge.openDownloadFolder?.() ?? Promise.resolve(), '无法打开下载目录')}><FolderOpen size={16} /></button>
        </>}
      </div>
    </article>
  );
}
