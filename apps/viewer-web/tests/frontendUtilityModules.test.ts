import { afterEach, describe, expect, it, vi } from "vitest";
import { JSDOM } from "jsdom";
import {
  deriveLatestBookmark,
  formatBookmarkLocation,
  formatNovelLastReadLabel,
  updateNovelBookmarkSummary
} from "../src/features/library/bookmarkSummary";
import { extractDroppedDownloadTarget } from "../src/features/library/downloadTarget";
import { paginateItems } from "../src/features/library/pagination";
import { filterNovelsByQuery } from "../src/features/library/search";
import { createStoryPreview, normalizeStoryText } from "../src/features/library/text";
import {
  buildEpisodeDisplayLookup,
  buildEpisodeLabel,
  formatEpisodeIndexLabel,
  formatEpisodeOrderLabel,
  formatEpisodeReferenceLabel,
  shouldUseFriendlyEpisodeLabels
} from "../src/features/reader/episodeLabels";
import {
  getReaderSwipeDirection,
  isReaderEdgeClick
} from "../src/features/reader/gestureNavigation";
import {
  calculateImageViewerWidth,
  extractImageViewerState
} from "../src/features/reader/imageViewer";
import { prepareEpisodeHtmlForReader } from "../src/features/reader/readerHtml";
import {
  exitDocumentFullscreen,
  getFullscreenElement,
  requestElementFullscreen,
  resolveReaderFullscreenToggleAction,
  shouldExitReaderFullscreenOnReturn,
  supportsElementFullscreen
} from "../src/features/reader/fullscreen";
import {
  buildVerticalColumnBoundaries,
  buildVerticalPages,
  buildVerticalPageOffsets,
  detectWebKitEngine,
  hasMeaningfulVerticalReserveChange,
  isRectWithinVerticalPage,
  normalizeVerticalReservePx,
  resolveVerticalPagingContentMetrics,
  toViewportContentOffset
} from "../src/features/reader/verticalPagination";
import { parseRouteSelection } from "../src/routing/readerRoute";

