import { act, createElement, useState } from "react";
import { createRoot } from "react-dom/client";
import { JSDOM } from "jsdom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { EpisodeResponse } from "../src/features/reader/types";
import { useAutoClearedNotice } from "../src/hooks/useAutoClearedNotice";
import { useMobileLibraryViewport } from "../src/hooks/useMobileLibraryViewport";
import { useReaderControlsLayout } from "../src/hooks/useReaderControlsLayout";
import { useReaderPanels } from "../src/hooks/useReaderPanels";
import { useReaderRouteSync } from "../src/hooks/useReaderRouteSync";
import type { ScreenMode } from "../src/hooks/useReaderState";

type MediaQueryStub = {
  addEventListener: ReturnType<typeof vi.fn>;
  addListener: ReturnType<typeof vi.fn>;
  dispatchChange: () => void;
  readonly matches: boolean;
  media: string;
  removeEventListener: ReturnType<typeof vi.fn>;
  removeListener: ReturnType<typeof vi.fn>;
  setMatches: (matches: boolean) => void;
};

function installDom(url = "http://localhost/"): JSDOM {
  const dom = new JSDOM(
    "<!doctype html><html><body><div id=\"root\"></div><div id=\"controls\"></div><div id=\"overflow\"></div><section id=\"panel\"></section></body></html>",
    { url }
  );

  vi.stubGlobal("window", dom.window);
  vi.stubGlobal("document", dom.window.document);
  vi.stubGlobal("navigator", dom.window.navigator);

  return dom;
}

async function flushAsyncWork(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
}

function createMediaQueryRegistry(initialMatches: Record<string, boolean> = {}) {
  const registry = new Map<string, MediaQueryStub & { listeners: Set<() => void> }>();
  const matchMedia = vi.fn((media: string) => {
    const existing = registry.get(media);
    if (existing) {
      return existing;
    }

    let matches = initialMatches[media] ?? false;
    const listeners = new Set<() => void>();
    const stub: MediaQueryStub & { listeners: Set<() => void> } = {
      addEventListener: vi.fn((_eventName: string, listener: () => void) => listeners.add(listener)),
      addListener: vi.fn((listener: () => void) => listeners.add(listener)),
      dispatchChange: () => {
        for (const listener of listeners) {
          listener();
        }
      },
      get matches() {
        return matches;
      },
      media,
      removeEventListener: vi.fn((_eventName: string, listener: () => void) => listeners.delete(listener)),
      removeListener: vi.fn((listener: () => void) => listeners.delete(listener)),
      setMatches: (nextMatches: boolean) => {
        matches = nextMatches;
      },
      listeners
    };
    registry.set(media, stub);
    return stub;
  });

  vi.stubGlobal("matchMedia", matchMedia);
  Object.defineProperty(window, "matchMedia", { configurable: true, value: matchMedia });

  return registry;
}

function installResizeObserverStub() {
  class ResizeObserverStub {
    disconnect = vi.fn();
    observe = vi.fn();
    unobserve = vi.fn();
  }

  vi.stubGlobal("ResizeObserver", ResizeObserverStub);
  Object.defineProperty(window, "ResizeObserver", { configurable: true, value: ResizeObserverStub });
}

function setReadonlyNumberProperty(element: Element, propertyName: "clientWidth" | "offsetWidth", value: number) {
  Object.defineProperty(element, propertyName, { configurable: true, value });
}

function createEpisode(contentEtag = "etag-1"): EpisodeResponse {
  return {
    chapter: null,
    contentEtag,
    episodeIndex: "1",
    html: "<p>本文</p>",
    novelId: "novel-a",
    plainTextLength: 2,
    readerDocument: { version: 1, blocks: [] },
    subchapter: null,
    title: "第一話",
    updatedAt: "2026-06-16T00:00:00.000Z"
  };
}

