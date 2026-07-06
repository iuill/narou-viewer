import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement, type ComponentProps } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import { LibraryPanel } from "../src/LibraryPanel";

type PanelProps = ComponentProps<typeof LibraryPanel>;

function createProps(overrides: Partial<PanelProps> = {}): PanelProps {
  return {
    activeFetcherTaskEntries: [
      {
        key: "task-1",
        task: {
          id: "task-1",
          type: "download",
          target: "https://example.com/n1",
          novelId: "n1",
          novelIds: [],
          novelTitle: "小説A",
          novelAuthor: "作者A",
          status: "running",
          message: "取得中",
          warnings: [],
          errorMessage: null,
          createdAt: "2026-03-22T10:00:00Z",
          startedAt: "2026-03-22T10:01:00Z",
          completedAt: null,
          elapsedTime: 1000,
          progress: 50,
          totalSteps: 10,
          currentStep: 5,
          savedEpisodeCount: null,
          failedEpisodeId: null,
          resumeEpisodeId: null
        }
      }
    ],
    activeFetcherTasksCount: 1,
    cancelingFetcherTaskIds: new Set(),
    downloadForce: true,
    downloadTarget: "n1234ab",
    filteredNovelsCount: 2,
    hasActiveFetcherTasks: true,
    isDownloadComposerOpen: true,
    isDownloadDropActive: true,
    isDownloadSubmitting: false,
    isLibraryExporting: false,
    libraryFilterQuery: "小説",
    libraryNotice: "ダウンロードを準備中です。",
    libraryPagination: {
      currentPage: 2,
      endItemNumber: 12,
      startItemNumber: 7,
      totalItems: 18,
      totalPages: 3
    },
    fetcherQueueRunning: true,
    fetcherStatusCheckedAt: "2026-03-22T10:05:00Z",
    fetcherStatusError: null,
    novelsCount: 3,
    onClearLibraryFilter: vi.fn(),
    onCloseDownloadComposer: vi.fn(),
    onDownloadDragEnter: vi.fn(),
    onDownloadDragLeave: vi.fn(),
    onDownloadDragOver: vi.fn(),
    onDownloadDrop: vi.fn(),
    onDownloadForceChange: vi.fn(),
    onDownloadSubmit: vi.fn(),
    onDownloadTargetChange: vi.fn(),
    onExportLibrary: vi.fn(),
    onCancelFetcherTask: vi.fn(),
    onLibraryFilterQueryChange: vi.fn(),
    onLibraryPageChange: vi.fn(),
    onResumeNovel: vi.fn(),
    onSelectNovel: vi.fn(),
    onToggleDownloadComposer: vi.fn(),
    queueStatusLabel: "実行中",
    resumingNovelIds: new Set(),
    selectedNovelId: "n1",
    showStoryAction: false,
    visibleLibraryNovels: [
      {
        novelId: "n1",
        title: "小説A",
        author: "作者A",
        siteName: "小説家になろう",
        tocUrl: "https://example.com/n1",
        story: "小説Aのあらすじです。",
        updatedAt: "2026-03-22T10:00:00Z",
        lastReadEpisodeIndex: "12",
        lastReadEpisodeTitle: "第12話",
        bookmarkCount: 2,
        totalEpisodes: 30
      },
      {
        novelId: "n2",
        title: "小説B",
        author: "",
        siteName: "カクヨム",
        tocUrl: "https://kakuyomu.jp/works/1",
        story: "小説Bのあらすじです。",
        updatedAt: "2026-03-22T09:00:00Z",
        lastReadEpisodeIndex: null,
        lastReadEpisodeTitle: null,
        bookmarkCount: 0,
        totalEpisodes: 10
      }
    ],
    ...overrides
  };
}

function installDom(): JSDOM {
  const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
    url: "http://localhost/"
  });

  vi.stubGlobal("window", dom.window);
  vi.stubGlobal("document", dom.window.document);
  vi.stubGlobal("navigator", dom.window.navigator);
  vi.stubGlobal("HTMLElement", dom.window.HTMLElement);
  vi.stubGlobal("HTMLInputElement", dom.window.HTMLInputElement);
  vi.stubGlobal("HTMLButtonElement", dom.window.HTMLButtonElement);
  vi.stubGlobal("HTMLFormElement", dom.window.HTMLFormElement);
  vi.stubGlobal("Node", dom.window.Node);
  vi.stubGlobal("Event", dom.window.Event);
  vi.stubGlobal("MouseEvent", dom.window.MouseEvent);
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);

  return dom;
}

