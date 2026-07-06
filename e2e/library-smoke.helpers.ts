import {
  expect,
  type test as baseTest,
  type APIRequestContext,
  type Locator,
  type Page
} from "@playwright/test";

type EpisodeIndex = string;

type TocResponse = {
  title: string;
  episodes: Array<{
    episodeIndex: EpisodeIndex;
    title: string;
    chapter?: string | null;
    subchapter?: string | null;
  }>;
};

type Bookmark = {
  id: string;
  episodeIndex: EpisodeIndex;
  position: number;
  label?: string | null;
};

type BookmarksResponse = {
  bookmarks: Bookmark[];
};

type ReaderState = {
  lastReadEpisodeIndex: EpisodeIndex | null;
  position: number;
  stateVersion: number;
};

type LibraryResponse = {
  novels: Array<{
    novelId: string;
    title: string;
    siteName: string;
    story?: string | null;
  }>;
};

export const appTitle = "Web小説ビューア";
export const novelStoryPreviewLength = 120;
export const libraryPageSize = 12;
export const tocPageSize = 50;
export const illustratedNarouTitle = "E2E ケースA 挿絵表示";
export const readingStateNarouTitle = "E2E ケースB 読書ログ";
export const readingStateAnchorNarouTitle = "E2E ケースC 活動アンカー";
export const readerControlsNarouTitle = "E2E ケースD 本文操作";
export const bookmarksNarouTitle = "E2E ケースE 栞";
export const exportNarouTitle = "E2E ケースF エクスポート";
export const kakuyomuTitle = "E2E ケースG カクヨム形式";
export const longNarouEpisodeIndex = "2";

export function setupLibrarySmokeSuite(test: typeof baseTest) {
  test.beforeEach(async ({ page }) => {
    await disableReaderStateSave(page);
  });
}

export function normalizeStoryText(value: string) {
  return value.replace(/\s+/g, " ").trim();
}

export async function loadLibrary(request: APIRequestContext) {
  const response = await request.get("/api/library/novels");
  expect(response.ok(), "failed to load library").toBeTruthy();
  return (await response.json()) as LibraryResponse;
}

export function buildEpisodeLabel(episode: { title: string; chapter?: string | null; subchapter?: string | null }) {
  const chapter = [episode.chapter, episode.subchapter].filter(Boolean).join(" / ");
  return chapter.length > 0 ? `${chapter} - ${episode.title}` : episode.title;
}

export async function gotoLibrary(page: Page, options?: { checkChrome?: boolean }) {
  await page.goto("/");

  await expect(page.getByRole("heading", { name: appTitle })).toBeVisible();
  await expect(page.locator(".library-panel .panel-header p")).toContainText(/作品/u);

  if (options?.checkChrome) {
    const statusButton = page.getByRole("button", { name: /動作状況/ });
    if (await statusButton.isVisible()) {
      await expect(statusButton).toContainText(/正常|一部制限あり|要確認/);
      await expect(page.getByLabel("取得状況")).toBeVisible();
    } else {
      await page.getByRole("button", { name: "状況" }).click();
      const statusPanel = page.locator(".mobile-status-panel");
      await expect(statusPanel.getByRole("heading", { name: "状況" })).toBeVisible();
      await expect(statusPanel.getByText("動作状況", { exact: true })).toBeVisible();
      await expect(statusPanel.getByText("取得状況", { exact: true })).toBeVisible();
      await expect(statusPanel.getByText("AI機能", { exact: true })).toBeVisible();
      await page.getByRole("button", { name: "ライブラリ" }).click();
      await expect(page.locator(".library-panel")).toBeVisible();
    }
  }
}

export async function ensureLibraryPanelVisible(page: Page) {
  const libraryPanel = page.locator(".library-panel");
  if (await libraryPanel.isVisible()) {
    return;
  }

  const backToListButton = page.getByRole("button", { name: "一覧へ戻る" });
  if (await backToListButton.isVisible()) {
    await backToListButton.click();
    await expect(libraryPanel).toBeVisible();
    return;
  }

  const mobileToggle = page.locator(".mobile-workspace-toggle");
  if ((await mobileToggle.count()) === 0) {
    throw new Error("library panel is hidden and no mobile workspace toggle is available");
  }

  await mobileToggle.getByRole("button", { name: "Library" }).click();
  await expect(libraryPanel).toBeVisible();
}

