import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import type { NovelSummary } from "../src/features/library/types";
import type { RuntimeStatusResponse } from "../src/features/runtime/types";
import { useLibrary } from "../src/hooks/useLibrary";

type HookResult = ReturnType<typeof useLibrary>;

function installDom(): JSDOM {
  const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
    url: "http://localhost/"
  });

  vi.stubGlobal("window", dom.window);
  vi.stubGlobal("document", dom.window.document);
  vi.stubGlobal("navigator", dom.window.navigator);

  return dom;
}

async function flushAsyncWork(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
}

function renderHookHarness(props: {
  initialNovelId?: string | null;
  isSinglePaneLibraryViewport?: boolean;
  fetchData: () => Promise<{ runtimeStatus: RuntimeStatusResponse; novels: NovelSummary[] }>;
  onRuntimeStatusLoaded?: (status: RuntimeStatusResponse) => void;
  onError?: (message: string | null) => void;
  onRender: (result: HookResult) => void;
}): Root {
  const rootElement = document.getElementById("root");
  if (!rootElement) {
    throw new Error("root element is missing");
  }
  const onRuntimeStatusLoaded = props.onRuntimeStatusLoaded ?? vi.fn();
  const onError = props.onError ?? vi.fn();

  function Harness() {
    const result = useLibrary({
      initialNovelId: props.initialNovelId ?? null,
      isSinglePaneLibraryViewport: props.isSinglePaneLibraryViewport ?? false,
      fetchData: props.fetchData,
      onRuntimeStatusLoaded,
      onError
    });
    props.onRender(result);
    return null;
  }

  const root = createRoot(rootElement);
  root.render(createElement(Harness));
  return root;
}

function createRuntimeStatus(): RuntimeStatusResponse {
  return {
    status: "ok",
    checkedAt: "2026-06-15T00:00:00.000Z",
    services: []
  };
}

function createNovel(novelId: string, title: string): NovelSummary {
  return {
    novelId,
    fetcherWorkId: novelId,
    title,
    author: "著者",
    siteName: "narou",
    tocUrl: `https://example.test/${novelId}/`,
    totalEpisodes: 10,
    lastReadEpisodeIndex: null,
    lastReadEpisodeTitle: null,
    latestBookmarkEpisodeIndex: null,
    bookmarkCount: 0,
    updatedAt: null
  };
}

describe("useLibrary", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("loads initial library data and selects the first novel on wide viewports", async () => {
    installDom();
    const runtimeStatus = createRuntimeStatus();
    const novels = [createNovel("novel-a", "Alpha"), createNovel("novel-b", "Beta")];
    const fetchData = vi.fn().mockResolvedValue({ runtimeStatus, novels });
    const onRuntimeStatusLoaded = vi.fn();
    const onError = vi.fn();

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        fetchData,
        onRuntimeStatusLoaded,
        onError,
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    expect(fetchData).toHaveBeenCalledTimes(1);
    expect(onRuntimeStatusLoaded).toHaveBeenCalledWith(runtimeStatus);
    expect(onError).toHaveBeenCalledWith(null);
    expect(latest?.isInitialLoading).toBe(false);
    expect(latest?.novels).toEqual(novels);
    expect(latest?.selectedNovelId).toBe("novel-a");
    expect(latest?.currentNovel?.title).toBe("Alpha");

    await act(async () => {
      root?.unmount();
    });
  });

  it("keeps an existing initial selection when it still exists", async () => {
    installDom();
    const novels = [createNovel("novel-a", "Alpha"), createNovel("novel-b", "Beta")];
    const fetchData = vi.fn().mockResolvedValue({ runtimeStatus: createRuntimeStatus(), novels });

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        initialNovelId: "novel-b",
        fetchData,
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    expect(latest?.selectedNovelId).toBe("novel-b");
    expect(latest?.currentNovel?.title).toBe("Beta");

    await act(async () => {
      root?.unmount();
    });
  });

  it("does not auto-select a novel on single pane viewports", async () => {
    installDom();
    const fetchData = vi.fn().mockResolvedValue({
      runtimeStatus: createRuntimeStatus(),
      novels: [createNovel("novel-a", "Alpha")]
    });

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        isSinglePaneLibraryViewport: true,
        fetchData,
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    expect(latest?.selectedNovelId).toBeNull();

    await act(async () => {
      root?.unmount();
    });
  });

  it("filters library entries and resets pagination when the query changes", async () => {
    installDom();
    const novels = Array.from({ length: 14 }, (_, index) =>
      createNovel(`novel-${index + 1}`, index === 13 ? "Needle" : `Title ${index + 1}`)
    );
    const fetchData = vi.fn().mockResolvedValue({ runtimeStatus: createRuntimeStatus(), novels });

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        fetchData,
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    await act(async () => {
      latest?.setLibraryPage(2);
      await flushAsyncWork();
    });
    expect(latest?.libraryPagination.currentPage).toBe(2);

    await act(async () => {
      latest?.changeLibraryFilterQuery("needle");
      await flushAsyncWork();
    });

    expect(latest?.libraryFilterQuery).toBe("needle");
    expect(latest?.libraryPagination.currentPage).toBe(1);
    expect(latest?.visibleLibraryNovels.map((novel) => novel.title)).toEqual(["Needle"]);

    await act(async () => {
      latest?.clearLibraryFilter();
      await flushAsyncWork();
    });

    expect(latest?.libraryFilterQuery).toBe("");
    expect(latest?.libraryPagination.currentPage).toBe(1);
    expect(latest?.visibleLibraryNovels).toHaveLength(12);

    await act(async () => {
      root?.unmount();
    });
  });

  it("reloads library data on request", async () => {
    installDom();
    const fetchData = vi
      .fn()
      .mockResolvedValueOnce({
        runtimeStatus: createRuntimeStatus(),
        novels: [createNovel("novel-a", "Alpha")]
      })
      .mockResolvedValueOnce({
        runtimeStatus: createRuntimeStatus(),
        novels: [createNovel("novel-b", "Beta")]
      });

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        fetchData,
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    expect(latest?.selectedNovelId).toBe("novel-a");

    await act(async () => {
      latest?.requestLibraryReload();
      await flushAsyncWork();
    });

    expect(fetchData).toHaveBeenCalledTimes(2);
    expect(latest?.selectedNovelId).toBe("novel-b");
    expect(latest?.currentNovel?.title).toBe("Beta");

    await act(async () => {
      root?.unmount();
    });
  });
});