async function renderPanel(props: PanelProps): Promise<{ container: HTMLElement; root: Root; dom: JSDOM }> {
  const dom = installDom();
  const container = dom.window.document.getElementById("root");

  if (!container) {
    throw new Error("root container not found");
  }

  const root = createRoot(container);

  await act(async () => {
    root.render(createElement(LibraryPanel, props));
  });

  return { container, root, dom };
}

function getButtonByText(container: HTMLElement, text: string): HTMLButtonElement {
  const normalizedTarget = text.replace(/\s+/g, " ").trim();
  const button = Array.from(container.querySelectorAll("button")).find((candidate) => {
    const normalizedText = candidate.textContent?.replace(/\s+/g, " ").trim() ?? "";

    return normalizedText === normalizedTarget || normalizedText.includes(normalizedTarget);
  });

  if (!(button instanceof HTMLButtonElement)) {
    throw new Error(`button not found: ${text}`);
  }

  return button;
}

async function click(element: Element, dom: JSDOM): Promise<void> {
  await act(async () => {
    element.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
  });
}

async function changeInput(input: HTMLInputElement, value: string, dom: JSDOM): Promise<void> {
  await act(async () => {
    const descriptor = Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value");
    descriptor?.set?.call(input, value);
    input.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
    input.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
  });
}

async function changeCheckbox(input: HTMLInputElement, checked: boolean, dom: JSDOM): Promise<void> {
  await act(async () => {
    const descriptor = Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "checked");
    descriptor?.set?.call(input, checked);
    input.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
  });
}

