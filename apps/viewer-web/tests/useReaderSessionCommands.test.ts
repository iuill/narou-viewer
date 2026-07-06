import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement, useRef, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import type { EpisodeIndex, TocResponse } from "../src/features/reader/types";
import { useReaderSessionCommands } from "../src/features/reader/useReaderSessionCommands";
import type { ScreenMode } from "../src/hooks/useReaderState";

type HookResult = ReturnType<typeof useReaderSessionCommands>;

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

function createToc(): TocResponse {
  return {
    novelId: "novel-a",
    fetcherWorkId: "work-a",
    title: "作品",
    author: "著者",
    siteName: "narou",
    tocUrl: null,
    updatedAt: "2026-06-25T00:00:00.000Z",
    lastActivityAt: null,
    totalEpisodes: 2,
    story: "",
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
        bodyStatus: "missing"
      }
    ]
  };
}

function renderHookHarness(props: {
  currentEpisodeIndex?: EpisodeIndex | null;
  onRender: (
    result: HookResult,
    state: {
      layoutAnchorPosition: number | null;
      openSelectedNovelInReader: boolean;
      shouldCapturePageAnchor: boolean;
    }
  ) => void;
  toc?: TocResponse | null;
}): Root {
  const rootElement = document.getElementById("root");
  if (!rootElement) {
    throw new Error("root element is missing");
  }

  function Harness() {
    const layoutAnchorPositionRef = useRef<number | null>(88);
    const openSelectedNovelInReaderRef = useRef(true);
    const shouldCapturePageAnchorRef = useRef(true);
    const [screenMode, setScreenMode] = useState<ScreenMode>("library");
    const [isEpisodeLayoutReady, setIsEpisodeLayoutReady] = useState(true);
    const [selectedEpisodeIndex, setSelectedEpisodeIndex] = useState<EpisodeIndex | null>(null);
    const [selectedNovelId, setSelectedNovelId] = useState<string | null>(null);
    const [selectedPosition, setSelectedPosition] = useState<number | null>(null);
    const onError = vi.fn();
    const sessionCommands = {
      clearSelection: (options: { clearNovel?: boolean } = {}) => {
        if (options.clearNovel) {
          setSelectedNovelId(null);
        }
        setSelectedEpisodeIndex(null);
        setSelectedPosition(null);
        setScreenMode("library");
      },
      openEpisodeSelection: (episodeIndex: EpisodeIndex, position: number | null) => {
        setSelectedEpisodeIndex(episodeIndex);
        setSelectedPosition(position);
        setScreenMode("reader");
      },
      returnToLibrary: () => {
        setScreenMode("library");
      },
      selectNovel: (novelId: string, options: { openInReader?: boolean } = {}) => {
        openSelectedNovelInReaderRef.current = options.openInReader ?? false;
        setSelectedNovelId(novelId);
        setSelectedEpisodeIndex(null);
        setSelectedPosition(null);
        setScreenMode("library");
      },
      updateSelectedPosition: setSelectedPosition
    };

    const result = useReaderSessionCommands({
      currentEpisodeIndex: props.currentEpisodeIndex ?? "1",
      layoutAnchorPositionRef,
      onError,
      openSelectedNovelInReaderRef,
      sessionCommands,
      setIsEpisodeLayoutReady,
      shouldCapturePageAnchorRef,
      toc: props.toc === undefined ? createToc() : props.toc
    });

    props.onRender(result, {
      layoutAnchorPosition: layoutAnchorPositionRef.current,
      openSelectedNovelInReader: openSelectedNovelInReaderRef.current,
      shouldCapturePageAnchor: shouldCapturePageAnchorRef.current
    });

    return createElement("output", {
      "data-layout-ready": String(isEpisodeLayoutReady),
      "data-screen-mode": screenMode,
      "data-selected-episode": selectedEpisodeIndex ?? "",
      "data-selected-novel": selectedNovelId ?? "",
      "data-selected-position": selectedPosition ?? ""
    });
  }

  const root = createRoot(rootElement);
  act(() => {
    root.render(createElement(Harness));
  });
  mountedRoot = root;
  return root;
}

