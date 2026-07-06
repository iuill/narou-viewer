export type JsonRecord = Record<string, unknown>;

export type FetcherQueueResponse = {
  total: number;
  webWorker: number;
  worker: number;
  running: boolean;
  available?: boolean;
  degraded?: boolean;
};

export type FetcherTaskSummaryResponse = {
  current?: JsonRecord | null;
  queued?: JsonRecord[];
  recentCompleted?: JsonRecord[];
  recentFailed?: JsonRecord[];
  completedCount?: number;
  failedCount?: number;
  convertCurrent?: JsonRecord | null;
  convertQueued?: JsonRecord[];
  available?: boolean;
  degraded?: boolean;
};

export type FetcherStatusSnapshot = {
  queue: FetcherQueueResponse | null;
  tasks: FetcherTaskSummaryResponse | null;
  error: string | null;
  didUpdate: boolean;
};

export type FetcherDownloadRequest = {
  targets: string[];
  force: boolean;
  convertAfterDownload: boolean;
  mail: boolean;
};

export type FetcherDownloadResponse = FetcherDownloadRequest & {
  taskIds: string[];
  message: string;
};

export type FetcherUpdateRequest = {
  novelIds: string[];
  forceRedownload: boolean;
  includeFrozen: boolean;
  convertAfterUpdate: boolean;
  skipUnchanged: boolean;
};

export type FetcherUpdateResponse = {
  taskIds: string[];
  message: string;
};

export type FetcherResumeRequest = {
  novelIds: string[];
};

export type FetcherResumeResponse = {
  taskIds: string[];
  message: string;
};

export type FetcherRemoveRequest = {
  novelIds: string[];
  withFiles: boolean;
};

export type FetcherRemoveResponse = {
  novelIds: string[];
  message: string;
};

export type FetcherCancelTaskResponse = {
  message?: string;
};