export async function ensureTocPanelVisible(page: Page) {
  const tocPanel = page.locator(".toc-panel");
  if (await tocPanel.isVisible()) {
    return;
  }

  try {
    await expect(tocPanel).toBeVisible({ timeout: 5_000 });
  } catch {
    throw new Error("toc panel did not become visible; it may be unavailable in the current single-pane mode");
  }
}

async function setReaderStateSaveDisabled(page: Page, disabled: boolean) {
  await page.addInitScript(({ nextDisabled }) => {
    const currentWindow = window as Window & Record<string, unknown>;
    const currentStore =
      typeof currentWindow.__NAROU_VIEWER_E2E__ === "object" && currentWindow.__NAROU_VIEWER_E2E__ !== null
        ? (currentWindow.__NAROU_VIEWER_E2E__ as Record<string, unknown>)
        : {};

    currentWindow.__NAROU_VIEWER_E2E__ = {
      ...currentStore,
      disableReaderStateSave: nextDisabled
    };
  }, { nextDisabled: disabled });

  try {
    await page.evaluate((nextDisabled) => {
      const currentWindow = window as Window & Record<string, unknown>;
      const currentStore =
        typeof currentWindow.__NAROU_VIEWER_E2E__ === "object" && currentWindow.__NAROU_VIEWER_E2E__ !== null
          ? (currentWindow.__NAROU_VIEWER_E2E__ as Record<string, unknown>)
          : {};

      currentWindow.__NAROU_VIEWER_E2E__ = {
        ...currentStore,
        disableReaderStateSave: nextDisabled
      };
    }, disabled);
  } catch {
    // Ignore pages that are already closed or not yet ready to evaluate.
  }
}

export async function enableReaderStateSave(page: Page) {
  await setReaderStateSaveDisabled(page, false);
}

export async function disableReaderStateSave(page: Page) {
  await setReaderStateSaveDisabled(page, true);
}

async function readPaginationStatus(pager: Locator) {
  const text = (await pager.locator(".list-pagination-controls span").innerText()).trim();
  const match = text.match(/(\d+)\s*\/\s*(\d+)/);

  if (!match) {
    throw new Error(`想定外のページャ表示です: ${text}`);
  }

  return {
    current: Number.parseInt(match[1], 10),
    total: Number.parseInt(match[2], 10)
  };
}

async function goToTocPage(page: Page, targetPage: number) {
  const pager = page.getByLabel("話一覧ページ切り替え");
  if ((await pager.count()) === 0) {
    return;
  }

  for (let attempt = 0; attempt < 32; attempt += 1) {
    const { current } = await readPaginationStatus(pager);
    if (current === targetPage) {
      return;
    }

    const buttonName = targetPage > current ? "次へ" : "前へ";
    const button = pager.getByRole("button", { name: buttonName });
    await expect(button).toBeEnabled();
    await button.click();
  }

  throw new Error(`話一覧を ${targetPage} ページ目へ移動できませんでした`);
}

export async function getLibraryCardInfo(card: Locator) {
  const text = await card.innerText();
  const title = (await card.locator("strong").innerText()).trim();
  const siteName = (await card.locator(".library-site").innerText()).trim();
  const totalEpisodes = Number.parseInt(text.match(/話数:\s*(\d+)/)?.[1] ?? "", 10);
  const bookmarkCount = Number.parseInt(text.match(/栞:\s*(\d+)/)?.[1] ?? "", 10);

  if (!Number.isFinite(totalEpisodes)) {
    throw new Error(`話数を解析できませんでした: ${text}`);
  }

  if (!Number.isFinite(bookmarkCount)) {
    throw new Error(`栞数を解析できませんでした: ${text}`);
  }

  return {
    text,
    title,
    siteName,
    totalEpisodes,
    bookmarkCount
  };
}

export async function findNovelIdByTitle(request: APIRequestContext, title: string) {
  const library = await loadLibrary(request);
  const novel = library.novels.find((entry) => entry.title === title);

  if (!novel) {
    throw new Error(`${title} の novelId を library API から特定できませんでした`);
  }

  return novel.novelId;
}

export async function clickLibraryCardPrimaryAction(card: Locator) {
  const primaryButton = card.getByRole("button").filter({ hasNotText: "あらすじ" }).first();
  if ((await primaryButton.count()) > 0) {
    await primaryButton.click();
    return;
  }

  await card.click();
}

