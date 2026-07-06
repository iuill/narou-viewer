import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement, useRef, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import type { TocEpisode } from "../src/features/reader/types";
import { useReaderCommands } from "../src/screens/reader/useReaderCommands";

vi.mock("../src/screens/reader/useReaderControlActions", () => ({
  useReaderControlActions: () => ({
    readerOverflowActions: [{ id: "overflow" }],
    readerVisibleActions: [{ id: "visible" }]
  })
}));

type HookResult = ReturnType<typeof useReaderCommands>;
type HookOptions = Parameters<typeof useReaderCommands>[0];
const mountedRoots = new Set<Root>();

function installDom(): JSDOM {
  const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div><button id=\"focus-source\"></button></body></html>", {
    url: "http://localhost/"
  });
  vi.stubGlobal("window", dom.window);
  vi.stubGlobal("document", dom.window.document);
  vi.stubGlobal("navigator", dom.window.navigator);
  vi.stubGlobal("HTMLElement", dom.window.HTMLElement);
  vi.stubGlobal("Element", dom.window.Element);
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  Object.defineProperty(dom.window, "requestAnimationFrame", {
    configurable: true,
    value: (callback: FrameRequestCallback) => {
      setTimeout(() => callback(0), 0);
      return 1;
    }
  });
  Object.defineProperty(dom.window, "cancelAnimationFrame", {
    configurable: true,
    value: vi.fn()
  });
  return dom;
}

async function flushAsyncWork(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await new Promise((resolve) => setTimeout(resolve, 0));
}

function createEpisode(index: string, bodyStatus = "complete"): TocEpisode {
  return {
    bodyStatus,
    chapter: null,
    contentEtag: `toc-${index}`,
    episodeIndex: index,
    sourceUrl: null,
    subchapter: null,
    title: `第${index}話`,
    updatedAt: null
  };
}

function renderHookHarness(
  overrides: Partial<HookOptions>,
  onRender: (result: HookResult, pending: TocEpisode | null, notices: string[]) => void
): Root {
  const rootElement = document.getElementById("root");
  if (!rootElement) {
    throw new Error("root element is missing");
  }
  const notices: string[] = [];

  function Harness() {
    const [pending, setPending] = useState<TocEpisode | null>(overrides.pendingNextEpisodeConfirmation ?? null);
    const viewportRef = useRef<HTMLDivElement | null>({
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      getBoundingClientRect: () => ({ left: 0, width: 200 }) as DOMRect
    } as unknown as HTMLDivElement);
    const returnFocusRef = useRef<HTMLElement | null>(null);
    const primaryRef = useRef<HTMLButtonElement | null>({ focus: vi.fn() } as unknown as HTMLButtonElement);
    const options: HookOptions = {
      canMoveForwardReaderPage: true,
      canOpenNextEpisode: true,
      canOpenPreviousEpisode: true,
      canUseNextPageButton: true,
      canUsePreviousPageButton: true,
      canUseReaderPageActions: true,
      closeReaderPanel: vi.fn(),
      edgeTapPageMoveDirections: { next: 1, previous: -1 },
      episode: {},
      episodeContentEtag: "etag",
      handleCreateBookmark: vi.fn(),
      handleOpenCharacterSummary: vi.fn(),
      handleReturnToLibrary: vi.fn(),
      handleToggleReaderFullscreen: vi.fn(),
      hasUnlistedEpisodes: false,
      isBookmarkSaving: false,
      isCharacterSummaryOpen: false,
      isEpisodeLoading: false,
      isReaderAiAssistantAvailable: true,
      isReaderAiAssistantOpen: false,
      isReaderAiAssistantUnavailableMessage: null,
      isReaderBookmarksOpen: false,
      isReaderExperimentalFontOpen: false,
      isReaderFullscreen: false,
      isReaderInfoOpen: false,
      isReaderKeyboardPagingBlocked: false,
      isReaderModalOpen: false,
      isReaderSettingsOpen: false,
      isReaderSpeechOpen: false,
      isReaderSpeechPaused: false,
      isReaderSpeechPlaying: false,
      isReaderTocOpen: false,
      isTouchDevice: false,
      movePage: vi.fn(),
      nextEpisode: createEpisode("2"),
      nextEpisodeConfirmPrimaryButtonRef: primaryRef,
      nextEpisodeConfirmReturnFocusRef: returnFocusRef,
      nextPageActionLabel: "次ページ",
      pageMoveDirections: { next: 1, previous: -1 },
      pendingNextEpisodeConfirmation: pending,
      previousEpisode: createEpisode("0"),
      previousPageActionLabel: "前ページ",
      readerCommands: {
        clearSelection: vi.fn(),
        openEpisode: vi.fn(() => true),
        returnToLibrary: vi.fn(),
        selectNovel: vi.fn(),
        updateSelectedPosition: vi.fn()
      },
      readerControlViewportWidth: 800,
      readerForwardPageDirection: 1,
      readerPageIndicatorWidth: 80,
      readerViewportRef: viewportRef,
      screenMode: "reader",
      setIsReaderOverflowOpen: vi.fn(),
      setPendingNextEpisodeConfirmation: setPending,
      setReaderNotice: (nextNotice) => {
        if (typeof nextNotice === "function") {
          return;
        }
        if (nextNotice) {
          notices.push(nextNotice);
        }
      },
      setReaderSpeechDebugHighlight: vi.fn(),
      setReaderSpeechEnabled: vi.fn(),
      setReaderSpeechPreferRubyText: vi.fn(),
      setReaderSpeechRate: vi.fn(),
      setReaderSpeechVoiceUri: vi.fn(),
      shouldShowReaderSpeechControls: true,
      stopReaderSpeech: vi.fn(),
      toggleReaderPanel: vi.fn()
    };
    Object.assign(options, overrides, {
      pendingNextEpisodeConfirmation: pending,
      readerViewportRef: overrides.readerViewportRef ?? viewportRef,
      nextEpisodeConfirmPrimaryButtonRef: overrides.nextEpisodeConfirmPrimaryButtonRef ?? primaryRef,
      nextEpisodeConfirmReturnFocusRef: overrides.nextEpisodeConfirmReturnFocusRef ?? returnFocusRef,
      setPendingNextEpisodeConfirmation: setPending
    });
    const result = useReaderCommands(options);

    onRender(result, pending, notices);
    return null;
  }

  const root = createRoot(rootElement);
  mountedRoots.add(root);
  act(() => {
    root.render(createElement(Harness));
  });
  return root;
}

