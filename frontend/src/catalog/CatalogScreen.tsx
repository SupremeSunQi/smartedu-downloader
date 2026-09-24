import {Download, RefreshCw, Settings} from 'lucide-react';
import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import type {Bridge, CatalogQuery, CatalogSnapshot, Textbook} from '../shared/types';
import {deriveFilters, filterCachedBooks, selectAvailableQuery, updateFilter} from './catalogState';
import {CatalogFilterRows, FilterPane} from './FilterPane';
import {TextbookList} from './TextbookList';

interface CatalogScreenProps {
  bridge: Bridge;
  initialSnapshot: CatalogSnapshot;
  activeTaskCount?: number;
  onOpenSettings?(): void;
  onOpenDownloads?(): void;
  onTasksAdded?(tasks: Awaited<ReturnType<Bridge['enqueueDownloads']>>): void;
}

export function CatalogScreen({bridge, initialSnapshot, activeTaskCount = 0, onOpenSettings, onOpenDownloads, onTasksAdded}: CatalogScreenProps) {
  const [snapshot, setSnapshot] = useState(initialSnapshot);
  const [query, setQuery] = useState<CatalogQuery>(() => selectAvailableQuery(initialSnapshot.textbooks ?? []));
  const [books, setBooks] = useState<Textbook[]>(() => filterCachedBooks(initialSnapshot.textbooks ?? [], query));
  const [loading, setLoading] = useState(initialSnapshot.version === 0);
  const [syncing, setSyncing] = useState(false);
  const [busyBookId, setBusyBookId] = useState('');
  const [error, setError] = useState('');
  const [stale, setStale] = useState(false);
  const initialSyncStarted = useRef(false);
  const queryRef = useRef(query);
  const queryGeneration = useRef(0);
  const syncGeneration = useRef(0);
  queryRef.current = query;

  const filters = useMemo(() => deriveFilters(snapshot.textbooks ?? [], query), [query, snapshot.textbooks]);

  const refresh = useCallback(async () => {
    const refreshGeneration = ++syncGeneration.current;
    setSyncing(true);
    setError('');
    try {
      const next = await bridge.syncCatalog();
      if (refreshGeneration !== syncGeneration.current) return;
      setSnapshot(next);
      const activeQuery = selectAvailableQuery(next.textbooks ?? [], queryRef.current);
      setQuery(activeQuery);
      setBooks(filterCachedBooks(next.textbooks ?? [], activeQuery));
      const queryGenerationAfterRefresh = ++queryGeneration.current;
      setLoading(true);
      const filtered = await bridge.queryCatalog(activeQuery);
      if (queryGenerationAfterRefresh !== queryGeneration.current) return;
      setBooks(filtered);
      setError('');
      setStale(false);
    } catch {
      if (snapshot.version > 0) setStale(true);
      else setError('教材目录同步失败，请检查网络后重试');
    } finally {
      setSyncing(false);
      setLoading(false);
    }
  }, [bridge, snapshot.version]);

  useEffect(() => {
    if (initialSnapshot.version === 0 && !initialSyncStarted.current) {
      initialSyncStarted.current = true;
      void refresh();
    }
  }, [initialSnapshot.version, refresh]);

  useEffect(() => {
    if (snapshot.version === 0) return;
    let active = true;
    const currentGeneration = ++queryGeneration.current;
    const timer = window.setTimeout(() => {
      setLoading(true);
      bridge.queryCatalog(query).then((result) => {
        if (active && currentGeneration === queryGeneration.current) {
          setBooks(result);
          setError('');
        }
      }).catch(() => {
        if (active && currentGeneration === queryGeneration.current) setError('无法查询教材，请稍后重试');
      }).finally(() => {
        if (active && currentGeneration === queryGeneration.current) setLoading(false);
      });
    }, 200);
    return () => {
      active = false;
      window.clearTimeout(timer);
    };
  }, [bridge, query, snapshot.version]);

  const changeFilter = (field: keyof Omit<CatalogQuery, 'search'>, value: string) => {
    const next = selectAvailableQuery(snapshot.textbooks ?? [], updateFilter(queryRef.current, field, value));
    queryRef.current = next;
    setQuery(next);
    setBooks(filterCachedBooks(snapshot.textbooks ?? [], next));
  };

  const enqueue = async (ids: string[], rowId = '') => {
    if (ids.length === 0) return;
    setError('');
    setBusyBookId(rowId);
    try {
      const created = await bridge.enqueueDownloads(ids);
      onTasksAdded?.(created);
    } catch {
      setError('加入下载队列失败，请重新登录或稍后重试');
    } finally {
      setBusyBookId('');
    }
  };

  const hasFilters = Boolean(query.search || query.stageId || query.subjectId || query.editionId || query.gradeId || query.volumeId);

  return (
    <main className="catalog-workspace" aria-label="教材列表">
      <header className="app-header">
        <div className="app-identity"><span><img className="app-logo" src="/appicon.png" alt="智教教材图标" /></span><div><strong>智教教材</strong><small>教材下载器</small></div></div>
        <div className="header-actions">
          <button type="button" className="icon-command" aria-label="刷新教材目录" title="刷新教材目录" disabled={syncing} onClick={() => void refresh()}><RefreshCw className={syncing ? 'spin' : ''} size={19} /></button>
          <button type="button" className="icon-command download-toggle" aria-label="查看下载任务" title="下载任务" onClick={onOpenDownloads}><Download size={19} />{activeTaskCount > 0 && <span aria-hidden="true" className="task-counter">{activeTaskCount}</span>}</button>
          <button type="button" className="icon-command" aria-label="打开设置" title="设置" onClick={onOpenSettings}><Settings size={19} /></button>
        </div>
      </header>

      {stale && <div className="status-banner" role="status">目录刷新失败，正在显示本地缓存。<button type="button" onClick={() => void refresh()}>重试</button></div>}
      {error && <div className="status-banner is-error" role="alert">{error}<button type="button" onClick={() => void refresh()}>重试</button></div>}

      <div className="catalog-layout">
        <FilterPane filters={filters} query={query} onChange={changeFilter} />
        <section className="catalog-results" aria-label="教材结果">
          <div className="catalog-query-area">
            <CatalogFilterRows filters={filters} query={query} onChange={changeFilter} />
          </div>
          <TextbookList
            books={books}
            loading={loading}
            busyBookId={busyBookId}
            hasFilters={hasFilters}
            onDownload={(id) => void enqueue([id], id)}
          />
        </section>
      </div>
    </main>
  );
}