async function getSelectedNovelId(page: Page) {
  await expect
    .poll(() => new URL(page.url()).searchParams.get("novelId"), {
      message: "selected novelId should be reflected in the URL"
    })
    .not.toBeNull();

  const novelId = new URL(page.url()).searchParams.get("novelId");
  if (novelId === null) {
    throw new Error("selected novelId should be reflected in the URL");
  }
  return novelId;
}

async function waitForNovelSurface(page: Page) {
  await expect
    .poll(
      async () => {
        if (await page.locator(".reader-shell").isVisible().catch(() => false)) {
          return "reader";
        }

        if (await page.locator(".toc-panel .panel-header h2").isVisible().catch(() => false)) {
          return "toc";
        }

        return "pending";
      },
      {
        message: "novel selection should open either the toc panel or the reader"
      }
    )
    .not.toBe("pending");
}

async function loadToc(request: APIRequestContext, novelId: string) {
  const response = await request.get(`/api/library/novels/${encodeURIComponent(novelId)}/toc`);
  expect(response.ok(), "failed to load toc").toBeTruthy();
  return (await response.json()) as TocResponse;
}

export async function openNovelByTitle(page: Page, request: APIRequestContext, title: string) {
  await ensureLibraryPanelVisible(page);
  const card = page.locator(".library-card").filter({ hasText: title }).first();
  await expect(card).toBeVisible();

  const info = await getLibraryCardInfo(card);
  await clickLibraryCardPrimaryAction(card);

  const novelId = await getSelectedNovelId(page);
  const toc = await loadToc(request, novelId);
  await waitForNovelSurface(page);

  if (await page.locator(".reader-shell").isVisible()) {
    await expect(page.getByRole("button", { name: "一覧へ戻る" })).toBeVisible();
    return {
      ...info,
      novelId,
      toc
    };
  }

  await expect(page.locator(".toc-panel .panel-header h2")).toHaveText(info.title);
  await goToTocPage(page, 1);
  await expect.poll(async () => page.locator(".toc-item").count()).toBe(Math.min(toc.episodes.length, tocPageSize));

  return {
    ...info,
    novelId,
    toc
  };
}

export async function openNovelDetailsByTitle(page: Page, request: APIRequestContext, title: string) {
  const result = await openNovelByTitle(page, request, title);

  if (await page.locator(".reader-shell").isVisible()) {
    await page.getByRole("button", { name: "一覧へ戻る" }).click();
  }

  await ensureTocPanelVisible(page);
  await expect(page.locator(".toc-panel .panel-header h2")).toHaveText(result.title);
  return result;
}

export async function openEpisodeByIndex(page: Page, episodeIndex: EpisodeIndex) {
  if (await page.locator(".reader-shell").isVisible()) {
    await clickReaderActionButton(page, "目次");
    const readerTocPanel = page.getByLabel("本文画面の目次");
    await expect(readerTocPanel).toBeVisible();

    for (let attempt = 0; attempt < 32; attempt += 1) {
      const tocItem = readerTocPanel
        .locator(`[data-reader-panel-item="toc-episode"][data-episode-index="${episodeIndex}"]`)
        .first();
      if ((await tocItem.count()) > 0) {
        await expect(tocItem).toBeVisible();
        await tocItem.click();
        await expect(page.locator(".reader-toc-panel")).toHaveCount(0);
        await expect(page.getByRole("button", { name: "一覧へ戻る" })).toBeVisible();
        await expect(page.locator(".reader-title")).toBeVisible();
        await expect(page.locator(".reader-page-indicator")).toContainText("/");
        await expect
          .poll(async () => page.locator(".reader-prose-paged p").count(), {
            message: "reader paragraphs should be attached after the episode renders"
          })
          .toBeGreaterThan(0);
        await expect
          .poll(() => new URL(page.url()).searchParams.get("episode"), {
            message: "selected episode should be reflected in the URL"
          })
          .toBe(episodeIndex);
        return;
      }

      const pager = readerTocPanel.getByLabel("本文画面の話一覧ページ切り替え");
      if ((await pager.count()) === 0) {
        break;
      }

      const nextButton = pager.getByRole("button", { name: "次へ" });
      if (await nextButton.isDisabled()) {
        break;
      }

      await nextButton.click();
    }

    throw new Error(`episode ${episodeIndex} was not found in the reader toc`);
  }

  await ensureTocPanelVisible(page);
  await goToTocPage(page, 1);

  for (let attempt = 0; attempt < 32; attempt += 1) {
    const tocItem = page.locator(`.toc-item[data-episode-index="${episodeIndex}"]`).first();
    if ((await tocItem.count()) > 0) {
      await expect(tocItem).toBeVisible();
      await tocItem.click();

      await expect(page.getByRole("button", { name: "一覧へ戻る" })).toBeVisible();
      await expect(page.locator(".reader-title")).toBeVisible();
      await expect(page.locator(".reader-page-indicator")).toContainText("/");
      await expect
        .poll(async () => page.locator(".reader-prose-paged p").count(), {
          message: "reader paragraphs should be attached after the episode renders"
        })
        .toBeGreaterThan(0);
      await expect
        .poll(() => new URL(page.url()).searchParams.get("episode"), {
          message: "selected episode should be reflected in the URL"
        })
        .toBe(episodeIndex);
      return;
    }

    const pager = page.getByLabel("話一覧ページ切り替え");
    if ((await pager.count()) === 0) {
      break;
    }

    const nextButton = pager.getByRole("button", { name: "次へ" });
    if (await nextButton.isDisabled()) {
      break;
    }

    await nextButton.click();
  }

  throw new Error(`episode ${episodeIndex} was not found in the paginated toc`);
}

