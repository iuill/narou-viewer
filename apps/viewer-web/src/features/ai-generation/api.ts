import { formatAiGenerationPlaygroundError } from "./model";
import { formatApiResponseError } from "../../api/errors";
import { apiFetch, createApiResponseErrorFromResponse, requestJson } from "../../api/http";
import type {
  AiGenerationJobsResponse,
  AiGenerationPlaygroundBatchTiming,
  AiGenerationPlaygroundProgress,
  AiGenerationPlaygroundPromptPreview,
  AiGenerationPlaygroundRequest,
  AiGenerationPlaygroundResponse,
  AiGenerationPlaygroundStreamEvent,
  AiGenerationPreferredModeResponse,
  AiGenerationSettingsRequest,
  AiGenerationSettingsResponse,
  AiUsageResponse
} from "./types";
import type { EpisodeIndex } from "../reader/types";

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isString(value: unknown): value is string {
  return typeof value === "string";
}

function isNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

export async function fetchAiGenerationSettings(): Promise<AiGenerationSettingsResponse> {
  return requestJson<AiGenerationSettingsResponse>("/api/ai-generation/settings", undefined, "AI機能設定の取得に失敗しました。");
}

export async function putAiGenerationSettings(payload: AiGenerationSettingsRequest): Promise<AiGenerationSettingsResponse> {
  return requestJson<AiGenerationSettingsResponse>(
    "/api/ai-generation/settings",
    {
      method: "PUT",
      headers: {
        "content-type": "application/json"
      },
      body: JSON.stringify(payload)
    },
    "AI機能設定の保存に失敗しました。"
  );
}

export async function fetchAiGenerationJobs(): Promise<AiGenerationJobsResponse> {
  return requestJson<AiGenerationJobsResponse>("/api/ai-generation/jobs", undefined, "キャラ生成履歴の取得に失敗しました。");
}

export async function fetchAiUsage(): Promise<AiUsageResponse> {
  return requestJson<AiUsageResponse>("/api/ai-generation/usage", undefined, "AI使用量の取得に失敗しました。");
}

export async function fetchAiUsageRunJson(runId: string): Promise<Record<string, unknown>> {
  return requestJson<Record<string, unknown>>(
    `/api/ai-generation/usage/${encodeURIComponent(runId)}`,
    undefined,
    "AI使用量JSONの取得に失敗しました。"
  );
}

export async function putAiGenerationPreferredMode(mode: "llm" | "heuristic"): Promise<AiGenerationPreferredModeResponse> {
  return requestJson<AiGenerationPreferredModeResponse>(
    "/api/ai-generation/settings/preferred-mode",
    {
      method: "PUT",
      headers: {
        "content-type": "application/json"
      },
      body: JSON.stringify({ preferredMode: mode })
    },
    "連携モードの更新に失敗しました。"
  );
}

export async function postAiGenerationPlaygroundStream(payload: AiGenerationPlaygroundRequest): Promise<Response> {
  const response = await apiFetch("/api/ai-generation/playground/character-summary/stream", {
    method: "POST",
    headers: {
      "content-type": "application/json"
    },
    body: JSON.stringify(payload)
  }).catch((error: unknown) => {
    throw new Error(formatAiGenerationPlaygroundError(error instanceof Error ? error.message : "Unknown error"));
  });

  if (!response.ok) {
    const error = await createApiResponseErrorFromResponse(response, "生成テストの実行に失敗しました。");
    throw formatApiResponseError(error, formatAiGenerationPlaygroundError);
  }

  return response;
}

export type AiGenerationPlaygroundStreamHandlers = {
  onProgress?: (progress: AiGenerationPlaygroundProgress) => void;
  onPromptPreview?: (preview: AiGenerationPlaygroundPromptPreview) => void;
  onBatchTimings?: (batchTimings: AiGenerationPlaygroundBatchTiming[]) => void;
};

