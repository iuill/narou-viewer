export type JsonRecord = Record<string, unknown>;

export type FetcherQueueResponse = {
  total: number;
  queued?: number;
  webWorker: number;
  worker: number;
  running: boolean;
  paused?: number;
  interrupted?: number;
  available?: boolean;
  degraded?: boolean;
};

export type FetcherTaskSummaryResponse = {
  current?: JsonRecord | null;
  queued?: JsonRecord[];
  paused?: JsonRecord[];
  interrupted?: JsonRecord[];
  recentCompleted?: JsonRecord[];
  recentFailed?: JsonRecord[];
  completedCount?: number;
  failedCount?: number;
  canceledCount?: number;
  pausedCount?: number;
  interruptedCount?: number;
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

export type FetcherTaskControlResponse = {
  taskId?: string;
  status?: string;
  requestedAction?: string;
  changed?: boolean;
  cancelled?: boolean;
  message?: string;
};

export type FetcherCancelTaskResponse = FetcherTaskControlResponse;