export async function readPageIndicator(page: Page) {
  const text = await page.locator(".reader-page-indicator").innerText();
  const match = text.match(/(\d+)\s*\/\s*(\d+)/);

  if (!match) {
    throw new Error(`想定外のページ表示です: ${text}`);
  }

  return {
    current: Number.parseInt(match[1], 10),
    total: Number.parseInt(match[2], 10)
  };
}

async function isReaderInFullscreen(page: Page) {
  return page.evaluate(() => {
    const readerShell = document.querySelector(".reader-shell");
    return Boolean(readerShell?.classList.contains("reader-shell-fullscreen"));
  });
}

async function getReaderViewportBox(page: Page) {
  const viewport = page.locator(".reader-page-viewport");
  await expect(viewport).toBeVisible();

  const box = await viewport.boundingBox();
  if (!box) {
    throw new Error("reader viewport is not visible");
  }

  return { viewport, box };
}

export async function activateReaderViewportEdge(page: Page, edge: "left" | "right", useTouch: boolean) {
  const { viewport, box } = await getReaderViewportBox(page);
  const edgeZoneWidth = Math.min(box.width * 0.16, 120);
  const edgeOffset = Math.max(12, Math.floor(edgeZoneWidth / 2));
  const relativeX = edge === "left" ? edgeOffset : Math.max(edgeOffset, Math.floor(box.width - edgeOffset));
  const relativeY = Math.max(1, Math.floor(box.height / 2));

  if (useTouch) {
    await viewport.tap({
      position: {
        x: relativeX,
        y: relativeY
      }
    });
    return;
  }

  await viewport.click({
    position: {
      x: relativeX,
      y: relativeY
    }
  });
}

export async function selectReaderViewportTextThenEndAtEdge(page: Page, edge: "left" | "right") {
  const { viewport, box } = await getReaderViewportBox(page);
  const edgeZoneWidth = Math.min(box.width * 0.16, 120);
  const edgeOffset = Math.max(12, Math.floor(edgeZoneWidth / 2));
  const relativeX = edge === "left" ? edgeOffset : Math.max(edgeOffset, Math.floor(box.width - edgeOffset));
  const relativeY = Math.max(1, Math.floor(box.height / 2));
  const startRelativeX = Math.max(edgeOffset, Math.floor(box.width / 2));
  const startRelativeY = relativeY;

  await viewport.evaluate(
    (element, input) => {
      const target = element as HTMLElement;
      const startTouch = {
        identifier: 1,
        target,
        clientX: input.startClientX,
        clientY: input.startClientY
      };
      const endTouch = {
        identifier: 1,
        target,
        clientX: input.endClientX,
        clientY: input.endClientY
      };

      const dispatchTouchEvent = (
        type: "touchstart" | "touchmove" | "touchend",
        touches: Array<typeof startTouch>,
        changedTouches: Array<typeof startTouch>
      ) => {
        const event = new Event(type, { bubbles: true, cancelable: true });
        Object.defineProperties(event, {
          touches: { value: touches },
          targetTouches: { value: touches },
          changedTouches: { value: changedTouches }
        });
        target.dispatchEvent(event);
      };

      dispatchTouchEvent("touchstart", [startTouch], [startTouch]);
      dispatchTouchEvent("touchmove", [endTouch], [endTouch]);
      dispatchTouchEvent("touchend", [], [endTouch]);

      const textWalker = document.createTreeWalker(target, NodeFilter.SHOW_TEXT, {
        acceptNode(node) {
          return node.textContent?.trim() ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_REJECT;
        }
      });
      const textNode = textWalker.nextNode();
      const selection = window.getSelection();
      if (!textNode || !selection) {
        throw new Error("reader viewport text selection target was not found");
      }

      const range = document.createRange();
      range.setStart(textNode, 0);
      range.setEnd(textNode, Math.min(textNode.textContent?.length ?? 0, 4));
      selection.removeAllRanges();
      selection.addRange(range);
      document.dispatchEvent(new Event("selectionchange"));
    },
    {
      startClientX: box.x + startRelativeX,
      startClientY: box.y + startRelativeY,
      endClientX: box.x + relativeX,
      endClientY: box.y + relativeY
    }
  );
}

