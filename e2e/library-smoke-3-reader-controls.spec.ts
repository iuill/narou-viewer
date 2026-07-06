import { expect, test } from "@playwright/test";
import {
  activateReaderViewportEdge,
  appTitle,
  clickReaderBackToLibrary,
  clearReaderViewportTextSelection,
  clickReaderActionButton,
  expectReaderFullscreenState,
  findNovelIdByTitle,
  getLongNarouEpisodeIndex,
  goToNextReaderPage,
  gotoLibrary,
  illustratedNarouTitle,
  inspectImageViewerStage,
  inspectReaderImageBlankArea,
  openEpisodeByIndex,
  openNovelByTitle,
  putReadingState,
  readPageIndicator,
  readerControlsNarouTitle,
  selectReaderViewportTextThenEndAtEdge,
  setupLibrarySmokeSuite
} from "./library-smoke.helpers";

setupLibrarySmokeSuite(test);

test("本文ページでページ移動と各アイコンの機能が動作する", async ({ page, request }, testInfo) => {
  testInfo.setTimeout(60_000);
  const useTouch = testInfo.project.use.hasTouch === true;

  await gotoLibrary(page);

  const novelId = await findNovelIdByTitle(request, readerControlsNarouTitle);
  await putReadingState(request, novelId, null);

  const { title } = await openNovelByTitle(page, request, readerControlsNarouTitle);
  const longestEpisodeIndex = await getLongNarouEpisodeIndex(request, novelId);

  await openEpisodeByIndex(page, longestEpisodeIndex);

  await clickReaderActionButton(page, "読書設定");
  const readerSettingsPanel = page.locator(".reader-settings-panel");
  await expect(readerSettingsPanel).toBeVisible();
  await expect(page.getByRole("slider", { name: /^文字サイズ:/ })).toHaveValue("20");
  await expect(page.getByRole("slider", { name: /^文字間隔:/ })).toHaveValue("0.08");
  const resetReaderSettingsButton = readerSettingsPanel.getByRole("button", { name: "読書設定を初期化" });
  await resetReaderSettingsButton.scrollIntoViewIfNeeded();
  await expect(resetReaderSettingsButton).toBeVisible();

  await page.getByRole("combobox", { name: "フォント" }).selectOption("gothic");
  await expect(page.getByRole("combobox", { name: "フォント" })).toHaveValue("gothic");
  await page.getByRole("combobox", { name: "テーマ" }).selectOption("forest");
  await expect(page.getByRole("combobox", { name: "テーマ" })).toHaveValue("forest");
  await resetReaderSettingsButton.click();
  await expect(page.locator(".reader-notice")).toContainText("読書設定を初期化しました");
  await expect(page.getByRole("combobox", { name: "フォント" })).toHaveValue("mincho");
  await expect(page.getByRole("combobox", { name: "テーマ" })).toHaveValue("classic");

  await clickReaderActionButton(page, "情報");
  await expect(page.locator(".reader-settings-panel")).toHaveCount(0);
  await expect(page.locator(".reader-info-panel")).toBeVisible();
  await expect(page.locator(".reader-info-panel")).toContainText(title);
  await expect(page.locator(".reader-info-panel")).toContainText("現在の話");
  await expect(page.locator(".reader-info-panel")).toContainText("閲覧ページ");

  await clickReaderActionButton(page, "目次");
  await expect(page.locator(".reader-info-panel")).toHaveCount(0);
  const readerTocPanel = page.getByLabel("本文画面の目次");
  const readerTocEpisodes = readerTocPanel.locator('[data-reader-panel-item="toc-episode"]');
  await expect(readerTocPanel).toBeVisible();
  await expect
    .poll(async () => readerTocEpisodes.count(), {
      message: "reader toc should list at least one episode"
    })
    .toBeGreaterThan(0);
  await expect(
    readerTocPanel.locator(`[data-reader-panel-item="toc-episode"][data-episode-index="${longestEpisodeIndex}"]`)
  ).toHaveCount(1);
  await readerTocPanel.getByRole("button", { name: "目次を閉じる" }).click();
  await expect(page.locator(".reader-toc-panel")).toHaveCount(0);

  await expect
    .poll(async () => (await readPageIndicator(page)).total, {
      message: "reader should have multiple pages for the longest episode"
    })
    .toBeGreaterThan(1);

  await goToNextReaderPage(page, useTouch);
  await expect.poll(async () => (await readPageIndicator(page)).current).toBe(2);

  if (useTouch) {
    await selectReaderViewportTextThenEndAtEdge(page, "right");
    await expect.poll(async () => (await readPageIndicator(page)).current).toBe(2);
    await clearReaderViewportTextSelection(page);
  }

  await activateReaderViewportEdge(page, "right", useTouch);
  await expect.poll(async () => (await readPageIndicator(page)).current).toBe(1);

  if (!useTouch) {
    await clickReaderActionButton(page, "フルスクリーン表示");
    await expectReaderFullscreenState(page, true);

    await clickReaderActionButton(page, "フルスクリーン解除");
    await expectReaderFullscreenState(page, false);
  }

  await clickReaderBackToLibrary(page);
  await expect(page.getByRole("heading", { name: appTitle })).toBeVisible();
  if (useTouch) {
    const selectedCard = page.locator(`.library-card.selected[data-novel-id="${novelId}"], .library-card.selected`).first();
    await expect(selectedCard).toBeVisible();
    await selectedCard.getByRole("button").first().click();
    await expect(page.getByRole("button", { name: "一覧へ戻る" })).toBeVisible();
    return;
  }
  await expect(page.locator(".toc-item").first()).toBeVisible();
});

