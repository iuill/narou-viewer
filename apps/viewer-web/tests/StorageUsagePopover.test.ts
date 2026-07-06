import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import { StorageUsagePopover, formatStorageBytes } from "../src/StorageUsagePopover";
import { fetchStorageUsage, fetchStorageUsageProgress } from "../src/features/storage/api";

vi.mock("../src/features/storage/api", () => ({
  fetchStorageUsage: vi.fn(),
  fetchStorageUsageProgress: vi.fn()
}));

function installDom(): JSDOM {
  const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
    url: "http://localhost/"
  });

  vi.stubGlobal("window", dom.window);
  vi.stubGlobal("document", dom.window.document);
  vi.stubGlobal("navigator", dom.window.navigator);
  vi.stubGlobal("HTMLElement", dom.window.HTMLElement);
  vi.stubGlobal("HTMLButtonElement", dom.window.HTMLButtonElement);
  vi.stubGlobal("Node", dom.window.Node);
  vi.stubGlobal("Event", dom.window.Event);
  vi.stubGlobal("MouseEvent", dom.window.MouseEvent);
  vi.stubGlobal("KeyboardEvent", dom.window.KeyboardEvent);
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);

  return dom;
}

async function flushAsyncWork(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
}

describe("StorageUsagePopover", () => {
  beforeEach(() => {
    vi.mocked(fetchStorageUsageProgress).mockResolvedValue({
      state: "running",
      phase: "preparing",
      checkedNovels: 0,
      totalNovels: 0
    });
  });

  afterEach(() => {
    vi.mocked(fetchStorageUsage).mockReset();
    vi.mocked(fetchStorageUsageProgress).mockReset();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("formats byte values for compact display", () => {
    expect(formatStorageBytes(0)).toBe("0 B");
    expect(formatStorageBytes(1024)).toBe("1.00 KB");
    expect(formatStorageBytes(1536)).toBe("1.50 KB");
    expect(formatStorageBytes(2 * 1024 * 1024)).toBe("2.00 MB");
  });

  it("loads and renders category, selected novel, and novel ranking usage", async () => {
    const dom = installDom();
    vi.mocked(fetchStorageUsage).mockResolvedValue({
      checkedAt: "2026-07-02T00:00:00Z",
      totalBytes: 4096,
      categories: [
        { id: "novelData", label: "小説データ", bytes: 2048, fileCount: 2 },
        { id: "cache", label: "キャッシュ", bytes: 1024, fileCount: 1 },
        { id: "other", label: "その他", bytes: 1024, fileCount: 1 }
      ],
      novels: [
        {
          novelId: "novel-a",
          title: "小説A",
          author: "作者A",
          siteName: "小説家になろう",
          source: "novel-fetcher",
          totalBytes: 3072,
          novelDataBytes: 2048,
          cacheBytes: 1024,
          otherBytes: 0,
          fileCount: 3
        }
      ]
    });
    const rootElement = dom.window.document.getElementById("root");
    if (!rootElement) {
      throw new Error("root element not found");
    }
    const root: Root = createRoot(rootElement);

    await act(async () => {
      root.render(createElement(StorageUsagePopover, { selectedNovelId: "novel-a" }));
    });

    const trigger = rootElement.querySelector("button");
    if (!(trigger instanceof dom.window.HTMLButtonElement)) {
      throw new Error("trigger not found");
    }
    await act(async () => {
      trigger.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
      await flushAsyncWork();
    });

    expect(fetchStorageUsage).toHaveBeenCalledTimes(1);
    expect(fetchStorageUsage).toHaveBeenCalledWith(expect.any(String));
    expect(dom.window.document.body.textContent).toContain("ストレージ使用量");
    expect(dom.window.document.body.textContent).toContain("全体内訳");
    expect(dom.window.document.body.textContent).toContain("小説データ");
    expect(dom.window.document.body.textContent).toContain("キャッシュ");
    expect(dom.window.document.body.textContent).toContain("選択作品");
    expect(dom.window.document.body.textContent).toContain("小説A");
    expect(dom.window.document.body.textContent).toContain("3.00 KB");

    const rankingBar = dom.window.document.querySelector(".storage-usage-novel-list .storage-usage-split-bar .novel");
    if (!(rankingBar instanceof dom.window.HTMLElement)) {
      throw new Error("ranking bar not found");
    }
    expect(rankingBar.style.width).toBe("50%");
    expect(dom.window.document.querySelector('[role="img"][aria-label="全体 4.00 KB"]')).toBeTruthy();

    await act(async () => {
      root.unmount();
    });
  });

  it("shows rough novel progress while storage usage is loading", async () => {
    const dom = installDom();
    let resolveUsage: ((value: Awaited<ReturnType<typeof fetchStorageUsage>>) => void) | undefined;
    vi.mocked(fetchStorageUsage).mockReturnValue(
      new Promise((resolve) => {
        resolveUsage = resolve;
      })
    );
    vi.mocked(fetchStorageUsageProgress).mockResolvedValue({
      state: "running",
      phase: "scanning",
      checkedNovels: 3,
      totalNovels: 10,
      startedAt: "2026-07-02T00:00:00Z",
      updatedAt: "2026-07-02T00:00:01Z"
    });
    const rootElement = dom.window.document.getElementById("root");
    if (!rootElement) {
      throw new Error("root element not found");
    }
    const root: Root = createRoot(rootElement);

    await act(async () => {
      root.render(createElement(StorageUsagePopover, { selectedNovelId: null }));
    });

    const trigger = rootElement.querySelector("button");
    if (!(trigger instanceof dom.window.HTMLButtonElement)) {
      throw new Error("trigger not found");
    }
    await act(async () => {
      trigger.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
      await flushAsyncWork();
    });

    const requestId = vi.mocked(fetchStorageUsage).mock.calls[0]?.[0];
    expect(typeof requestId).toBe("string");
    expect(fetchStorageUsageProgress).toHaveBeenCalledWith(requestId);
    expect(dom.window.document.body.textContent).toContain("確認中");
    expect(dom.window.document.body.textContent).toContain("目安 3 / 10 作品");

    await act(async () => {
      resolveUsage?.({
        checkedAt: "2026-07-02T00:00:02Z",
        totalBytes: 0,
        categories: [
          { id: "novelData", label: "小説データ", bytes: 0, fileCount: 0 },
          { id: "cache", label: "キャッシュ", bytes: 0, fileCount: 0 },
          { id: "other", label: "その他", bytes: 0, fileCount: 0 }
        ],
        novels: []
      });
      await flushAsyncWork();
    });

    expect(dom.window.document.body.textContent).not.toContain("目安 3 / 10 作品");

    await act(async () => {
      root.unmount();
    });
  });

  it("keeps the selected novel visible when it is outside the top storage entries", async () => {
    const dom = installDom();
    const largeNovels = Array.from({ length: 13 }, (_, index) => ({
      novelId: `novel-${index}`,
      title: `大きい小説${index + 1}`,
      siteName: "小説家になろう",
      source: "novel-fetcher" as const,
      totalBytes: 10_000 - index,
      novelDataBytes: 10_000 - index,
      cacheBytes: 0,
      otherBytes: 0,
      fileCount: 1
    }));
    vi.mocked(fetchStorageUsage).mockResolvedValue({
      checkedAt: "2026-07-02T00:00:00Z",
      totalBytes: 140_000,
      categories: [
        { id: "novelData", label: "小説データ", bytes: 130_000, fileCount: 14 },
        { id: "cache", label: "キャッシュ", bytes: 10_000, fileCount: 1 },
        { id: "other", label: "その他", bytes: 0, fileCount: 0 }
      ],
      novels: [
        ...largeNovels,
        {
          novelId: "selected-small",
          title: "選択中の小説",
          siteName: "小説家になろう",
          source: "novel-fetcher",
          totalBytes: 512,
          novelDataBytes: 256,
          cacheBytes: 256,
          otherBytes: 0,
          fileCount: 2
        }
      ]
    });
    const rootElement = dom.window.document.getElementById("root");
    if (!rootElement) {
      throw new Error("root element not found");
    }
    const root: Root = createRoot(rootElement);

    await act(async () => {
      root.render(createElement(StorageUsagePopover, { selectedNovelId: "selected-small" }));
    });

    const trigger = rootElement.querySelector("button");
    if (!(trigger instanceof dom.window.HTMLButtonElement)) {
      throw new Error("trigger not found");
    }
    await act(async () => {
      trigger.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
      await flushAsyncWork();
    });

    const firstNovelTitle = dom.window.document.querySelector(".storage-usage-novel-list .storage-usage-novel strong");
    expect(firstNovelTitle?.textContent).toBe("選択中の小説");
    expect(dom.window.document.body.textContent).toContain("12 / 14 作品");

    await act(async () => {
      root.unmount();
    });
  });

  it("closes the dialog with Escape", async () => {
    const dom = installDom();
    vi.mocked(fetchStorageUsage).mockResolvedValue({
      checkedAt: "2026-07-02T00:00:00Z",
      totalBytes: 0,
      categories: [
        { id: "novelData", label: "小説データ", bytes: 0, fileCount: 0 },
        { id: "cache", label: "キャッシュ", bytes: 0, fileCount: 0 },
        { id: "other", label: "その他", bytes: 0, fileCount: 0 }
      ],
      novels: []
    });
    const rootElement = dom.window.document.getElementById("root");
    if (!rootElement) {
      throw new Error("root element not found");
    }
    const root: Root = createRoot(rootElement);

    await act(async () => {
      root.render(createElement(StorageUsagePopover, { selectedNovelId: null }));
    });

    const trigger = rootElement.querySelector("button");
    if (!(trigger instanceof dom.window.HTMLButtonElement)) {
      throw new Error("trigger not found");
    }
    await act(async () => {
      trigger.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
      await flushAsyncWork();
    });
    expect(dom.window.document.querySelector('[role="dialog"]')).toBeTruthy();

    await act(async () => {
      dom.window.document.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    });
    expect(dom.window.document.querySelector('[role="dialog"]')).toBeNull();

    await act(async () => {
      root.unmount();
    });
  });

  it("retries automatically after reopening when the first load fails", async () => {
    const dom = installDom();
    vi.mocked(fetchStorageUsage)
      .mockRejectedValueOnce(new Error("network down"))
      .mockResolvedValueOnce({
        checkedAt: "2026-07-02T00:00:00Z",
        totalBytes: 1024,
        categories: [
          { id: "novelData", label: "小説データ", bytes: 1024, fileCount: 1 },
          { id: "cache", label: "キャッシュ", bytes: 0, fileCount: 0 },
          { id: "other", label: "その他", bytes: 0, fileCount: 0 }
        ],
        novels: []
      });
    const rootElement = dom.window.document.getElementById("root");
    if (!rootElement) {
      throw new Error("root element not found");
    }
    const root: Root = createRoot(rootElement);

    await act(async () => {
      root.render(createElement(StorageUsagePopover, { selectedNovelId: null }));
    });

    const trigger = rootElement.querySelector("button");
    if (!(trigger instanceof dom.window.HTMLButtonElement)) {
      throw new Error("trigger not found");
    }
    await act(async () => {
      trigger.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
      await flushAsyncWork();
    });
    expect(fetchStorageUsage).toHaveBeenCalledTimes(1);
    expect(dom.window.document.body.textContent).toContain("network down");

    await act(async () => {
      trigger.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
    });
    await act(async () => {
      trigger.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
      await flushAsyncWork();
    });

    expect(fetchStorageUsage).toHaveBeenCalledTimes(2);
    expect(dom.window.document.body.textContent).toContain("1.00 KB");
    expect(dom.window.document.body.textContent).not.toContain("network down");

    await act(async () => {
      root.unmount();
    });
  });
});
