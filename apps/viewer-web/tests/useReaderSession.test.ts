import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import type { NovelReaderSettingsResponse } from "../src/features/reader/types";
import { putNovelReaderSettings } from "../src/features/reader/api";
import { useReaderSession } from "../src/features/reader/useReaderSession";

vi.mock("../src/features/reader/api", () => ({
  putNovelReaderSettings: vi.fn()
}));

const setReaderSettings = vi.fn();

vi.mock("../src/hooks/useReaderRouteSync", () => ({
  useReaderRouteSync: vi.fn()
}));

vi.mock("../src/hooks/useReaderState", () => ({
  useReaderState: vi.fn(() => ({
    bookmarks: [],
    episode: null,
    isEpisodeLoading: false,
    isNovelLoading: false,
    isReaderLoadingOverlayVisible: false,
    readerSyncConflictResolutionState: "idle",
    openSelectedNovelInReaderRef: { current: false },
    pendingReadingStateKeyRef: { current: null },
    putReaderState: vi.fn(),
    resetReaderStateCache: vi.fn(),
    readerState: null,
    readerSettings: {
      novelId: "novel-a",
      correction: {
        quoteNormalization: true,
        hyphenDashNormalization: true,
        parenthesisNormalization: true,
        halfwidthAlnumPunctuationNormalization: true
      },
      updatedAt: null
    },
    readerSyncConflict: null,
    screenMode: "library",
    screenModeRef: { current: "library" },
    selectedEpisodeIndex: null,
    selectedEpisodeIndexRef: { current: null },
    selectedPosition: null,
    selectedPositionRef: { current: null },
    setBookmarks: vi.fn(),
    setReaderSyncConflictResolutionState: vi.fn(),
    setReaderState: vi.fn(),
    setReaderSettings,
    setReaderSyncConflict: vi.fn(),
    setScreenMode: vi.fn(),
    setSelectedEpisodeIndex: vi.fn(),
    setSelectedPosition: vi.fn(),
    toc: null
  }))
}));

type HookResult = ReturnType<typeof useReaderSession>;

let mountedRoot: Root | null = null;

function installDom(): JSDOM {
  const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
    url: "http://localhost/"
  });

  vi.stubGlobal("window", dom.window);
  vi.stubGlobal("document", dom.window.document);
  vi.stubGlobal("navigator", dom.window.navigator);
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);

  return dom;
}

function createSettings(updatedAt: string): NovelReaderSettingsResponse {
  return {
    novelId: "novel-a",
    correction: {
      quoteNormalization: false,
      hyphenDashNormalization: true,
      parenthesisNormalization: true,
      halfwidthAlnumPunctuationNormalization: true
    },
    updatedAt
  };
}

function deferred<T>(): {
  promise: Promise<T>;
  resolve: (value: T) => void;
} {
  let resolve: (value: T) => void = () => undefined;
  const promise = new Promise<T>((nextResolve) => {
    resolve = nextResolve;
  });
  return { promise, resolve };
}

async function flushAsyncWork(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
}

function renderHookHarness(props: {
  onRender: (result: HookResult) => void;
  selectedNovelId: string | null;
}): { rerender: (selectedNovelId: string | null) => void; root: Root } {
  const rootElement = document.getElementById("root");
  if (!rootElement) {
    throw new Error("root element is missing");
  }

  function Harness({ selectedNovelId }: { selectedNovelId: string | null }) {
    const result = useReaderSession({
      initialEpisodeIndex: null,
      initialPosition: null,
      initialScreenMode: "library",
      libraryReloadKey: 0,
      onError: vi.fn(),
      readerClientId: "reader-client",
      selectedNovelId,
      setNovels: vi.fn(),
      setSelectedNovelId: vi.fn()
    });

    props.onRender(result);
    return createElement("output");
  }

  const root = createRoot(rootElement);
  act(() => {
    root.render(createElement(Harness, { selectedNovelId: props.selectedNovelId }));
  });
  mountedRoot = root;
  return {
    root,
    rerender: (selectedNovelId: string | null) => {
      act(() => {
        root.render(createElement(Harness, { selectedNovelId }));
      });
    }
  };
}

afterEach(() => {
  if (mountedRoot) {
    act(() => mountedRoot?.unmount());
    mountedRoot = null;
  }
  setReaderSettings.mockReset();
  vi.mocked(putNovelReaderSettings).mockReset();
  vi.unstubAllGlobals();
});

describe("useReaderSession", () => {
  it("ignores reader correction save responses after switching novels", async () => {
    installDom();
    const save = deferred<NovelReaderSettingsResponse>();
    vi.mocked(putNovelReaderSettings).mockReturnValueOnce(save.promise);
    let latest: HookResult | null = null;

    const { rerender } = renderHookHarness({
      selectedNovelId: "novel-a",
      onRender: (result) => {
        latest = result;
      }
    });
    act(() => {
      latest?.commands.changeNovelReaderCorrection({ quoteNormalization: false });
    });
    rerender("novel-b");

    save.resolve(createSettings("older"));
    await act(async () => {
      await flushAsyncWork();
    });

    expect(setReaderSettings).not.toHaveBeenCalled();
  });

  it("applies only the latest reader correction save response for the current novel", async () => {
    installDom();
    const firstSave = deferred<NovelReaderSettingsResponse>();
    const secondSave = deferred<NovelReaderSettingsResponse>();
    vi.mocked(putNovelReaderSettings).mockReturnValueOnce(firstSave.promise).mockReturnValueOnce(secondSave.promise);
    let latest: HookResult | null = null;

    renderHookHarness({
      selectedNovelId: "novel-a",
      onRender: (result) => {
        latest = result;
      }
    });

    act(() => {
      latest?.commands.changeNovelReaderCorrection({ quoteNormalization: false });
      latest?.commands.changeNovelReaderCorrection({ quoteNormalization: true });
    });

    secondSave.resolve(createSettings("newer"));
    await act(async () => {
      await flushAsyncWork();
    });
    firstSave.resolve(createSettings("older"));
    await act(async () => {
      await flushAsyncWork();
    });

    expect(setReaderSettings).toHaveBeenCalledTimes(1);
    expect(setReaderSettings).toHaveBeenCalledWith(expect.objectContaining({ updatedAt: "newer" }));
  });
});
