import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import { ReaderPager } from "../src/ReaderPager";
import type { EpisodeResponse } from "../src/features/reader/types";

const episodeWithLink: EpisodeResponse = {
  novelId: "n1",
  episodeIndex: "1",
  title: "第1話",
  chapter: null,
  subchapter: null,
  html: "",
  readerDocument: { version: 1, blocks: [] },
  plainTextLength: 0,
  updatedAt: "2026-06-22T00:00:00Z",
  contentEtag: "etag-1"
};

function installDom(): JSDOM {
  const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
    url: "http://localhost/"
  });

  vi.stubGlobal("window", dom.window);
  vi.stubGlobal("document", dom.window.document);
  vi.stubGlobal("HTMLElement", dom.window.HTMLElement);
  vi.stubGlobal("MouseEvent", dom.window.MouseEvent);
  vi.stubGlobal("TouchEvent", dom.window.TouchEvent);
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);

  return dom;
}

async function renderPager(
  options: {
    episode?: EpisodeResponse | null;
    onViewportClick?: ReturnType<typeof vi.fn>;
    renderedEpisodeHtml?: string;
  } = {}
): Promise<{ container: HTMLElement; dom: JSDOM; root: Root }> {
  const dom = installDom();
  const container = dom.window.document.getElementById("root");

  if (!container) {
    throw new Error("root container not found");
  }

  const root = createRoot(container);

  await act(async () => {
    root.render(
      createElement(ReaderPager, {
        articleFontFamilyCss: "serif",
        articleFontWeight: null,
        displayedPageNumber: 1,
        episode: options.episode ?? null,
        isEpisodeLoading: !options.episode,
        isFullscreen: false,
        isLoadingOverlayVisible: !options.episode,
        isTouchDevice: false,
        letterSpacingEm: 0.08,
        loadingEpisodeTitle: "第一章 - 第1話",
        loadingNovelTitle: "小説A",
        onViewportClick: options.onViewportClick ?? vi.fn(),
        onViewportTouchCancel: vi.fn(),
        onViewportTouchEnd: vi.fn(),
        onViewportTouchStart: vi.fn(),
        pageIndicatorRef: { current: null },
        readerFontSizePx: 20,
        readingMode: "vertical",
        renderedEpisodeHtml: options.renderedEpisodeHtml ?? "",
        totalPages: 0,
        verticalLastPageReservePx: 0,
        viewportRef: { current: null }
      })
    );
  });

  return { container, dom, root };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("ReaderPager", () => {
  it("ロード表示に作品名と話タイトルを表示する", async () => {
    const { container, root } = await renderPager();

    expect(container.querySelector(".reader-loading-overlay")).not.toBeNull();
    expect(container.querySelector(".reader-loading-logo")).not.toBeNull();
    expect(container.textContent).toContain("本文を読み込み中");
    expect(container.textContent).toContain("小説A");
    expect(container.textContent).toContain("第一章 - 第1話");

    await act(async () => {
      root.unmount();
    });
  });

  it("本文内リンクの Enter 操作を本文ページクリックとして扱わない", async () => {
    const onViewportClick = vi.fn();
    const { container, dom, root } = await renderPager({
      episode: episodeWithLink,
      onViewportClick,
      renderedEpisodeHtml: '<p><a href="https://example.com/story">外部リンク</a></p>'
    });
    const link = container.querySelector("a");

    if (!link) {
      throw new Error("reader link not found");
    }

    await act(async () => {
      link.dispatchEvent(new dom.window.KeyboardEvent("keydown", { bubbles: true, key: "Enter" }));
    });

    expect(onViewportClick).not.toHaveBeenCalled();

    await act(async () => {
      root.unmount();
    });
  });
});