export async function clearReaderViewportTextSelection(page: Page) {
  await page.evaluate(() => {
    const selection = window.getSelection();
    selection?.removeAllRanges();
    document.dispatchEvent(new Event("selectionchange"));
  });
}

export async function goToNextReaderPage(page: Page, useTouch: boolean) {
  if (useTouch) {
    await activateReaderViewportEdge(page, "left", true);
    return;
  }

  await page.keyboard.press("ArrowLeft");
}

async function waitForReaderInteractive(page: Page) {
  await expect(page.locator(".reader-shell")).toBeVisible();
  await expect(page.locator(".reader-loading-overlay")).toHaveCount(0);
  await expect(page.locator(".reader-page-viewport")).toBeVisible();
  await expect(page.locator(".reader-page-indicator")).toContainText("/");
}

async function clickVisibleReaderButton(button: Locator, label: string) {
  await expect(button).toBeVisible();
  await expect(button).toBeEnabled();
  await button.evaluate((element, elementLabel) => {
    const rect = element.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) {
      throw new Error(`${elementLabel} did not have a clickable box`);
    }
    element.click();
  }, label);
}

export async function clickReaderActionButton(page: Page, label: string) {
  await waitForReaderInteractive(page);

  const visibleButton = page.getByRole("button", { name: label, exact: true });
  if (await visibleButton.count()) {
    await visibleButton.first().click();
    return;
  }

  const overflowButton = page.getByRole("button", { name: "その他の操作" });
  await clickVisibleReaderButton(overflowButton, "reader overflow button");
  const overflowAction = page.locator(".reader-overflow-panel").getByRole("menuitem", { name: label, exact: true });
  await expect(overflowAction).toBeVisible();
  await overflowAction.click({ force: true });
}

export async function clickReaderBackToLibrary(page: Page) {
  await waitForReaderInteractive(page);

  const backToListButton = page.getByRole("button", { name: "一覧へ戻る" });
  await clickVisibleReaderButton(backToListButton, "reader back button");
}

export async function expectReaderFullscreenState(page: Page, expected: boolean) {
  await expect.poll(async () => isReaderInFullscreen(page)).toBe(expected);
}

export async function inspectReaderImageBlankArea(page: Page) {
  return page.evaluate(() => {
    const article = document.querySelector<HTMLElement>(".reader-prose-paged");
    const viewport = document.querySelector<HTMLDivElement>(".reader-page-viewport");
    if (!article || !viewport) {
      throw new Error("reader article is not visible");
    }

    const viewportRect = viewport.getBoundingClientRect();
    const paragraphs = Array.from(article.querySelectorAll<HTMLElement>("p"));

    for (const paragraph of paragraphs) {
      const image = paragraph.querySelector("img");
      if (!(image instanceof HTMLImageElement)) {
        continue;
      }

      const anchorCandidate = image.closest("a");
      const anchor = anchorCandidate instanceof HTMLAnchorElement ? anchorCandidate : null;
      const paragraphRect = paragraph.getBoundingClientRect();
      const imageRect = image.getBoundingClientRect();
      const clickableRect = (anchor ?? paragraph).getBoundingClientRect();
      const blankHeight = clickableRect.bottom - imageRect.bottom;
      const clickX = imageRect.left + Math.min(imageRect.width / 2, 40);
      const clickY = imageRect.bottom + Math.min(blankHeight / 2, 24);
      const edgeZoneWidth = Math.min(viewportRect.width * 0.16, 120);
      const isInsideEdgeZone =
        clickX - viewportRect.left <= edgeZoneWidth || viewportRect.right - clickX <= edgeZoneWidth;

      return {
        anchorExists: Boolean(anchor),
        paragraphHeight: paragraphRect.height,
        imageHeight: imageRect.height,
        blankHeight,
        hasClickableBlankArea: blankHeight > 12 && !isInsideEdgeZone,
        clickX,
        clickY
      };
    }

    throw new Error("reader image paragraph was not found");
  });
}

