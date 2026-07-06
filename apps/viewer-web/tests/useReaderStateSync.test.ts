import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, createElement, useRef, useState, type MutableRefObject } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import { fetchReaderState, putReaderState, ReaderStateConflictError } from "../src/features/reader/api";
import type { NovelSummary } from "../src/features/library/types";
import type { EpisodeIndex, ReaderState } from "../src/features/reader/types";
import { useReaderStateSync } from "../src/hooks/useReaderStateSync";
import type { ScreenMode } from "../src/hooks/readerStateSyncCore";

vi.mock("../src/features/reader/api", () => ({
  fetchReaderState: vi.fn(),
  ReaderStateConflictError: class ReaderStateConflictError extends Error {
    constructor(readonly serverState: ReaderState) {
      super("conflict");
    }
  },
  putReaderState: vi.fn()
}));

type HookResult = ReturnType<typeof useReaderStateSync>;

type HarnessControls = {
  pendingReadingStateKeyRef: MutableRefObject<string | null>;
  setScreenMode: (screenMode: ScreenMode) => void;
  setSelectedEpisodeIndex: (episodeIndex: EpisodeIndex | null) => void;
  setSelectedNovelId: (novelId: string | null) => void;
  setSelectedPosition: (position: number | null) => void;
};

let mountedRoot: Root | null = null;

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

function createReaderState(overrides: Partial<ReaderState> = {}): ReaderState {
  return {
    novelId: "novel-a",
    lastReadEpisodeIndex: "2",
    position: 12,
    updatedAt: "2026-06-15T00:00:00.000Z",
    stateVersion: 1,
    updatedByClientId: "other-client",
    ...overrides
  };
}

