import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement, type ComponentProps } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import { TocPanel } from "../src/TocPanel";

type PanelProps = ComponentProps<typeof TocPanel>;

function createProps(overrides: Partial<PanelProps> = {}): PanelProps {
  return {
    bookmarks: [
      {
        id: "b1",
        novelId: "n1",
        episodeIndex: "12",
        position: 100,
        label: "しおり1",
        createdAt: "2026-03-22T10:00:00Z"
      },
      {
        id: "b2",
        novelId: "n1",
        episodeIndex: "11",
        position: 50,
        label: null,
        createdAt: "2026-03-22T09:00:00Z"
      },
      {
        id: "b3",
        novelId: "n1",
        episodeIndex: "10",
        position: 10,
        label: null,
        createdAt: "2026-03-22T08:00:00Z"
      },
      {
        id: "b4",
        novelId: "n1",
        episodeIndex: "9",
        position: 10,
        label: null,
        createdAt: "2026-03-22T07:00:00Z"
      }
    ],
    currentNovel: {
      novelId: "n1",
      title: "小説A",
      author: "作者A",
      fetcherWorkId: "work-1",
      totalEpisodes: 18
    },
    episodeDisplayLookup: new Map([
      ["9", { episodeIndex: "9", title: "第9話", chapter: null, subchapter: null, order: 9 }],
      ["10", { episodeIndex: "10", title: "第10話", chapter: null, subchapter: null, order: 10 }],
      ["11", { episodeIndex: "11", title: "第11話", chapter: null, subchapter: null, order: 11 }],
      ["12", { episodeIndex: "12", title: "第12話", chapter: "第一章", subchapter: null, order: 12 }]
    ]),
    isMobileLibraryViewport: true,
    isNovelActionSubmitting: false,
    isResumeSubmitting: false,
    isNovelLoading: false,
    publicationProps: {
      displayCoverEntryId: "",
      entries: [],
      isLoading: false,
      savingEntryId: null,
      onCreateISBN: vi.fn(),
      onSaveISBN: vi.fn(),
      onClear: vi.fn(),
      onDisable: vi.fn(),
      onRedisplay: vi.fn(),
      onSetDisplayCover: vi.fn()
    },
    isShowingAllBookmarks: false,
    isTocStoryExpanded: false,
    isTocStoryTruncated: true,
    lastReadEpisodeIndex: "12",
    latestBookmark: {
      id: "b1",
      novelId: "n1",
      episodeIndex: "12",
      position: 100,
      label: "しおり1",
      createdAt: "2026-03-22T10:00:00Z"
    },
    novelsCount: 1,
    onBackToLibrary: vi.fn(),
    onDeleteBookmark: vi.fn(),
    onOpenBookmark: vi.fn(),
    onOpenEpisode: vi.fn(),
    onRemoveNovel: vi.fn(),
    onResumeNovel: vi.fn(),
    onTocPageChange: vi.fn(),
    onToggleShowingAllBookmarks: vi.fn(),
    onToggleStoryExpanded: vi.fn(),
    onUpdateNovel: vi.fn(),
    pendingBookmarkId: null,
    preferFriendlyEpisodeLabels: false,
    readerState: {
      lastReadEpisodeIndex: "12",
      position: 321
    },
    selectedEpisodeIndex: "12",
    toc: {
      updatedAt: "2026-03-22T10:00:00Z"
    },
    tocPagination: {
      currentPage: 2,
      endItemNumber: 12,
      startItemNumber: 7,
      totalItems: 18,
      totalPages: 3
    },
    tocStoryText: "長いあらすじです。",
    visibleBookmarks: [
      {
        id: "b1",
        novelId: "n1",
        episodeIndex: "12",
        position: 100,
        label: "しおり1",
        createdAt: "2026-03-22T10:00:00Z"
      },
      {
        id: "b2",
        novelId: "n1",
        episodeIndex: "11",
        position: 50,
        label: null,
        createdAt: "2026-03-22T09:00:00Z"
      }
    ],
    visibleTocEpisodes: [
      {
        episodeIndex: "12",
        title: "第12話",
        chapter: "第一章",
        subchapter: null,
        updatedAt: "2026-03-22T10:00:00Z",
        contentEtag: "etag-12"
      },
      {
        episodeIndex: "11",
        title: "第11話",
        chapter: null,
        subchapter: null,
        updatedAt: "2026-03-21T10:00:00Z",
        contentEtag: "etag-11"
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
    root.render(createElement(TocPanel, props));
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

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("TocPanel", () => {
  it("目次と栞と話一覧を描画して主要操作を処理する", async () => {
    const props = createProps();
    const { container, root, dom } = await renderPanel(props);

    expect(container.textContent).toContain("小説A");
    expect(container.textContent).toContain("最終既読");
    expect(container.textContent).toContain("最新栞");
    expect(container.textContent).toContain("第12話");
    expect(container.querySelector(".episode-block")).toBeTruthy();
    expect(container.querySelector(".publication-block")).toBeNull();
    expect(container.querySelector(".bookmark-block")).toBeNull();

    await click(getButtonByText(container, "ライブラリへ"), dom);
    await click(getButtonByText(container, "すべて表示"), dom);
    expect(getButtonByText(container, "再開").disabled).toBe(true);
    await click(getButtonByText(container, "更新"), dom);
    await click(getButtonByText(container, "削除"), dom);
    await click(getButtonByText(container, "書籍情報"), dom);
    expect(container.querySelector(".publication-block")).toBeTruthy();
    expect(container.querySelector(".episode-block")).toBeNull();
    await click(getButtonByText(container, "栞"), dom);
    const bookmarkDeleteButton = container.querySelector(".bookmark-item .danger");
    if (!(bookmarkDeleteButton instanceof dom.window.HTMLButtonElement)) {
      throw new Error("bookmark delete button not found");
    }
    await click(getButtonByText(container, "開く"), dom);
    await click(bookmarkDeleteButton, dom);
    await click(getButtonByText(container, "話"), dom);
    await click(getButtonByText(container, "前へ"), dom);
    await click(getButtonByText(container, "次へ"), dom);
    await click(getButtonByText(container, "第一章 - 第12話"), dom);

    expect(props.onBackToLibrary).toHaveBeenCalledTimes(1);
    expect(props.onToggleStoryExpanded).toHaveBeenCalledTimes(1);
    expect(props.onResumeNovel).not.toHaveBeenCalled();
    expect(props.onUpdateNovel).toHaveBeenCalledTimes(1);
    expect(props.onRemoveNovel).toHaveBeenCalledTimes(1);
    expect(props.onOpenBookmark).toHaveBeenCalled();
    expect(props.onDeleteBookmark).toHaveBeenCalledWith("b1");
    expect(props.onTocPageChange).toHaveBeenNthCalledWith(1, 1);
    expect(props.onTocPageChange).toHaveBeenNthCalledWith(2, 3);
    expect(props.onOpenEpisode).toHaveBeenCalled();

    await act(async () => {
      root.unmount();
    });
  });

  it("toc がないときは空メッセージを出す", async () => {
    const props = createProps({
      currentNovel: null,
      novelsCount: 0,
      toc: null
    });
    const { container, root } = await renderPanel(props);

    expect(container.textContent).toContain("ライブラリに作品を追加すると、ここに目次が表示されます。");

    await act(async () => {
      root.unmount();
    });
  });

  it("栞や話が空なら補助メッセージを出す", async () => {
    const props = createProps({
      bookmarks: [],
      lastReadEpisodeIndex: null,
      latestBookmark: null,
      tocPagination: {
        currentPage: 1,
        endItemNumber: 0,
        startItemNumber: 0,
        totalItems: 0,
        totalPages: 1
      },
      visibleBookmarks: [],
      visibleTocEpisodes: []
    });
    const { container, root, dom } = await renderPanel(props);

    expect(container.textContent).toContain("話データがありません。");
    expect(container.textContent).toContain("未読");
    expect(container.textContent).toContain("なし");
    await act(async () => {
      getButtonByText(container, "栞").dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
    });
    expect(container.textContent).toContain("まだ栞はありません。");

    await act(async () => {
      root.unmount();
    });
  });

  it("作品が切り替わったら話タブへ戻る", async () => {
    const props = createProps();
    const { container, root, dom } = await renderPanel(props);

    await click(getButtonByText(container, "書籍情報"), dom);
    expect(container.querySelector(".publication-block")).toBeTruthy();

    await act(async () => {
      root.render(
        createElement(
          TocPanel,
          createProps({
            currentNovel: {
              ...props.currentNovel,
              novelId: "n2",
              title: "小説B"
            }
          })
        )
      );
    });

    expect(container.querySelector(".episode-block")).toBeTruthy();
    expect(container.querySelector(".publication-block")).toBeNull();

    await act(async () => {
      root.unmount();
    });
  });

  it("再開中は再開ボタンを無効化する", async () => {
    const props = createProps({
      currentNovel: {
        novelId: "n1",
        title: "小説A",
        author: "作者A",
        fetcherWorkId: "work-1",
        fetchStatus: "failed",
        savedEpisodes: 12,
        totalEpisodes: 30,
        resumeEpisodeId: "13"
      },
      isResumeSubmitting: true
    });
    const { container, root, dom } = await renderPanel(props);

    const resumeButton = getButtonByText(container, "再開中...");
    expect(resumeButton.disabled).toBe(true);
    await click(resumeButton, dom);
    expect(props.onResumeNovel).not.toHaveBeenCalled();

    await act(async () => {
      root.unmount();
    });
  });

  it("未取得話があるときは操作列から再開できる", async () => {
    const props = createProps({
      currentNovel: {
        novelId: "n1",
        title: "小説A",
        author: "作者A",
        fetcherWorkId: "work-1",
        fetchStatus: "partial",
        savedEpisodes: 12,
        totalEpisodes: 30,
        resumeEpisodeId: "13"
      }
    });
    const { container, root, dom } = await renderPanel(props);

    const resumeButton = getButtonByText(container, "再開");
    expect(resumeButton.disabled).toBe(false);
    await click(resumeButton, dom);
    expect(props.onResumeNovel).toHaveBeenCalledWith("n1");

    await act(async () => {
      root.unmount();
    });
  });

  it("resumeEpisodeId がなくても表示中の未取得話から再開できる", async () => {
    const props = createProps({
      currentNovel: {
        novelId: "n1",
        title: "小説A",
        author: "作者A",
        fetcherWorkId: "work-1",
        totalEpisodes: 30
      },
      visibleTocEpisodes: [
        {
          episodeIndex: "13",
          title: "第13話",
          chapter: null,
          subchapter: null,
          updatedAt: "2026-03-23T10:00:00Z",
          contentEtag: "etag-13",
          bodyStatus: "missing"
        }
      ]
    });
    const { container, root, dom } = await renderPanel(props);

    const resumeButton = getButtonByText(container, "再開");
    expect(resumeButton.disabled).toBe(false);
    await click(resumeButton, dom);
    expect(props.onResumeNovel).toHaveBeenCalledWith("n1");

    await act(async () => {
      root.unmount();
    });
  });

  it("初期表示セクションを書籍情報にできる", async () => {
    const props = createProps({ initialSection: "publications" });
    const { container, root } = await renderPanel(props);

    expect(container.querySelector(".publication-block")).toBeTruthy();
    expect(container.querySelector(".episode-block")).toBeNull();
    expect(container.querySelector(".bookmark-block")).toBeNull();

    await act(async () => {
      root.unmount();
    });
  });
});
