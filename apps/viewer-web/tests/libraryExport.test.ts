import { afterEach, describe, expect, it, vi } from "vitest";
import { parse } from "yaml";

import {
  buildLibraryExportDocument,
  createLibraryExportFileName,
  downloadTextFile,
  serializeLibraryExportToYaml
} from "../src/features/library/export";
import type { NovelSummary } from "../src/features/library/types";

function createNovel(novelId: string, title: string): NovelSummary {
  return {
    novelId,
    fetcherWorkId: `work-${novelId}`,
    title,
    author: "作者",
    siteName: "小説家になろう",
    tocUrl: `https://example.test/${novelId}`,
    updatedAt: "2026-06-28T10:00:00.000Z",
    lastActivityAt: "2026-06-28T10:01:00.000Z",
    lastReadEpisodeIndex: "2",
    lastReadEpisodeTitle: "第2話",
    latestBookmarkEpisodeIndex: "2",
    bookmarkCount: 1,
    totalEpisodes: 3,
    savedEpisodes: 3,
    fetchStatus: "complete"
  };
}

function jsonResponse(payload: unknown, status = 200): Response {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { "content-type": "application/json" }
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("library export", () => {
  it("ライブラリ一覧・既読・栞を YAML 化できる文書へ集約する", async () => {
    const novels = [createNovel("n1", "小説A"), createNovel("n2", "小説B")];
    const requestedPaths: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = new URL(String(input), "http://localhost");
        requestedPaths.push(url.pathname + url.search);

        if (url.pathname === "/api/bookmarks") {
          return Promise.resolve(
            jsonResponse({
              bookmarks: [
                {
                  id: "bm-1",
                  novelId: "n1",
                  episodeIndex: "2",
                  position: 40,
                  label: "読み返す",
                  createdAt: "2026-06-28T10:02:00.000Z"
                },
                {
                  id: "bm-other",
                  novelId: "missing",
                  episodeIndex: "1",
                  position: 1,
                  label: null,
                  createdAt: "2026-06-28T10:03:00.000Z"
                }
              ]
            })
          );
        }
        if (url.pathname === "/api/reader/state" && url.searchParams.get("novelId") === "n1") {
          return Promise.resolve(
            jsonResponse({
              novelId: "n1",
              lastReadEpisodeIndex: "2",
              position: 120,
              updatedAt: "2026-06-28T10:04:00.000Z",
              stateVersion: 3,
              updatedByClientId: "client-a"
            })
          );
        }
        if (url.pathname === "/api/reader/state" && url.searchParams.get("novelId") === "n2") {
          return Promise.resolve(
            jsonResponse({
              novelId: "n2",
              lastReadEpisodeIndex: null,
              position: 0,
              updatedAt: null,
              stateVersion: 0,
              updatedByClientId: null
            })
          );
        }

        return Promise.resolve(jsonResponse({ error: "not found" }, 404));
      })
    );

    const document = await buildLibraryExportDocument(novels, "2026-06-28T10:10:00.000Z");
    const yaml = serializeLibraryExportToYaml(document);
    const parsed = parse(yaml) as typeof document;

    expect(requestedPaths).toEqual(
      expect.arrayContaining(["/api/bookmarks", "/api/reader/state?novelId=n1", "/api/reader/state?novelId=n2"])
    );
    expect(parsed.formatVersion).toBe(1);
    expect(parsed.exportedAt).toBe("2026-06-28T10:10:00.000Z");
    expect(parsed.novelsCount).toBe(2);
    expect(parsed.exportWarnings).toEqual([]);
    expect(parsed.novels[0].title).toBe("小説A");
    expect(parsed.novels[0].readingState?.position).toBe(120);
    expect(parsed.novels[0].readingState && "stateVersion" in parsed.novels[0].readingState).toBe(false);
    expect(parsed.novels[0].readingState && "updatedByClientId" in parsed.novels[0].readingState).toBe(false);
    expect(parsed.novels[0].bookmarks).toHaveLength(1);
    expect(parsed.novels[0].bookmarks[0].label).toBe("読み返す");
    expect(parsed.novels[1].bookmarks).toEqual([]);
  });

  it("既読情報の取得は同時実行数を制限し、失敗した作品は warning と null にする", async () => {
    const novels = Array.from({ length: 8 }, (_, index) => createNovel(`n${index + 1}`, `小説${index + 1}`));
    let activeReaderStateRequests = 0;
    let maxActiveReaderStateRequests = 0;

    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = new URL(String(input), "http://localhost");

        if (url.pathname === "/api/bookmarks") {
          return jsonResponse({ bookmarks: [] });
        }
        if (url.pathname === "/api/reader/state") {
          activeReaderStateRequests += 1;
          maxActiveReaderStateRequests = Math.max(maxActiveReaderStateRequests, activeReaderStateRequests);
          await new Promise((resolve) => setTimeout(resolve, 1));
          activeReaderStateRequests -= 1;

          const novelId = url.searchParams.get("novelId") ?? "";
          if (novelId === "n7") {
            return jsonResponse({ error: "reader state failed" }, 500);
          }

          return jsonResponse({
            novelId,
            lastReadEpisodeIndex: "1",
            position: 10,
            updatedAt: "2026-06-28T10:04:00.000Z",
            stateVersion: 4,
            updatedByClientId: "internal-client"
          });
        }

        return jsonResponse({ error: "not found" }, 404);
      })
    );

    const document = await buildLibraryExportDocument(novels, "2026-06-28T10:10:00.000Z");
    const failedNovel = document.novels.find((novel) => novel.novelId === "n7");
    const successfulNovel = document.novels.find((novel) => novel.novelId === "n1");

    expect(maxActiveReaderStateRequests).toBeLessThanOrEqual(6);
    expect(document.exportWarnings).toEqual([
      {
        novelId: "n7",
        field: "readingState",
        message: "reader state failed"
      }
    ]);
    expect(failedNovel?.readingState).toBeNull();
    expect(successfulNovel?.readingState).toEqual({
      lastReadEpisodeIndex: "1",
      position: 10,
      updatedAt: "2026-06-28T10:04:00.000Z"
    });
  });

  it("エクスポートファイル名を安全な YAML 名にする", () => {
    expect(createLibraryExportFileName("2026-06-28T10:10:00.000Z")).toBe(
      "narou-viewer-library-2026-06-28T10-10-00-000Z.yaml"
    );
  });

  it("テキストファイルとしてブラウザ保存を開始する", () => {
    const anchor = {
      click: vi.fn(),
      remove: vi.fn(),
      download: "",
      href: ""
    } as unknown as HTMLAnchorElement;
    const append = vi.fn();
    const createObjectURL = vi.fn(() => "blob:library-export");
    const revokeObjectURL = vi.fn();

    vi.stubGlobal("Blob", Blob);
    vi.stubGlobal("URL", { createObjectURL, revokeObjectURL });
    vi.stubGlobal("document", {
      createElement: vi.fn(() => anchor),
      body: { append }
    });

    downloadTextFile("formatVersion: 1\n", "export.yaml", "application/x-yaml;charset=utf-8");

    expect(createObjectURL).toHaveBeenCalledTimes(1);
    expect(anchor.href).toBe("blob:library-export");
    expect(anchor.download).toBe("export.yaml");
    expect(append).toHaveBeenCalledWith(anchor);
    expect(anchor.click).toHaveBeenCalledTimes(1);
    expect(anchor.remove).toHaveBeenCalledTimes(1);
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:library-export");
  });
});
