import { requestJson } from "../../api/http";
import type { StorageUsageProgressResponse, StorageUsageResponse } from "./types";

function storageUsageQuery(requestId?: string): string {
  return requestId ? `?requestId=${encodeURIComponent(requestId)}` : "";
}

export function fetchStorageUsage(requestId?: string): Promise<StorageUsageResponse> {
  return requestJson<StorageUsageResponse>(
    `/api/system/storage${storageUsageQuery(requestId)}`,
    undefined,
    "ストレージ使用量の取得に失敗しました。"
  );
}

export function fetchStorageUsageProgress(requestId?: string): Promise<StorageUsageProgressResponse> {
  return requestJson<StorageUsageProgressResponse>(
    `/api/system/storage/progress${storageUsageQuery(requestId)}`,
    undefined,
    "ストレージ使用量の進捗取得に失敗しました。"
  );
}