export async function readAiGenerationPlaygroundStream(
  response: Response,
  handlers: AiGenerationPlaygroundStreamHandlers = {}
): Promise<AiGenerationPlaygroundResponse> {
  if (!response.body) {
    throw new Error("生成テストの進捗ストリームを開始できませんでした。");
  }

  let result: AiGenerationPlaygroundResponse | null = null;
  let batchTimings: AiGenerationPlaygroundBatchTiming[] = [];

  await readNdjsonStream(response, isAiGenerationPlaygroundStreamEvent, (streamEvent) => {
    if (streamEvent.type === "status") {
      handlers.onProgress?.({
        stage: streamEvent.stage,
        message: streamEvent.message,
        progress: streamEvent.progress,
        step: streamEvent.step,
        stepCount: streamEvent.stepCount,
        batchIndex: streamEvent.batchIndex,
        batchCount: streamEvent.batchCount
      });
      return;
    }

    if (streamEvent.type === "promptPreview") {
      handlers.onPromptPreview?.(streamEvent.preview);
      return;
    }

    if (streamEvent.type === "batchTiming") {
      batchTimings = mergeAiGenerationPlaygroundBatchTimings(batchTimings, {
        batchIndex: streamEvent.batchIndex,
        batchCount: streamEvent.batchCount,
        episodeIndexes: streamEvent.episodeIndexes,
        chunkCount: streamEvent.chunkCount,
        elapsedMs: streamEvent.elapsedMs,
        generatedCharacterCount: streamEvent.generatedCharacterCount,
        mergedCharacterCount: streamEvent.mergedCharacterCount,
        message: streamEvent.message
      });
      handlers.onBatchTimings?.(batchTimings);
      return;
    }

    if (streamEvent.type === "result") {
      result = streamEvent.result;
      return;
    }

    const rawError =
      typeof streamEvent.error === "string" && streamEvent.error.trim().length > 0
        ? streamEvent.error
        : "Received AI generation stream error event with invalid or missing 'error' message.";
    throw new Error(formatAiGenerationPlaygroundError(rawError));
  });

  if (!result) {
    throw new Error("生成テストの実行結果を受信できませんでした。");
  }

  return result;
}

function mergeAiGenerationPlaygroundBatchTimings(
  current: AiGenerationPlaygroundBatchTiming[],
  timing: AiGenerationPlaygroundBatchTiming
): AiGenerationPlaygroundBatchTiming[] {
  const next = current.filter((item) => item.batchIndex !== timing.batchIndex);
  next.push(timing);
  return next.sort((left, right) => left.batchIndex - right.batchIndex);
}

async function readNdjsonStream<T>(
  response: Response,
  validate: (event: unknown) => event is T,
  onEvent: (event: T) => void,
  missingBodyMessage = "応答ストリームを開始できませんでした。"
): Promise<void> {
  if (!response.body) {
    throw new Error(missingBodyMessage);
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    buffer += decoder.decode(value ?? new Uint8Array(), { stream: !done });
    const lines = buffer.split("\n");
    buffer = lines.pop() ?? "";

    for (const line of lines) {
      const trimmed = line.trim();
      if (trimmed.length === 0) {
        continue;
      }

      onEvent(validateNdjsonEvent(JSON.parse(trimmed) as unknown, validate));
    }

    if (done) {
      break;
    }
  }

  const trailing = buffer.trim();
  if (trailing.length > 0) {
    onEvent(validateNdjsonEvent(JSON.parse(trailing) as unknown, validate));
  }
}

function validateNdjsonEvent<T>(event: unknown, validate: (event: unknown) => event is T): T {
  if (!validate(event)) {
    throw new Error("応答ストリームに未対応のイベントが含まれています。");
  }
  return event;
}

function isEpisodeIndexArray(value: unknown): value is EpisodeIndex[] {
  return Array.isArray(value) && value.every(isString);
}

function isAiGenerationPlaygroundStreamEvent(event: unknown): event is AiGenerationPlaygroundStreamEvent {
  if (!isRecord(event) || !isString(event.type)) {
    return false;
  }
  switch (event.type) {
    case "status":
      return (
        isString(event.stage) &&
        ["preparing", "loadingEpisodes", "generating", "buildingResponse"].includes(event.stage) &&
        isString(event.message) &&
        isNumber(event.progress) &&
        isNumber(event.step) &&
        isNumber(event.stepCount) &&
        (event.batchIndex === undefined || isNumber(event.batchIndex)) &&
        (event.batchCount === undefined || isNumber(event.batchCount))
      );
    case "promptPreview":
      return isRecord(event.preview) && isString(event.preview.systemPrompt) && Array.isArray(event.preview.batches);
    case "batchTiming":
      return (
        isNumber(event.batchIndex) &&
        isNumber(event.batchCount) &&
        isEpisodeIndexArray(event.episodeIndexes) &&
        isNumber(event.chunkCount) &&
        isNumber(event.elapsedMs) &&
        isNumber(event.generatedCharacterCount) &&
        isNumber(event.mergedCharacterCount) &&
        isString(event.message)
      );
    case "result":
      return isRecord(event.result);
    case "error":
      return isString(event.error);
    default:
      return false;
  }
}
