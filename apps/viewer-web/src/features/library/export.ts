import { stringify } from "yaml";
import { requestJson } from "../../api/http";
import { normalizeReaderStateResponse } from "../reader/api";
import type { Bookmark, BookmarksResponse, ReaderState } from "../reader/types";
import type { NovelSummary } from "./types";

const READER_STATE_EXPORT_CONCURRENCY = 6;

export type LibraryExportReadingState = {
  lastReadEpisodeIndex: string | null;
  position: number;
  updatedAt: string | null;
};

export type LibraryExportWarning = {
  novelId: string;
  field: "readingState";
  message: string;
};

export type LibraryExportNovel = {
  novelId: string;
  fetcherWorkId: string;
  title: string;
  author: string;
  siteName: string;
  tocUrl: string | null;
  updatedAt: string | null;
  lastActivityAt: string | null;
  totalEpisodes: number;
  savedEpisodes: number | null;
  fetchStatus: string | null;
  readingState: LibraryExportReadingState | null;
  bookmarks: Bookmark[];
};

export type LibraryExportDocument = {
  formatVersion: 1;
  exportedAt: string;
  novelsCount: number;
  exportWarnings: LibraryExportWarning[];
  novels: LibraryExportNovel[];
};

type ReaderStateExportResult =
  | {
      status: "fulfilled";
      novelId: string;
      readingState: LibraryExportReadingState;
    }
  | {
      status: "rejected";
      novelId: string;
      warning: LibraryExportWarning;
    };

function toExportReadingState(state: ReaderState): LibraryExportReadingState {
  return {
    lastReadEpisodeIndex: state.lastReadEpisodeIndex,
    position: state.position,
    updatedAt: state.updatedAt
  };
}

function toErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Unknown error";
}

function toExportNovel(
  novel: NovelSummary,
  readingState: LibraryExportReadingState | null,
  bookmarks: Bookmark[]
): LibraryExportNovel {
  return {
    novelId: novel.novelId,
    fetcherWorkId: novel.fetcherWorkId,
    title: novel.title,
    author: novel.author,
    siteName: novel.siteName,
    tocUrl: novel.tocUrl,
    updatedAt: novel.updatedAt,
    lastActivityAt: novel.lastActivityAt ?? null,
    totalEpisodes: novel.totalEpisodes,
    savedEpisodes: novel.savedEpisodes ?? null,
    fetchStatus: novel.fetchStatus ?? null,
    readingState,
    bookmarks
  };
}

async function fetchAllBookmarks(): Promise<Bookmark[]> {
  const response = await requestJson<BookmarksResponse>("/api/bookmarks", undefined, "栞情報の取得に失敗しました。");
  return response.bookmarks;
}

async function fetchExportReaderState(novelId: string): Promise<ReaderState> {
  const response = await requestJson<unknown>(
    `/api/reader/state?novelId=${encodeURIComponent(novelId)}`,
    undefined,
    "既読情報の取得に失敗しました。"
  );
  return normalizeReaderStateResponse(response, novelId);
}

async function mapWithConcurrency<TInput, TResult>(
  inputs: TInput[],
  concurrency: number,
  mapper: (input: TInput, index: number) => Promise<TResult>
): Promise<TResult[]> {
  const results = new Array<TResult>(inputs.length);
  let nextIndex = 0;
  const workerCount = Math.max(1, Math.min(concurrency, inputs.length));

  async function runWorker(): Promise<void> {
    while (nextIndex < inputs.length) {
      const currentIndex = nextIndex;
      nextIndex += 1;
      results[currentIndex] = await mapper(inputs[currentIndex], currentIndex);
    }
  }

  await Promise.all(Array.from({ length: workerCount }, () => runWorker()));
  return results;
}

async function fetchExportReaderStates(novels: NovelSummary[]): Promise<ReaderStateExportResult[]> {
  return mapWithConcurrency(novels, READER_STATE_EXPORT_CONCURRENCY, async (novel) => {
    try {
      return {
        status: "fulfilled",
        novelId: novel.novelId,
        readingState: toExportReadingState(await fetchExportReaderState(novel.novelId))
      };
    } catch (error) {
      return {
        status: "rejected",
        novelId: novel.novelId,
        warning: {
          novelId: novel.novelId,
          field: "readingState",
          message: toErrorMessage(error)
        }
      };
    }
  });
}

export async function buildLibraryExportDocument(
  novels: NovelSummary[],
  exportedAt = new Date().toISOString()
): Promise<LibraryExportDocument> {
  const [readerStates, bookmarks] = await Promise.all([
    fetchExportReaderStates(novels),
    fetchAllBookmarks()
  ]);
  const bookmarksByNovelId = new Map<string, Bookmark[]>();
  const readingStatesByNovelId = new Map<string, LibraryExportReadingState>();
  const exportWarnings: LibraryExportWarning[] = [];

  for (const bookmark of bookmarks) {
    const current = bookmarksByNovelId.get(bookmark.novelId) ?? [];
    current.push(bookmark);
    bookmarksByNovelId.set(bookmark.novelId, current);
  }

  for (const result of readerStates) {
    if (result.status === "fulfilled") {
      readingStatesByNovelId.set(result.novelId, result.readingState);
    } else {
      exportWarnings.push(result.warning);
    }
  }

  return {
    formatVersion: 1,
    exportedAt,
    novelsCount: novels.length,
    exportWarnings,
    novels: novels.map((novel) =>
      toExportNovel(
        novel,
        readingStatesByNovelId.get(novel.novelId) ?? null,
        bookmarksByNovelId.get(novel.novelId) ?? []
      )
    )
  };
}

export function serializeLibraryExportToYaml(document: LibraryExportDocument): string {
  return stringify(document, {
    lineWidth: 0,
    nullStr: "null",
    singleQuote: false
  });
}

function sanitizeFileNamePart(value: string): string {
  return value.replace(/[^a-zA-Z0-9._-]+/g, "-").replace(/^-+|-+$/g, "") || "library";
}

export function createLibraryExportFileName(exportedAt: string): string {
  return `narou-viewer-library-${sanitizeFileNamePart(exportedAt.replace(/[:.]/g, "-"))}.yaml`;
}

export function downloadTextFile(content: string, fileName: string, mimeType: string): void {
  const blob = new Blob([content], { type: mimeType });
  const objectUrl = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = objectUrl;
  anchor.download = fileName;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(objectUrl);
}
