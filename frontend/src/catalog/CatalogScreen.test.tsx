import {render, screen, waitFor, within} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {describe, expect, it, vi} from 'vitest';
import type {Bridge, BootstrapState, CatalogSnapshot, Textbook} from '../shared/types';
import {CatalogScreen} from './CatalogScreen';

const books: Textbook[] = [
  {
    id: 'book-1', title: '语文一年级上册', provider: '人民教育出版社', size: 12_500_000,
    stageId: 'primary', stage: '小学', subjectId: 'chinese', subject: '语文',
    editionId: 'pep', edition: '人教版', gradeId: 'grade-1', grade: '一年级', volumeId: 'up', volume: '上册',
  },
  {
    id: 'book-2', title: '数学一年级上册', provider: '人民教育出版社', size: 10_000_000,
    stageId: 'primary', stage: '小学', subjectId: 'math', subject: '数学',
    editionId: 'pep', edition: '人教版', gradeId: 'grade-1', grade: '一年级', volumeId: 'up', volume: '上册',
  },
  {
    id: 'book-3', title: '高中语文必修一', provider: '人民教育出版社', size: 18_000_000,
    stageId: 'high', stage: '高中', subjectId: 'chinese', subject: '语文',
    editionId: 'pep', edition: '人教版', gradeId: 'grade-10', grade: '高一', volumeId: 'required-1', volume: '必修一',
  },
];

const snapshot: CatalogSnapshot = {
  version: 1,
  syncedAt: '2026-09-23T09:00:00Z',
  textbooks: books,
  filters: {
    stages: [{id: 'primary', name: '小学'}, {id: 'high', name: '高中'}],
    subjects: [{id: 'chinese', name: '语文'}, {id: 'math', name: '数学'}],
    editions: [{id: 'pep', name: '人教版'}],
    grades: [{id: 'grade-1', name: '一年级'}, {id: 'grade-10', name: '高一'}],
    volumes: [{id: 'up', name: '上册'}, {id: 'required-1', name: '必修一'}],
  },
};

function catalogBridge(): Bridge {
  const state: BootstrapState = {
    session: {authenticated: true}, catalog: snapshot,
    config: {dataRoot: 'data', downloadDir: 'downloads', concurrentDownloads: 2, duplicatePolicy: 'ask'},
    tasks: [], recoverable: [],
    paths: {dataRoot: 'data', downloadDir: 'downloads', cacheDir: 'cache', logsDir: 'logs', webviewDir: 'webview'},
  };
  return {
    bootstrap: vi.fn().mockResolvedValue(state), detectBrowserSession: vi.fn(), openBrowserLogin: vi.fn(), submitToken: vi.fn(), logout: vi.fn(),
    syncCatalog: vi.fn().mockResolvedValue(snapshot),
    queryCatalog: vi.fn().mockImplementation(async (query) => books.filter((book) =>
      (!query.stageId || book.stageId === query.stageId)
      && (!query.subjectId || book.subjectId === query.subjectId)
      && (!query.editionId || book.editionId === query.editionId)
      && (!query.gradeId || book.gradeId === query.gradeId)
      && (!query.volumeId || book.volumeId === query.volumeId)
      && (!query.search || book.title.includes(query.search)))),
    enqueueDownloads: vi.fn().mockResolvedValue([]), pauseDownload: vi.fn(), resumeDownload: vi.fn(),
    cancelDownload: vi.fn(), retryDownload: vi.fn(), updateSettings: vi.fn(), clearCache: vi.fn(),
    clearIncomplete: vi.fn(), exportDiagnostics: vi.fn(), shutdown: vi.fn(), onDownloadUpdate: vi.fn(() => () => {}),
  };
}

