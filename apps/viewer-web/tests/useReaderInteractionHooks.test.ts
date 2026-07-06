import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { JSDOM } from "jsdom";

import { createBookmark, deleteBookmark } from "../src/features/reader/api";
import type { Bookmark } from "../src/features/reader/types";
import { useReaderBookmarks } from "../src/hooks/useReaderBookmarks";
import { useReaderFullscreen } from "../src/hooks/useReaderFullscreen";
import { useReaderImageViewer } from "../src/hooks/useReaderImageViewer";

vi.mock("../src/features/reader/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../src/features/reader/api")>()),
  createBookmark: vi.fn(),
  deleteBookmark: vi.fn()
}));

vi.mock("../src/readerPosition", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../src/readerPosition")>()),
  findReaderPositionTarget: vi.fn(() => ({ innerText: "  現在の段落  " })),
  getReaderPositionFromViewport: vi.fn(() => 12)
}));

function installDom(): JSDOM {
  const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div><main id=\"reader\"></main></body></html>", {
    url: "http://localhost/"
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
}

describe("reader interaction hooks", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("falls back to pseudo fullscreen and reports failed native exit", async () => {
    const dom = installDom();
    const notices: string[] = [];
    let latest: ReturnType<typeof useReaderFullscreen> | null = null;
    let returnCalls = 0;

    function Harness() {
      const readerShellRef = useRef<HTMLElement | null>(dom.window.document.getElementById("reader"));
      latest = useReaderFullscreen({
        onReturnToLibrary: () => {
          returnCalls += 1;
        },
        readerShellRef,
        screenMode: "reader",
        setReaderNotice: (nextNotice) => {
          if (typeof nextNotice !== "function" && nextNotice) {
            notices.push(nextNotice);
          }
        }
      });
      return null;
    }

    const root = createRoot(dom.window.document.getElementById("root") as HTMLElement);
    await act(async () => {
      root.render(createElement(Harness));
      await flushAsyncWork();
    });

    await act(async () => {
      await latest?.handleToggleReaderFullscreen();
      await flushAsyncWork();
    });
    expect(latest?.isReaderPseudoFullscreen).toBe(true);

    const reader = dom.window.document.getElementById("reader");
    Object.defineProperty(dom.window.document, "fullscreenElement", { configurable: true, value: reader });
    Object.defineProperty(dom.window.document, "exitFullscreen", {
      configurable: true,
      value: vi.fn().mockRejectedValue(new Error("exit failed"))
    });
    dom.window.document.dispatchEvent(new dom.window.Event("fullscreenchange"));

    await act(async () => {
      await latest?.handleReturnToLibrary();
      await flushAsyncWork();
    });

    expect(returnCalls).toBe(1);
    expect(notices).toContain("フルスクリーン表示を解除できませんでした。");
    await act(async () => {
      root.unmount();
      await flushAsyncWork();
    });
  });

  it("opens, drags, zooms, and closes the reader image viewer", async () => {
    const dom = installDom();
    let latest: ReturnType<typeof useReaderImageViewer> | null = null;

    function Harness() {
      latest = useReaderImageViewer();
      return null;
    }

    const root = createRoot(document.getElementById("root") as HTMLElement);
    await act(async () => {
      root.render(createElement(Harness));
      await flushAsyncWork();
    });

    await act(async () => {
      latest?.openImageViewer({
        alt: "alt",
        naturalHeight: 300,
        naturalWidth: 400,
        originalUrl: "https://example.test/original.png",
        src: "thumb.png",
        title: "image"
      });
      latest?.setImageViewerZoomPercent(150);
      latest?.setIsImageViewerInfoOpen(true);
      await flushAsyncWork();
    });

    expect(latest?.imageViewer?.title).toBe("image");
    expect(latest?.imageViewerZoomPercent).toBe(150);
    expect(latest?.isImageViewerInfoOpen).toBe(true);

    const target = {
      hasPointerCapture: vi.fn(() => true),
      releasePointerCapture: vi.fn(),
      scrollLeft: 10,
      scrollTop: 20,
      setPointerCapture: vi.fn()
    };
    await act(async () => {
      latest?.handleImageViewerPointerDown({
        button: 0,
        clientX: 100,
        clientY: 100,
        currentTarget: target,
        pointerId: 1,
        pointerType: "mouse",
        preventDefault: vi.fn()
      } as never);
      latest?.handleImageViewerPointerMove({
        clientX: 110,
        clientY: 80,
        currentTarget: target,
        pointerId: 1,
        preventDefault: vi.fn()
      } as never);
      latest?.handleImageViewerPointerUp({ currentTarget: target, pointerId: 1 } as never);
      await flushAsyncWork();
    });

    expect(target.scrollLeft).toBe(0);
    expect(target.scrollTop).toBe(40);
    expect(target.releasePointerCapture).toHaveBeenCalledWith(1);
    expect(latest?.isImageViewerDragging).toBe(false);

    await act(async () => {
      window.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "Escape" }));
      await flushAsyncWork();
    });
    expect(latest?.imageViewer).toBeNull();
    await act(async () => {
      root.unmount();
      await flushAsyncWork();
    });
  });

  it("creates and deletes bookmarks while updating reader state and errors", async () => {
    installDom();
    const created: Bookmark = {
      id: "bookmark-a",
      novelId: "novel-a",
      episodeIndex: "1",
      position: 12,
      label: "現在の段落",
      createdAt: "2026-07-01T00:00:00.000Z"
    };
    vi.mocked(createBookmark).mockResolvedValue(created);
    vi.mocked(deleteBookmark).mockRejectedValueOnce(new Error("delete failed")).mockResolvedValueOnce(undefined);

    const errors: string[] = [];
    const notices: string[] = [];
    let latest: ReturnType<typeof useReaderBookmarks> | null = null;
    let selectedPosition: number | null = null;

    function Harness() {
      const [bookmarks, setBookmarks] = useState<Bookmark[]>([]);
      const [novels, setNovels] = useState([{ novelId: "novel-a", bookmarkCount: 0, latestBookmarkEpisodeIndex: null }]);
      void novels;
      latest = useReaderBookmarks({
        bookmarks,
        episodeDisplayLookup: new Map([
          ["1", { chapter: null, episodeIndex: "1", order: 1, subchapter: null, title: "第一話" }]
        ]),
        isShowingAllBookmarks: false,
        preferFriendlyEpisodeLabels: true,
        readerViewportRef: { current: document.getElementById("reader") as HTMLDivElement },
        readingMode: "horizontal",
        selectedEpisodeIndex: "1",
        selectedNovelId: "novel-a",
        setBookmarks,
        setError: (nextError) => {
          if (typeof nextError !== "function" && nextError) {
            errors.push(nextError);
          }
        },
        setNovels,
        setReaderNotice: (nextNotice) => {
          if (typeof nextNotice !== "function" && nextNotice) {
            notices.push(nextNotice);
          }
        },
        setSelectedPosition: (nextPosition) => {
          selectedPosition = typeof nextPosition === "function" ? nextPosition(selectedPosition) : nextPosition;
        }
      });
      return null;
    }

    const root = createRoot(document.getElementById("root") as HTMLElement);
    await act(async () => {
      root.render(createElement(Harness));
      await flushAsyncWork();
    });

    await act(async () => {
      await latest?.createCurrentBookmark();
      await flushAsyncWork();
    });
    expect(createBookmark).toHaveBeenCalledWith({
      novelId: "novel-a",
      episodeIndex: "1",
      position: 12,
      label: "現在の段落"
    });
    expect(selectedPosition).toBe(12);
    expect(notices).toContain("第一話 に栞を保存しました。");
    expect(latest?.visibleBookmarks).toHaveLength(1);

    await act(async () => {
      await latest?.deleteBookmarkById("bookmark-a");
      await flushAsyncWork();
    });
    expect(errors).toContain("delete failed");

    await act(async () => {
      await latest?.deleteBookmarkById("bookmark-a");
      await flushAsyncWork();
    });
    expect(latest?.visibleBookmarks).toHaveLength(0);
    await act(async () => {
      root.unmount();
      await flushAsyncWork();
    });
  });
});
