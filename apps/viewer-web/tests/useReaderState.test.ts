import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import { fetchEpisode, fetchNovelContext, fetchReaderState, putReaderState, ReaderStateConflictError } from "../src/features/reader/api";
import type { NovelSummary } from "../src/features/library/types";
import type { EpisodeResponse, NovelReaderSettingsResponse, ReaderState, TocResponse } from "../src/features/reader/types";
import { useReaderState } from "../src/hooks/useReaderState";

vi.mock("../src/features/reader/api", () => ({
  fetchEpisode: vi.fn(),
  fetchNovelContext: vi.fn(),
  fetchReaderState: vi.fn(),
  ReaderStateConflictError: class ReaderStateConflictError extends Error {
    constructor(readonly serverState: ReaderState) {
      super("conflict");
    }
  },
  putReaderState: vi.fn()
}));

type HookResult = ReturnType<typeof useReaderState>;

function installDom(): JSDOM {
  const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
    url: "http://localhost/"
  });

  Object.defineProperty(dom.window.document, "hidden", {
    configurable: true,
    value: false
  });
  vi.stubGlobal("window", dom.window);
  vi.stubGlobal("document", dom.window.document);
  vi.stubGlobal("navigator", dom.window.navigator);
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);

  return dom;
}

async function flushAsyncWork(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
}

function createNovel(novelId: string, overrides: Partial<NovelSummary> = {}): NovelSummary {
  return {
    novelId,
    fetcherWorkId: novelId,
    title: overrides.title ?? "作品",
    author: "著者",
    siteName: "narou",
    tocUrl: null,
    totalEpisodes: 3,
    lastReadEpisodeIndex: null,
    lastReadEpisodeTitle: null,
    latestBookmarkEpisodeIndex: null,
    bookmarkCount: 0,
    updatedAt: null,
    ...overrides
  };
}

function createToc(novelId = "novel-a"): TocResponse {
  return {
    novelId,
    fetcherWorkId: novelId,
    title: "作品",
    author: "著者",
    siteName: "narou",
    tocUrl: null,
    updatedAt: "2026-06-15T00:00:00.000Z",
    lastActivityAt: "2026-06-15T00:00:00.000Z",
    story: "",
    totalEpisodes: 3,
    episodes: [
      {
        episodeIndex: "1",
        title: "一",
        chapter: null,
        subchapter: null,
        sourceUrl: null,
        updatedAt: null,
        contentEtag: "toc-1",
        bodyStatus: "complete"
      },
      {
        episodeIndex: "2",
        title: "二",
        chapter: null,
        subchapter: null,
        sourceUrl: null,
        updatedAt: null,
        contentEtag: "toc-2",
        bodyStatus: "complete"
      }
    ]
  };
}

function createReaderState(overrides: Partial<ReaderState> = {}): ReaderState {
  const novelId = overrides.novelId ?? "novel-a";
  return {
    novelId,
    lastReadEpisodeIndex: "2",
    position: 12,
    updatedAt: "2026-06-15T00:00:00.000Z",
    stateVersion: 1,
    updatedByClientId: "other-client",
    ...overrides
  };
}

function createEpisode(overrides: Partial<EpisodeResponse> = {}): EpisodeResponse {
  const novelId = overrides.novelId ?? "novel-a";
  const episodeIndex = overrides.episodeIndex ?? "2";
  return {
    novelId,
    episodeIndex,
    title: "二",
    chapter: null,
    subchapter: null,
    sourceUrl: null,
    html: "<p>本文</p>",
    plainTextLength: 2,
    updatedAt: "2026-06-15T00:00:00.000Z",
    contentEtag: "episode-2",
    readerDocument: {
      version: 1,
      blocks: []
    },
    ...overrides
  };
}

function createReaderSettings(
  overrides: Partial<NovelReaderSettingsResponse["correction"]> = {},
  novelId = "novel-a"
): NovelReaderSettingsResponse {
  return {
    novelId,
    correction: {
      quoteNormalization: true,
      hyphenDashNormalization: true,
      parenthesisNormalization: true,
      halfwidthAlnumPunctuationNormalization: true,
      ...overrides
    },
    updatedAt: null
  };
}

type HookHarnessControls = {
  reloadLibrary: () => void;
  setSelectedNovelId: (novelId: string | null) => void;
};

function renderHookHarness(props: {
  initialSelectedNovelId?: string | null;
  initialNovels?: NovelSummary[];
  onRender: (result: HookResult, novels: NovelSummary[], controls: HookHarnessControls) => void;
}): Root {
  const rootElement = document.getElementById("root");
  if (!rootElement) {
    throw new Error("root element is missing");
  }
  const onError = vi.fn();

  function Harness() {
    const [libraryReloadKey, setLibraryReloadKey] = useState(0);
    const [selectedNovelId, setSelectedNovelId] = useState<string | null>(props.initialSelectedNovelId ?? "novel-a");
    const [novels, setNovels] = useState<NovelSummary[]>(props.initialNovels ?? [createNovel("novel-a"), createNovel("novel-b")]);
    const result = useReaderState({
      initialEpisodeIndex: null,
      initialPosition: null,
      initialScreenMode: "reader",
      libraryReloadKey,
      onError,
      readerClientId: "client-a",
      selectedNovelId,
      setNovels
    });
    props.onRender(result, novels, { reloadLibrary: () => setLibraryReloadKey((current) => current + 1), setSelectedNovelId });
    return null;
  }

  const root = createRoot(rootElement);
  root.render(createElement(Harness));
  return root;
}

