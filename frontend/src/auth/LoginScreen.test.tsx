import {act, render, screen, waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {LoginScreen} from './LoginScreen';

describe('LoginScreen', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('opens the official login page automatically when no browser session is found', async () => {
    const user = userEvent.setup();
    const detect = vi.fn().mockResolvedValue({authenticated: false});
    const openLogin = vi.fn().mockResolvedValue(undefined);
    render(<LoginScreen detect={detect} openLogin={openLogin} submitToken={vi.fn()} onAuthenticated={vi.fn()} />);

    expect(await screen.findByRole('heading', {name: '需要登录 国家中小学智慧教育平台'})).toBeVisible();
    expect(screen.getByRole('link', {name: '国家中小学智慧教育平台'})).toBeVisible();
    expect(await screen.findByText('支持 Chrome 和 Microsoft Edge')).toBeVisible();
    await waitFor(() => expect(openLogin).toHaveBeenCalledTimes(1));
    await user.click(screen.getByRole('link', {name: '国家中小学智慧教育平台'}));
    await waitFor(() => expect(openLogin).toHaveBeenCalledTimes(2));
    expect(screen.getByRole('button', {name: '重新检测'})).toBeEnabled();
    expect(screen.getByRole('button', {name: '打开登录页'})).toBeEnabled();
    expect(screen.queryByRole('button', {name: '立即前往登录'})).not.toBeInTheDocument();
  });

  it('detects again when the window regains focus and enters the catalog after success', async () => {
    const detect = vi.fn()
      .mockResolvedValueOnce({authenticated: false})
      .mockResolvedValueOnce({authenticated: true});
    const onAuthenticated = vi.fn().mockResolvedValue(undefined);
    render(<LoginScreen detect={detect} openLogin={vi.fn()} submitToken={vi.fn()} onAuthenticated={onAuthenticated} />);
    await waitFor(() => expect(detect).toHaveBeenCalledTimes(1));

    act(() => window.dispatchEvent(new Event('focus')));
    await waitFor(() => expect(onAuthenticated).toHaveBeenCalledTimes(1));
  });

  it('keeps login and retry commands available when automatic browser launch fails', async () => {
    const user = userEvent.setup();
    const openLogin = vi.fn().mockRejectedValue({code: 'AUTH_BROWSER_LAUNCH_FAILED', message: '无法打开默认浏览器'});
    render(<LoginScreen detect={vi.fn().mockResolvedValue({authenticated: false})} openLogin={openLogin} submitToken={vi.fn()} onAuthenticated={vi.fn()} />);

    expect(await screen.findByRole('alert')).toHaveTextContent('无法打开默认浏览器');
    expect(await screen.findByRole('button', {name: '打开登录页'})).toBeEnabled();
    expect(screen.getByRole('button', {name: '重新检测'})).toBeEnabled();
    await user.click(screen.getByRole('button', {name: '打开登录页'}));
    await waitFor(() => expect(openLogin).toHaveBeenCalledTimes(2));
  });

  it('keeps the reopen command after focus rechecks a failed automatic launch', async () => {
    const detect = vi.fn().mockResolvedValue({authenticated: false});
    const openLogin = vi.fn().mockRejectedValue({code: 'AUTH_BROWSER_LAUNCH_FAILED', message: '无法打开默认浏览器'});
    render(<LoginScreen detect={detect} openLogin={openLogin} submitToken={vi.fn()} onAuthenticated={vi.fn()} />);
    await screen.findByRole('alert');
    expect(screen.getAllByRole('alert')).toHaveLength(1);

    act(() => window.dispatchEvent(new Event('focus')));
    await waitFor(() => expect(detect).toHaveBeenCalledTimes(2));
    expect(screen.getByRole('button', {name: '打开登录页'})).toBeEnabled();
  });

  it('stops updating after unmount while a detection is pending', async () => {
    let resolveDetect!: (status: {authenticated: boolean}) => void;
    const detect = vi.fn(() => new Promise<{authenticated: boolean}>((resolve) => { resolveDetect = resolve; }));
    const onAuthenticated = vi.fn();
    const view = render(<LoginScreen detect={detect} openLogin={vi.fn()} submitToken={vi.fn()} onAuthenticated={onAuthenticated} />);
    view.unmount();
    await act(async () => resolveDetect({authenticated: true}));
    expect(onAuthenticated).not.toHaveBeenCalled();
  });

  it('keeps the waiting status visible while silent browser polling finds no session', async () => {
    vi.useFakeTimers();
    const detect = vi.fn().mockResolvedValue({authenticated: false});
    const openLogin = vi.fn().mockResolvedValue(undefined);
    render(<LoginScreen detect={detect} openLogin={openLogin} submitToken={vi.fn()} onAuthenticated={vi.fn()} />);

    await act(async () => { await Promise.resolve(); });
    expect(screen.getByText('请在浏览器完成登录，然后返回本程序')).toBeVisible();
    expect(document.querySelector('.browser-login-status-icon .spin')).toBeInTheDocument();
    expect(document.querySelector('.browser-login-status-icon.is-spinning')).toBeInTheDocument();

    await act(async () => { await vi.advanceTimersByTimeAsync(2000); });

    expect(screen.getByText('请在浏览器完成登录，然后返回本程序')).toBeVisible();
    expect(document.querySelector('.browser-login-status-icon .spin')).toBeInTheDocument();
    expect(document.querySelector('.browser-login-status-icon.is-spinning')).toBeInTheDocument();
    vi.useRealTimers();
  });

  it('opens a novice-friendly manual Token tutorial and validates the pasted Token', async () => {
    const user = userEvent.setup();
    const submitToken = vi.fn().mockResolvedValue({authenticated: true});
    const onAuthenticated = vi.fn().mockResolvedValue(undefined);
    render(<LoginScreen
      detect={vi.fn().mockResolvedValue({authenticated: false})}
      openLogin={vi.fn()}
      submitToken={submitToken}
      onAuthenticated={onAuthenticated}
    />);

    await user.click(await screen.findByRole('button', {name: '手动输入 Token'}));
    expect(screen.getByRole('dialog', {name: '手动输入 Token'})).toBeVisible();
    expect(screen.getByText('basic.smartedu.cn')).toBeVisible();
    expect(screen.queryByText('https://basic.smartedu.cn/tchMaterial')).not.toBeInTheDocument();
    expect(screen.getByText('access_token')).toBeVisible();
    await user.type(screen.getByLabelText('Token'), 'pasted-token');
    await user.click(screen.getByRole('button', {name: '验证 Token'}));

    await waitFor(() => expect(submitToken).toHaveBeenCalledWith('pasted-token'));
    expect(onAuthenticated).toHaveBeenCalledOnce();
  });

  it('closes manual Token entry with Escape and returns focus to its trigger', async () => {
    const user = userEvent.setup();
    render(<LoginScreen detect={vi.fn().mockResolvedValue({authenticated: false})} openLogin={vi.fn()} submitToken={vi.fn()} onAuthenticated={vi.fn()} />);

    const trigger = screen.getByRole('button', {name: '手动输入 Token'});
    await user.click(trigger);
    expect(screen.getByRole('dialog', {name: '手动输入 Token'})).toBeVisible();
    expect(screen.getByRole('dialog', {name: '手动输入 Token'})).toHaveFocus();
    await user.keyboard('{Escape}');

    expect(screen.queryByRole('dialog', {name: '手动输入 Token'})).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });

  it('shows one concise login status for a missing browser session', async () => {
    const user = userEvent.setup();
    const detect = vi.fn().mockResolvedValueOnce({authenticated: false}).mockRejectedValue({code: 'AUTH_BROWSER_SESSION_MISSING', message: '尚未检测到登录状态，请先前往网页登录'});
    render(<LoginScreen detect={detect} openLogin={vi.fn()} submitToken={vi.fn()} onAuthenticated={vi.fn()} />);

    await screen.findByText('请在浏览器完成登录，然后返回本程序');
    await user.click(screen.getByRole('button', {name: '重新检测'}));
    expect(await screen.findByText('未检测到登录状态，请完成浏览器登录后重新检测')).toBeVisible();
    expect(document.querySelector('.browser-login-status-icon .lucide-circle-alert')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('keeps the waiting status stable while manually rechecking', async () => {
    const user = userEvent.setup();
    let resolveRecheck!: (status: {authenticated: boolean}) => void;
    const detect = vi.fn()
      .mockResolvedValueOnce({authenticated: false})
      .mockImplementationOnce(() => new Promise((resolve) => { resolveRecheck = resolve; }));
    render(<LoginScreen detect={detect} openLogin={vi.fn().mockResolvedValue(undefined)} submitToken={vi.fn()} onAuthenticated={vi.fn()} />);

    expect(await screen.findByText('请在浏览器完成登录，然后返回本程序')).toBeVisible();
    await user.click(screen.getByRole('button', {name: '重新检测'}));
    expect(screen.queryByText('正在检测浏览器登录状态…')).not.toBeInTheDocument();
    expect(screen.getByText('请在浏览器完成登录，然后返回本程序')).toBeVisible();

    await act(async () => resolveRecheck({authenticated: false}));
  });

  it('offers manual Token entry when browser storage is incompatible', async () => {
    const detect = vi.fn().mockRejectedValue({code: 'AUTH_BROWSER_STORAGE', message: '浏览器登录数据格式暂不兼容，请更新本程序'});
    const openLogin = vi.fn();
    render(<LoginScreen detect={detect} openLogin={openLogin} submitToken={vi.fn()} onAuthenticated={vi.fn()} />);

    expect(await screen.findByText('无法自动读取浏览器登录状态，可手动输入 Token')).toBeVisible();
    expect(screen.getByRole('button', {name: '手动输入 Token'})).toBeEnabled();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(openLogin).not.toHaveBeenCalled();
  });
});
