import { normalizeEpisodeIndex } from "./episodeIndex";
import {
  apiFetch,
  createApiResponseErrorFromResponse,
  mutateJson,
  parseApiResponseBody,
  readApiResponse,
  requestJson
} from "../../api/http";
import { createApiResponseError } from "../../api/errors";
import type {
  Bookmark,
  BookmarksResponse,
  EpisodeIndex,
  EpisodeResponse,
  NovelReaderCorrectionPatch,
  NovelReaderSettingsResponse,
  ReaderAiAssistantChatRequest,
  ReaderAiAssistantStreamEvent,
  ReaderPreferencesResponse,
  ReaderState,
  TocResponse
} from "./types";

type ReaderStatePutRequest = {
  novelId: string;
  lastReadEpisodeIndex: EpisodeIndex | null;
  position: number;
  scroll: null;
  clientId: string;
  expectedStateVersion: number;
};

type BookmarkCreateRequest = {
  novelId: string;
  episodeIndex: EpisodeIndex;
  position: number;
  label: string | null;
};

export class ReaderStateConflictError extends Error {
  constructor(readonly serverState: ReaderState) {
    super("別端末で最終既読が更新されています。");
    this.name = "ReaderStateConflictError";
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isString(value: unknown): value is string {
  return typeof value === "string";
}

function isReaderStateConflictPayload(value: unknown): boolean {
  return (
    isRecord(value) &&
    typeof value.novelId === "string" &&
    typeof value.stateVersion === "number" &&
    Number.isInteger(value.stateVersion) &&
    value.stateVersion >= 0 &&
    !("error" in value) &&
    !("code" in value)
  );
}

export function normalizeReaderStateResponse(value: unknown, fallbackNovelId = ""): ReaderState {
  if (!isRecord(value)) {
    return {
      novelId: fallbackNovelId,
      lastReadEpisodeIndex: null,
      position: 0,
      updatedAt: null,
      stateVersion: 0,
      updatedByClientId: null
    };
  }

  return {
    novelId: typeof value.novelId === "string" ? value.novelId : fallbackNovelId,
    lastReadEpisodeIndex: normalizeEpisodeIndex(value.lastReadEpisodeIndex),
    position: typeof value.position === "number" && Number.isInteger(value.position) && value.position >= 0 ? value.position : 0,
    updatedAt: typeof value.updatedAt === "string" ? value.updatedAt : null,
    stateVersion:
      typeof value.stateVersion === "number" && Number.isInteger(value.stateVersion) && value.stateVersion >= 0
        ? value.stateVersion
        : 0,
    updatedByClientId:
      typeof value.updatedByClientId === "string" && value.updatedByClientId.trim().length > 0 ? value.updatedByClientId.trim() : null
  };
}

export async function fetchReaderPreferences(): Promise<ReaderPreferencesResponse> {
  return requestJson<ReaderPreferencesResponse>("/api/reader/preferences", undefined, "読書設定の取得に失敗しました。");
}

export async function putReaderPreferences(preferences: {
  readingMode: ReaderPreferencesResponse["readingMode"];
  fontFamily: ReaderPreferencesResponse["fontFamily"];
  theme: ReaderPreferencesResponse["theme"];
}): Promise<ReaderPreferencesResponse> {
  return requestJson<ReaderPreferencesResponse>(
    "/api/reader/preferences",
    {
      method: "PUT",
      headers: {
        "content-type": "application/json"
      },
      body: JSON.stringify(preferences)
    },
    "読書設定の保存に失敗しました。"
  );
}

export async function fetchNovelContext(novelId: string): Promise<{
  toc: TocResponse;
  readerState: ReaderState;
  bookmarks: Bookmark[];
  readerSettings: NovelReaderSettingsResponse;
}> {
  const encodedNovelId = encodeURIComponent(novelId);
  const [toc, readerStateRaw, bookmarksResponse, readerSettings] = await Promise.all([
    requestJson<TocResponse>(`/api/library/novels/${encodedNovelId}/toc`, undefined, "作品データの取得に失敗しました。"),
    requestJson<unknown>(`/api/reader/state?novelId=${encodedNovelId}`, undefined, "作品データの取得に失敗しました。"),
    requestJson<BookmarksResponse>(`/api/bookmarks?novelId=${encodedNovelId}`, undefined, "作品データの取得に失敗しました。"),
    requestJson<NovelReaderSettingsResponse>(
      `/api/library/novels/${encodedNovelId}/reader-settings`,
      undefined,
      "作品データの取得に失敗しました。"
    )
  ]);

  return {
    toc,
    readerState: normalizeReaderStateResponse(readerStateRaw, novelId),
    bookmarks: bookmarksResponse.bookmarks,
    readerSettings
  };
}

export async function fetchEpisode(novelId: string, episodeIndex: EpisodeIndex): Promise<EpisodeResponse> {
  return requestJson<EpisodeResponse>(
    `/api/library/novels/${encodeURIComponent(novelId)}/episodes/${episodeIndex}`,
    undefined,
    "本文の取得に失敗しました。"
  );
}

export async function putNovelReaderSettings(
  novelId: string,
  settings: { correction: NovelReaderCorrectionPatch }
): Promise<NovelReaderSettingsResponse> {
  return requestJson<NovelReaderSettingsResponse>(
    `/api/library/novels/${encodeURIComponent(novelId)}/reader-settings`,
    {
      method: "PUT",
      headers: {
        "content-type": "application/json"
      },
      body: JSON.stringify(settings)
    },
    "作品ごとの読書設定の保存に失敗しました。"
  );
}

export async function putReaderState(payload: ReaderStatePutRequest, signal?: AbortSignal): Promise<ReaderState> {
  const response = await apiFetch("/api/reader/state", {
    method: "PUT",
    headers: {
      "content-type": "application/json"
    },
    signal,
    body: JSON.stringify(payload)
  });
  const bodyText = await response.text();
  const contentType = response.headers.get("content-type") ?? "";
  let responsePayload: unknown;
  try {
    responsePayload = parseApiResponseBody(bodyText, contentType, "既読位置の保存に失敗しました。");
  } catch {
    responsePayload = undefined;
  }
  if (!response.ok) {
    if (response.status === 409 && isReaderStateConflictPayload(responsePayload)) {
      throw new ReaderStateConflictError(normalizeReaderStateResponse(responsePayload, payload.novelId));
    }
    throw createApiResponseError(
      response,
      responsePayload,
      contentType.toLowerCase().includes("json") ? "" : bodyText,
      "既読位置の保存に失敗しました。"
    );
  }

  return normalizeReaderStateResponse(responsePayload, payload.novelId);
}

export async function fetchReaderState(novelId: string): Promise<ReaderState | null> {
  const response = await apiFetch(`/api/reader/state?novelId=${encodeURIComponent(novelId)}`);
  if (!response.ok) {
    return null;
  }
  return normalizeReaderStateResponse(await response.json(), novelId);
}

export async function createBookmark(payload: BookmarkCreateRequest): Promise<Bookmark> {
  return mutateJson<Bookmark, BookmarkCreateRequest>("/api/bookmarks", payload, "栞の保存に失敗しました。");
}

export async function deleteBookmark(bookmarkId: string): Promise<void> {
  const response = await apiFetch(`/api/bookmarks/${bookmarkId}`, {
    method: "DELETE"
  });
  await readApiResponse<void>(response, "栞の削除に失敗しました。");
}

export async function postReaderAiAssistantChatStream(
  novelId: string,
  payload: ReaderAiAssistantChatRequest,
  signal?: AbortSignal
): Promise<Response> {
  const response = await apiFetch(`/api/library/novels/${encodeURIComponent(novelId)}/reader-assistant/chat/stream`, {
    method: "POST",
    headers: {
      "content-type": "application/json"
    },
    body: JSON.stringify(payload),
    signal
  });

  if (!response.ok) {
    throw await createApiResponseErrorFromResponse(response, "読書AIの応答取得に失敗しました。");
  }

  return response;
}

export async function readReaderAiAssistantStream(
  response: Response,
  onEvent: (event: ReaderAiAssistantStreamEvent) => void
): Promise<void> {
  await readNdjsonStream(
    response,
    isReaderAiAssistantStreamEvent,
    onEvent,
    "読書AIの応答ストリームを開始できませんでした。"
  );
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

function isReaderAiAssistantStreamEvent(event: unknown): event is ReaderAiAssistantStreamEvent {
  if (!isRecord(event) || !isString(event.type)) {
    return false;
  }
  switch (event.type) {
    case "status":
      return isString(event.message);
    case "tool_call":
    case "tool_result":
      return isString(event.toolName) && isString(event.message);
    case "result":
      return isRecord(event.response);
    case "error":
      return isString(event.error);
    default:
      return false;
  }
}
