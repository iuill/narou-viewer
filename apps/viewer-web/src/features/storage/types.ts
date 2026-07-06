export type StorageUsageCategoryId = "novelData" | "cache" | "other";

export type StorageUsageCategory = {
  id: StorageUsageCategoryId;
  label: string;
  bytes: number;
  fileCount: number;
};

export type NovelStorageUsage = {
  novelId: string;
  title: string;
  author?: string;
  siteName: string;
  source: string;
  totalBytes: number;
  novelDataBytes: number;
  cacheBytes: number;
  otherBytes: number;
  fileCount: number;
};

export type StorageUsageResponse = {
  checkedAt: string;
  totalBytes: number;
  categories: StorageUsageCategory[];
  novels: NovelStorageUsage[];
  warnings?: string[];
};

export type StorageUsageProgressPhase = "preparing" | "scanning" | "completed";

export type StorageUsageProgressState = "idle" | "running" | "completed" | "error";

export type StorageUsageProgressResponse = {
  requestId?: string;
  state: StorageUsageProgressState;
  phase: StorageUsageProgressPhase;
  checkedNovels: number;
  totalNovels: number;
  startedAt?: string;
  updatedAt?: string;
  error?: string;
};