function createNovel(novelId: string, overrides: Partial<NovelSummary> = {}): NovelSummary {
  return {
    novelId,
    fetcherWorkId: novelId,
    title: "作品",
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

function renderHookHarness(props: {
  initialSelectedNovelId?: string | null;
  onRender: (result: HookResult, controls: HarnessControls, novels: NovelSummary[]) => void;
}): Root {
  const rootElement = document.getElementById("root");
  if (!rootElement) {
    throw new Error("root element is missing");
  }

  function Harness() {
    const [novels, setNovels] = useState<NovelSummary[]>([createNovel("novel-a"), createNovel("novel-b")]);
    const [screenMode, setScreenMode] = useState<ScreenMode>("reader");
    const [selectedEpisodeIndex, setSelectedEpisodeIndex] = useState<EpisodeIndex | null>("2");
    const [selectedNovelId, setSelectedNovelId] = useState<string | null>(props.initialSelectedNovelId ?? "novel-a");
    const [selectedPosition, setSelectedPosition] = useState<number | null>(12);
    const pendingReadingStateKeyRef = useRef<string | null>(null);
    const screenModeRef = useRef<ScreenMode>(screenMode);
    const selectedEpisodeIndexRef = useRef<EpisodeIndex | null>(selectedEpisodeIndex);
    const selectedNovelIdRef = useRef<string | null>(selectedNovelId);
    const selectedPositionRef = useRef<number | null>(selectedPosition);

    screenModeRef.current = screenMode;
    selectedEpisodeIndexRef.current = selectedEpisodeIndex;
    selectedNovelIdRef.current = selectedNovelId;
    selectedPositionRef.current = selectedPosition;

    const result = useReaderStateSync({
      pendingReadingStateKeyRef,
      readerClientId: "client-a",
      screenMode,
      screenModeRef,
      selectedEpisodeIndexRef,
      selectedNovelId,
      selectedNovelIdRef,
      selectedPositionRef,
      setNovels
    });
    props.onRender(
      result,
      {
        pendingReadingStateKeyRef,
        setScreenMode,
        setSelectedEpisodeIndex,
        setSelectedNovelId,
        setSelectedPosition
      },
      novels
    );
    return null;
  }

  const root = createRoot(rootElement);
  mountedRoot = root;
  root.render(createElement(Harness));
  return root;
}

describe("useReaderStateSync", () => {
  beforeEach(() => {
    vi.mocked(fetchReaderState).mockReset();
    vi.mocked(putReaderState).mockReset();
  });

  afterEach(async () => {
    await act(async () => {
      try {
        mountedRoot?.unmount();
      } catch {
        // A test may have already unmounted the root during its own cleanup path.
      }
    });
    mountedRoot = null;
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("serializes per-novel saves and uses the latest acknowledged version", async () => {
    installDom();
    const initialState = createReaderState({ stateVersion: 1, position: 12 });
    let resolveFirstSave: ((state: ReaderState) => void) | null = null;
    vi.mocked(putReaderState)
      .mockReturnValueOnce(
        new Promise((resolve) => {
          resolveFirstSave = resolve;
        })
      )
      .mockResolvedValueOnce(createReaderState({ stateVersion: 3, position: 20, updatedByClientId: "client-a" }));

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
      latest?.setReaderState(initialState);
      await flushAsyncWork();
    });

    const firstSave = latest?.putReaderState({ novelId: "novel-a", episodeIndex: "2", position: 10 });
    const secondSave = latest?.putReaderState({ novelId: "novel-a", episodeIndex: "2", position: 20 });

    await flushAsyncWork();
    expect(putReaderState).toHaveBeenCalledTimes(1);
    expect(putReaderState).toHaveBeenNthCalledWith(
      1,
      expect.objectContaining({ expectedStateVersion: 1, novelId: "novel-a", position: 10 })
    );

    await act(async () => {
      resolveFirstSave?.(createReaderState({ stateVersion: 2, position: 10, updatedByClientId: "client-a" }));
      await firstSave;
      await flushAsyncWork();
    });

    expect(putReaderState).toHaveBeenCalledTimes(2);
    expect(putReaderState).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({ expectedStateVersion: 2, novelId: "novel-a", position: 20 })
    );

    await act(async () => {
      await secondSave;
      root?.unmount();
    });
  });

  it("keeps delayed save responses and acknowledged versions scoped to each novel", async () => {
    installDom();
    const stateA = createReaderState({ novelId: "novel-a", lastReadEpisodeIndex: "2", position: 12, stateVersion: 1 });
    const stateB = createReaderState({ novelId: "novel-b", lastReadEpisodeIndex: "1", position: 70, stateVersion: 7 });
    let resolveSaveA: ((state: ReaderState) => void) | null = null;
    let resolveSaveB: ((state: ReaderState) => void) | null = null;
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
    let controls: HarnessControls | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result, nextControls) => {
          latest = result;
          controls = nextControls;
        }
      });
      await flushAsyncWork();
    });
    await act(async () => {
      latest?.setReaderState(stateA);
      await flushAsyncWork();
    });

    const saveA = latest?.putReaderState({ novelId: "novel-a", episodeIndex: "2", position: 10 });
    await flushAsyncWork();
    expect(putReaderState).toHaveBeenNthCalledWith(1, expect.objectContaining({ expectedStateVersion: 1, position: 10 }));

    await act(async () => {
      controls?.setSelectedNovelId("novel-b");
      controls?.setSelectedEpisodeIndex("1");
      controls?.setSelectedPosition(70);
      latest?.setReaderState(stateB);
      await flushAsyncWork();
    });

    const saveB = latest?.putReaderState({ novelId: "novel-b", episodeIndex: "1", position: 80 });
    await flushAsyncWork();
    expect(putReaderState).toHaveBeenNthCalledWith(2, expect.objectContaining({ expectedStateVersion: 7, position: 80 }));

    await act(async () => {
      resolveSaveA?.(createReaderState({ novelId: "novel-a", stateVersion: 2, position: 10, updatedByClientId: "client-a" }));
      await saveA;
      await flushAsyncWork();
    });
    expect(latest?.readerState?.novelId).toBe("novel-b");
    expect(latest?.readerState?.stateVersion).toBe(7);

    await act(async () => {
      resolveSaveB?.(
        createReaderState({
          novelId: "novel-b",
          lastReadEpisodeIndex: "1",
          stateVersion: 8,
          position: 80,
          updatedByClientId: "client-a"
        })
      );
      await saveB;
      root?.unmount();
    });
  });

  it("invalidates in-flight saves when the reader state generation is reset", async () => {
    installDom();
    const deletedState = createReaderState({ lastReadEpisodeIndex: "2", position: 55, stateVersion: 5 });
    const reloadedState = createReaderState({ lastReadEpisodeIndex: "1", position: 0, stateVersion: 0, updatedByClientId: null });
    let resolveOldSave: ((state: ReaderState) => void) | null = null;
    vi.mocked(putReaderState)
      .mockReturnValueOnce(
        new Promise((resolve) => {
          resolveOldSave = resolve;
        })
      )
      .mockResolvedValueOnce(createReaderState({ lastReadEpisodeIndex: "1", position: 5, stateVersion: 1, updatedByClientId: "client-a" }));

    let latest: HookResult | null = null;
    let controls: HarnessControls | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result, nextControls) => {
          latest = result;
          controls = nextControls;
        }
      });
      await flushAsyncWork();
    });
    await act(async () => {
      latest?.setReaderState(deletedState);
      await flushAsyncWork();
    });

    const oldSave = latest?.putReaderState({ novelId: "novel-a", episodeIndex: "2", position: 60 });
    await flushAsyncWork();
    expect(putReaderState).toHaveBeenNthCalledWith(1, expect.objectContaining({ expectedStateVersion: 5, position: 60 }));

    await act(async () => {
      latest?.resetReaderStateCache("novel-a");
      controls?.setSelectedEpisodeIndex("1");
      controls?.setSelectedPosition(0);
      latest?.setReaderState(reloadedState);
      await flushAsyncWork();
    });

    await act(async () => {
      await latest?.putReaderState({ novelId: "novel-a", episodeIndex: "1", position: 5 });
      await flushAsyncWork();
    });
    expect(putReaderState).toHaveBeenNthCalledWith(2, expect.objectContaining({ expectedStateVersion: 0, position: 5 }));

    let oldSaveError: unknown;
    await act(async () => {
      resolveOldSave?.(createReaderState({ stateVersion: 6, position: 60, updatedByClientId: "client-a" }));
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

    await act(async () => {
      root?.unmount();
    });
  });

  it("blocks queued saves after a sync conflict until the conflict is resolved", async () => {
    installDom();
    const initialState = createReaderState({ stateVersion: 1, position: 12 });
    const conflictState = createReaderState({ stateVersion: 2, position: 80, updatedByClientId: "other-client" });
    let rejectFirstSave: ((error: Error) => void) | null = null;
    vi.mocked(putReaderState)
      .mockReturnValueOnce(
        new Promise((_resolve, reject) => {
          rejectFirstSave = reject;
        })
      )
      .mockResolvedValueOnce(createReaderState({ stateVersion: 3, position: 90, updatedByClientId: "client-a" }));

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
      latest?.setReaderState(initialState);
      await flushAsyncWork();
    });

    const firstSave = latest?.putReaderState({ novelId: "novel-a", episodeIndex: "2", position: 30 });
    const queuedSave = latest?.putReaderState({ novelId: "novel-a", episodeIndex: "2", position: 40 });

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
    expect(latest?.readerSyncConflict?.serverState).toEqual(conflictState);
    expect(putReaderState).toHaveBeenCalledTimes(1);

    await act(async () => {
      latest?.setReaderState(conflictState);
      latest?.setReaderSyncConflict(null);
      await flushAsyncWork();
    });
    await act(async () => {
      await latest?.putReaderState({ novelId: "novel-a", episodeIndex: "2", position: 90 });
      root?.unmount();
    });
    expect(putReaderState).toHaveBeenCalledTimes(2);
  });

  it("keeps only the actively sent save in the pending reading state key", async () => {
    installDom();
    let rejectFirstSave: ((error: Error) => void) | null = null;
    vi.mocked(putReaderState)
      .mockReturnValueOnce(
        new Promise((_resolve, reject) => {
          rejectFirstSave = reject;
        })
      )
      .mockResolvedValueOnce(createReaderState({ stateVersion: 2, position: 20, updatedByClientId: "client-a" }));

    let latest: HookResult | null = null;
    let controls: HarnessControls | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result, nextControls) => {
          latest = result;
          controls = nextControls;
        }
      });
      await flushAsyncWork();
    });
    await act(async () => {
      latest?.setReaderState(createReaderState());
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
    expect(controls?.pendingReadingStateKeyRef.current).toBe("novel-a:2:10");
    const replacementSave = latest?.putReaderState({
      novelId: "novel-a",
      episodeIndex: "2",
      position: 20,
      readingStateKey: "novel-a:2:20"
    });

    let queuedError: unknown;
    await act(async () => {
      rejectFirstSave?.(new Error("failed to save first position"));
      await expect(firstSave).rejects.toThrow("failed to save first position");
      try {
        await queuedSave;
      } catch (error) {
        queuedError = error;
      }
      await flushAsyncWork();
    });

    expect(queuedError).toBeInstanceOf(Error);
    expect((queuedError as Error).name).toBe("AbortError");
    expect(putReaderState).toHaveBeenCalledTimes(2);

    await act(async () => {
      await replacementSave;
      await flushAsyncWork();
    });
    expect(controls?.pendingReadingStateKeyRef.current).toBeNull();

    await act(async () => {
      root?.unmount();
    });
  });

  it("reconciles incoming context states inside the sync boundary", async () => {
    installDom();
    const initialState = createReaderState({ lastReadEpisodeIndex: "2", position: 20, stateVersion: 1 });
    const externalState = createReaderState({
      lastReadEpisodeIndex: "2",
      position: 80,
      stateVersion: 2,
      updatedByClientId: "other-client"
    });

    let latest: HookResult | null = null;
    let controls: HarnessControls | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result, nextControls) => {
          latest = result;
          controls = nextControls;
        }
      });
      await flushAsyncWork();
    });
    await act(async () => {
      latest?.setReaderState(initialState);
      controls?.setSelectedPosition(20);
      await flushAsyncWork();
    });

    let result: ReturnType<HookResult["reconcileIncomingReaderState"]> | null = null;
    await act(async () => {
      result = latest?.reconcileIncomingReaderState(externalState) ?? null;
      await flushAsyncWork();
    });

    expect(result?.disposition).toBe("conflict");
    expect(result?.activeReaderState).toEqual(initialState);
    expect(latest?.readerSyncConflict?.serverState).toEqual(externalState);
    expect(latest?.readerState).toEqual(initialState);

    await act(async () => {
      root?.unmount();
    });
  });

  it("syncs newer server states on focus and visibilitychange", async () => {
    installDom();
    const initialState = createReaderState({ lastReadEpisodeIndex: "2", position: 10, stateVersion: 1 });
    const matchingVisiblePosition = createReaderState({
      lastReadEpisodeIndex: "2",
      position: 20,
      stateVersion: 2,
      updatedByClientId: "other-client"
    });
    const conflictingVisiblePosition = createReaderState({
      lastReadEpisodeIndex: "2",
      position: 30,
      stateVersion: 3,
      updatedByClientId: "other-client"
    });
    vi.mocked(fetchReaderState).mockResolvedValueOnce(matchingVisiblePosition).mockResolvedValueOnce(conflictingVisiblePosition);

    let latest: HookResult | null = null;
    let controls: HarnessControls | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result, nextControls) => {
          latest = result;
          controls = nextControls;
        }
      });
      await flushAsyncWork();
    });
    await act(async () => {
      latest?.setReaderState(initialState);
      controls?.setSelectedPosition(20);
      await flushAsyncWork();
    });

    await act(async () => {
      window.dispatchEvent(new window.Event("focus"));
      await flushAsyncWork();
    });

    expect(latest?.readerSyncConflict).toBeNull();
    expect(latest?.readerState).toEqual(matchingVisiblePosition);

    await act(async () => {
      document.dispatchEvent(new window.Event("visibilitychange"));
      await flushAsyncWork();
    });

    expect(latest?.readerSyncConflict?.serverState).toEqual(conflictingVisiblePosition);
    expect(latest?.readerState).toEqual(matchingVisiblePosition);

    await act(async () => {
      root?.unmount();
    });
  });
});
