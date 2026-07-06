import type { FetcherTaskSummaryResponse, JsonRecord } from "./types";

export type FetcherTask = {
  id: string;
  type: string;
  target: string | null;
  novelId: string | null;
  novelIds: string[];
  novelTitle: string | null;
  novelAuthor: string | null;
  status: string;
  message: string | null;
  warnings: string[];
  errorMessage: string | null;
  createdAt: string | null;
  startedAt: string | null;
  completedAt: string | null;
  elapsedTime: number | null;
  progress: number | null;
  totalSteps: number | null;
  currentStep: number | null;
  savedEpisodeCount: number | null;
  failedEpisodeId: string | null;
  resumeEpisodeId: string | null;
};

export type FetcherTaskSummary = {
  current: FetcherTask | null;
  queued: FetcherTask[];
  recentCompleted: FetcherTask[];
  recentFailed: FetcherTask[];
  completedCount: number;
  failedCount: number;
  convertCurrent: FetcherTask | null;
  convertQueued: FetcherTask[];
  available?: boolean;
  degraded?: boolean;
};

function isRecord(value: unknown): value is JsonRecord {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function normalizeString(value: unknown): string | null {
  return typeof value === "string" && value.trim().length > 0 ? value.trim() : null;
}

function pickTextValue(record: JsonRecord, keys: string[]): string | null {
  for (const key of keys) {
    const normalized = normalizeTextValue(record[key]);
    if (normalized !== null) {
      return normalized;
    }
  }

  return null;
}

function pickFirstTextFromArray(value: unknown): string | null {
  if (!Array.isArray(value)) {
    return null;
  }

  for (const entry of value) {
    const normalized = normalizeTextValue(entry);
    if (normalized !== null) {
      return normalized;
    }
  }

  return null;
}

function normalizeTextArray(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }

  return value.map((entry) => normalizeTextValue(entry)).filter((entry): entry is string => entry !== null);
}

function normalizeTextValue(value: unknown): string | null {
  const normalizedString = normalizeString(value);
  if (normalizedString !== null) {
    return normalizedString;
  }

  if (typeof value === "number" && Number.isFinite(value)) {
    return String(value);
  }

  return null;
}

function normalizeNumber(value: unknown): number | null {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }

  if (typeof value === "string" && value.trim().length > 0) {
    const parsed = Number.parseFloat(value);
    return Number.isFinite(parsed) ? parsed : null;
  }

  return null;
}

function normalizeInteger(value: unknown): number | null {
  const normalized = normalizeNumber(value);
  if (normalized === null || normalized < 0) {
    return null;
  }

  return Math.floor(normalized);
}

function clampPercentage(value: number): number {
  return Math.min(100, Math.max(0, value));
}

function pickNumberValue(record: JsonRecord, keys: string[]): number | null {
  for (const key of keys) {
    const normalized = normalizeNumber(record[key]);
    if (normalized !== null) {
      return normalized;
    }
  }

  return null;
}

function pickIntegerValue(record: JsonRecord, keys: string[]): number | null {
  for (const key of keys) {
    const normalized = normalizeInteger(record[key]);
    if (normalized !== null) {
      return normalized;
    }
  }

  return null;
}

function normalizeTask(task: unknown): FetcherTask | null {
  if (!isRecord(task)) {
    return null;
  }

  const type = pickTextValue(task, ["type"]) ?? "task";
  const target = pickTextValue(task, ["target", "url", "sourceUrl"]) ?? pickFirstTextFromArray(task.targets);
  const novelId = pickTextValue(task, ["novelId"]);
  const novelIds = normalizeTextArray(task.novelIds);
  const novelTitle = pickTextValue(task, ["novelTitle", "title"]);
  const novelAuthor = pickTextValue(task, ["novelAuthor", "author"]);
  const status = pickTextValue(task, ["status", "state"]) ?? "unknown";
  const message = pickTextValue(task, ["message", "detail"]);
  const warnings = normalizeTextArray(task.warnings);
  const errorMessage = pickTextValue(task, ["error", "errorMessage", "reason"]);
  const createdAt = pickTextValue(task, ["createdAt"]);
  const startedAt = pickTextValue(task, ["startedAt"]);
  const completedAt = pickTextValue(task, ["completedAt", "finishedAt"]);
  const rawProgress = pickNumberValue(task, ["progress"]);
  const totalSteps = pickIntegerValue(task, ["totalSteps"]);
  const currentStep = pickIntegerValue(task, ["currentStep"]);
  const derivedProgress =
    rawProgress !== null
      ? clampPercentage(rawProgress)
      : totalSteps && totalSteps > 0 && currentStep !== null
        ? clampPercentage((currentStep / totalSteps) * 100)
        : null;
  const id =
    pickTextValue(task, ["id", "taskId"]) ??
    [
      type,
      novelId ?? novelTitle ?? target ?? "unknown",
      createdAt ?? startedAt ?? completedAt ?? message ?? errorMessage ?? "0"
    ].join(":");

  return {
    id,
    type,
    target,
    novelId,
    novelIds,
    novelTitle,
    novelAuthor,
    status,
    message,
    warnings,
    errorMessage,
    createdAt,
    startedAt,
    completedAt,
    elapsedTime: pickNumberValue(task, ["elapsedTime"]),
    progress: derivedProgress,
    totalSteps,
    currentStep,
    savedEpisodeCount: pickIntegerValue(task, ["savedEpisodeCount"]),
    failedEpisodeId: pickTextValue(task, ["failedEpisodeId"]),
    resumeEpisodeId: pickTextValue(task, ["resumeEpisodeId"]),
  };
}

function normalizeTaskArray(tasks: unknown): FetcherTask[] {
  if (!Array.isArray(tasks)) {
    return [];
  }

  return tasks.map((task) => normalizeTask(task)).filter((task): task is FetcherTask => task !== null);
}