describe('CatalogScreen', () => {
  it('renders the catalog filters without an all option or search field', async () => {
    render(<CatalogScreen bridge={catalogBridge()} initialSnapshot={snapshot} />);

    expect(await screen.findByRole('heading', {name: '语文一年级上册'})).toBeVisible();
    expect(screen.getByRole('img', {name: '智教教材图标'})).toHaveAttribute('src', '/appicon.png');
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', {name: '全部取消'})).not.toBeInTheDocument();
    expect(screen.queryByRole('button', {name: /下载所选/})).not.toBeInTheDocument();
    expect(screen.queryByRole('radio', {name: '全部'})).not.toBeInTheDocument();
    expect(screen.queryByRole('radio', {name: '全部教材'})).not.toBeInTheDocument();
    expect(screen.queryByRole('textbox', {name: '搜索教材'})).not.toBeInTheDocument();
    expect(screen.getByRole('radio', {name: '小学'})).toBeChecked();
    expect(screen.getByRole('radio', {name: '语文'})).toBeChecked();
    expect(screen.getByRole('radio', {name: '人教版'})).toBeChecked();
    expect(screen.getByRole('radio', {name: '一年级'})).toBeChecked();
    expect(screen.queryByRole('group', {name: '册次'})).not.toBeInTheDocument();
    expect(screen.queryByRole('radio', {name: '上册'})).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', {name: '数学一年级上册'})).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', {name: '高中语文必修一'})).not.toBeInTheDocument();
  });

  it('filters out server-provided all options', async () => {
    const allBook: Textbook = {...books[0], id: 'all-book', stageId: 'all', stage: '全部', subjectId: 'all-subject', subject: '全部教材'};
    const allSnapshot: CatalogSnapshot = {...snapshot, textbooks: [...books, allBook]};
    render(<CatalogScreen bridge={catalogBridge()} initialSnapshot={allSnapshot} />);

    expect(await screen.findByRole('heading', {name: '语文一年级上册'})).toBeVisible();
    expect(screen.queryByRole('radio', {name: '全部'})).not.toBeInTheDocument();
    expect(screen.queryByRole('radio', {name: '全部教材'})).not.toBeInTheDocument();
  });

  it('keeps school stages in the platform order', () => {
    const stageNames = ['小学', '初中', '小学（五·四学制）', '初中（五·四学制）', '高中', '特殊教育'];
    const sourceStageNames = ['小学', '初中', '小学（五•四学制）', '初中（五•四学制）', '高中', '特殊教育'];
    const stageBooks = sourceStageNames.map((stage, index) => ({...books[0], id: `stage-${index}`, stageId: `stage-${index}`, stage}));
    render(<CatalogScreen bridge={catalogBridge()} initialSnapshot={{...snapshot, textbooks: stageBooks}} />);

    const stageGroup = within(screen.getByRole('group', {name: '学段'}));
    expect(stageGroup.getAllByRole('radio').map((radio) => radio.parentElement?.textContent?.trim())).toEqual(stageNames);
  });

  it('does not render textbook selection controls', async () => {
    render(<CatalogScreen bridge={catalogBridge()} initialSnapshot={snapshot} />);

    expect(await screen.findByRole('heading', {name: '语文一年级上册'})).toBeVisible();
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', {name: /下载所选/})).not.toBeInTheDocument();
  });

  it('syncs an empty catalog only once during startup', async () => {
    const bridge = catalogBridge();
    const emptySnapshot: CatalogSnapshot = {...snapshot, version: 0, textbooks: []};
    render(<CatalogScreen bridge={bridge} initialSnapshot={emptySnapshot} />);

    expect(await screen.findByRole('heading', {name: '语文一年级上册'})).toBeVisible();
    await new Promise((resolve) => window.setTimeout(resolve, 50));

    expect(bridge.syncCatalog).toHaveBeenCalledTimes(1);
  });

  it('clears invalid child filters when the school stage changes', async () => {
    const user = userEvent.setup();
    render(<CatalogScreen bridge={catalogBridge()} initialSnapshot={snapshot} />);

    await user.click(screen.getByRole('radio', {name: '小学'}));
    await user.click(screen.getByRole('radio', {name: '一年级'}));
    await user.click(screen.getByRole('radio', {name: '高中'}));

    expect(screen.queryByRole('radio', {name: '一年级'})).not.toBeInTheDocument();
    expect(screen.getByRole('radio', {name: '高一'})).toBeChecked();
  });

  it('does not expose selection or batch commands after filtering', async () => {
    const user = userEvent.setup();
    render(<CatalogScreen bridge={catalogBridge()} initialSnapshot={snapshot} />);

    await user.click(screen.getByRole('radio', {name: '高中'}));

    await waitFor(() => expect(screen.queryByRole('checkbox')).not.toBeInTheDocument());
    expect(screen.queryByRole('button', {name: /下载所选/})).not.toBeInTheDocument();
  });

  it('queues a single textbook from its row action', async () => {
    const user = userEvent.setup();
    const bridge = catalogBridge();
    render(<CatalogScreen bridge={bridge} initialSnapshot={snapshot} />);

    await user.click(await screen.findByRole('button', {name: '下载 语文一年级上册'}));

    expect(bridge.enqueueDownloads).toHaveBeenCalledWith(['book-1']);
  });

  it('keeps the active filters when refreshing the catalog', async () => {
    const user = userEvent.setup();
    const bridge = catalogBridge();
    render(<CatalogScreen bridge={bridge} initialSnapshot={snapshot} />);

    await user.click(screen.getByRole('radio', {name: '高中'}));
    await waitFor(() => expect(screen.queryByRole('heading', {name: '语文一年级上册'})).not.toBeInTheDocument());
    vi.mocked(bridge.queryCatalog).mockClear();

    await user.click(screen.getByRole('button', {name: '刷新教材目录'}));

    await waitFor(() => expect(bridge.queryCatalog).toHaveBeenCalledWith({
      search: '', stageId: 'high', subjectId: 'chinese', editionId: 'pep', gradeId: 'grade-10', volumeId: '',
    }));
    expect(screen.getByRole('heading', {name: '高中语文必修一'})).toBeVisible();
    expect(screen.queryByRole('heading', {name: '语文一年级上册'})).not.toBeInTheDocument();
  });

  it('uses a refreshed snapshot after filters change during synchronization', async () => {
    const user = userEvent.setup();
    const bridge = catalogBridge();
    let resolveSync!: (value: CatalogSnapshot) => void;
    vi.mocked(bridge.syncCatalog).mockImplementation(() => new Promise((resolve) => { resolveSync = resolve; }));
    const updated: CatalogSnapshot = {...snapshot, version: 2, textbooks: [books[2]], filters: {...snapshot.filters, stages: [{id: 'high', name: '高中'}]}};
    render(<CatalogScreen bridge={bridge} initialSnapshot={snapshot} />);

    await user.click(screen.getByRole('button', {name: '刷新教材目录'}));
    await user.click(screen.getByRole('radio', {name: '高中'}));
    resolveSync(updated);

    await waitFor(() => expect(screen.queryByRole('radio', {name: '小学'})).not.toBeInTheDocument());
    expect(screen.getByRole('heading', {name: '高中语文必修一'})).toBeVisible();
  });
});
