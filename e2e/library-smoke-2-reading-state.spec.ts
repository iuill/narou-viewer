import { expect, test, type Page } from "@playwright/test";
import {
  disableReaderStateSave,
  enableReaderStateSave,
  ensureLibraryPanelVisible,
  findNovelIdByTitle,
  getLongNarouEpisodeIndex,
  getReadingState,
  goToNextReaderPage,
  gotoLibrary,
  openEpisodeByIndex,
  openNovelByTitle,
  putReadingState,
  readPageIndicator,
  readingStateAnchorNarouTitle,
  readingStateNarouTitle,
  setupLibrarySmokeSuite
} from "./library-smoke.helpers";

setupLibrarySmokeSuite(test);

async function expectNovelBefore(page: Page, earlierNovelId: string, laterNovelId: string) {
  const novelIds = await page.locator(".library-card").evaluateAll((cards) =>
    cards.map((card) => card.getAttribute("data-novel-id") ?? "")
  );
  const earlierIndex = novelIds.indexOf(earlierNovelId);
  const laterIndex = novelIds.indexOf(laterNovelId);

  expect(earlierIndex, `${earlierNovelId} should be visible in the library`).toBeGreaterThanOrEqual(0);
  expect(laterIndex, `${laterNovelId} should be visible in the library`).toBeGreaterThanOrEqual(0);
  expect(earlierIndex, `${earlierNovelId} should be listed before ${laterNovelId}`).toBeLessThan(laterIndex);
}

test("既読位置が保存・復元され、一覧へ戻ると対象作品が活動順の先頭へ移動する", async ({ page, request }, testInfo) => {
  const useTouch = testInfo.project.use.hasTouch === true;
  testInfo.setTimeout(60_000);
  await enableReaderStateSave(page);

  await gotoLibrary(page);

  const novelId = await findNovelIdByTitle(request, readingStateNarouTitle);
  const anchorNovelId = await findNovelIdByTitle(request, readingStateAnchorNarouTitle);
  const resumeEpisodeIndex = await getLongNarouEpisodeIndex(request, novelId);

  await putReadingState(request, novelId, null);
  await putReadingState(request, anchorNovelId, null);

  try {
    await gotoLibrary(page);
    await ensureLibraryPanelVisible(page);
    await expectNovelBefore(page, anchorNovelId, novelId);

    const openedNovel = await openNovelByTitle(page, request, readingStateNarouTitle);
    expect(openedNovel.novelId).toBe(novelId);

    await openEpisodeByIndex(page, resumeEpisodeIndex);
    await expect
      .poll(async () => (await readPageIndicator(page)).total, {
        message: "reading position restore test requires a multi-page episode"
      })
      .toBeGreaterThan(1);

    await goToNextReaderPage(page, useTouch);
    await expect.poll(async () => (await readPageIndicator(page)).current).toBe(2);

    const currentPage = await readPageIndicator(page);
    expect(currentPage.current).toBe(2);

    await expect
      .poll(
        async () => {
          const state = await getReadingState(request, novelId);
          return state.position;
        },
        {
          message: "reading state should persist the current position"
        }
      )
      .toBeGreaterThan(0);
    const savedState = await getReadingState(request, novelId);
    expect(savedState.lastReadEpisodeIndex).toBe(resumeEpisodeIndex);

    await page.getByRole("button", { name: "一覧へ戻る" }).click();
    await ensureLibraryPanelVisible(page);
    await expectNovelBefore(page, novelId, anchorNovelId);

    if (useTouch) {
      const selectedCard = page.locator(`.library-card.selected[data-novel-id="${novelId}"], .library-card.selected`).first();
      await expect(selectedCard).toBeVisible();
      await selectedCard.getByRole("button").first().click();
    } else {
      const lastReadRow = page.locator(".novel-summary dl div").filter({ hasText: "最終既読" }).first();
      await expect(lastReadRow.getByRole("button")).toBeVisible();
      await lastReadRow.getByRole("button").click();
    }

    await expect(page.getByRole("button", { name: "一覧へ戻る" })).toBeVisible();
    await expect
      .poll(() => new URL(page.url()).searchParams.get("pos"), {
        message: "restored reader URL should include the saved position"
      })
      .toBe(String(savedState.position));
    await expect(page.locator(".reader-page-viewport")).toBeVisible();
    if (useTouch) {
      await expect
        .poll(async () => (await readPageIndicator(page)).current, {
          message: "touch reader should reopen on a readable page after restoring the saved position"
        })
        .toBeGreaterThan(0);
      return;
    }
    await expect
      .poll(async () => (await readPageIndicator(page)).current, {
        message: "reader should reopen on the saved page"
      })
      .toBe(currentPage.current);
  } finally {
    await disableReaderStateSave(page);
    await page.goto("about:blank");
    await putReadingState(request, novelId, null);
    await putReadingState(request, anchorNovelId, null);
  }
});