async function submitForm(form: HTMLFormElement, dom: JSDOM): Promise<void> {
  await act(async () => {
    form.dispatchEvent(new dom.window.Event("submit", { bubbles: true, cancelable: true }));
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("LibraryPanel", () => {
  it("composer と進捗と一覧を描画して主要操作を処理する", async () => {
    const props = createProps();
    const { container, root, dom } = await renderPanel(props);

    expect(container.textContent).toContain("Library");
    expect(container.textContent).toContain("ダウンロードを準備中です。");
    expect(container.textContent).toContain("ダウンロード進捗");
    expect(container.textContent).toContain("小説A");

    const textInput = container.querySelector('input[type="text"]');
    const checkbox = container.querySelector('input[type="checkbox"]');
    const searchInput = container.querySelector('input[type="search"]');
    const form = container.querySelector("form");

    if (!(textInput instanceof dom.window.HTMLInputElement) || !(checkbox instanceof dom.window.HTMLInputElement) || !(searchInput instanceof dom.window.HTMLInputElement) || !(form instanceof dom.window.HTMLFormElement)) {
      throw new Error("form controls not found");
    }

    await changeInput(textInput, "n9999xy", dom);
    await changeCheckbox(checkbox, false, dom);
    await changeInput(searchInput, "作者A", dom);
    await submitForm(form, dom);
    await click(getButtonByText(container, "閉じる"), dom);
    await click(getButtonByText(container, "クリア"), dom);
    await click(getButtonByText(container, "前へ"), dom);
    await click(getButtonByText(container, "次へ"), dom);
    await click(getButtonByText(container, "エクスポート"), dom);
    await click(getButtonByText(container, "小説B"), dom);
    await click(container.querySelector('button[aria-label="小説を追加"]') as Element, dom);

    expect(props.onDownloadSubmit).toHaveBeenCalledTimes(1);
    expect(props.onCloseDownloadComposer).toHaveBeenCalledTimes(1);
    expect(props.onClearLibraryFilter).toHaveBeenCalledTimes(1);
    expect(props.onLibraryPageChange).toHaveBeenNthCalledWith(1, 1);
    expect(props.onLibraryPageChange).toHaveBeenNthCalledWith(2, 3);
    expect(props.onExportLibrary).toHaveBeenCalledTimes(1);
    expect(props.onSelectNovel).toHaveBeenCalledWith("n2");
    expect(props.onToggleDownloadComposer).toHaveBeenCalledTimes(1);

    await act(async () => {
      root.unmount();
    });
  });

  it("空状態を作品数に応じて切り替える", async () => {
    const props = createProps({
      activeFetcherTaskEntries: [],
      activeFetcherTasksCount: 0,
      filteredNovelsCount: 0,
      hasActiveFetcherTasks: false,
      isDownloadComposerOpen: false,
      libraryFilterQuery: "",
      novelsCount: 0,
      visibleLibraryNovels: []
    });
    const { container, root } = await renderPanel(props);

    expect(container.textContent).toContain("まだ作品がありません");
    expect(container.textContent).toContain("`＋` から Nコードや作品 URL を追加できます。");

    await act(async () => {
      root.unmount();
    });
  });

  it("書籍化カバーがある作品は一覧カードに表紙を表示する", async () => {
    const props = createProps({
      visibleLibraryNovels: [
        {
          novelId: "n1",
          title: "小説A",
          author: "作者A",
          siteName: "小説家になろう",
          tocUrl: "https://example.com/n1",
          story: "小説Aのあらすじです。",
          updatedAt: "2026-03-22T10:00:00Z",
          lastReadEpisodeIndex: "12",
          lastReadEpisodeTitle: "第12話",
          bookmarkCount: 2,
          totalEpisodes: 30,
          publicationCoverImageUrl: "https://example.test/cover.jpg",
          publicationCoverKind: "novel",
          publicationCoverSource: "Google Books",
          publicationCoverSourceUrl: "https://books.google.test/volume"
        }
      ]
    });
    const { container, root, dom } = await renderPanel(props);
    const cover = container.querySelector(".library-card-cover img");
    const coverSourceLink = container.querySelector(".library-card-cover-source-link");

    expect(cover).toBeInstanceOf(dom.window.HTMLImageElement);
    expect(cover?.getAttribute("src")).toBe("https://example.test/cover.jpg");
    expect(cover?.getAttribute("alt")).toBe("");
    expect(coverSourceLink).toBeInstanceOf(dom.window.HTMLAnchorElement);
    expect(coverSourceLink?.getAttribute("href")).toBe("https://books.google.test/volume");
    expect(coverSourceLink?.getAttribute("aria-label")).toContain("Google Books で見る");
    expect(container.textContent).toContain("カバー出典");
    expect(container.textContent).toContain("Powered by Google");
    expect(container.textContent).toContain("Google Books で見る");

    await act(async () => {
      root.unmount();
    });
  });

  it("作品がないときと出力中はエクスポートボタンを無効化する", async () => {
    const onExportLibrary = vi.fn();
    const emptyProps = createProps({
      activeFetcherTaskEntries: [],
      activeFetcherTasksCount: 0,
      filteredNovelsCount: 0,
      hasActiveFetcherTasks: false,
      isDownloadComposerOpen: false,
      novelsCount: 0,
      onExportLibrary,
      visibleLibraryNovels: []
    });
    const { container, root, dom } = await renderPanel(emptyProps);
    const exportButton = getButtonByText(container, "エクスポート");

    expect(exportButton.disabled).toBe(true);
    await click(exportButton, dom);
    expect(onExportLibrary).not.toHaveBeenCalled();

    await act(async () => {
      root.render(createElement(LibraryPanel, createProps({ isLibraryExporting: true, onExportLibrary })));
    });

    const exportingButton = getButtonByText(container, "出力中...");
    expect(exportingButton.disabled).toBe(true);
    await click(exportingButton, dom);
    expect(onExportLibrary).not.toHaveBeenCalled();

    await act(async () => {
      root.unmount();
    });
  });

  it("モバイル向けにはカード内のあらすじを開閉できる", async () => {
    const props = createProps({
      showStoryAction: true,
      visibleLibraryNovels: [
        {
          novelId: "n1",
          title: "小説A",
          author: "作者A",
          siteName: "小説家になろう",
          tocUrl: "https://example.com/n1",
          story: "これはとても長いあらすじです。".repeat(10),
          updatedAt: "2026-03-22T10:00:00Z",
          lastReadEpisodeIndex: "12",
          lastReadEpisodeTitle: "第12話",
          bookmarkCount: 2,
          totalEpisodes: 30
        }
      ]
    });
    const { container, root, dom } = await renderPanel(props);

    await click(getButtonByText(container, "あらすじ"), dom);
    expect(props.onSelectNovel).not.toHaveBeenCalled();
    expect(container.textContent).toContain("これはとても長いあらすじです。");
    expect(container.textContent).toContain("続きを読む");

    await click(getButtonByText(container, "続きを読む"), dom);
    expect(container.textContent).toContain("折りたたみ");

    await click(getButtonByText(container, "あらすじを閉じる"), dom);
    expect(container.textContent).not.toContain("折りたたみ");

    await act(async () => {
      root.unmount();
    });
  });

  it("モバイル向けにはカードから書籍化情報を開ける", async () => {
    const onOpenNovelPublications = vi.fn();
    const props = createProps({
      onOpenNovelPublications,
      showStoryAction: true,
      visibleLibraryNovels: [
        {
          novelId: "n1",
          title: "小説A",
          author: "作者A",
          siteName: "小説家になろう",
          tocUrl: "https://example.com/n1",
          story: "小説Aのあらすじです。",
          updatedAt: "2026-03-22T10:00:00Z",
          lastReadEpisodeIndex: "12",
          lastReadEpisodeTitle: "第12話",
          bookmarkCount: 2,
          totalEpisodes: 30
        }
      ]
    });
    const { container, root, dom } = await renderPanel(props);

    await click(getButtonByText(container, "書籍情報"), dom);
    expect(onOpenNovelPublications).toHaveBeenCalledWith("n1");
    expect(props.onSelectNovel).not.toHaveBeenCalled();

    await act(async () => {
      root.unmount();
    });
  });

  it("再開中の作品は再開ボタンを無効化する", async () => {
    const props = createProps({
      resumingNovelIds: new Set(["n1"]),
      showStoryAction: true,
      visibleLibraryNovels: [
        {
          novelId: "n1",
          title: "小説A",
          author: "作者A",
          siteName: "小説家になろう",
          tocUrl: "https://example.com/n1",
          story: "小説Aのあらすじです。",
          updatedAt: "2026-03-22T10:00:00Z",
          lastReadEpisodeIndex: "12",
          lastReadEpisodeTitle: "第12話",
          bookmarkCount: 2,
          totalEpisodes: 30,
          fetchStatus: "failed",
          savedEpisodes: 12,
          failedEpisodeId: "13",
          resumeEpisodeId: "13"
        }
      ]
    });
    const { container, root, dom } = await renderPanel(props);

    const resumeButton = container.querySelector('button[aria-label="小説A の未取得話を再開"]');
    expect(resumeButton?.textContent).toContain("再開中...");
    expect(resumeButton).toBeInstanceOf(dom.window.HTMLButtonElement);
    if (!(resumeButton instanceof dom.window.HTMLButtonElement)) {
      throw new Error("resume button not found");
    }
    expect(resumeButton.disabled).toBe(true);
    await click(resumeButton, dom);
    expect(props.onResumeNovel).not.toHaveBeenCalled();

    await act(async () => {
      root.unmount();
    });
  });

  it("取得タブでは更新と再開の全対象を操作できる", async () => {
    const onResumeNovel = vi.fn();
    const onUpdateNovel = vi.fn();
    const updatableNovels = Array.from({ length: 9 }, (_, index) => {
      const sequence = index + 1;

      return {
        novelId: `u${sequence}`,
        fetcherWorkId: `work-u${sequence}`,
        title: `更新${sequence}`,
        author: "作者",
        siteName: "小説家になろう",
        tocUrl: `https://example.com/u${sequence}`,
        updatedAt: "2026-03-22T10:00:00Z",
        lastReadEpisodeIndex: null,
        lastReadEpisodeTitle: null,
        latestBookmarkEpisodeIndex: null,
        bookmarkCount: 0,
        totalEpisodes: 10
      };
    });
    const resumableNovels = Array.from({ length: 9 }, (_, index) => {
      const sequence = index + 1;

      return {
        novelId: `r${sequence}`,
        fetcherWorkId: `work-r${sequence}`,
        title: `再開${sequence}`,
        author: "作者",
        siteName: "小説家になろう",
        tocUrl: `https://example.com/r${sequence}`,
        updatedAt: "2026-03-22T10:00:00Z",
        lastReadEpisodeIndex: null,
        lastReadEpisodeTitle: null,
        latestBookmarkEpisodeIndex: null,
        bookmarkCount: 0,
        totalEpisodes: 10,
        savedEpisodes: 4,
        fetchStatus: "partial"
      };
    });
    const props = createProps({
      activeFetcherTaskEntries: [],
      activeFetcherTasksCount: 0,
      hasActiveFetcherTasks: false,
      isDownloadComposerOpen: false,
      mobileHomeTab: "download",
      onResumeNovel,
      onUpdateNovel,
      resumableNovels,
      updatableNovels
    });
    const { container, root, dom } = await renderPanel(props);

    const rows = Array.from(container.querySelectorAll(".library-maintenance-row"));
    const updateRow = rows.find((row) => row.textContent?.includes("更新9"));
    const resumeRow = rows.find((row) => row.textContent?.includes("再開9"));
    const updateButton = updateRow?.querySelector("button");
    const resumeButton = resumeRow?.querySelector("button");

    expect(container.textContent).toContain("保存済み作品の更新");
    expect(container.textContent).toContain("9 作品");
    expect(updateRow).toBeTruthy();
    expect(resumeRow).toBeTruthy();
    expect(updateButton).toBeInstanceOf(dom.window.HTMLButtonElement);
    expect(resumeButton).toBeInstanceOf(dom.window.HTMLButtonElement);

    if (!(updateButton instanceof dom.window.HTMLButtonElement) || !(resumeButton instanceof dom.window.HTMLButtonElement)) {
      throw new Error("maintenance buttons not found");
    }

    await click(updateButton, dom);
    await click(resumeButton, dom);

    expect(onUpdateNovel).toHaveBeenCalledWith("u9");
    expect(onResumeNovel).toHaveBeenCalledWith("r9");

    await act(async () => {
      root.unmount();
    });
  });

  it("取得タブではタスク中の作品の更新と再開を無効化する", async () => {
    const onResumeNovel = vi.fn();
    const onUpdateNovel = vi.fn();
    const busyNovelIds = new Set(["busy-update", "busy-resume"]);
    const props = createProps({
      activeFetcherTaskEntries: [],
      activeFetcherTasksCount: 0,
      hasActiveFetcherTasks: false,
      isDownloadComposerOpen: false,
      mobileHomeTab: "download",
      onResumeNovel,
      onUpdateNovel,
      resumingNovelIds: busyNovelIds,
      resumableNovels: [
        {
          novelId: "busy-resume",
          fetcherWorkId: "work-busy-resume",
          title: "再開中の作品",
          author: "作者",
          siteName: "小説家になろう",
          tocUrl: "https://example.com/resume",
          updatedAt: "2026-03-22T10:00:00Z",
          lastReadEpisodeIndex: null,
          lastReadEpisodeTitle: null,
          latestBookmarkEpisodeIndex: null,
          bookmarkCount: 0,
          totalEpisodes: 10,
          savedEpisodes: 4,
          fetchStatus: "partial"
        }
      ],
      updatableNovels: [
        {
          novelId: "busy-update",
          fetcherWorkId: "work-busy-update",
          title: "更新中の作品",
          author: "作者",
          siteName: "小説家になろう",
          tocUrl: "https://example.com/update",
          updatedAt: "2026-03-22T10:00:00Z",
          lastReadEpisodeIndex: null,
          lastReadEpisodeTitle: null,
          latestBookmarkEpisodeIndex: null,
          bookmarkCount: 0,
          totalEpisodes: 10
        }
      ],
      updatingNovelIds: busyNovelIds
    });
    const { container, root, dom } = await renderPanel(props);

    const rows = Array.from(container.querySelectorAll(".library-maintenance-row"));
    const updateButton = rows.find((row) => row.textContent?.includes("更新中の作品"))?.querySelector("button");
    const resumeButton = rows.find((row) => row.textContent?.includes("再開中の作品"))?.querySelector("button");

    expect(updateButton).toBeInstanceOf(dom.window.HTMLButtonElement);
    expect(resumeButton).toBeInstanceOf(dom.window.HTMLButtonElement);

    if (!(updateButton instanceof dom.window.HTMLButtonElement) || !(resumeButton instanceof dom.window.HTMLButtonElement)) {
      throw new Error("busy maintenance buttons not found");
    }

    expect(updateButton.disabled).toBe(true);
    expect(resumeButton.disabled).toBe(true);

    await click(updateButton, dom);
    await click(resumeButton, dom);

    expect(onUpdateNovel).not.toHaveBeenCalled();
    expect(onResumeNovel).not.toHaveBeenCalled();

    await act(async () => {
      root.unmount();
    });
  });
});
