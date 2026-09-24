import {AlertTriangle, X} from 'lucide-react';
import {useEffect} from 'react';
import {useModalFocus} from './focus';

interface ConfirmDialogProps {
  title: string;
  message: string;
  confirmLabel: string;
  danger?: boolean;
  busy?: boolean;
  onConfirm(): void;
  onCancel(): void;
}

export function ConfirmDialog({title, message, confirmLabel, danger = false, busy = false, onConfirm, onCancel}: ConfirmDialogProps) {
  const dialogRef = useModalFocus<HTMLElement>();
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        event.stopPropagation();
        if (!busy) onCancel();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [busy, onCancel]);

  return (
    <div className="dialog-scrim dialog-scrim-nested">
      <section ref={dialogRef} tabIndex={-1} className="confirm-dialog" role="dialog" aria-modal="true" aria-labelledby="confirm-title">
        <div className="confirm-icon" aria-hidden="true"><AlertTriangle size={22} /></div>
        <div className="confirm-copy">
          <h2 id="confirm-title">{title}</h2>
          <p>{message}</p>
        </div>
        <button type="button" className="dialog-close" aria-label="取消" title="取消" disabled={busy} onClick={onCancel}><X size={18} /></button>
        <div className="confirm-actions">
          <button type="button" className="secondary-command" disabled={busy} onClick={onCancel}>取消</button>
          <button type="button" className={danger ? 'danger-command' : 'primary-command'} disabled={busy} onClick={onConfirm}>{confirmLabel}</button>
        </div>
      </section>
    </div>
  );
}
