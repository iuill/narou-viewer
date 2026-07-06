import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement, useRef } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import type { EpisodeResponse } from "../src/features/reader/types";
import { useReaderSpeech } from "../src/hooks/useReaderSpeech";

type HookResult = ReturnType<typeof useReaderSpeech>;

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

function createEpisode(): EpisodeResponse {
  return {
    novelId: "novel-a",
    episodeIndex: "1",
    title: "一",
    chapter: null,
    subchapter: null,
    sourceUrl: null,
    html: "<p>本文</p>",
    plainTextLength: 2,
    updatedAt: "2026-06-15T00:00:00.000Z",
    contentEtag: "episode-1",
    readerDocument: {
      version: 1,
      blocks: [
        {
          type: "paragraph",
          section: "body",
          inlines: [{ type: "text", text: "本文" }]
        }
      ]
    }
  };
}

function renderHookHarness(props: {
  initialEnabled?: boolean;
  onRender: (result: HookResult, notices: string[], errors: string[]) => void;
}): Root {
  const rootElement = document.getElementById("root");
  if (!rootElement) {
    throw new Error("root element is missing");
  }

  const notices: string[] = [];
  const errors: string[] = [];

  function Harness() {
    const readerViewportRef = useRef<HTMLDivElement | null>(null);
    const selectedEpisodeIndexRef = useRef("1");
    const selectedPositionRef = useRef<number | null>(null);
    const result = useReaderSpeech({
      currentPageIndex: 0,
      episode: createEpisode(),
      getCurrentPageIndexFromViewport: vi.fn(() => 0),
      getCurrentReaderViewportPosition: vi.fn(() => 0),
      getPagingMetrics: vi.fn(() => ({ verticalPages: null })),
      initialDebugHighlight: false,
      initialEnabled: props.initialEnabled ?? true,
      initialPreferRubyText: true,
      initialRate: 1,
      initialVoiceUri: null,
      readerViewportRef,
      readingMode: "horizontal",
      renderedEpisodeHtml: "<p>本文</p>",
      screenMode: "reader",
      scrollToPage: vi.fn(),
      selectedEpisodeIndex: "1",
      selectedEpisodeIndexRef,
      selectedPositionRef,
      setCurrentPageIndex: vi.fn(),
      setError: (nextError) => {
        if (typeof nextError === "function") {
          return;
        }
        if (nextError !== null) {
          errors.push(nextError);
        }
      },
      setIsReaderOverflowOpen: vi.fn(),
      setReaderNotice: (nextNotice) => {
        if (typeof nextNotice === "function") {
          return;
        }
        if (nextNotice !== null) {
          notices.push(nextNotice);
        }
      },
      setSelectedPosition: vi.fn()
    });
    props.onRender(result, notices, errors);
    return null;
  }

  const root = createRoot(rootElement);
  root.render(createElement(Harness));
  return root;
}

describe("useReaderSpeech", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("builds speech chunks and reports unsupported browsers", async () => {
    installDom();

    let latest: HookResult | null = null;
    let latestNotices: string[] = [];
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result, notices) => {
          latest = result;
          latestNotices = notices;
        }
      });
      await flushAsyncWork();
    });

    expect(latest?.readerSpeechChunks).toHaveLength(1);
    expect(latest?.isReaderSpeechSupported).toBe(false);

    await act(async () => {
      await latest?.handleReaderSpeechPlay();
    });

    expect(latestNotices).toContain("このブラウザでは読み上げを利用できません。");

    await act(async () => {
      root?.unmount();
    });
  });

  it("asks the panel to enable speech before checking browser support", async () => {
    installDom();

    let latest: HookResult | null = null;
    let latestNotices: string[] = [];
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        initialEnabled: false,
        onRender: (result, notices) => {
          latest = result;
          latestNotices = notices;
        }
      });
      await flushAsyncWork();
    });

    await act(async () => {
      await latest?.handleReaderSpeechPlay();
    });

    expect(latestNotices).toContain("読み上げパネルで読み上げを有効にしてください。");

    await act(async () => {
      root?.unmount();
    });
  });
});
