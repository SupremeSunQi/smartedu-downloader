import {Archive, Check, ChevronDown, Database, Download, FolderOpen, HardDrive, Info, Save, Trash2, X} from 'lucide-react';
import {useEffect, useRef, useState} from 'react';
import type {AppConfig, Bridge, DuplicatePolicy, PathState} from '../shared/types';
import {ConfirmDialog} from '../shared/ConfirmDialog';
import {useModalFocus} from '../shared/focus';

type Confirmation = 'cache' | 'incomplete' | null;

const duplicatePolicyOptions: Array<{value: DuplicatePolicy; label: string}> = [
  {value: 'ask', label: '每次询问'},
  {value: 'rename', label: '自动重命名'},
  {value: 'overwrite', label: '覆盖'},
  {value: 'skip', label: '跳过'},
];

interface SettingsDialogProps {
  bridge: Bridge;
  config: AppConfig;
  paths: PathState;
  activeTaskCount: number;
  onClose(): void;
  onSaved(config: AppConfig): void;
  onLogout(): void;
}

export function SettingsDialog({bridge, config: initialConfig, paths, activeTaskCount, onClose, onSaved, onLogout}: SettingsDialogProps) {
  const dialogRef = useModalFocus<HTMLElement>();
  const [config, setConfig] = useState(initialConfig);
  const [confirmation, setConfirmation] = useState<Confirmation>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !event.defaultPrevented && !document.querySelector('.dialog-scrim-nested')) {
        event.preventDefault();
        onClose();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onClose]);

  const run = async (operation: () => Promise<void>, success: string) => {
    setBusy(true); setError(''); setMessage('');
    try { await operation(); setMessage(success); } catch { setError('操作失败，请稍后重试'); }
    finally { setBusy(false); setConfirmation(null); }
  };

  const save = () => run(async () => {
    const saved = await bridge.updateSettings(config);
    setConfig(saved);
    onSaved(saved);
  }, '设置已保存，重启程序后生效');

  const pickDirectory = async () => {
    if (!bridge.chooseDownloadDirectory) { setError('当前窗口暂不支持选择目录'); return; }
    setError('');
    try {
      const selected = await bridge.chooseDownloadDirectory(config.downloadDir);
      if (selected) setConfig((current) => ({...current, downloadDir: selected}));
    } catch { setError('无法选择下载目录'); }
  };

  const confirmationDetails = getConfirmationDetails(confirmation);

  return (
    <div className="dialog-scrim">
      <section ref={dialogRef} tabIndex={-1} className="settings-dialog" role="dialog" aria-modal="true" aria-labelledby="settings-title">
        <header><div><h1 id="settings-title">设置</h1><p>下载偏好与本机数据</p></div><button type="button" className="dialog-close" aria-label="关闭设置" title="关闭" onClick={onClose}><X size={19} /></button></header>
        <div className="settings-content">
          <section className="settings-section settings-section--download" aria-labelledby="download-settings-title">
            <div className="settings-section-heading">
              <h2 id="download-settings-title"><Download size={17} aria-hidden="true" />下载</h2>
              <span className="settings-restart-hint" title="下载目录、同时下载数量和同名文件处理会在重启程序后生效"><Info size={14} aria-hidden="true" />重启程序后生效</span>
            </div>
            <label>同时下载数量<input className="settings-control" type="number" min={1} max={5} value={config.concurrentDownloads} onChange={(event) => setConfig((current) => ({...current, concurrentDownloads: Number(event.target.value)}))} /></label>
            <label className="settings-select-field"><span>同名文件处理</span><DuplicatePolicySelect value={config.duplicatePolicy} onChange={(value) => setConfig((current) => ({...current, duplicatePolicy: value}))} disabled={busy} /></label>
            <label className="directory-setting">下载目录<span className="settings-control"><input aria-label="下载目录" readOnly value={config.downloadDir} /><button type="button" className="icon-command" aria-label="选择下载目录" title="选择下载目录" onClick={() => void pickDirectory()}><FolderOpen size={18} /></button></span></label>
          </section>

          <section className="settings-section settings-section--storage" aria-labelledby="storage-settings-title">
            <h2 id="storage-settings-title"><HardDrive size={17} aria-hidden="true" />本机数据</h2>
            <dl className="path-list"><div><dt>当前下载目录</dt><dd title={paths.downloadDir}>{paths.downloadDir}</dd></div><div><dt>数据目录</dt><dd title={paths.dataRoot}>{paths.dataRoot}</dd></div><div><dt>缓存目录</dt><dd title={paths.cacheDir}>{paths.cacheDir}</dd></div><div><dt>日志目录</dt><dd title={paths.logsDir}>{paths.logsDir}</dd></div></dl>
            <div className="maintenance-actions">
              <button type="button" className="secondary-command" onClick={() => setConfirmation('cache')}><Database size={16} />清理目录缓存</button>
              <button type="button" className="secondary-command danger-text" onClick={() => setConfirmation('incomplete')}><Trash2 size={16} />删除未完成文件</button>
              <button type="button" className="secondary-command" onClick={() => void run(async () => { await bridge.exportDiagnostics(paths.downloadDir); }, '诊断包已导出到下载目录')}><Archive size={16} />导出诊断信息</button>
              <button type="button" className="secondary-command" disabled={!bridge.openDownloadFolder} onClick={() => void bridge.openDownloadFolder?.()}><FolderOpen size={16} />打开下载目录</button>
            </div>
          </section>
        </div>
        {(message || error) && <div className={error ? 'settings-message is-error' : 'settings-message'} role={error ? 'alert' : 'status'}>{error || message}</div>}
        <footer className="settings-footer"><div><button type="button" className="primary-command settings-save" disabled={busy || config.concurrentDownloads < 1 || config.concurrentDownloads > 5} onClick={() => void save()}><Save size={17} />保存设置</button></div></footer>
      </section>
      {confirmationDetails && <ConfirmDialog {...confirmationDetails} busy={busy} onCancel={() => setConfirmation(null)} onConfirm={() => {
        if (confirmation === 'cache') void run(() => bridge.clearCache(), '目录缓存已清理');
        if (confirmation === 'incomplete') void run(() => bridge.clearIncomplete(), '未完成文件已删除');
      }} />}
    </div>
  );
}