describe("useReaderState", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("restores the saved reader state and loads the selected episode", async () => {
    installDom();
    vi.mocked(fetchNovelContext).mockResolvedValue({
      toc: createToc(),
      readerState: createReaderState(),
      bookmarks: [],
      readerSettings: createReaderSettings()
    });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());

    let latest: HookResult | null = null;
    let latestNovels: NovelSummary[] = [];
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result, novels) => {
          latest = result;
          latestNovels = novels;
        }
      });
      await flushAsyncWork();
    });

    expect(fetchNovelContext).toHaveBeenCalledWith("novel-a");
    expect(fetchEpisode).toHaveBeenCalledWith("novel-a", "2");
    expect(latest?.selectedEpisodeIndex).toBe("2");
    expect(latest?.selectedPosition).toBe(12);
    expect(latest?.episode?.title).toBe("二");
    expect(latest?.readerSettings?.correction.quoteNormalization).toBe(true);
    expect(latestNovels[0]?.lastReadEpisodeIndex).toBe("2");
    expect(latestNovels[0]?.lastReadEpisodeTitle).toBe("二");

    await act(async () => {
      root?.unmount();
    });
  });

  it("updates the local library order when a reader state save becomes the latest activity", async () => {
    installDom();
    const initialState = createReaderState({
      novelId: "novel-b",
      updatedAt: "2026-06-14T00:00:00.000Z",
      stateVersion: 1
    });
    const savedState = createReaderState({
      novelId: "novel-b",
      updatedAt: "2026-06-16T00:00:00.000Z",
      stateVersion: 2,
      updatedByClientId: "client-a"
    });
    vi.mocked(fetchNovelContext).mockResolvedValue({
      toc: createToc("novel-b"),
      readerState: initialState,
      bookmarks: [],
      readerSettings: createReaderSettings({}, "novel-b")
    });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode({ novelId: "novel-b" }));
    vi.mocked(putReaderState).mockResolvedValue(savedState);

    let latest: HookResult | null = null;
    let latestNovels: NovelSummary[] = [];
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        initialSelectedNovelId: "novel-b",
        initialNovels: [
          createNovel("novel-a", {
            title: "先頭だった作品",
            lastActivityAt: "2026-06-15T00:00:00.000Z"
          }),
          createNovel("novel-b", {
            title: "読んだ作品",
            lastActivityAt: "2026-06-14T00:00:00.000Z"
          })
        ],
        onRender: (result, novels) => {
          latest = result;
          latestNovels = novels;
        }
      });
      await flushAsyncWork();
    });

    await act(async () => {
      await latest?.putReaderState({
        novelId: "novel-b",
        episodeIndex: "2",
        position: 44
      });
      await flushAsyncWork();
    });

    expect(latestNovels[0]?.novelId).toBe("novel-b");
    expect(latestNovels[0]?.lastActivityAt).toBe(savedState.updatedAt);
    expect(latestNovels[0]?.lastReadEpisodeIndex).toBe("2");
    expect(latestNovels[1]?.novelId).toBe("novel-a");

    await act(async () => {
      root?.unmount();
    });
  });

  it("uses novel id as the stable tiebreaker when locally sorting equal activity timestamps", async () => {
    installDom();
    const sameActivityAt = "2026-06-16T00:00:00.000Z";
    const initialState = createReaderState({
      novelId: "novel-b",
      updatedAt: "2026-06-14T00:00:00.000Z",
      stateVersion: 1
    });
    const savedState = createReaderState({
      novelId: "novel-b",
      updatedAt: sameActivityAt,
      stateVersion: 2,
      updatedByClientId: "client-a"
    });
    vi.mocked(fetchNovelContext).mockResolvedValue({
      toc: createToc("novel-b"),
      readerState: initialState,
      bookmarks: [],
      readerSettings: createReaderSettings({}, "novel-b")
    });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode({ novelId: "novel-b" }));
    vi.mocked(putReaderState).mockResolvedValue(savedState);

    let latest: HookResult | null = null;
    let latestNovels: NovelSummary[] = [];
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        initialSelectedNovelId: "novel-b",
        initialNovels: [
          createNovel("novel-b", {
            title: "A title that would sort first by title",
            lastActivityAt: "2026-06-14T00:00:00.000Z"
          }),
          createNovel("novel-a", {
            title: "Z title that would sort last by title",
            lastActivityAt: sameActivityAt
          })
        ],
        onRender: (result, novels) => {
          latest = result;
          latestNovels = novels;
        }
      });
      await flushAsyncWork();
    });

    await act(async () => {
      await latest?.putReaderState({
        novelId: "novel-b",
        episodeIndex: "2",
        position: 44
      });
      await flushAsyncWork();
    });

    expect(latestNovels.map((novel) => novel.novelId)).toEqual(["novel-a", "novel-b"]);

    await act(async () => {
      root?.unmount();
    });
  });

  it("detects a newer reader state from another client on focus", async () => {
    installDom();
    vi.mocked(fetchNovelContext).mockResolvedValue({
      toc: createToc(),
      readerState: createReaderState(),
      bookmarks: [],
      readerSettings: createReaderSettings()
    });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());
    vi.mocked(fetchReaderState).mockResolvedValue({
      ...createReaderState(),
      position: 99,
      stateVersion: 2,
      updatedByClientId: "other-client"
    });

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    await act(async () => {
      window.dispatchEvent(new window.Event("focus"));
      await flushAsyncWork();
    });

    expect(fetchReaderState).toHaveBeenCalledWith("novel-a");
    expect(latest?.readerSyncConflict?.serverState.position).toBe(99);

    await act(async () => {
      root?.unmount();
    });
  });

  it("accepts newer focus sync states written by the same page writer", async () => {
    installDom();
    vi.mocked(fetchNovelContext).mockResolvedValue({
      toc: createToc(),
      readerState: createReaderState(),
      bookmarks: [],
      readerSettings: createReaderSettings()
    });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());
    vi.mocked(fetchReaderState).mockResolvedValue({
      ...createReaderState(),
      position: 99,
      stateVersion: 2,
      updatedByClientId: "client-a"
    });

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    await act(async () => {
      window.dispatchEvent(new window.Event("focus"));
      await flushAsyncWork();
    });

    expect(latest?.readerSyncConflict).toBeNull();
    expect(latest?.readerState?.position).toBe(99);
    expect(latest?.readerState?.stateVersion).toBe(2);

    await act(async () => {
      root?.unmount();
    });
  });

  it("turns reader state save version conflicts into sync conflicts", async () => {
    installDom();
    const initialState = createReaderState();
    const conflictState = {
      ...initialState,
      position: 88,
      stateVersion: 2,
      updatedByClientId: "other-client"
    };
    vi.mocked(fetchNovelContext).mockResolvedValue({
      toc: createToc(),
      readerState: initialState,
      bookmarks: [],
      readerSettings: createReaderSettings()
    });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());
    vi.mocked(putReaderState).mockRejectedValue(new ReaderStateConflictError(conflictState));

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    let saveError: unknown;
    await act(async () => {
      try {
        await latest?.putReaderState({
          novelId: "novel-a",
          episodeIndex: "2",
          position: 15
        });
      } catch (error) {
        saveError = error;
      }
    });

    expect(saveError).toBeInstanceOf(ReaderStateConflictError);
    expect(putReaderState).toHaveBeenCalledWith(
      expect.objectContaining({
        expectedStateVersion: initialState.stateVersion
      })
    );
    expect(latest?.readerSyncConflict?.serverState).toEqual(conflictState);

    await act(async () => {
      root?.unmount();
    });
  });

  it("treats same-writer version conflicts as successful only when the saved position matches", async () => {
    installDom();
    const initialState = createReaderState();
    const selfSavedState = {
      ...initialState,
      position: 30,
      stateVersion: 2,
      updatedByClientId: "client-a"
    };
    vi.mocked(fetchNovelContext).mockResolvedValue({
      toc: createToc(),
      readerState: initialState,
      bookmarks: [],
      readerSettings: createReaderSettings()
    });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());
    vi.mocked(putReaderState).mockClear();
    vi.mocked(putReaderState).mockRejectedValue(new ReaderStateConflictError(selfSavedState));

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    let savedState: ReaderState | null = null;
    await act(async () => {
      savedState =
        (await latest?.putReaderState({
          novelId: "novel-a",
          episodeIndex: "2",
          position: 30
        })) ?? null;
    });

    expect(savedState).toEqual(selfSavedState);
    expect(latest?.readerSyncConflict).toBeNull();
    expect(latest?.readerState?.position).toBe(30);
    expect(latest?.readerState?.stateVersion).toBe(2);

    await act(async () => {
      root?.unmount();
    });
  });

  it("turns same-writer version conflicts with a different position into sync conflicts", async () => {
    installDom();
    const initialState = createReaderState();
    const selfSavedState = {
      ...initialState,
      position: 22,
      stateVersion: 2,
      updatedByClientId: "client-a"
    };
    vi.mocked(fetchNovelContext).mockResolvedValue({
      toc: createToc(),
      readerState: initialState,
      bookmarks: [],
      readerSettings: createReaderSettings()
    });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());
    vi.mocked(putReaderState).mockClear();
    vi.mocked(putReaderState).mockRejectedValue(new ReaderStateConflictError(selfSavedState));

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    let saveError: unknown;
    await act(async () => {
      try {
        await latest?.putReaderState({
          novelId: "novel-a",
          episodeIndex: "2",
          position: 30
        });
      } catch (error) {
        saveError = error;
      }
    });

    expect(saveError).toBeInstanceOf(ReaderStateConflictError);
    expect(latest?.readerState?.position).toBe(initialState.position);
    expect(latest?.readerSyncConflict?.serverState).toEqual(selfSavedState);

    await act(async () => {
      root?.unmount();
    });
  });

  it("serializes reader state saves and uses the latest acknowledged version", async () => {
    installDom();
    const initialState = createReaderState();
    let resolveFirstSave: ((state: ReaderState) => void) | null = null;
    vi.mocked(fetchNovelContext).mockResolvedValue({
      toc: createToc(),
      readerState: initialState,
      bookmarks: [],
      readerSettings: createReaderSettings()
    });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());
    vi.mocked(putReaderState).mockReset();
    vi.mocked(putReaderState)
      .mockReturnValueOnce(
        new Promise((resolve) => {
          resolveFirstSave = resolve;
        })
      )
      .mockResolvedValueOnce({
        ...initialState,
        position: 20,
        stateVersion: 3,
        updatedByClientId: "client-a"
      });

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    const firstSave = latest?.putReaderState({
      novelId: "novel-a",
      episodeIndex: "2",
      position: 10
    });
    const secondSave = latest?.putReaderState({
      novelId: "novel-a",
      episodeIndex: "2",
      position: 20
    });

    await flushAsyncWork();
    const baseSaveCallCount = vi.mocked(putReaderState).mock.calls.length;
    expect(baseSaveCallCount).toBe(1);
    expect(putReaderState).toHaveBeenNthCalledWith(
      baseSaveCallCount,
      expect.objectContaining({ expectedStateVersion: initialState.stateVersion, position: 10 })
    );

    await act(async () => {
      resolveFirstSave?.({
        ...initialState,
        position: 10,
        stateVersion: 2,
        updatedByClientId: "client-a"
      });
      await firstSave;
      await flushAsyncWork();
    });

    expect(putReaderState).toHaveBeenCalledTimes(baseSaveCallCount + 1);
    expect(putReaderState).toHaveBeenNthCalledWith(
      baseSaveCallCount + 1,
      expect.objectContaining({ expectedStateVersion: 2, position: 20 })
    );

    await act(async () => {
      await secondSave;
    });

    await act(async () => {
      root?.unmount();
    });
  });

  it("keeps delayed save responses and acknowledged versions scoped to each novel", async () => {
    installDom();
    const stateA = createReaderState({ novelId: "novel-a", lastReadEpisodeIndex: "2", position: 12, stateVersion: 1 });
    const stateB = createReaderState({ novelId: "novel-b", lastReadEpisodeIndex: "1", position: 70, stateVersion: 7 });
    let resolveSaveA: ((state: ReaderState) => void) | null = null;
    let resolveSaveB: ((state: ReaderState) => void) | null = null;
    vi.mocked(fetchNovelContext).mockImplementation(async (novelId) => ({
      toc: createToc(novelId),
      readerState: novelId === "novel-b" ? stateB : stateA,
      bookmarks: [],
      readerSettings: createReaderSettings({}, novelId)
    }));
    vi.mocked(fetchEpisode).mockImplementation(async (novelId, episodeIndex) => createEpisode({ novelId, episodeIndex }));
    vi.mocked(putReaderState).mockReset();
    vi.mocked(putReaderState).mockImplementation(
      (payload) =>
        new Promise<ReaderState>((resolve) => {
          if (payload.novelId === "novel-b") {
            resolveSaveB = resolve;
            return;
          }
          resolveSaveA = resolve;
        })
    );

    let latest: HookResult | null = null;
    let controls: HookHarnessControls | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result, _novels, nextControls) => {
          latest = result;
          controls = nextControls;
        }
      });
      await flushAsyncWork();
    });

    const saveA = latest?.putReaderState({
      novelId: "novel-a",
      episodeIndex: "2",
      position: 10
    });

    await flushAsyncWork();
    expect(putReaderState).toHaveBeenCalledTimes(1);
    expect(putReaderState).toHaveBeenNthCalledWith(
      1,
      expect.objectContaining({ expectedStateVersion: 1, novelId: "novel-a", position: 10 })
    );

    await act(async () => {
      controls?.setSelectedNovelId("novel-b");
      await flushAsyncWork();
    });

    expect(latest?.readerState?.novelId).toBe("novel-b");
    expect(latest?.readerState?.stateVersion).toBe(7);

    const saveB = latest?.putReaderState({
      novelId: "novel-b",
      episodeIndex: "1",
      position: 80
    });

    await flushAsyncWork();
    expect(putReaderState).toHaveBeenCalledTimes(2);
    expect(putReaderState).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({ expectedStateVersion: 7, novelId: "novel-b", position: 80 })
    );

    await act(async () => {
      resolveSaveA?.({
        ...stateA,
        position: 10,
        stateVersion: 2,
        updatedByClientId: "client-a"
      });
      await saveA;
      await flushAsyncWork();
    });

    expect(latest?.readerState?.novelId).toBe("novel-b");
    expect(latest?.readerState?.stateVersion).toBe(7);

    await act(async () => {
      resolveSaveB?.({
        ...stateB,
        position: 80,
        stateVersion: 8,
        updatedByClientId: "client-a"
      });
      await saveB;
      await flushAsyncWork();
    });

    expect(latest?.readerState?.novelId).toBe("novel-b");
    expect(latest?.readerState?.position).toBe(80);
    expect(latest?.readerState?.stateVersion).toBe(8);

    await act(async () => {
      root?.unmount();
    });
  });

  it("does not let an older context response roll back a newer acknowledged version", async () => {
    installDom();
    const initialState = createReaderState({ stateVersion: 1, position: 12 });
    const savedState = createReaderState({ stateVersion: 2, position: 40, updatedByClientId: "client-a" });
    let resolveReloadContext: ((value: Awaited<ReturnType<typeof fetchNovelContext>>) => void) | null = null;
    vi.mocked(fetchNovelContext)
      .mockResolvedValueOnce({
        toc: createToc(),
        readerState: initialState,
        bookmarks: [],
        readerSettings: createReaderSettings()
      })
      .mockReturnValueOnce(
        new Promise((resolve) => {
          resolveReloadContext = resolve;
        })
      );
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());
    vi.mocked(putReaderState)
      .mockReset()
      .mockResolvedValueOnce(savedState)
      .mockResolvedValueOnce({
        ...savedState,
        position: 44,
        stateVersion: 3
      });

    let latest: HookResult | null = null;
    let controls: HookHarnessControls | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result, _novels, nextControls) => {
          latest = result;
          controls = nextControls;
        }
      });
      await flushAsyncWork();
    });

    await act(async () => {
      controls?.reloadLibrary();
      await flushAsyncWork();
    });

    await act(async () => {
      await latest?.putReaderState({
        novelId: "novel-a",
        episodeIndex: "2",
        position: 40
      });
      latest?.setReaderSyncConflict({
        serverState: createReaderState({ stateVersion: 5, position: 90, updatedByClientId: "other-client" })
      });
      resolveReloadContext?.({
        toc: createToc(),
        readerState: initialState,
        bookmarks: [],
        readerSettings: createReaderSettings()
      });
      await flushAsyncWork();
    });

    expect(latest?.readerState?.stateVersion).toBe(2);
    expect(latest?.readerState?.position).toBe(40);
    expect(latest?.readerSyncConflict?.serverState.stateVersion).toBe(5);

    await act(async () => {
      latest?.setReaderSyncConflict(null);
      await flushAsyncWork();
    });

    await act(async () => {
      await latest?.putReaderState({
        novelId: "novel-a",
        episodeIndex: "2",
        position: 44
      });
    });

    expect(putReaderState).toHaveBeenNthCalledWith(2, expect.objectContaining({ expectedStateVersion: 2, position: 44 }));

    await act(async () => {
      root?.unmount();
    });
  });

  it("turns newer external context reader states into conflicts without adopting their version for local saves", async () => {
    installDom();
    const initialState = createReaderState({ lastReadEpisodeIndex: "2", position: 20, stateVersion: 1 });
    const externalState = createReaderState({
      lastReadEpisodeIndex: "2",
      position: 80,
      stateVersion: 2,
      updatedByClientId: "other-client"
    });
    vi.mocked(fetchNovelContext)
      .mockResolvedValueOnce({
        toc: createToc(),
        readerState: initialState,
        bookmarks: [],
        readerSettings: createReaderSettings()
      })
      .mockResolvedValueOnce({
        toc: createToc(),
        readerState: externalState,
        bookmarks: [],
        readerSettings: createReaderSettings()
      });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());
    vi.mocked(putReaderState).mockReset();

    let latest: HookResult | null = null;
    let controls: HookHarnessControls | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result, _novels, nextControls) => {
          latest = result;
          controls = nextControls;
        }
      });
      await flushAsyncWork();
    });

    expect(latest?.selectedPosition).toBe(20);

    await act(async () => {
      controls?.reloadLibrary();
      await flushAsyncWork();
    });

    expect(latest?.readerSyncConflict?.serverState).toEqual(externalState);
    expect(latest?.readerState?.stateVersion).toBe(1);
    expect(latest?.readerState?.position).toBe(20);

    let saveError: unknown;
    await act(async () => {
      try {
        await latest?.putReaderState({
          novelId: "novel-a",
          episodeIndex: "2",
          position: 20
        });
      } catch (error) {
        saveError = error;
      }
      await flushAsyncWork();
    });

    expect(saveError).toBeInstanceOf(Error);
    expect((saveError as Error).name).toBe("AbortError");
    expect(putReaderState).not.toHaveBeenCalled();

    await act(async () => {
      root?.unmount();
    });
  });

  it("does not adopt newer external context reader states while the current page position is unresolved", async () => {
    installDom();
    const initialState = createReaderState({ lastReadEpisodeIndex: "2", position: 20, stateVersion: 1 });
    const externalState = createReaderState({
      lastReadEpisodeIndex: "1",
      position: 80,
      stateVersion: 2,
      updatedByClientId: "other-client"
    });
    vi.mocked(fetchNovelContext)
      .mockResolvedValueOnce({
        toc: createToc(),
        readerState: initialState,
        bookmarks: [],
        readerSettings: createReaderSettings()
      })
      .mockResolvedValueOnce({
        toc: createToc(),
        readerState: externalState,
        bookmarks: [],
        readerSettings: createReaderSettings()
      });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());
    vi.mocked(putReaderState).mockReset();

    let latest: HookResult | null = null;
    let controls: HookHarnessControls | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result, _novels, nextControls) => {
          latest = result;
          controls = nextControls;
        }
      });
      await flushAsyncWork();
    });

    await act(async () => {
      latest?.setSelectedPosition(null);
      await flushAsyncWork();
    });
    expect(latest?.selectedEpisodeIndex).toBe("2");
    expect(latest?.selectedPosition).toBeNull();

    await act(async () => {
      controls?.reloadLibrary();
      await flushAsyncWork();
    });

    expect(latest?.readerSyncConflict?.serverState).toEqual(externalState);
    expect(latest?.readerState?.stateVersion).toBe(1);
    expect(latest?.readerState?.position).toBe(20);

    let saveError: unknown;
    await act(async () => {
      try {
        await latest?.putReaderState({
          novelId: "novel-a",
          episodeIndex: "2",
          position: 42
        });
      } catch (error) {
        saveError = error;
      }
      await flushAsyncWork();
    });

    expect(saveError).toBeInstanceOf(Error);
    expect((saveError as Error).name).toBe("AbortError");
    expect(putReaderState).not.toHaveBeenCalled();

    await act(async () => {
      root?.unmount();
    });
  });

  it("uses the visible reader position, not the last acknowledged position, when focus sync detects conflicts", async () => {
    installDom();
    const acknowledgedState = createReaderState({ lastReadEpisodeIndex: "2", position: 10, stateVersion: 1 });
    const externalAcknowledgedPosition = createReaderState({
      lastReadEpisodeIndex: "2",
      position: 10,
      stateVersion: 2,
      updatedByClientId: "other-client"
    });
    vi.mocked(fetchNovelContext).mockResolvedValue({
      toc: createToc(),
      readerState: acknowledgedState,
      bookmarks: [],
      readerSettings: createReaderSettings()
    });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());
    vi.mocked(fetchReaderState).mockResolvedValue(externalAcknowledgedPosition);

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    await act(async () => {
      latest?.setSelectedPosition(20);
      await flushAsyncWork();
    });

    await act(async () => {
      window.dispatchEvent(new window.Event("focus"));
      await flushAsyncWork();
    });

    expect(latest?.readerSyncConflict?.serverState).toEqual(externalAcknowledgedPosition);
    expect(latest?.readerState).toEqual(acknowledgedState);

    await act(async () => {
      root?.unmount();
    });
  });

  it("accepts focus sync states that match the visible reader position even when the acknowledged position differs", async () => {
    installDom();
    const acknowledgedState = createReaderState({ lastReadEpisodeIndex: "2", position: 10, stateVersion: 1 });
    const externalVisiblePosition = createReaderState({
      lastReadEpisodeIndex: "2",
      position: 20,
      stateVersion: 2,
      updatedByClientId: "other-client"
    });
    vi.mocked(fetchNovelContext).mockResolvedValue({
      toc: createToc(),
      readerState: acknowledgedState,
      bookmarks: [],
      readerSettings: createReaderSettings()
    });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());
    vi.mocked(fetchReaderState).mockResolvedValue(externalVisiblePosition);

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    await act(async () => {
      latest?.setSelectedPosition(20);
      await flushAsyncWork();
    });

    await act(async () => {
      window.dispatchEvent(new window.Event("focus"));
      await flushAsyncWork();
    });

    expect(latest?.readerSyncConflict).toBeNull();
    expect(latest?.readerState).toEqual(externalVisiblePosition);

    await act(async () => {
      root?.unmount();
    });
  });

  it("updates the local library order when an external sync conflict is applied", async () => {
    installDom();
    const initialState = createReaderState({
      novelId: "novel-b",
      lastReadEpisodeIndex: "2",
      position: 10,
      updatedAt: "2026-06-14T00:00:00.000Z",
      stateVersion: 1
    });
    const externalState = createReaderState({
      novelId: "novel-b",
      lastReadEpisodeIndex: "2",
      position: 80,
      updatedAt: "2026-06-16T00:00:00.000Z",
      stateVersion: 2,
      updatedByClientId: "other-client"
    });
    vi.mocked(fetchNovelContext).mockResolvedValue({
      toc: createToc("novel-b"),
      readerState: initialState,
      bookmarks: [],
      readerSettings: createReaderSettings({}, "novel-b")
    });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode({ novelId: "novel-b" }));
    vi.mocked(fetchReaderState).mockResolvedValue(externalState);

    let latest: HookResult | null = null;
    let latestNovels: NovelSummary[] = [];
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        initialSelectedNovelId: "novel-b",
        initialNovels: [
          createNovel("novel-a", {
            title: "先頭だった作品",
            lastActivityAt: "2026-06-15T00:00:00.000Z"
          }),
          createNovel("novel-b", {
            title: "別端末で読んだ作品",
            lastActivityAt: "2026-06-14T00:00:00.000Z"
          })
        ],
        onRender: (result, novels) => {
          latest = result;
          latestNovels = novels;
        }
      });
      await flushAsyncWork();
    });

    await act(async () => {
      latest?.setSelectedPosition(20);
      await flushAsyncWork();
    });
    await act(async () => {
      window.dispatchEvent(new window.Event("focus"));
      await flushAsyncWork();
    });

    expect(latest?.readerSyncConflict?.serverState).toEqual(externalState);

    await act(async () => {
      latest?.setReaderState(externalState);
      latest?.setReaderSyncConflict(null);
      await flushAsyncWork();
    });

    expect(latest?.readerSyncConflict).toBeNull();
    expect(latestNovels[0]?.novelId).toBe("novel-b");
    expect(latestNovels[0]?.lastActivityAt).toBe(externalState.updatedAt);
    expect(latestNovels[0]?.lastReadEpisodeIndex).toBe("2");

    await act(async () => {
      root?.unmount();
    });
  });

  it("treats external advanced focus sync states as conflicts while the visible position is unresolved", async () => {
    installDom();
    const acknowledgedState = createReaderState({ lastReadEpisodeIndex: "2", position: 10, stateVersion: 1 });
    const externalState = createReaderState({
      lastReadEpisodeIndex: "2",
      position: 10,
      stateVersion: 2,
      updatedByClientId: "other-client"
    });
    vi.mocked(fetchNovelContext).mockResolvedValue({
      toc: createToc(),
      readerState: acknowledgedState,
      bookmarks: [],
      readerSettings: createReaderSettings()
    });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());
    vi.mocked(fetchReaderState).mockResolvedValue(externalState);

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    await act(async () => {
      latest?.setSelectedPosition(null);
      await flushAsyncWork();
    });

    await act(async () => {
      window.dispatchEvent(new window.Event("focus"));
      await flushAsyncWork();
    });

    expect(latest?.readerSyncConflict?.serverState).toEqual(externalState);

    await act(async () => {
      root?.unmount();
    });
  });

  it("does not surface focus sync network failures as unhandled rejections", async () => {
    installDom();
    vi.mocked(fetchNovelContext).mockResolvedValue({
      toc: createToc(),
      readerState: createReaderState(),
      bookmarks: [],
      readerSettings: createReaderSettings()
    });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());
    vi.mocked(fetchReaderState).mockRejectedValue(new TypeError("network down"));

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    await act(async () => {
      window.dispatchEvent(new window.Event("focus"));
      await flushAsyncWork();
    });

    expect(latest?.readerSyncConflict).toBeNull();

    await act(async () => {
      root?.unmount();
    });
  });

  it("keeps only the actively sent save in the pending reading state key", async () => {
    installDom();
    const initialState = createReaderState();
    let rejectFirstSave: ((error: Error) => void) | null = null;
    vi.mocked(fetchNovelContext).mockResolvedValue({
      toc: createToc(),
      readerState: initialState,
      bookmarks: [],
      readerSettings: createReaderSettings()
    });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());
    vi.mocked(putReaderState).mockReset();
    vi.mocked(putReaderState)
      .mockReturnValueOnce(
        new Promise((_resolve, reject) => {
          rejectFirstSave = reject;
        })
      )
      .mockResolvedValueOnce({
        ...initialState,
        position: 20,
        stateVersion: 2,
        updatedByClientId: "client-a"
      });

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    const firstSave = latest?.putReaderState({
      novelId: "novel-a",
      episodeIndex: "2",
      position: 10,
      readingStateKey: "novel-a:2:10"
    });
    const abortController = new AbortController();
    const queuedSave = latest?.putReaderState({
      novelId: "novel-a",
      episodeIndex: "2",
      position: 20,
      readingStateKey: "novel-a:2:20",
      signal: abortController.signal
    });
    abortController.abort();

    await flushAsyncWork();
    expect(putReaderState).toHaveBeenCalledTimes(1);
    expect(latest?.pendingReadingStateKeyRef.current).toBe("novel-a:2:10");
    const replacementSave =
      latest?.pendingReadingStateKeyRef.current === "novel-a:2:20"
        ? null
        : latest?.putReaderState({
            novelId: "novel-a",
            episodeIndex: "2",
            position: 20,
            readingStateKey: "novel-a:2:20"
          });
    expect(replacementSave).not.toBeNull();

    let queuedError: unknown;
    let firstError: unknown;
    await act(async () => {
      rejectFirstSave?.(new Error("failed to save first position"));
      try {
        await firstSave;
      } catch (error) {
        firstError = error;
      }
      try {
        await queuedSave;
      } catch (error) {
        queuedError = error;
      }
      await flushAsyncWork();
    });

    expect(firstError).toBeInstanceOf(Error);
    expect(queuedError).toBeInstanceOf(Error);
    expect((queuedError as Error).name).toBe("AbortError");
    expect(putReaderState).toHaveBeenCalledTimes(2);
    expect(putReaderState).toHaveBeenNthCalledWith(2, expect.objectContaining({ expectedStateVersion: 1, position: 20 }));

    await act(async () => {
      await replacementSave;
      await flushAsyncWork();
    });
    expect(latest?.pendingReadingStateKeyRef.current).toBeNull();

    await act(async () => {
      root?.unmount();
    });
  });

  it("resets acknowledged reader state generations when a novel is removed and reloaded", async () => {
    installDom();
    const deletedState = createReaderState({ lastReadEpisodeIndex: "2", position: 55, stateVersion: 5 });
    const reloadedState = createReaderState({ lastReadEpisodeIndex: "1", position: 0, stateVersion: 0, updatedByClientId: null });
    const savedReloadedState = createReaderState({
      lastReadEpisodeIndex: "1",
      position: 5,
      stateVersion: 1,
      updatedByClientId: "client-a"
    });
    let resolveOldSave: ((state: ReaderState) => void) | null = null;
    vi.mocked(fetchNovelContext)
      .mockResolvedValueOnce({
        toc: createToc(),
        readerState: deletedState,
        bookmarks: [],
        readerSettings: createReaderSettings()
      })
      .mockResolvedValueOnce({
        toc: createToc(),
        readerState: reloadedState,
        bookmarks: [],
        readerSettings: createReaderSettings()
      });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());
    vi.mocked(putReaderState)
      .mockReset()
      .mockReturnValueOnce(
        new Promise((resolve) => {
          resolveOldSave = resolve;
        })
      )
      .mockResolvedValueOnce(savedReloadedState);

    let latest: HookResult | null = null;
    let controls: HookHarnessControls | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result, _novels, nextControls) => {
          latest = result;
          controls = nextControls;
        }
      });
      await flushAsyncWork();
    });

    const oldSave = latest?.putReaderState({
      novelId: "novel-a",
      episodeIndex: "2",
      position: 60
    });

    await flushAsyncWork();
    expect(putReaderState).toHaveBeenCalledTimes(1);
    expect(putReaderState).toHaveBeenNthCalledWith(1, expect.objectContaining({ expectedStateVersion: 5, position: 60 }));

    await act(async () => {
      latest?.resetReaderStateCache("novel-a");
      controls?.reloadLibrary();
      await flushAsyncWork();
    });

    expect(latest?.readerState?.stateVersion).toBe(0);
    expect(latest?.readerState?.position).toBe(0);

    await act(async () => {
      await latest?.putReaderState({
        novelId: "novel-a",
        episodeIndex: "1",
        position: 5
      });
      await flushAsyncWork();
    });

    expect(putReaderState).toHaveBeenNthCalledWith(2, expect.objectContaining({ expectedStateVersion: 0, position: 5 }));
    expect(latest?.readerState?.stateVersion).toBe(1);
    expect(latest?.readerState?.position).toBe(5);

    let oldSaveError: unknown;
    await act(async () => {
      resolveOldSave?.({
        ...deletedState,
        position: 60,
        stateVersion: 6,
        updatedByClientId: "client-a"
      });
      try {
        await oldSave;
      } catch (error) {
        oldSaveError = error;
      }
      await flushAsyncWork();
    });

    expect(oldSaveError).toBeInstanceOf(Error);
    expect((oldSaveError as Error).name).toBe("AbortError");
    expect(latest?.readerState?.stateVersion).toBe(1);
    expect(latest?.readerState?.position).toBe(5);

    await act(async () => {
      root?.unmount();
    });
  });

  it("blocks queued automatic saves until an external reader state conflict is resolved", async () => {
    installDom();
    const initialState = createReaderState({ stateVersion: 1, position: 12 });
    const conflictState = createReaderState({ stateVersion: 2, position: 80, updatedByClientId: "other-client" });
    let rejectFirstSave: ((error: Error) => void) | null = null;
    vi.mocked(fetchNovelContext).mockResolvedValue({
      toc: createToc(),
      readerState: initialState,
      bookmarks: [],
      readerSettings: createReaderSettings()
    });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());
    vi.mocked(putReaderState)
      .mockReset()
      .mockReturnValueOnce(
        new Promise((_resolve, reject) => {
          rejectFirstSave = reject;
        })
      );

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    const firstSave = latest?.putReaderState({
      novelId: "novel-a",
      episodeIndex: "2",
      position: 30
    });
    const queuedSave = latest?.putReaderState({
      novelId: "novel-a",
      episodeIndex: "2",
      position: 40
    });

    await flushAsyncWork();
    expect(putReaderState).toHaveBeenCalledTimes(1);

    let firstError: unknown;
    let queuedError: unknown;
    await act(async () => {
      rejectFirstSave?.(new ReaderStateConflictError(conflictState));
      try {
        await firstSave;
      } catch (error) {
        firstError = error;
      }
      try {
        await queuedSave;
      } catch (error) {
        queuedError = error;
      }
      await flushAsyncWork();
    });

    expect(firstError).toBeInstanceOf(ReaderStateConflictError);
    expect(queuedError).toBeInstanceOf(Error);
    expect((queuedError as Error).name).toBe("AbortError");
    expect(putReaderState).toHaveBeenCalledTimes(1);
    expect(latest?.readerSyncConflict?.serverState).toEqual(conflictState);

    vi.mocked(putReaderState).mockResolvedValueOnce({
      ...conflictState,
      position: 90,
      stateVersion: 3,
      updatedByClientId: "client-a"
    });
    await act(async () => {
      latest?.setReaderState(conflictState);
      latest?.setReaderSyncConflict(null);
      await flushAsyncWork();
    });
    await act(async () => {
      await latest?.putReaderState({
        novelId: "novel-a",
        episodeIndex: "2",
        position: 90
      });
      await flushAsyncWork();
    });

    expect(putReaderState).toHaveBeenCalledTimes(2);
    expect(putReaderState).toHaveBeenNthCalledWith(2, expect.objectContaining({ expectedStateVersion: 2, position: 90 }));

    await act(async () => {
      root?.unmount();
    });
  });

  it("does not leave a hidden save block when an external 409 arrives for an unselected novel", async () => {
    installDom();
    const stateA = createReaderState({ novelId: "novel-a", lastReadEpisodeIndex: "2", position: 12, stateVersion: 1 });
    const stateB = createReaderState({ novelId: "novel-b", lastReadEpisodeIndex: "1", position: 70, stateVersion: 1 });
    const externalStateA = createReaderState({
      novelId: "novel-a",
      lastReadEpisodeIndex: "2",
      position: 80,
      stateVersion: 2,
      updatedByClientId: "other-client"
    });
    let rejectSaveA: ((error: Error) => void) | null = null;
    vi.mocked(fetchNovelContext).mockImplementation(async (novelId) => ({
      toc: createToc(novelId),
      readerState: novelId === "novel-b" ? stateB : stateA,
      bookmarks: [],
      readerSettings: createReaderSettings({}, novelId)
    }));
    vi.mocked(fetchEpisode).mockImplementation(async (novelId, episodeIndex) => createEpisode({ novelId, episodeIndex }));
    vi.mocked(putReaderState)
      .mockReset()
      .mockReturnValueOnce(
        new Promise((_resolve, reject) => {
          rejectSaveA = reject;
        })
      )
      .mockResolvedValueOnce({
        ...stateA,
        position: 90,
        stateVersion: 3,
        updatedByClientId: "client-a"
      });

    let latest: HookResult | null = null;
    let controls: HookHarnessControls | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result, _novels, nextControls) => {
          latest = result;
          controls = nextControls;
        }
      });
      await flushAsyncWork();
    });

    const saveA = latest?.putReaderState({
      novelId: "novel-a",
      episodeIndex: "2",
      position: 30
    });
    await flushAsyncWork();
    expect(putReaderState).toHaveBeenCalledTimes(1);

    await act(async () => {
      controls?.setSelectedNovelId("novel-b");
      await flushAsyncWork();
    });

    let saveAError: unknown;
    await act(async () => {
      rejectSaveA?.(new ReaderStateConflictError(externalStateA));
      try {
        await saveA;
      } catch (error) {
        saveAError = error;
      }
      await flushAsyncWork();
    });

    expect(saveAError).toBeInstanceOf(ReaderStateConflictError);
    expect(latest?.readerSyncConflict).toBeNull();

    await act(async () => {
      await latest?.putReaderState({
        novelId: "novel-a",
        episodeIndex: "2",
        position: 90
      });
      await flushAsyncWork();
    });

    expect(putReaderState).toHaveBeenCalledTimes(2);
    expect(putReaderState).toHaveBeenNthCalledWith(2, expect.objectContaining({ novelId: "novel-a", position: 90 }));

    await act(async () => {
      root?.unmount();
    });
  });

  it("does not reopen a resolved sync conflict when a delayed 409 returns the same version", async () => {
    installDom();
    const initialState = createReaderState({ stateVersion: 1, position: 12 });
    const conflictState = createReaderState({ stateVersion: 2, position: 80, updatedByClientId: "other-client" });
    let rejectPendingSave: ((error: Error) => void) | null = null;
    vi.mocked(fetchNovelContext).mockResolvedValue({
      toc: createToc(),
      readerState: initialState,
      bookmarks: [],
      readerSettings: createReaderSettings()
    });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());
    vi.mocked(fetchReaderState).mockResolvedValue(conflictState);
    vi.mocked(putReaderState)
      .mockReset()
      .mockReturnValueOnce(
        new Promise((_resolve, reject) => {
          rejectPendingSave = reject;
        })
      );

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    const pendingSave = latest?.putReaderState({
      novelId: "novel-a",
      episodeIndex: "2",
      position: 30
    });
    await flushAsyncWork();

    await act(async () => {
      window.dispatchEvent(new window.Event("focus"));
      await flushAsyncWork();
    });
    expect(latest?.readerSyncConflict?.serverState).toEqual(conflictState);

    await act(async () => {
      latest?.setReaderState(conflictState);
      latest?.setReaderSyncConflict(null);
      await flushAsyncWork();
    });

    let saveError: unknown;
    await act(async () => {
      rejectPendingSave?.(new ReaderStateConflictError(conflictState));
      try {
        await pendingSave;
      } catch (error) {
        saveError = error;
      }
      await flushAsyncWork();
    });

    expect(saveError).toBeInstanceOf(ReaderStateConflictError);
    expect(latest?.readerSyncConflict).toBeNull();

    await act(async () => {
      root?.unmount();
    });
  });

  it("keeps a newer unresolved sync conflict when an older delayed save succeeds", async () => {
    installDom();
    const initialState = createReaderState({ stateVersion: 1, position: 12 });
    const savedState = createReaderState({
      stateVersion: 2,
      position: 30,
      updatedByClientId: "client-a"
    });
    const conflictState = createReaderState({
      stateVersion: 3,
      position: 80,
      updatedByClientId: "other-client"
    });
    let resolvePendingSave: ((state: ReaderState) => void) | null = null;
    vi.mocked(fetchNovelContext).mockResolvedValue({
      toc: createToc(),
      readerState: initialState,
      bookmarks: [],
      readerSettings: createReaderSettings()
    });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());
    vi.mocked(fetchReaderState).mockResolvedValue(conflictState);
    vi.mocked(putReaderState)
      .mockReset()
      .mockReturnValueOnce(
        new Promise((resolve) => {
          resolvePendingSave = resolve;
        })
      );

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    const pendingSave = latest?.putReaderState({
      novelId: "novel-a",
      episodeIndex: "2",
      position: 30
    });
    await flushAsyncWork();
    expect(putReaderState).toHaveBeenCalledTimes(1);

    await act(async () => {
      window.dispatchEvent(new window.Event("focus"));
      await flushAsyncWork();
    });
    expect(latest?.readerSyncConflict?.serverState).toEqual(conflictState);

    await act(async () => {
      resolvePendingSave?.(savedState);
      await pendingSave;
      await flushAsyncWork();
    });

    expect(latest?.readerSyncConflict?.serverState).toEqual(conflictState);

    let saveError: unknown;
    await act(async () => {
      try {
        await latest?.putReaderState({
          novelId: "novel-a",
          episodeIndex: "2",
          position: 35
        });
      } catch (error) {
        saveError = error;
      }
      await flushAsyncWork();
    });

    expect(saveError).toBeInstanceOf(Error);
    expect((saveError as Error).name).toBe("AbortError");
    expect(putReaderState).toHaveBeenCalledTimes(1);

    await act(async () => {
      root?.unmount();
    });
  });

  it("ignores stale reader states passed through the external setter", async () => {
    installDom();
    const currentState = createReaderState({ stateVersion: 3, position: 80 });
    const staleState = createReaderState({ stateVersion: 2, position: 30 });
    vi.mocked(fetchNovelContext).mockResolvedValue({
      toc: createToc(),
      readerState: currentState,
      bookmarks: [],
      readerSettings: createReaderSettings()
    });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    expect(latest?.readerState).toEqual(currentState);

    await act(async () => {
      latest?.setReaderState(staleState);
      await flushAsyncWork();
    });

    expect(latest?.readerState).toEqual(currentState);

    await act(async () => {
      root?.unmount();
    });
  });

  it("reloads the current episode when novel reader correction settings change", async () => {
    installDom();
    vi.mocked(fetchNovelContext).mockResolvedValue({
      toc: createToc(),
      readerState: createReaderState(),
      bookmarks: [],
      readerSettings: createReaderSettings()
    });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    const initialFetchCount = vi.mocked(fetchEpisode).mock.calls.length;
    expect(initialFetchCount).toBeGreaterThan(0);

    await act(async () => {
      latest?.setReaderSettings(createReaderSettings({ quoteNormalization: false }));
      await flushAsyncWork();
    });

    expect(fetchEpisode).toHaveBeenCalledTimes(initialFetchCount + 1);

    await act(async () => {
      root?.unmount();
    });
  });

  it("does not reload the current episode when reader settings belong to another novel", async () => {
    installDom();
    vi.mocked(fetchNovelContext).mockResolvedValue({
      toc: createToc(),
      readerState: createReaderState(),
      bookmarks: [],
      readerSettings: createReaderSettings()
    });
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    const initialFetchCount = vi.mocked(fetchEpisode).mock.calls.length;
    expect(initialFetchCount).toBeGreaterThan(0);

    await act(async () => {
      latest?.setReaderSettings(createReaderSettings({ quoteNormalization: true }, "novel-b"));
      await flushAsyncWork();
    });

    expect(fetchEpisode).toHaveBeenCalledTimes(initialFetchCount);

    await act(async () => {
      root?.unmount();
    });
  });

  it("does not overwrite newer reader settings with an older novel context response", async () => {
    installDom();
    let resolveNovelContext: ((value: Awaited<ReturnType<typeof fetchNovelContext>>) => void) | null = null;
    vi.mocked(fetchNovelContext).mockReturnValue(
      new Promise((resolve) => {
        resolveNovelContext = resolve;
      })
    );
    vi.mocked(fetchEpisode).mockResolvedValue(createEpisode());

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    await act(async () => {
      latest?.setReaderSettings(createReaderSettings({ quoteNormalization: true }));
      resolveNovelContext?.({
        toc: createToc(),
        readerState: createReaderState(),
        bookmarks: [],
        readerSettings: createReaderSettings({ quoteNormalization: false })
      });
      await flushAsyncWork();
    });

    expect(latest?.readerSettings?.correction.quoteNormalization).toBe(true);

    await act(async () => {
      root?.unmount();
    });
  });
});