describe("reader shell hooks", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("clears notices after the requested delay", async () => {
    installDom();
    vi.useFakeTimers();

    type HookResult = ReturnType<typeof useAutoClearedNotice>;
    let latest: HookResult | null = null;

    function Harness() {
      latest = useAutoClearedNotice(1200);
      return null;
    }

    const root = createRoot(document.getElementById("root") as HTMLElement);
    await act(async () => {
      root.render(createElement(Harness));
      await flushAsyncWork();
    });

    await act(async () => {
      latest?.[1]("保存しました");
      await flushAsyncWork();
    });
    expect(latest?.[0]).toBe("保存しました");

    await act(async () => {
      vi.advanceTimersByTime(1199);
      await flushAsyncWork();
    });
    expect(latest?.[0]).toBe("保存しました");

    await act(async () => {
      vi.advanceTimersByTime(1);
      await flushAsyncWork();
    });
    expect(latest?.[0]).toBeNull();

    await act(async () => {
      root.unmount();
    });
  });

  it("tracks the single-pane library breakpoint for compact and touch viewports", async () => {
    installDom();
    Object.defineProperty(window.navigator, "maxTouchPoints", { configurable: true, value: 0 });
    const mediaQueries = createMediaQueryRegistry({
      "(max-width: 800px)": false,
      "(max-width: 1100px)": false,
      "(pointer: coarse)": false
    });
    let latest: boolean | null = null;

    function Harness() {
      latest = useMobileLibraryViewport();
      return null;
    }

    const root = createRoot(document.getElementById("root") as HTMLElement);
    await act(async () => {
      root.render(createElement(Harness));
      await flushAsyncWork();
    });
    expect(latest).toBe(false);

    const compactQuery = mediaQueries.get("(max-width: 800px)");
    compactQuery?.setMatches(true);
    await act(async () => {
      compactQuery?.dispatchChange();
      await flushAsyncWork();
    });
    expect(latest).toBe(true);

    compactQuery?.setMatches(false);
    mediaQueries.get("(pointer: coarse)")?.setMatches(true);
    mediaQueries.get("(max-width: 1100px)")?.setMatches(true);
    await act(async () => {
      mediaQueries.get("(pointer: coarse)")?.dispatchChange();
      await flushAsyncWork();
    });
    expect(latest).toBe(true);

    await act(async () => {
      root.unmount();
    });
  });

  it("closes reader panels and overflow from shared reader shell events", async () => {
    installDom();
    let latest: ReturnType<typeof useReaderPanels> | null = null;
    const closeImageViewer = vi.fn();

    function Harness(props: { screenMode: ScreenMode }) {
      latest = useReaderPanels({
        closeImageViewer,
        readerControlsRef: { current: document.getElementById("controls") as HTMLDivElement },
        readerOverflowRef: { current: document.getElementById("overflow") as HTMLDivElement },
        readerPanelRef: { current: document.getElementById("panel") as HTMLElement },
        screenMode: props.screenMode
      });
      return null;
    }

    const root = createRoot(document.getElementById("root") as HTMLElement);
    await act(async () => {
      root.render(createElement(Harness, { screenMode: "reader" }));
      await flushAsyncWork();
    });

    await act(async () => {
      latest?.toggleReaderPanel("toc");
      latest?.setIsReaderOverflowOpen(true);
      await flushAsyncWork();
    });
    expect(latest?.activeReaderPanel).toBe("toc");
    expect(latest?.isReaderOverflowOpen).toBe(true);

    await act(async () => {
      latest?.closeExtractionPanel();
      await flushAsyncWork();
    });
    expect(latest?.activeReaderPanel).toBe("toc");

    await act(async () => {
      latest?.openTermsPanel();
      latest?.closeExtractionPanel();
      await flushAsyncWork();
    });
    expect(latest?.activeReaderPanel).toBeNull();

    await act(async () => {
      latest?.setIsReaderOverflowOpen(true);
      latest?.closeActiveReaderPanel();
      await flushAsyncWork();
    });
    expect(latest?.activeReaderPanel).toBeNull();
    expect(latest?.isReaderOverflowOpen).toBe(true);

    await act(async () => {
      latest?.closeReaderPanel();
      await flushAsyncWork();
    });
    expect(latest?.activeReaderPanel).toBeNull();
    expect(latest?.isReaderOverflowOpen).toBe(false);

    await act(async () => {
      latest?.toggleReaderPanel("bookmarks");
      await flushAsyncWork();
    });
    document.body.dispatchEvent(new window.Event("pointerdown", { bubbles: true }));
    await act(async () => {
      await flushAsyncWork();
    });
    expect(latest?.activeReaderPanel).toBeNull();

    await act(async () => {
      latest?.toggleReaderPanel("settings");
      root.render(createElement(Harness, { screenMode: "library" }));
      await flushAsyncWork();
    });
    expect(latest?.activeReaderPanel).toBeNull();
    expect(closeImageViewer).toHaveBeenCalled();

    await act(async () => {
      root.unmount();
    });
  });

  it("measures reader controls layout and resets outside reader mode", async () => {
    installDom();
    installResizeObserverStub();
    Object.defineProperty(window, "innerWidth", { configurable: true, value: 1280 });

    const shell = document.createElement("section");
    const viewport = document.createElement("div");
    const indicator = document.createElement("p");
    document.body.append(shell, viewport, indicator);
    setReadonlyNumberProperty(shell, "clientWidth", 940);
    setReadonlyNumberProperty(viewport, "clientWidth", 720);
    setReadonlyNumberProperty(indicator, "offsetWidth", 86);

    type HookResult = ReturnType<typeof useReaderControlsLayout>;
    let latest: HookResult | null = null;

    function Harness(props: {
      episode: EpisodeResponse | null;
      readerViewport: HTMLDivElement | null;
      screenMode: ScreenMode;
    }) {
      latest = useReaderControlsLayout({
        currentPageIndex: 0,
        episode: props.episode,
        readerPageIndicatorRef: { current: indicator },
        readerShellRef: { current: shell },
        readerViewportRef: { current: props.readerViewport },
        screenMode: props.screenMode,
        totalPages: 3
      });
      return null;
    }

    const root = createRoot(document.getElementById("root") as HTMLElement);
    await act(async () => {
      root.render(createElement(Harness, { episode: createEpisode(), readerViewport: viewport, screenMode: "reader" }));
      await flushAsyncWork();
    });
    expect(latest).toEqual({
      readerControlViewportWidth: 720,
      readerPageIndicatorWidth: 86
    });

    setReadonlyNumberProperty(viewport, "clientWidth", 0);
    Object.defineProperty(window, "innerWidth", { configurable: true, value: 1024 });
    await act(async () => {
      root.render(createElement(Harness, { episode: createEpisode("etag-2"), readerViewport: viewport, screenMode: "reader" }));
      window.dispatchEvent(new window.Event("resize"));
      await flushAsyncWork();
    });
    expect(latest?.readerControlViewportWidth).toBe(1024);

    await act(async () => {
      root.render(createElement(Harness, { episode: createEpisode("etag-3"), readerViewport: null, screenMode: "reader" }));
      await flushAsyncWork();
    });
    expect(latest?.readerControlViewportWidth).toBe(940);

    await act(async () => {
      root.render(createElement(Harness, { episode: null, readerViewport: viewport, screenMode: "reader" }));
      await flushAsyncWork();
    });
    expect(latest?.readerPageIndicatorWidth).toBe(0);

    Object.defineProperty(window, "innerWidth", { configurable: true, value: 800 });
    await act(async () => {
      root.render(createElement(Harness, { episode: createEpisode("etag-4"), readerViewport: viewport, screenMode: "library" }));
      await flushAsyncWork();
    });
    expect(latest).toEqual({
      readerControlViewportWidth: 800,
      readerPageIndicatorWidth: 0
    });

    await act(async () => {
      root.unmount();
    });
  });

  it("syncs reader selection with location search and browser history", async () => {
    installDom("http://localhost/viewer?novelId=old&episode=2&pos=30");

    type RouteResult = {
      screenMode: ScreenMode;
      selectedEpisodeIndex: string | null;
      selectedNovelId: string | null;
      selectedPosition: number | null;
    };
    let latest: RouteResult | null = null;
    let setRoute: ((route: RouteResult) => void) | null = null;

    function Harness() {
      const [route, updateRoute] = useState<RouteResult>({
        screenMode: "reader",
        selectedEpisodeIndex: "2",
        selectedNovelId: "old",
        selectedPosition: 30
      });

      latest = route;
      setRoute = updateRoute;
      useReaderRouteSync({
        screenMode: route.screenMode,
        selectedEpisodeIndex: route.selectedEpisodeIndex,
        selectedNovelId: route.selectedNovelId,
        selectedPosition: route.selectedPosition,
        setScreenMode: (screenMode) => updateRoute((current) => ({ ...current, screenMode })),
        setSelectedEpisodeIndex: (selectedEpisodeIndex) => updateRoute((current) => ({ ...current, selectedEpisodeIndex })),
        setSelectedNovelId: (selectedNovelId) => updateRoute((current) => ({ ...current, selectedNovelId })),
        setSelectedPosition: (selectedPosition) => updateRoute((current) => ({ ...current, selectedPosition }))
      });
      return null;
    }

    const root = createRoot(document.getElementById("root") as HTMLElement);
    await act(async () => {
      root.render(createElement(Harness));
      await flushAsyncWork();
    });

    await act(async () => {
      setRoute?.({
        screenMode: "reader",
        selectedEpisodeIndex: "7",
        selectedNovelId: "novel-a",
        selectedPosition: 123
      });
      await flushAsyncWork();
    });
    expect(window.location.search).toBe("?novelId=novel-a&episode=7&pos=123");

    window.history.pushState(null, "", "/viewer?novelId=novel-b&episode=9&pos=5");
    await act(async () => {
      window.dispatchEvent(new window.PopStateEvent("popstate"));
      await flushAsyncWork();
    });

    expect(latest).toEqual({
      screenMode: "reader",
      selectedEpisodeIndex: "9",
      selectedNovelId: "novel-b",
      selectedPosition: 5
    });

    await act(async () => {
      root.unmount();
    });
  });
});