test("PCでは画像下の空白をクリックしても何も起きない", async ({ page, request }, testInfo) => {
  test.skip(testInfo.project.use.hasTouch === true, "PC 向けの回帰テスト");

  await gotoLibrary(page);
  await openNovelByTitle(page, request, illustratedNarouTitle);
  await openEpisodeByIndex(page, "1");

  const initialUrl = page.url();
  const imageArea = await inspectReaderImageBlankArea(page);
  let popupCount = 0;
  const onPopup = () => {
    popupCount += 1;
  };

  page.on("popup", onPopup);
  try {
    if (imageArea.hasClickableBlankArea) {
      await page.mouse.click(imageArea.clickX, imageArea.clickY);
      await page.waitForTimeout(300);
      await expect(page.locator(".reader-image-viewer")).toHaveCount(0);
      await expect.poll(() => page.url()).toBe(initialUrl);
      expect(popupCount).toBe(0);
    } else {
      expect(imageArea.anchorExists).toBe(false);
    }
  } finally {
    page.off("popup", onPopup);
  }
});

test("PCでは拡大画像をドラッグして見切れた部分を表示できる", async ({ page, request }, testInfo) => {
  test.skip(testInfo.project.use.hasTouch === true, "PC 向けの回帰テスト");

  await gotoLibrary(page);
  await openNovelByTitle(page, request, illustratedNarouTitle);
  await openEpisodeByIndex(page, "1");

  const inlineImage = page.locator(".reader-prose-paged img").first();
  await expect(inlineImage).toBeVisible();
  await inlineImage.click();
  await expect(page.locator(".reader-image-viewer")).toBeVisible();

  await page.locator(".reader-image-viewer-zoom-control input").evaluate((input) => {
    const element = input as HTMLInputElement;
    element.value = "300";
    element.dispatchEvent(new Event("input", { bubbles: true }));
    element.dispatchEvent(new Event("change", { bubbles: true }));
  });

  const initialStage = await inspectImageViewerStage(page);
  expect(
    initialStage.scrollWidth > initialStage.clientWidth || initialStage.scrollHeight > initialStage.clientHeight
  ).toBeTruthy();

  const dragOffsetX = initialStage.scrollWidth > initialStage.clientWidth ? 160 : 0;
  const dragOffsetY = initialStage.scrollHeight > initialStage.clientHeight ? 160 : 0;

  await page.mouse.move(initialStage.centerX, initialStage.centerY);
  await page.mouse.down();
  await page.mouse.move(initialStage.centerX - dragOffsetX, initialStage.centerY - dragOffsetY, { steps: 12 });
  await page.mouse.up();

  await expect
    .poll(async () => {
      const stage = await inspectImageViewerStage(page);
      return {
        scrollLeft: stage.scrollLeft,
        scrollTop: stage.scrollTop
      };
    })
    .not.toEqual({
      scrollLeft: initialStage.scrollLeft,
      scrollTop: initialStage.scrollTop
    });
});
