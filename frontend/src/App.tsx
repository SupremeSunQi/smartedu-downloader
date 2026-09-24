import {LoaderCircle, RefreshCw} from 'lucide-react';
import {useCallback, useEffect, useState} from 'react';
import {LoginScreen} from './auth/LoginScreen';
import {CatalogScreen} from './catalog/CatalogScreen';
import {DownloadPanel} from './downloads/DownloadPanel';
import {mergeTask} from './downloads/downloadState';
import {RecoveryPrompt} from './downloads/RecoveryPrompt';
import {SettingsDialog} from './settings/SettingsDialog';
import {desktopBridge} from './shared/bridge';
import {ConfirmDialog} from './shared/ConfirmDialog';
import {emptyCatalog, type BootstrapState, type Bridge, type DownloadTask} from './shared/types';

interface AppProps {
  bridge?: Bridge;
}

export function App({bridge = desktopBridge}: AppProps) {
  const [state, setState] = useState<BootstrapState | null>(null);
  const [startupError, setStartupError] = useState('');
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [downloadsOpen, setDownloadsOpen] = useState(false);
  const [closeRequested, setCloseRequested] = useState(false);

  const bootstrap = useCallback(async () => {
    setStartupError('');
    try {
      setState(await bridge.bootstrap());
    } catch {
      setStartupError('程序初始化失败，请重新启动');
    }
  }, [bridge]);

  useEffect(() => { void bootstrap(); }, [bootstrap]);

  const refreshAfterBrowserLogin = useCallback(async () => {
    const next = await bridge.bootstrap();
    if (!next.session.authenticated) throw new Error('browser session validation did not persist');
    setState(next);
  }, [bridge]);

  const updateTasks = useCallback((tasks: DownloadTask[]) => {
    setState((current) => current ? {...current, tasks} : current);
  }, []);

  const addTasks = useCallback((tasks: DownloadTask[]) => {
    setState((current) => current ? {...current, tasks: tasks.reduce(mergeTask, current.tasks)} : current);
    if (tasks.length > 0) setDownloadsOpen(true);
  }, []);

  const finishLogout = useCallback(() => {
    setSettingsOpen(false);
    setDownloadsOpen(false);
    setState((current) => current ? {
      ...current,
      session: {authenticated: false},
      catalog: emptyCatalog,
      recoverable: [],
    } : current);
  }, []);

  useEffect(() => {
    const cancelClose = bridge.onCloseRequested?.(() => setCloseRequested(true)) ?? (() => {});
    const cancelExpiry = bridge.onSessionExpired?.(() => finishLogout()) ?? (() => {});
    return () => { cancelClose(); cancelExpiry(); };
  }, [bridge, finishLogout]);

  if (startupError) {
    return <div className="startup-state" role="alert"><strong>{startupError}</strong><button type="button" className="secondary-command" onClick={() => void bootstrap()}><RefreshCw size={16} />重试</button></div>;
  }
  if (!state) {
    return <div className="startup-state" aria-label="正在启动"><LoaderCircle className="spin" /><span>正在启动</span></div>;
  }
  if (!state.session.authenticated) {
    return <LoginScreen detect={bridge.detectBrowserSession} openLogin={bridge.openBrowserLogin} submitToken={bridge.submitToken} onAuthenticated={refreshAfterBrowserLogin} />;
  }
  const activeTaskCount = state.tasks.filter((task) => ['queued', 'resolving', 'downloading', 'paused', 'waiting_auth'].includes(task.status)).length;
  return (
    <div className="authenticated-shell">
      <CatalogScreen bridge={bridge} initialSnapshot={state.catalog} activeTaskCount={activeTaskCount} onOpenSettings={() => setSettingsOpen(true)} onOpenDownloads={() => setDownloadsOpen(true)} onTasksAdded={addTasks} />
      {downloadsOpen && <div className="download-backdrop" aria-hidden="true" onClick={() => setDownloadsOpen(false)} />}
      <DownloadPanel bridge={bridge} tasks={state.tasks} open={downloadsOpen} onClose={() => setDownloadsOpen(false)} onTasksChange={updateTasks} />
      {settingsOpen && <SettingsDialog
        bridge={bridge}
        config={state.config}
        paths={state.paths}
        activeTaskCount={activeTaskCount}
        onClose={() => setSettingsOpen(false)}
        onSaved={(config) => setState((current) => current ? {...current, config} : current)}
        onLogout={finishLogout}
      />}
      {state.recoverable.length > 0 && <RecoveryPrompt bridge={bridge} records={state.recoverable} onResolved={() => setState((current) => current ? {...current, recoverable: []} : current)} />}
      {closeRequested && <ConfirmDialog
        title="退出并暂停下载？"
        message={`当前有 ${activeTaskCount} 个活动任务，退出前会安全暂停并记录进度。`}
        confirmLabel="确认退出"
        danger
        onCancel={() => setCloseRequested(false)}
        onConfirm={() => void bridge.shutdown()}
      />}
    </div>
  );
}