export async function inspectImageViewerStage(page: Page) {
  return page.evaluate(() => {
    const stage = document.querySelector<HTMLDivElement>(".reader-image-viewer-stage");
    if (!stage) {
      throw new Error("image viewer stage is not visible");
    }

    const rect = stage.getBoundingClientRect();
    return {
      centerX: rect.left + rect.width / 2,
      centerY: rect.top + rect.height / 2,
      clientWidth: stage.clientWidth,
      clientHeight: stage.clientHeight,
      scrollWidth: stage.scrollWidth,
      scrollHeight: stage.scrollHeight,
      scrollLeft: stage.scrollLeft,
      scrollTop: stage.scrollTop
    };
  });
}

export async function getLongNarouEpisodeIndex(request: APIRequestContext, novelId: string) {
  const toc = await loadToc(request, novelId);

  if (!toc.episodes.some((episode) => episode.episodeIndex === longNarouEpisodeIndex)) {
    throw new Error(`長文 fixture 話 ${longNarouEpisodeIndex} が toc に見つかりませんでした。`);
  }

  return longNarouEpisodeIndex;
}

export async function findFirstReadableEpisodeIndex(request: APIRequestContext, novelId: string) {
  const toc = await loadToc(request, novelId);
  const encodedNovelId = encodeURIComponent(novelId);

  for (const tocEpisode of toc.episodes) {
    const response = await request.get(
      `/api/library/novels/${encodedNovelId}/episodes/${encodeURIComponent(tocEpisode.episodeIndex)}`
    );
    if (response.ok()) {
      return tocEpisode.episodeIndex;
    }
  }

  throw new Error("本文ページの検証に使える話が見つかりませんでした。");
}

export async function listBookmarks(request: APIRequestContext, novelId: string) {
  const response = await request.get(`/api/bookmarks?novelId=${encodeURIComponent(novelId)}`);
  expect(response.ok(), "failed to list bookmarks").toBeTruthy();
  return ((await response.json()) as BookmarksResponse).bookmarks;
}

export async function clearBookmarks(request: APIRequestContext, novelId: string) {
  const bookmarks = await listBookmarks(request, novelId);

  for (const bookmark of bookmarks) {
    const response = await request.delete(`/api/bookmarks/${bookmark.id}`);
    expect(response.ok(), `failed to delete bookmark ${bookmark.id}`).toBeTruthy();
  }
}

export async function putReadingState(request: APIRequestContext, novelId: string, episodeIndex: EpisodeIndex | null) {
  const currentState = await getReadingState(request, novelId);
  const response = await request.put("/api/reader/state", {
    data: {
      novelId,
      lastReadEpisodeIndex: episodeIndex,
      position: 0,
      scroll: null,
      expectedStateVersion: currentState.stateVersion
    }
  });

  expect(response.ok(), "failed to update reading state").toBeTruthy();
}

export async function getReadingState(request: APIRequestContext, novelId: string) {
  const response = await request.get(`/api/reader/state?novelId=${encodeURIComponent(novelId)}`);
  expect(response.ok(), "failed to load reading state").toBeTruthy();
  return (await response.json()) as ReaderState;
}

export async function createBookmark(
  request: APIRequestContext,
  novelId: string,
  episodeIndex: EpisodeIndex,
  position: number,
  label?: string | null
) {
  const response = await request.post("/api/bookmarks", {
    data: {
      novelId,
      episodeIndex,
      position,
      label: label ?? null
    }
  });

  expect(response.ok(), "failed to create bookmark").toBeTruthy();
}
