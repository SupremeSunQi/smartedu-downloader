import {Download, FolderOpen, ListX, X} from 'lucide-react';
import {useEffect, useMemo, useRef, useState} from 'react';
import type {Bridge, DownloadTask} from '../shared/types';
import {trapTabKey} from '../shared/focus';
import {DownloadTaskRow} from './DownloadTaskRow';
import {mergeTask} from './downloadState';

interface DownloadPanelProps {
  bridge: Bridge;
  tasks: DownloadTask[];
  open?: boolean;
  onClose?(): void;
  onTasksChange?(tasks: DownloadTask[]): void;
}

export function DownloadPanel({bridge, tasks: initialTasks, open = true, onClose, onTasksChange}: DownloadPanelProps) {
  const panelRef = useRef<HTMLElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const [tasks, setTasks] = useState(initialTasks);
  const [dismissed, setDismissed] = useState<Set<string>>(new Set());
  const [error, setError] = useState('');

  useEffect(() => setTasks(initialTasks), [initialTasks]);
  useEffect(() => bridge.onDownloadUpdate((task) => {
    setTasks((current) => {
      const next = mergeTask(current, task);
      onTasksChange?.(next);
      return next;
    });
  }), [bridge, onTasksChange]);
  useEffect(() => {
    if (!open || !onClose) return;
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || event.defaultPrevented || document.querySelector('.dialog-scrim')) return;
      event.preventDefault();
      onClose();
    };
    window.addEventListener('keydown', closeOnEscape);
    return () => window.removeEventListener('keydown', closeOnEscape);
  }, [onClose, open]);
  useEffect(() => {
    if (!open || !panelRef.current) return;
    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    (closeButtonRef.current ?? panelRef.current).focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (!document.querySelector('.dialog-scrim') && panelRef.current) trapTabKey(event, panelRef.current);
    };
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('keydown', onKeyDown);
      if (previousFocus?.isConnected) previousFocus.focus();
    };
  }, [open]);

  const visible = useMemo(() => tasks.filter((task) => !dismissed.has(task.id)), [dismissed, tasks]);
  const activeCount = tasks.filter((task) => ['queued', 'resolving', 'downloading', 'paused', 'waiting_auth'].includes(task.status)).length;
  const completedIDs = tasks.filter((task) => task.status === 'completed').map((task) => task.id);

  return (
    <aside ref={panelRef} tabIndex={-1} className="download-panel" aria-label="下载任务" hidden={!open}>
      <header>
        <div><Download size={18} /><strong>下载任务</strong>{activeCount > 0 && <span>{activeCount}</span>}</div>
        {onClose && <button type="button" ref={closeButtonRef} aria-label="关闭下载任务" title="关闭" onClick={onClose}><X size={18} /></button>}
      </header>
        <div className="download-tools">
          <button type="button" aria-label="打开下载目录" title="打开下载目录" disabled={!bridge.openDownloadFolder} onClick={() => void bridge.openDownloadFolder?.().catch(() => setError('无法打开下载目录'))}><FolderOpen size={17} /></button>
          <button type="button" aria-label="清除已完成任务" title="清除已完成任务" disabled={completedIDs.length === 0} onClick={() => setDismissed((current) => new Set([...current, ...completedIDs]))}><ListX size={17} /></button>
        </div>
        {error && <div className="panel-error" role="alert">{error}</div>}
        <div className="download-list">
          {visible.length === 0 ? <div className="download-empty"><Download size={24} /><span>暂无下载任务</span></div> : visible.map((task) => <DownloadTaskRow key={task.id} bridge={bridge} task={task} onError={setError} onCanceled={(id) => setDismissed((current) => new Set(current).add(id))} />)}
        </div>
    </aside>
  );
}
