import {FileClock, List, Play, Trash2, X} from 'lucide-react';
import {useState} from 'react';
import type {Bridge, TaskRecord} from '../shared/types';
import {ConfirmDialog} from '../shared/ConfirmDialog';
import {useModalFocus} from '../shared/focus';
import {formatAmount} from './downloadState';

interface RecoveryPromptProps {
  bridge: Bridge;
  records: TaskRecord[];
  onResolved(): void;
}

export function RecoveryPrompt({bridge, records, onResolved}: RecoveryPromptProps) {
  const dialogRef = useModalFocus<HTMLElement>();
  const [remaining, setRemaining] = useState(records);
  const [expanded, setExpanded] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const continueAll = async () => {
    setBusy(true); setError('');
    try {
      await Promise.all(remaining.map((record) => bridge.resumeDownload(record.id)));
      onResolved();
    } catch { setError('部分任务无法继续，请逐项处理'); }
    finally { setBusy(false); }
  };

  const handleOne = async (record: TaskRecord, remove: boolean) => {
    setBusy(true); setError('');
    try {
      if (remove) await bridge.cancelDownload(record.id, true);
      else await bridge.resumeDownload(record.id);
      if (remaining.length === 1) onResolved();
      else setRemaining((current) => current.filter((item) => item.id !== record.id));
    } catch { setError(remove ? '无法删除未完成文件' : '无法继续该任务'); }
    finally { setBusy(false); }
  };

  const deleteAll = async () => {
    setBusy(true); setError('');
    try { await bridge.clearIncomplete(); onResolved(); }
    catch { setError('无法删除未完成文件'); }
    finally { setBusy(false); setConfirmDelete(false); }
  };

  return (
    <div className="dialog-scrim">
      <section ref={dialogRef} tabIndex={-1} className="recovery-dialog" role="dialog" aria-modal="true" aria-labelledby="recovery-title">
        <button type="button" className="dialog-close" aria-label="稍后处理" title="稍后处理" onClick={onResolved}><X size={18} /></button>
        <div className="recovery-icon" aria-hidden="true"><FileClock size={26} /></div>
        <h1 id="recovery-title">发现未完成的下载</h1>
        <p>上次退出前保留了 {remaining.length} 个任务，可以从已有进度继续。</p>
        {error && <div className="panel-error" role="alert">{error}</div>}
        {expanded && <div className="recovery-list">{remaining.map((record) => (
          <div key={record.id}>
            <span><strong>{record.filename}</strong><small>{formatAmount(record.partialBytes)} / {formatAmount(record.expectedSize)}</small></span>
            <button type="button" aria-label={`继续 ${record.filename}`} title="继续" disabled={busy} onClick={() => void handleOne(record, false)}><Play size={16} /></button>
            <button type="button" className="danger-icon" aria-label={`删除 ${record.filename}`} title="删除未完成文件" disabled={busy} onClick={() => void handleOne(record, true)}><Trash2 size={16} /></button>
          </div>
        ))}</div>}
        <div className="recovery-actions">
          <button type="button" className="primary-command" disabled={busy} onClick={() => void continueAll()}><Play size={17} />继续全部</button>
          <button type="button" className="secondary-command" disabled={busy} onClick={() => setExpanded((value) => !value)}><List size={16} />逐项处理</button>
          <button type="button" className="secondary-command danger-text" disabled={busy} onClick={() => setConfirmDelete(true)}><Trash2 size={16} />删除未完成文件</button>
        </div>
      </section>
      {confirmDelete && <ConfirmDialog title="删除所有未完成文件？" message="保存的下载进度将永久丢失，已完成的 PDF 不会被删除。" confirmLabel="确认删除" danger busy={busy} onCancel={() => setConfirmDelete(false)} onConfirm={() => void deleteAll()} />}
    </div>
  );
}