async function unmountHookRoot(root: Root): Promise<void> {
  await act(async () => {
    root.unmount();
    mountedRoots.delete(root);
    await flushAsyncWork();
  });
}

describe("useReaderCommands", () => {
  afterEach(async () => {
    await act(async () => {
      for (const root of mountedRoots) {
        root.unmount();
      }
      mountedRoots.clear();
      await flushAsyncWork();
    });
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("handles keyboard paging, edge click paging, and terminal next-page notices", async () => {
    const dom = installDom();
    const movePage = vi.fn();
    let latest: HookResult | null = null;
    let latestNotices: string[] = [];
    const root = renderHookHarness(
      {
        canMoveForwardReaderPage: false,
        canOpenNextEpisode: false,
        hasUnlistedEpisodes: true,
        movePage,
        nextEpisode: createEpisode("2", "missing")
      },
      (result, _pending, notices) => {
        latest = result;
        latestNotices = notices;
      }
    );
    await act(async () => {
      await flushAsyncWork();
    });

    await act(async () => {
      window.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true }));
      await flushAsyncWork();
    });
    expect(latestNotices).toContain("次の話はまだ本文が取得されていません。再開して取得してください。");
    expect(movePage).not.toHaveBeenCalled();

    await act(async () => {
      latest?.handleViewportClick({
        clientX: 4,
        currentTarget: {
          getBoundingClientRect: () => ({ left: 0, width: 200 })
        }
      } as never);
      await flushAsyncWork();
    });
    expect(movePage).toHaveBeenCalledWith(-1);

    await unmountHookRoot(root);
  });

  it("confirms next episodes, traps focus, opens bookmarks, and resets speech settings", async () => {
    installDom();
    const openEpisode = vi.fn(() => true);
    const stopReaderSpeech = vi.fn().mockResolvedValue(undefined);
    const setReaderSpeechEnabled = vi.fn();
    const setReaderSpeechRate = vi.fn();
    const setReaderSpeechVoiceUri = vi.fn();
    const setReaderSpeechPreferRubyText = vi.fn();
    const setReaderSpeechDebugHighlight = vi.fn();
    const closeReaderPanel = vi.fn();
    let latest: HookResult | null = null;
    let latestPending: TocEpisode | null = null;
    const root = renderHookHarness(
      {
        pendingNextEpisodeConfirmation: createEpisode("2"),
        readerCommands: {
          clearSelection: vi.fn(),
          openEpisode,
          returnToLibrary: vi.fn(),
          selectNovel: vi.fn(),
          updateSelectedPosition: vi.fn()
        },
        closeReaderPanel,
        stopReaderSpeech,
        setReaderSpeechDebugHighlight,
        setReaderSpeechEnabled,
        setReaderSpeechPreferRubyText,
        setReaderSpeechRate,
        setReaderSpeechVoiceUri
      },
      (result, pending) => {
        latest = result;
        latestPending = pending;
      }
    );
    await act(async () => {
      await flushAsyncWork();
    });

    await act(async () => {
      latest?.handleConfirmNextEpisode();
      await flushAsyncWork();
    });
    expect(openEpisode).toHaveBeenCalledWith("2");
    expect(latestPending).toBeNull();

    await act(async () => {
      latest?.handleOpenBookmark({ id: "b", novelId: "n", episodeIndex: "1", position: 5, label: null, createdAt: "2026-07-01T00:00:00.000Z" });
      await flushAsyncWork();
    });
    expect(closeReaderPanel).toHaveBeenCalled();
    expect(openEpisode).toHaveBeenCalledWith("1", 5);

    latest?.handleResetReaderSpeechPreferences();
    expect(setReaderSpeechEnabled).toHaveBeenCalledWith(true);
    expect(setReaderSpeechRate).toHaveBeenCalledWith(1);
    expect(setReaderSpeechVoiceUri).toHaveBeenCalledWith(null);
    expect(setReaderSpeechPreferRubyText).toHaveBeenCalledWith(true);
    expect(setReaderSpeechDebugHighlight).toHaveBeenCalledWith(false);
    expect(stopReaderSpeech).toHaveBeenCalledWith({ notice: "読み上げ設定を初期化しました。" });

    await unmountHookRoot(root);
  });

  it("opens next episode confirmation and handles final-page notice variants", async () => {
    const dom = installDom();
    let latest: HookResult | null = null;
    let latestPending: TocEpisode | null = null;
    let latestNotices: string[] = [];
    const closeReaderPanel = vi.fn();
    const setOverflow = vi.fn();
    const root = renderHookHarness(
      {
        canMoveForwardReaderPage: false,
        canOpenNextEpisode: true,
        closeReaderPanel,
        hasUnlistedEpisodes: false,
        nextEpisode: createEpisode("2"),
        setIsReaderOverflowOpen: setOverflow
      },
      (result, pending, notices) => {
        latest = result;
        latestPending = pending;
        latestNotices = notices;
      }
    );
    await act(async () => {
      await flushAsyncWork();
    });

    (document.getElementById("focus-source") as HTMLButtonElement).focus();
    await act(async () => {
      window.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true }));
      await flushAsyncWork();
    });
    expect(closeReaderPanel).toHaveBeenCalled();
    expect(setOverflow).toHaveBeenCalledWith(false);
    expect(latestPending?.episodeIndex).toBe("2");

    await act(async () => {
      latest?.handleNextEpisodeConfirmationCloseClick();
      await flushAsyncWork();
    });
    expect(latestPending).toBeNull();

    await unmountHookRoot(root);

    const noNextRoot = renderHookHarness(
      {
        canMoveForwardReaderPage: false,
        hasUnlistedEpisodes: false,
        nextEpisode: null
      },
      (result, _pending, notices) => {
        latest = result;
        latestNotices = notices;
      }
    );
    await act(async () => {
      await flushAsyncWork();
      window.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true }));
      await flushAsyncWork();
    });
    expect(latestNotices).toContain("最終話の最終ページに到達しました。");

    await unmountHookRoot(noNextRoot);
  });

  it("handles touch swipe navigation and touch cancellation", async () => {
    installDom();
    const movePage = vi.fn();
    let latest: HookResult | null = null;
    const viewport = {
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      getBoundingClientRect: () => ({ left: 0, width: 240 }) as DOMRect
    } as unknown as HTMLDivElement;
    const root = renderHookHarness(
      {
        isTouchDevice: true,
        movePage,
        readerViewportRef: { current: viewport }
      },
      (result) => {
        latest = result;
      }
    );
    await act(async () => {
      await flushAsyncWork();
    });

    await act(async () => {
      latest?.handleViewportTouchStart({
        touches: [{ clientX: 200, clientY: 80 }]
      } as never);
      latest?.handleViewportTouchEnd({
        changedTouches: [{ clientX: 80, clientY: 82 }],
        currentTarget: viewport
      } as never);
      await flushAsyncWork();
    });
    expect(movePage).toHaveBeenCalledWith(1);

    await act(async () => {
      latest?.handleViewportTouchStart({ touches: [{ clientX: 20, clientY: 40 }] } as never);
      latest?.handleViewportTouchCancel();
      latest?.handleViewportTouchEnd({
        changedTouches: [{ clientX: 220, clientY: 40 }],
        currentTarget: viewport
      } as never);
      await flushAsyncWork();
    });
    expect(movePage).toHaveBeenCalledTimes(1);

    await unmountHookRoot(root);
  });
});
