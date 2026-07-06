import { expect, test } from "@playwright/test";
import {
  appTitle,
  bookmarksNarouTitle,
  clearBookmarks,
  clickReaderActionButton,
  findNovelIdByTitle,
  getLongNarouEpisodeIndex,
  goToNextReaderPage,
  gotoLibrary,
  listBookmarks,
  openEpisodeByIndex,
  openNovelDetailsByTitle,
  openNovelByTitle,
  putReadingState,
  readPageIndicator,
  setupLibrarySmokeSuite
} from "./library-smoke.helpers";

setupLibrarySmokeSuite(test);

test("栞を作成すると一覧に反映され、栞位置へ移動できる", async ({ page, request }, testInfo) => {
  testInfo.setTimeout(60_000);
  const useTouch = testInfo.project.use.hasTouch === true;

  await gotoLibrary(page);

  const novelId = await findNovelIdByTitle(request, bookmarksNarouTitle);
  const bookmarkEpisodeIndex = await getLongNarouEpisodeIndex(request, novelId);

  await putReadingState(request, novelId, null);
  await clearBookmarks(request, novelId);
  await gotoLibrary(page);
  await openNovelByTitle(page, request, bookmarksNarouTitle);

  let cleanupError: unknown;
  try {
    await openEpisodeByIndex(page, bookmarkEpisodeIndex);
    await expect
      .poll(async () => (await readPageIndicator(page)).total, {
        message: "bookmark test requires a multi-page episode"
      })
      .toBeGreaterThan(1);

    await goToNextReaderPage(page, useTouch);
    await expect.poll(async () => (await readPageIndicator(page)).current).toBe(2);

    await clickReaderActionButton(page, useTouch ? "栞" : "栞を追加");
    if (useTouch) {
      await page.getByRole("button", { name: "この位置に栞を追加" }).click();
    }
    await expect(page.locator(".reader-notice")).toContainText(`#${bookmarkEpisodeIndex}`);

    await expect.poll(async () => (await listBookmarks(request, novelId)).length).toBe(1);
    const bookmarks = await listBookmarks(request, novelId);
    const savedBookmark = bookmarks[0];
    expect(savedBookmark?.episodeIndex).toBe(bookmarkEpisodeIndex);

    if (!savedBookmark) {
      throw new Error("saved bookmark was not returned");
    }

    await expect(page.locator(".reader-notice")).toContainText(`#${savedBookmark.episodeIndex}`);

    await page.getByRole("button", { name: "一覧へ戻る" }).click();
    await expect(page.getByRole("heading", { name: appTitle })).toBeVisible();

    const novelCard = page.locator(".library-card").filter({ hasText: bookmarksNarouTitle }).first();
    await expect(novelCard).toContainText("栞: 1");

    if (useTouch) {
      await novelCard.getByRole("button").first().click();
      await expect(page.getByRole("button", { name: "一覧へ戻る" })).toBeVisible();

      await clickReaderActionButton(page, "栞");
      await expect(page.locator(".bookmark-item").first()).toContainText(`#${bookmarkEpisodeIndex}`);
      await page.locator(".bookmark-item").first().getByRole("button", { name: "開く" }).click();
      await expect.poll(() => new URL(page.url()).searchParams.get("episode")).toBe(bookmarkEpisodeIndex);
      await expect.poll(() => new URL(page.url()).searchParams.get("pos")).toBe(String(savedBookmark.position));
      await expect(page.locator(".reader-page-viewport")).toBeVisible();

      await clickReaderActionButton(page, "栞");
      await expect(page.locator(".bookmark-item")).toHaveCount(1);
      await page.locator(".bookmark-item").first().getByRole("button", { name: "削除" }).click();
      await expect(page.locator(".bookmark-item")).toHaveCount(0);
    } else {
      await openNovelDetailsByTitle(page, request, bookmarksNarouTitle);
      const latestBookmarkRow = page.locator(".novel-summary dl div").filter({ hasText: "最新栞" }).first();
      const latestBookmarkButton = latestBookmarkRow.getByRole("button");
      await expect(latestBookmarkButton).toBeVisible();
      await page.getByRole("tab", { name: /栞/ }).click();
      await expect(page.locator(".bookmark-item").first()).toContainText(`#${bookmarkEpisodeIndex}`);

      await latestBookmarkButton.click();
      await expect(page.getByRole("button", { name: "一覧へ戻る" })).toBeVisible();
      await expect.poll(() => new URL(page.url()).searchParams.get("episode")).toBe(bookmarkEpisodeIndex);
      await expect.poll(() => new URL(page.url()).searchParams.get("pos")).toBe(String(savedBookmark.position));
      await expect(page.locator(".reader-page-viewport")).toBeVisible();

      await page.getByRole("button", { name: "一覧へ戻る" }).click();
      await page.getByRole("tab", { name: /栞/ }).click();
      await expect(page.locator(".bookmark-item")).toHaveCount(1);
      await page.locator(".bookmark-item").first().getByRole("button", { name: "削除" }).click();
      await expect(page.locator(".bookmark-item")).toHaveCount(0);
    }

    await gotoLibrary(page);
    await expect(page.locator(".library-card").filter({ hasText: bookmarksNarouTitle }).first()).toContainText("栞: 0");
  } finally {
    try {
      await putReadingState(request, novelId, null);
      await clearBookmarks(request, novelId);
    } catch (error) {
      cleanupError = error;
    }
  }
  if (
    cleanupError instanceof Error &&
    cleanupError.message.includes("Target page, context or browser has been closed")
  ) {
    return;
  }
  if (cleanupError !== undefined) {
    throw cleanupError;
  }
});