export function toFetcherTaskSummary(value: FetcherTaskSummaryResponse): FetcherTaskSummary {
  return {
    current: normalizeTask(value.current),
    queued: normalizeTaskArray(value.queued),
    recentCompleted: normalizeTaskArray(value.recentCompleted),
    recentFailed: normalizeTaskArray(value.recentFailed),
    completedCount: normalizeInteger(value.completedCount) ?? 0,
    failedCount: normalizeInteger(value.failedCount) ?? 0,
    convertCurrent: normalizeTask(value.convertCurrent),
    convertQueued: normalizeTaskArray(value.convertQueued),
    available: value.available,
    degraded: value.degraded,
  };
}

export function formatFetcherTask(task: FetcherTask | null): string {
  if (!task) {
    return "タスクなし";
  }

  return [
    getFetcherTaskTypeLabel(task.type),
    getFetcherTaskStatusLabel(task.status),
    getFetcherTaskTargetLabel(task) ?? task.message,
  ]
    .filter((value): value is string => typeof value === "string" && value.length > 0)
    .join(" / ");
}

export function getFetcherTaskTargetLabel(task: FetcherTask): string | null {
  return task.novelTitle ?? task.novelId ?? task.target;
}

export function formatFetcherTaskFailure(task: FetcherTask): string {
  return task.errorMessage ?? task.message ?? "理由は取得できませんでした。";
}

export function getFetcherActiveTasks(summary: FetcherTaskSummary | null): FetcherTask[] {
  if (!summary) {
    return [];
  }

  return [
    summary.current,
    summary.convertCurrent,
    ...summary.queued,
    ...summary.convertQueued,
  ].filter((task): task is FetcherTask => task !== null);
}

export function getFetcherQueuedTasks(summary: FetcherTaskSummary | null): FetcherTask[] {
  if (!summary) {
    return [];
  }

  return [...summary.queued, ...summary.convertQueued];
}

export function getFetcherBusyFetcherWorkIds(summary: FetcherTaskSummary | null): Set<string> {
  const busyFetcherWorkIds = new Set<string>();

  for (const task of getFetcherActiveTasks(summary)) {
    if (!["download", "resume", "update"].includes(task.type)) {
      continue;
    }

    if (task.novelId) {
      busyFetcherWorkIds.add(task.novelId);
    }
    for (const novelId of task.novelIds) {
      busyFetcherWorkIds.add(novelId);
    }
  }

  return busyFetcherWorkIds;
}

export type FetcherTaskListEntry = {
  key: string;
  task: FetcherTask;
};

function getFetcherTaskListKeyBase(task: FetcherTask): string {
  return [
    task.id,
    task.type,
    task.status,
    task.novelId ?? "",
    task.novelIds.join(","),
    task.novelTitle ?? "",
    task.createdAt ?? "",
    task.startedAt ?? "",
    task.completedAt ?? "",
    task.message ?? "",
    task.warnings.join(","),
  ].join("|");
}

export function getFetcherTaskListEntries(tasks: FetcherTask[]): FetcherTaskListEntry[] {
  const keyCounts = new Map<string, number>();

  return tasks.map((task) => {
    const keyBase = getFetcherTaskListKeyBase(task);
    const duplicateCount = keyCounts.get(keyBase) ?? 0;
    keyCounts.set(keyBase, duplicateCount + 1);

    return {
      key: duplicateCount === 0 ? keyBase : `${keyBase}#${duplicateCount}`,
      task,
    };
  });
}

export function getFetcherActiveTaskEntries(summary: FetcherTaskSummary | null): FetcherTaskListEntry[] {
  return getFetcherTaskListEntries(getFetcherActiveTasks(summary));
}

export function getFetcherTaskTypeLabel(type: string): string {
  switch (type) {
    case "download":
      return "追加";
    case "update":
      return "更新";
    case "convert":
      return "変換";
    case "remove":
      return "削除";
    case "resume":
      return "再開";
    default:
      return type;
  }
}

export function getFetcherTaskStatusLabel(status: string): string {
  switch (status) {
    case "queued":
      return "待機中";
    case "running":
      return "実行中";
    case "completed":
      return "完了";
    case "failed":
      return "失敗";
    case "canceled":
      return "キャンセル";
    default:
      return status;
  }
}

export function getFetcherTaskProgressValue(task: FetcherTask): number {
  if (task.progress !== null) {
    return clampPercentage(task.progress);
  }

  if (task.totalSteps && task.totalSteps > 0 && task.currentStep !== null) {
    return clampPercentage((task.currentStep / task.totalSteps) * 100);
  }

  return task.status === "completed" ? 100 : 0;
}

export function hasFetcherTaskDeterminateProgress(task: FetcherTask): boolean {
  if (task.progress !== null) {
    return true;
  }

  return task.totalSteps !== null && task.totalSteps > 0 && task.currentStep !== null;
}

export function formatFetcherTaskProgress(task: FetcherTask): string {
  if (!hasFetcherTaskDeterminateProgress(task)) {
    return task.status === "running" ? "進行中" : task.status === "queued" ? "待機中" : getFetcherTaskStatusLabel(task.status);
  }

  const progress = getFetcherTaskProgressValue(task);
  return `${progress.toFixed(progress % 1 === 0 ? 0 : 1)}%`;
}

export function formatFetcherTaskStepProgress(task: FetcherTask): string | null {
  if (task.totalSteps === null || task.totalSteps <= 0 || task.currentStep === null) {
    return null;
  }

  const unit = task.type === "download" || task.type === "update" ? "話" : "ステップ";
  return `${task.currentStep} / ${task.totalSteps} ${unit}`;
}