describe("frontend utility modules", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("parses route selections and ignores invalid position or episode values", () => {
    expect(parseRouteSelection("?novelId=novelA&episode=12&pos=8")).toEqual({
      novelId: "novelA",
      episodeIndex: "12",
      position: 8,
      screenMode: "reader"
    });
    expect(parseRouteSelection("?novelId=novelA&episode=abc&pos=-1&line=9")).toEqual({
      novelId: "novelA",
      episodeIndex: null,
      position: null,
      screenMode: "library"
    });
  });

  it("normalizes story preview text and truncates safely", () => {
    const normalized = normalizeStoryText("  第一行\n\n 第二行\t第三行 ");
    const preview = createStoryPreview(normalized, 6);

    expect(normalized).toBe("第一行\n第二行 第三行");
    expect(preview).toEqual({
      text: "第一行\n第二…",
      isTruncated: true
    });
  });

  it("normalizes HTML fragments in story text", () => {
    expect(normalizeStoryText("<br/> 第一行 &amp; 第二行<br><br> <p>第三行&#x21;</p>")).toBe("第一行 & 第二行\n第三行!");
  });

  it("detects horizontal swipe direction for reader page moves", () => {
    expect(getReaderSwipeDirection(240, 120, 320, 126)).toBe("right");
    expect(getReaderSwipeDirection(320, 120, 240, 126)).toBe("left");
    expect(getReaderSwipeDirection(240, 120, 275, 126)).toBeNull();
    expect(getReaderSwipeDirection(240, 120, 300, 220)).toBeNull();
  });

  it("filters novels by title, author, site, and multi-keyword queries", () => {
    const novels = [
      {
        novelId: "a",
        title: "銀河図書館",
        author: "田中 太郎",
        siteName: "カクヨム",
        tocUrl: "https://kakuyomu.jp/works/1"
      },
      {
        novelId: "b",
        title: "海辺の手紙",
        author: "佐藤 花子",
        siteName: "小説家になろう",
        tocUrl: "https://ncode.syosetu.com/n0001/"
      }
    ];

    expect(filterNovelsByQuery(novels, "銀河")).toEqual([novels[0]]);
    expect(filterNovelsByQuery(novels, "田中 カクヨム")).toEqual([novels[0]]);
    expect(filterNovelsByQuery(novels, "なろう 佐藤")).toEqual([novels[1]]);
    expect(filterNovelsByQuery(novels, "存在しない")).toEqual([]);
    expect(filterNovelsByQuery(novels, "   ")).toEqual(novels);
  });

  it("paginates items and clamps invalid page values", () => {
    expect(paginateItems(["a", "b", "c", "d", "e"], 2, 2)).toEqual({
      items: ["c", "d"],
      currentPage: 2,
      totalPages: 3,
      totalItems: 5,
      startItemNumber: 3,
      endItemNumber: 4
    });

    expect(paginateItems(["a", "b", "c"], 9, 2)).toEqual({
      items: ["c"],
      currentPage: 2,
      totalPages: 2,
      totalItems: 3,
      startItemNumber: 3,
      endItemNumber: 3
    });

    expect(paginateItems([], 1, 10)).toEqual({
      items: [],
      currentPage: 1,
      totalPages: 1,
      totalItems: 0,
      startItemNumber: 0,
      endItemNumber: 0
    });
  });

  it("prefers text/uri-list over plain text and ignores comment lines", () => {
    const target = extractDroppedDownloadTarget({
      getData(type: string) {
        if (type === "text/uri-list") {
          return "# comment\nhttps://example.com/novel";
        }

        return "https://fallback.example.com";
      }
    });

    expect(target).toBe("https://example.com/novel");
  });

  it("calculates image width using viewport constraints and handles missing natural size", () => {
    expect(calculateImageViewerWidth({ naturalWidth: 1200, naturalHeight: 800 }, 100, { width: 1000, height: 900 })).toBe(
      920
    );
    expect(calculateImageViewerWidth({ naturalWidth: null, naturalHeight: 800 }, 100, { width: 1000, height: 900 })).toBeNull();
  });

  it("prepares linked reader images and preserves normal links", () => {
    const dom = new JSDOM("<!doctype html>");
    vi.stubGlobal("HTMLImageElement", dom.window.HTMLImageElement);
    const html = prepareEpisodeHtmlForReader(
      "<p><a href=\"https://img.example/original.png\" title=\" 原寸 \"><img src=\"thumb.png\" alt=\"挿絵\"></a><a href=\"https://example.com\">本文リンク</a></p>",
      dom.window.document
    );

    const output = new JSDOM(html).window.document;
    const image = output.querySelector("img");
    expect(image?.dataset.readerImageOriginalHref).toBe("https://img.example/original.png");
    expect(image?.dataset.readerImageOriginalTitle).toBe("原寸");
    expect(output.querySelector("a[href='https://example.com']")?.textContent).toBe("本文リンク");
  });

  it("extracts reader image viewer state from data attributes, anchors, and natural size", () => {
    const dom = new JSDOM(
      "<!doctype html><a href=\"javascript:alert(1)\" title=\"anchor title\"><img src=\"thumb.png\" alt=\"alt title\" data-reader-image-original-href=\"https://img.example/original.png\"></a>",
      { url: "https://viewer.example/reader" }
    );
    vi.stubGlobal("HTMLAnchorElement", dom.window.HTMLAnchorElement);
    const image = dom.window.document.querySelector("img") as HTMLImageElement;
    Object.defineProperty(image, "naturalWidth", { configurable: true, value: 640 });
    Object.defineProperty(image, "naturalHeight", { configurable: true, value: 480 });

    expect(extractImageViewerState(image)).toEqual({
      src: "thumb.png",
      originalUrl: "https://img.example/original.png",
      title: "anchor title",
      alt: "alt title",
      naturalWidth: 640,
      naturalHeight: 480
    });

    image.dataset.readerImageOriginalHref = "";
    expect(extractImageViewerState(image)?.originalUrl).toBe("thumb.png");
  });

  it("resolves fullscreen helpers across native, webkit, and pseudo fullscreen paths", async () => {
    const dom = new JSDOM("<!doctype html><main></main>");
    const doc = dom.window.document;
    const element = doc.querySelector("main") as HTMLElement & {
      webkitRequestFullscreen?: () => Promise<void>;
    };

    expect(getFullscreenElement(undefined)).toBeNull();
    expect(supportsElementFullscreen(element)).toBe(false);
    element.webkitRequestFullscreen = async () => undefined;
    expect(supportsElementFullscreen(element)).toBe(true);
    await expect(requestElementFullscreen(element)).resolves.toBeUndefined();

    Object.defineProperty(doc, "fullscreenElement", { configurable: true, value: element });
    expect(getFullscreenElement(doc)).toBe(element);
    const webkitDoc = doc as Document & {
      webkitExitFullscreen?: () => Promise<void>;
      webkitFullscreenElement?: Element | null;
    };
    Object.defineProperty(webkitDoc, "fullscreenElement", { configurable: true, value: null });
    Object.defineProperty(webkitDoc, "webkitFullscreenElement", { configurable: true, value: element });
    expect(getFullscreenElement(doc)).toBe(element);

    let exited = false;
    webkitDoc.webkitExitFullscreen = async () => {
      exited = true;
    };
    await exitDocumentFullscreen(doc);
    expect(exited).toBe(true);

    expect(resolveReaderFullscreenToggleAction({
      hasReaderShell: false,
      isNativeFullscreen: false,
      isPseudoFullscreen: false,
      supportsNativeFullscreen: true
    })).toBe("noop");
    expect(resolveReaderFullscreenToggleAction({
      hasReaderShell: true,
      isNativeFullscreen: true,
      isPseudoFullscreen: false,
      supportsNativeFullscreen: true
    })).toBe("exit-native");
    expect(resolveReaderFullscreenToggleAction({
      hasReaderShell: true,
      isNativeFullscreen: false,
      isPseudoFullscreen: true,
      supportsNativeFullscreen: true
    })).toBe("disable-pseudo");
    expect(resolveReaderFullscreenToggleAction({
      hasReaderShell: true,
      isNativeFullscreen: false,
      isPseudoFullscreen: false,
      supportsNativeFullscreen: false
    })).toBe("enable-pseudo");
    expect(resolveReaderFullscreenToggleAction({
      hasReaderShell: true,
      isNativeFullscreen: false,
      isPseudoFullscreen: false,
      supportsNativeFullscreen: true
    })).toBe("request-native");
    expect(shouldExitReaderFullscreenOnReturn({ hasReaderShell: true, isNativeFullscreen: true })).toBe(true);
    expect(shouldExitReaderFullscreenOnReturn({ hasReaderShell: false, isNativeFullscreen: true })).toBe(false);
  });

  it("builds vertical page offsets from measured boundaries and falls back when gaps are missing", () => {
    expect(buildVerticalPageOffsets([0, 120, 250, 500, 720, 1000], 1000, 300)).toEqual([700, 420, 200, 0]);
    expect(buildVerticalPageOffsets([0, 1000], 1000, 300)).toEqual([700, 400, 100, 0]);
    expect(buildVerticalPageOffsets([0, 180], 180, 300)).toEqual([0]);
  });

  it("keeps page starts on the next boundary and exposes trailing blank space", () => {
    expect(buildVerticalPages([0, 120, 250, 500, 720, 1000], 1000, 300)).toEqual([
      { start: 720, end: 1000, offset: 700, blankLeft: 20, blankRight: 0, shiftX: 0 },
      { start: 500, end: 720, offset: 420, blankLeft: 80, blankRight: 0, shiftX: 0 },
      { start: 250, end: 500, offset: 200, blankLeft: 50, blankRight: 0, shiftX: 0 },
      { start: 0, end: 250, offset: 0, blankLeft: 50, blankRight: 0, shiftX: 50 }
    ]);
  });

  it("right-aligns the final page and fills the remaining width with empty columns", () => {
    expect(buildVerticalPages([0, 180], 180, 300)).toEqual([
      { start: 0, end: 180, offset: 0, blankLeft: 120, blankRight: 0, shiftX: 120 }
    ]);
  });

  it("normalizes vertical reserve px and ignores sub-pixel reserve jitter", () => {
    expect(normalizeVerticalReservePx(Number.NaN)).toBe(0);
    expect(normalizeVerticalReservePx(0.49)).toBe(0);
    expect(normalizeVerticalReservePx(427.16999999999996)).toBe(427.17);
    expect(hasMeaningfulVerticalReserveChange(427.17, 427.18)).toBe(false);
    expect(hasMeaningfulVerticalReserveChange(427.17, 427.79)).toBe(true);
  });

  it("merges overlapping fragment rects into vertical column boundaries", () => {
    expect(
      buildVerticalColumnBoundaries(
        [
          { left: 860, right: 888 },
          { left: 864, right: 890 },
          { left: 822, right: 850 },
          { left: 440, right: 410 },
          { left: 410, right: 438 },
          { left: 412, right: 440 }
        ],
        920
      )
    ).toEqual([0, 410, 822, 860, 920]);
  });

  it("prefers article scroll metrics when resolving vertical paging content size", () => {
    expect(
      resolveVerticalPagingContentMetrics(
        {
          scrollWidth: 1120,
          scrollHeight: 900
        },
        {
          scrollWidth: 980,
          scrollHeight: 840
        }
      )
    ).toEqual({
      contentWidth: 980,
      contentHeight: 840
    });

    expect(
      resolveVerticalPagingContentMetrics(
        {
          scrollWidth: 1120,
          scrollHeight: 900
        },
        null
      )
    ).toEqual({
      contentWidth: 1120,
      contentHeight: 900
    });
  });

  it("converts client rect edges into scroll offsets relative to the viewport content box", () => {
    expect(toViewportContentOffset(420, 100, 240, 24)).toBe(536);
    expect(toViewportContentOffset(124, 100, 0, 24)).toBe(0);
  });

  it("判定対象の rect が現在ページの列に含まれるか判定する", () => {
    expect(
      isRectWithinVerticalPage(
        { left: 340, right: 368, width: 28, height: 120 },
        {
          viewportRectLeft: 100,
          scrollLeft: 240,
          clientLeft: 24,
          shiftX: 0
        },
        {
          start: 440,
          end: 500
        }
      )
    ).toBe(true);

    expect(
      isRectWithinVerticalPage(
        { left: 460, right: 488, width: 28, height: 120 },
        {
          viewportRectLeft: 100,
          scrollLeft: 240,
          clientLeft: 24,
          shiftX: 0
        },
        {
          start: 440,
          end: 500
        }
      )
    ).toBe(false);
  });

  it("formats episode labels, bookmark labels, and bookmark summary updates", () => {
    const episodes = [
      {
        episodeIndex: "1",
        title: "第一話",
        chapter: "第一章",
        subchapter: "開幕"
      },
      {
        episodeIndex: "2",
        title: "第二話",
        chapter: "第二章",
        subchapter: null
      }
    ];
    const lookup = buildEpisodeDisplayLookup(episodes);
    const novels = [
      {
        novelId: "novelA",
        bookmarkCount: 0,
        latestBookmarkEpisodeIndex: null
      }
    ];
    const bookmarks = [
      { episodeIndex: "1", position: 4, createdAt: "2025-01-01T00:00:00.000Z" },
      { episodeIndex: "2", position: 8, createdAt: "2025-01-02T00:00:00.000Z" }
    ];

    expect(buildEpisodeLabel(episodes[0])).toBe("第一章 / 開幕 - 第一話");
    expect(shouldUseFriendlyEpisodeLabels("カクヨム", null)).toBe(true);
    expect(formatEpisodeIndexLabel("2", lookup, true)).toBe("#2");
    expect(formatEpisodeOrderLabel("2", lookup)).toBe("2");
    expect(formatEpisodeOrderLabel("999", lookup)).toBe("999");
    expect(formatEpisodeReferenceLabel("2", lookup, true)).toBe("第二話");
    expect(formatBookmarkLocation(bookmarks[1], lookup, true)).toBe("第二話");
    expect(deriveLatestBookmark(bookmarks)).toEqual(bookmarks[1]);
    expect(updateNovelBookmarkSummary(novels, "novelA", bookmarks)).toEqual([
      {
        novelId: "novelA",
        bookmarkCount: 2,
        latestBookmarkEpisodeIndex: "2"
      }
    ]);
    expect(
      formatNovelLastReadLabel({
        siteName: "カクヨム",
        tocUrl: "https://kakuyomu.jp/works/1",
        lastReadEpisodeIndex: "2",
        lastReadEpisodeTitle: "第二話"
      })
    ).toBe("第二話");
  });

  it("detects reader edge clicks and webkit user agents", () => {
    expect(
      isReaderEdgeClick(
        {
          getBoundingClientRect() {
            return {
              left: 10,
              width: 600
            } as DOMRect;
          }
        },
        40
      )
    ).toBe(true);
    expect(
      isReaderEdgeClick(
        {
          getBoundingClientRect() {
            return {
              left: 10,
              width: 600
            } as DOMRect;
          }
        },
        200
      )
    ).toBe(false);
    expect(
      detectWebKitEngine(
        "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 Version/18.0 Mobile/15E148 Safari/604.1"
      )
    ).toBe(true);
    expect(
      detectWebKitEngine(
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/136.0.0.0 Safari/537.36"
      )
    ).toBe(false);
  });
});
