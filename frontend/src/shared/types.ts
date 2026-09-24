export interface SessionStatus {
  authenticated: boolean;
}

export type DuplicatePolicy = 'ask' | 'rename' | 'overwrite' | 'skip';

export interface AppConfig {
  dataRoot: string;
  downloadDir: string;
  concurrentDownloads: number;
  duplicatePolicy: DuplicatePolicy;
}

export interface TagOption {
  id: string;
  name: string;
}

export interface CatalogFilters {
  stages: TagOption[];
  subjects: TagOption[];
  editions: TagOption[];
  grades: TagOption[];
  volumes: TagOption[];
}

export interface Textbook {
  id: string;
  title: string;
  provider: string;
  thumbnailUrl?: string;
  size: number;
  stageId?: string;
  stage?: string;
  subjectId?: string;
  subject?: string;
  editionId?: string;
  edition?: string;
  gradeId?: string;
  grade?: string;
  volumeId?: string;
  volume?: string;
}

export interface CatalogSnapshot {
  version: number;
  syncedAt?: string;
  textbooks: Textbook[];
  filters: CatalogFilters;
  sourceTags?: unknown;
}

export interface CatalogQuery {
  search: string;
  stageId: string;
  subjectId: string;
  editionId: string;
  gradeId: string;
  volumeId: string;
}

export type DownloadStatus = 'queued' | 'resolving' | 'downloading' | 'paused' | 'waiting_auth' | 'completed' | 'failed' | 'canceled';

export interface DownloadTask {
  id: string;
  bookId: string;
  filename: string;
  status: DownloadStatus;
  bytesDone: number;
  totalBytes: number;
  speedBytes: number;
  etaSeconds: number;
  mirrorAttempt: number;
  error?: string;
}

export interface TaskRecord {
  id: string;
  bookId: string;
  filename: string;
  status: string;
  partialBytes: number;
  expectedSize: number;
}

export interface PathState {
  dataRoot: string;
  downloadDir: string;
  cacheDir: string;
  logsDir: string;
  webviewDir: string;
}

export interface BootstrapState {
  session: SessionStatus;
  catalog: CatalogSnapshot;
  config: AppConfig;
  tasks: DownloadTask[];
  recoverable: TaskRecord[];
  paths: PathState;
}

export interface Bridge {
  bootstrap(): Promise<BootstrapState>;
  detectBrowserSession(): Promise<SessionStatus>;
  openBrowserLogin(): Promise<void>;
  submitToken(token: string): Promise<SessionStatus>;
  logout(): Promise<void>;
  syncCatalog(): Promise<CatalogSnapshot>;
  queryCatalog(query: CatalogQuery): Promise<Textbook[]>;
  enqueueDownloads(ids: string[]): Promise<DownloadTask[]>;
  pauseDownload(id: string): Promise<void>;
  resumeDownload(id: string): Promise<void>;
  cancelDownload(id: string, removePartial: boolean): Promise<void>;
  retryDownload(id: string): Promise<void>;
  updateSettings(config: AppConfig): Promise<AppConfig>;
  clearCache(): Promise<void>;
  clearIncomplete(): Promise<void>;
  exportDiagnostics(destination: string): Promise<string>;
  chooseDownloadDirectory?(current: string): Promise<string>;
  openDownloadFolder?(): Promise<void>;
  shutdown(): Promise<void>;
  onDownloadUpdate(listener: (task: DownloadTask) => void): () => void;
  onCloseRequested?(listener: () => void): () => void;
  onSessionExpired?(listener: () => void): () => void;
}

export const emptyFilters: CatalogFilters = {stages: [], subjects: [], editions: [], grades: [], volumes: []};
export const emptyCatalog: CatalogSnapshot = {version: 0, textbooks: [], filters: emptyFilters};