afterEach(() => {
  if (mountedRoot) {
    act(() => mountedRoot?.unmount());
    mountedRoot = null;
  }
  vi.unstubAllGlobals();
});

describe("useReaderSessionCommands", () => {
  it("opens a fetched episode with the same side effects as the reader session operation", () => {
    installDom();
    let latest: HookResult | null = null;
    let latestState: {
      layoutAnchorPosition: number | null;
      openSelectedNovelInReader: boolean;
      shouldCapturePageAnchor: boolean;
    } | null = null;

    renderHookHarness({
      currentEpisodeIndex: "1",
      onRender: (result, state) => {
        latest = result;
        latestState = state;
      }
    });

    act(() => {
      expect(latest?.openEpisode("1", 42)).toBe(true);
    });

    const output = document.querySelector("output");
    expect(output?.dataset.screenMode).toBe("reader");
    expect(output?.dataset.selectedEpisode).toBe("1");
    expect(output?.dataset.selectedPosition).toBe("42");
    expect(latestState?.layoutAnchorPosition).toBe(42);
    expect(latestState?.openSelectedNovelInReader).toBe(false);
    expect(latestState?.shouldCapturePageAnchor).toBe(false);
  });

  it("marks episode layout as not ready when opening a different fetched episode", () => {
    installDom();
    let latest: HookResult | null = null;

    renderHookHarness({
      currentEpisodeIndex: "2",
      onRender: (result) => {
        latest = result;
      }
    });

    act(() => {
      expect(latest?.openEpisode("1", 10)).toBe(true);
    });

    const output = document.querySelector("output");
    expect(output?.dataset.screenMode).toBe("reader");
    expect(output?.dataset.selectedEpisode).toBe("1");
    expect(output?.dataset.selectedPosition).toBe("10");
    expect(output?.dataset.layoutReady).toBe("false");
  });

  it("rejects missing or unfetched episodes before mutating reader selection", () => {
    installDom();
    let latest: HookResult | null = null;

    renderHookHarness({
      onRender: (result) => {
        latest = result;
      }
    });

    act(() => {
      expect(latest?.openEpisode("missing")).toBe(false);
    });
    expect(document.querySelector("output")?.dataset.screenMode).toBe("library");

    act(() => {
      expect(latest?.openEpisode("2")).toBe(false);
    });
    expect(document.querySelector("output")?.dataset.screenMode).toBe("library");
  });

  it("selects and clears novels through the reader session command boundary", () => {
    installDom();
    let latest: HookResult | null = null;
    let latestState: {
      layoutAnchorPosition: number | null;
      openSelectedNovelInReader: boolean;
      shouldCapturePageAnchor: boolean;
    } | null = null;

    renderHookHarness({
      onRender: (result, state) => {
        latest = result;
        latestState = state;
      }
    });

    act(() => {
      latest?.selectNovel("novel-b", { openInReader: true });
    });

    const output = document.querySelector("output");
    expect(output?.dataset.screenMode).toBe("library");
    expect(output?.dataset.selectedNovel).toBe("novel-b");
    expect(output?.dataset.selectedEpisode).toBe("");
    expect(output?.dataset.selectedPosition).toBe("");
    expect(latestState?.layoutAnchorPosition).toBeNull();
    expect(latestState?.openSelectedNovelInReader).toBe(true);
    expect(latestState?.shouldCapturePageAnchor).toBe(false);

    act(() => {
      latest?.clearSelection({ clearNovel: true });
    });

    expect(output?.dataset.screenMode).toBe("library");
    expect(output?.dataset.selectedNovel).toBe("");
    expect(output?.dataset.selectedEpisode).toBe("");
    expect(output?.dataset.selectedPosition).toBe("");
    expect(latestState?.openSelectedNovelInReader).toBe(false);
  });
});
