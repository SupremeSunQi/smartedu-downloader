import {Chrome, CircleAlert, Copy, ExternalLink, KeyRound, LoaderCircle, RefreshCw, ShieldCheck, X} from 'lucide-react';
import {useCallback, useEffect, useRef, useState} from 'react';
import {useModalFocus} from '../shared/focus';
import type {SessionStatus} from '../shared/types';

interface LoginScreenProps {
  detect(): Promise<SessionStatus>;
  openLogin(): Promise<void>;
  submitToken(token: string): Promise<SessionStatus>;
  onAuthenticated(): Promise<void> | void;
}

type DetectionState = 'checking' | 'opening' | 'waiting' | 'missing' | 'expired' | 'storage' | 'error';
type DetectionOutcome = 'authenticated' | 'needs-login' | 'unavailable';

const POLL_INTERVAL_MS = 2000;
const POLL_TIMEOUT_MS = 120000;
const officialLoginURL = 'https://auth.smartedu.cn/uias/login';

export function LoginScreen({detect, openLogin, submitToken, onAuthenticated}: LoginScreenProps) {
  const [state, setState] = useState<DetectionState>('checking');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [manualOpen, setManualOpen] = useState(false);
  const [manualToken, setManualToken] = useState('');
  const [manualBusy, setManualBusy] = useState(false);
  const [manualError, setManualError] = useState('');
  const manualDialogRef = useModalFocus<HTMLElement>(manualOpen);
  const mounted = useRef(true);
  const inFlight = useRef(false);
  const generation = useRef(0);
  const pollTimer = useRef<number | undefined>(undefined);
  const pollDeadline = useRef<number | undefined>(undefined);
  const autoLaunchAttempted = useRef(false);
  const launchInFlight = useRef(false);
  const detectRef = useRef(detect);
  const openLoginRef = useRef(openLogin);
  const submitTokenRef = useRef(submitToken);
  const onAuthenticatedRef = useRef(onAuthenticated);
  detectRef.current = detect;
  openLoginRef.current = openLogin;
  submitTokenRef.current = submitToken;
  onAuthenticatedRef.current = onAuthenticated;

  const stopPolling = useCallback(() => {
    if (pollTimer.current !== undefined) {
      window.clearInterval(pollTimer.current);
      pollTimer.current = undefined;
    }
    pollDeadline.current = undefined;
  }, []);

  const runDetection = useCallback(async (foreground = true): Promise<DetectionOutcome> => {
    if (!mounted.current || inFlight.current) return 'unavailable';
    const currentGeneration = generation.current;
    inFlight.current = true;
    if (foreground) {
      setBusy(true);
      setError('');
    }
    try {
      const session = await detectRef.current();
      if (!mounted.current || currentGeneration !== generation.current) return 'unavailable';
      if (!session.authenticated) {
        if (foreground) setState('missing');
        return 'needs-login';
      }
      await onAuthenticatedRef.current();
      if (!mounted.current || currentGeneration !== generation.current) return 'unavailable';
      stopPolling();
      return 'authenticated';
    } catch (reason) {
      if (mounted.current && currentGeneration === generation.current) {
        const nextState = readDetectionState(reason);
        if (foreground) {
          setState(nextState);
          setError(nextState === 'missing' || nextState === 'expired' || nextState === 'storage' ? '' : formatDetectionError(reason));
        }
        return nextState === 'missing' || nextState === 'expired' ? 'needs-login' : 'unavailable';
      }
      return 'unavailable';
    } finally {
      inFlight.current = false;
      if (foreground && mounted.current && currentGeneration === generation.current) setBusy(false);
    }
  }, [stopPolling]);

  const startPolling = useCallback(() => {
    stopPolling();
    pollDeadline.current = Date.now() + POLL_TIMEOUT_MS;
    pollTimer.current = window.setInterval(() => {
      if (pollDeadline.current !== undefined && Date.now() >= pollDeadline.current) {
        stopPolling();
        return;
      }
      void runDetection(false).then((outcome) => {
        if (outcome === 'authenticated') stopPolling();
      });
    }, POLL_INTERVAL_MS);
  }, [runDetection, stopPolling]);

  const openOfficialLogin = useCallback(async () => {
    if (!mounted.current || launchInFlight.current) return;
    launchInFlight.current = true;
    setBusy(true);
    setState('opening');
    setError('');
    try {
      await openLoginRef.current();
      if (!mounted.current) return;
      setState('waiting');
      startPolling();
    } catch (reason) {
      if (mounted.current) {
        setState('error');
        setError(formatDetectionError(reason));
      }
    } finally {
      launchInFlight.current = false;
      if (mounted.current) setBusy(false);
    }
  }, [startPolling]);

  useEffect(() => {
    mounted.current = true;
    void runDetection().then((outcome) => {
      if (outcome === 'needs-login' && !autoLaunchAttempted.current) {
        autoLaunchAttempted.current = true;
        void openOfficialLogin();
      }
    });
    const handleFocus = () => { void runDetection(); };
    window.addEventListener('focus', handleFocus);
    return () => {
      mounted.current = false;
      generation.current += 1;
      stopPolling();
      window.removeEventListener('focus', handleFocus);
    };
  }, [openOfficialLogin, runDetection, stopPolling]);

  const openManualToken = () => {
    setManualError('');
    setManualToken('');
    setManualOpen(true);
  };

  const closeManualToken = useCallback(() => {
    if (manualBusy) return;
    setManualOpen(false);
    setManualToken('');
    setManualError('');
  }, [manualBusy]);

  useEffect(() => {
    if (!manualOpen) return;
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || event.defaultPrevented || manualBusy) return;
      event.preventDefault();
      closeManualToken();
    };
    window.addEventListener('keydown', closeOnEscape);
    return () => window.removeEventListener('keydown', closeOnEscape);
  }, [closeManualToken, manualBusy, manualOpen]);

  const submitManualToken = async () => {
    const token = manualToken.trim();
    if (!token) {
      setManualError('请粘贴 Token 后再验证');
      return;
    }
    setManualBusy(true);
    setManualError('');
    try {
      const session = await submitTokenRef.current(token);
      if (!session.authenticated) throw new Error('Token validation did not authenticate');
      await onAuthenticatedRef.current();
      if (!mounted.current) return;
      stopPolling();
      setManualOpen(false);
      setManualToken('');
    } catch {
      if (mounted.current) setManualError('Token 无效、已过期，或暂时无法验证。请重新获取后再试。');
    } finally {
      if (mounted.current) setManualBusy(false);
    }
  };

  const statusMessage = state === 'checking'
    ? '正在检测浏览器登录状态…'
    : state === 'opening'
      ? '正在打开官方登录页…'
      : state === 'waiting'
        ? '请在浏览器完成登录，然后返回本程序'
    : state === 'missing'
      ? '未检测到登录状态，请完成浏览器登录后重新检测'
      : state === 'expired'
        ? '浏览器中的登录状态已失效，请重新登录'
          : state === 'storage'
            ? '无法自动读取浏览器登录状态，可手动输入 Token'
            : error || '登录状态检测失败，请重试';
  const statusTitle = state === 'checking' ? '正在检测登录状态'
    : state === 'opening' ? '正在打开登录页'
      : state === 'waiting' ? '等待浏览器登录'
        : state === 'storage' ? '无法自动识别登录状态'
          : state === 'error' ? '连接遇到问题'
            : state === 'expired' ? '登录已过期'
            : '尚未连接平台';
  const statusSpinning = busy || state === 'checking' || state === 'opening' || state === 'waiting';

  return (
    <main className="login-shell" aria-labelledby="login-title">
      <div className="browser-login-page">
        <header className="login-brand">
          <div className="brand-mark"><img className="app-logo" src="/appicon.png" alt="智教教材图标" /></div>
          <span>智教教材下载器</span>
        </header>
        <section className="browser-login-workflow">
          <div className="login-intro">
            <span className="login-eyebrow">账号连接</span>
            <h1 id="login-title">需要登录 <a className="platform-link" href={officialLoginURL} onClick={(event) => { event.preventDefault(); void openOfficialLogin(); }}>国家中小学智慧教育平台<ExternalLink size={17} aria-hidden="true" /></a></h1>
            <p className="login-lead">官方登录页会自动打开。完成浏览器登录后返回本程序，即可继续使用。</p>
            <div className="browser-support"><Chrome size={19} aria-hidden="true" /><span>支持 Chrome 和 Microsoft Edge</span></div>
          </div>
          <section className="browser-login-status-panel" data-status={state} aria-label="登录状态">
            <div className="browser-login-status-label">当前状态</div>
            <div className={`browser-login-status-icon${statusSpinning ? ' is-spinning' : ''}`} aria-hidden="true">
              {statusSpinning ? <LoaderCircle className="spin" size={28} /> : <CircleAlert size={28} />}
            </div>
            <div className="browser-login-status" data-status={state} aria-live="polite">
              <h2>{statusTitle}</h2>
              <p role={state === 'error' ? 'alert' : undefined} title={state === 'error' ? statusMessage : undefined}>{statusMessage}</p>
            </div>
            <div className="browser-login-actions">
              <button type="button" className="primary-command reopen-command" onClick={() => void openOfficialLogin()} disabled={busy}>
                <ExternalLink size={16} />打开登录页
              </button>
              <button type="button" className="secondary-command recheck-command" onClick={() => void runDetection()} disabled={busy}>
                <RefreshCw size={16} />重新检测
              </button>
            </div>
          </section>
        </section>
        <footer className="login-footer"><div className="login-security"><ShieldCheck size={16} aria-hidden="true" /><span>仅使用登录状态，不读取或保存浏览器密码。</span></div><button type="button" className="text-command" onClick={openManualToken}><Copy size={15} />手动输入 Token</button></footer>
      </div>
      {manualOpen && <div className="dialog-scrim manual-token-scrim">
        <section ref={manualDialogRef} tabIndex={-1} className="manual-token-dialog" role="dialog" aria-modal="true" aria-labelledby="manual-token-title">
          <header><div className="manual-token-icon" aria-hidden="true"><KeyRound size={21} /></div><div><h2 id="manual-token-title">手动输入 Token</h2><p>使用其他浏览器，或无法自动识别登录状态时使用。</p></div><button type="button" className="dialog-close" aria-label="关闭 Token 输入" title="关闭" disabled={manualBusy} onClick={closeManualToken}><X size={19} /></button></header>
          <ol className="token-tutorial">
            <li>在官方登录页完成账号登录，进入平台主页。</li>
            <li>按 <kbd>F12</kbd>，选择“应用 / Application”选项卡。</li>
            <li>展开“本地存储 / Local Storage”，选择 <code>basic.smartedu.cn</code>。</li>
            <li>在键名中找到 <code>ND_UC_AUTH</code>，查看值中的 <code>access_token</code>，只复制引号内的内容，不要复制整段 JSON。</li>
          </ol>
          <label className="token-input"><span>Token</span><textarea value={manualToken} onChange={(event) => setManualToken(event.target.value)} placeholder="粘贴 access_token 内容" autoComplete="off" spellCheck="false" maxLength={16384} disabled={manualBusy} /></label>
          <p className="token-privacy">Token 相当于临时登录凭证，请勿发给他人。本程序只在当前运行期间使用。</p>
          {manualError && <p className="manual-token-error" role="alert">{manualError}</p>}
          <footer><button type="button" className="secondary-command" onClick={closeManualToken} disabled={manualBusy}>取消</button><button type="button" className="primary-command" onClick={() => void submitManualToken()} disabled={manualBusy}>{manualBusy ? <LoaderCircle className="spin" size={17} /> : <KeyRound size={17} />}验证 Token</button></footer>
        </section>
      </div>}
    </main>
  );
}

function readDetectionState(reason: unknown): DetectionState {
  const code = readErrorField(reason, 'code');
  if (code === 'AUTH_BROWSER_SESSION_MISSING') return 'missing';
  if (code === 'AUTH_BROWSER_SESSION_EXPIRED') return 'expired';
  if (code === 'AUTH_BROWSER_STORAGE') return 'storage';
  return 'error';
}

function formatDetectionError(reason: unknown): string {
  const message = readErrorField(reason, 'message');
  return message || '登录状态检测失败，请重试';
}

function readErrorField(reason: unknown, field: 'code' | 'message'): string {
  if (!reason || typeof reason !== 'object') return '';
  const value = reason as Record<string, unknown>;
  return typeof value[field] === 'string' ? value[field].trim().slice(0, 180) : '';
}
