import { mutateJson, requestJson } from "../../api/http";
import type {
  FetcherDownloadRequest,
  FetcherDownloadResponse,
  FetcherQueueResponse,
  FetcherRemoveRequest,
  FetcherRemoveResponse,
  FetcherResumeRequest,
  FetcherResumeResponse,
  FetcherStatusSnapshot,
  FetcherTaskControlResponse,
  FetcherUpdateRequest,
  FetcherUpdateResponse
} from "./types";

export async function fetchFetcherStatus(): Promise<FetcherStatusSnapshot> {
  const [queueResult, tasksResult] = await Promise.allSettled([
    requestJson<FetcherQueueResponse>("/api/fetcher/queue", undefined, "取得状況の取得に失敗しました。"),
    requestJson<FetcherStatusSnapshot["tasks"]>("/api/fetcher/tasks/summary", undefined, "タスク状態の取得に失敗しました。")
  ]);

  const queue = queueResult.status === "fulfilled" ? queueResult.value : null;
  const tasks = tasksResult.status === "fulfilled" ? tasksResult.value : null;
  const queueError =
    queueResult.status === "rejected"
      ? queueResult.reason instanceof Error
        ? queueResult.reason.message
        : "キュー状態の取得に失敗しました。"
      : null;
  const tasksError =
    tasksResult.status === "rejected"
      ? tasksResult.reason instanceof Error
        ? tasksResult.reason.message
        : "タスク状態の取得に失敗しました。"
      : null;

  return {
    queue,
    tasks,
    error: queueError ?? tasksError,
    didUpdate: queue !== null || tasks !== null
  };
}

export async function downloadFetcherWorks(payload: FetcherDownloadRequest): Promise<FetcherDownloadResponse> {
  return mutateJson<FetcherDownloadResponse, FetcherDownloadRequest>(
    "/api/fetcher/works/download",
    payload,
    "小説ダウンロードの開始に失敗しました。"
  );
}

export async function updateFetcherWorks(payload: FetcherUpdateRequest): Promise<FetcherUpdateResponse> {
  return mutateJson<FetcherUpdateResponse, FetcherUpdateRequest>(
    "/api/fetcher/works/update",
    payload,
    "小説更新の開始に失敗しました。"
  );
}

export async function resumeFetcherWorks(payload: FetcherResumeRequest): Promise<FetcherResumeResponse> {
  return mutateJson<FetcherResumeResponse, FetcherResumeRequest>(
    "/api/fetcher/works/resume",
    payload,
    "小説ダウンロードの再開に失敗しました。"
  );
}

export async function removeFetcherWorks(payload: FetcherRemoveRequest): Promise<FetcherRemoveResponse> {
  return mutateJson<FetcherRemoveResponse, FetcherRemoveRequest>(
    "/api/fetcher/works/remove",
    payload,
    "小説削除に失敗しました。"
  );
}

async function controlFetcherTask(taskId: string, action: "pause" | "resume" | "cancel"): Promise<FetcherTaskControlResponse> {
  const actionLabels = {
    cancel: "中止",
    pause: "一時停止",
    resume: "再開"
  } as const;

  return requestJson<FetcherTaskControlResponse>(
    `/api/fetcher/tasks/${encodeURIComponent(taskId)}/${action}`,
    {
      method: "POST"
    },
    `タスクの${actionLabels[action]}に失敗しました。`
  );
}

export function pauseFetcherTask(taskId: string): Promise<FetcherTaskControlResponse> {
  return controlFetcherTask(taskId, "pause");
}

export function resumeFetcherTask(taskId: string): Promise<FetcherTaskControlResponse> {
  return controlFetcherTask(taskId, "resume");
}

export function cancelFetcherTask(taskId: string): Promise<FetcherTaskControlResponse> {
  return controlFetcherTask(taskId, "cancel");
}