function DuplicatePolicySelect({value, onChange, disabled}: {value: DuplicatePolicy; onChange(value: DuplicatePolicy): void; disabled: boolean}) {
  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);
  const rootRef = useRef<HTMLSpanElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const optionRefs = useRef<Array<HTMLButtonElement | null>>([]);
  const listboxId = 'duplicate-policy-options';
  const selected = duplicatePolicyOptions.find((option) => option.value === value) ?? duplicatePolicyOptions[0];
  const selectedIndex = duplicatePolicyOptions.findIndex((option) => option.value === selected.value);

  const openMenu = (index = selectedIndex) => {
    setActiveIndex(index);
    setOpen(true);
  };

  const selectOption = (nextValue: DuplicatePolicy) => {
    onChange(nextValue);
    setOpen(false);
    triggerRef.current?.focus();
  };

  useEffect(() => {
    if (!open) return undefined;
    const closeOnOutsidePointer = (event: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(event.target as Node)) setOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        event.stopPropagation();
        setOpen(false);
        triggerRef.current?.focus();
      }
    };
    document.addEventListener('mousedown', closeOnOutsidePointer);
    document.addEventListener('keydown', closeOnEscape);
    return () => {
      document.removeEventListener('mousedown', closeOnOutsidePointer);
      document.removeEventListener('keydown', closeOnEscape);
    };
  }, [open]);

  useEffect(() => {
    if (open) optionRefs.current[activeIndex]?.focus();
  }, [activeIndex, open]);

  return (
    <span ref={rootRef} className={`select-control settings-control custom-select${open ? ' is-open' : ''}`}>
      <button
        type="button"
        className="custom-select-trigger"
        ref={triggerRef}
        role="combobox"
        aria-label="同名文件处理"
        aria-haspopup="listbox"
        aria-controls={listboxId}
        aria-expanded={open}
        disabled={disabled}
        onClick={() => { if (open) setOpen(false); else openMenu(); }}
        onKeyDown={(event) => {
          if (event.key === 'Escape') {
            event.preventDefault();
            event.stopPropagation();
            setOpen(false);
          } else if (event.key === 'ArrowDown') {
            event.preventDefault();
            openMenu(Math.min(selectedIndex + 1, duplicatePolicyOptions.length - 1));
          } else if (event.key === 'ArrowUp') {
            event.preventDefault();
            openMenu(Math.max(selectedIndex - 1, 0));
          } else if (event.key === 'Home') {
            event.preventDefault();
            openMenu(0);
          } else if (event.key === 'End') {
            event.preventDefault();
            openMenu(duplicatePolicyOptions.length - 1);
          } else if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault();
            if (open) setOpen(false); else openMenu();
          }
        }}
      >
        <span>{selected.label}</span><ChevronDown size={16} aria-hidden="true" />
      </button>
      {open && <div id={listboxId} className="custom-select-menu" role="listbox" aria-label="同名文件处理">
        {duplicatePolicyOptions.map((option) => {
          const isSelected = option.value === value;
          return <button
            key={option.value}
            id={`${listboxId}-${option.value}`}
            ref={(element) => { optionRefs.current[indexOfOption(option.value)] = element; }}
            type="button"
            role="option"
            tabIndex={activeIndex === indexOfOption(option.value) ? 0 : -1}
            aria-selected={isSelected}
            className={`custom-select-option${isSelected ? ' is-selected' : ''}`}
            onClick={() => selectOption(option.value)}
            onKeyDown={(event) => {
              const index = indexOfOption(option.value);
              if (event.key === 'Escape') {
                event.preventDefault();
                event.stopPropagation();
                setOpen(false);
                triggerRef.current?.focus();
              } else if (event.key === 'ArrowDown') {
                event.preventDefault();
                const next = Math.min(index + 1, duplicatePolicyOptions.length - 1);
                setActiveIndex(next);
                optionRefs.current[next]?.focus();
              } else if (event.key === 'ArrowUp') {
                event.preventDefault();
                const next = Math.max(index - 1, 0);
                setActiveIndex(next);
                optionRefs.current[next]?.focus();
              } else if (event.key === 'Home') {
                event.preventDefault();
                setActiveIndex(0);
                optionRefs.current[0]?.focus();
              } else if (event.key === 'End') {
                event.preventDefault();
                const last = duplicatePolicyOptions.length - 1;
                setActiveIndex(last);
                optionRefs.current[last]?.focus();
              } else if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault();
                selectOption(option.value);
              } else if (event.key === 'Tab') {
                setOpen(false);
              }
            }}
          ><span>{option.label}</span>{isSelected && <Check size={16} aria-hidden="true" />}</button>;
        })}
      </div>}
    </span>
  );
}

function indexOfOption(value: DuplicatePolicy) {
  return duplicatePolicyOptions.findIndex((option) => option.value === value);
}

function getConfirmationDetails(confirmation: Confirmation) {
  switch (confirmation) {
  case 'cache': return {title: '清理目录缓存？', message: '下次使用时需要重新同步教材目录，已下载的 PDF 不会被删除。', confirmLabel: '确认清理'};
  case 'incomplete': return {title: '删除未完成文件？', message: '所有可恢复的下载进度将永久丢失，已完成的 PDF 不受影响。', confirmLabel: '确认删除', danger: true};
  default: return null;
  }
}
