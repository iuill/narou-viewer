import { expect, test } from "@playwright/test";
import {
  buildEpisodeLabel,
  clearBookmarks,
  createBookmark,
  exportNarouTitle,
  findNovelIdByTitle,
  gotoLibrary,
  illustratedNarouTitle,
  kakuyomuTitle,
  libraryPageSize,
  loadLibrary,
  normalizeStoryText,
  novelStoryPreviewLength,
  openNovelDetailsByTitle,
  putReadingState,
  setupLibrarySmokeSuite
} from "./library-smoke.helpers";

setupLibrarySmokeSuite(test);

test.describe("pc-xga 代表の catalog checks", () => {
  test.skip(({ hasTouch }) => hasTouch === true, "catalog の非エンジン依存確認は pc-xga で代表する");

  test("ライブラリ一覧をタイトルや作者名で絞り込みできる", async ({ page, request }) => {
    const library = await loadLibrary(request);
    await gotoLibrary(page);

    const filterInput = page.getByRole("searchbox", { name: "ライブラリ絞り込み" });
    await filterInput.fill("カクヨム");
    await expect(page.locator(".library-card")).toHaveCount(1);
    await expect(page.locator(".library-card").first()).toContainText("カクヨム");
    await expect(page.locator(".library-panel .panel-header p")).toHaveText(`1 / ${library.novels.length} 作品`);

    await filterInput.fill("ケースA");
    await expect(page.locator(".library-card")).toHaveCount(1);
    await expect(page.locator(".library-card").first()).toContainText(illustratedNarouTitle);

    await page.getByRole("button", { name: "クリア" }).click();
    await expect(page.locator(".library-card")).toHaveCount(Math.min(library.novels.length, libraryPageSize));
    await expect(page.locator(".library-panel .panel-header p")).toHaveText(`${library.novels.length} 作品`);
  });

  test("ライブラリ一覧・既読・栞情報を YAML でエクスポートできる", async ({ page, request }) => {
    const library = await loadLibrary(request);
    await gotoLibrary(page);
    const novelId = await findNovelIdByTitle(request, exportNarouTitle);

    await clearBookmarks(request, novelId);
    await putReadingState(request, novelId, "1");

    try {
      await createBookmark(request, novelId, "1", 12, "E2E export bookmark");
      await gotoLibrary(page);

      const [download] = await Promise.all([
        page.waitForEvent("download"),
        page.getByRole("button", { name: "エクスポート", exact: true }).click()
      ]);
      const stream = await download.createReadStream();

      if (!stream) {
        throw new Error("export download stream was not available");
      }

      const chunks: Buffer[] = [];
      for await (const chunk of stream) {
        chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
      }

      const exportedYaml = Buffer.concat(chunks).toString("utf8");

      expect(download.suggestedFilename()).toMatch(/^narou-viewer-library-.+\.yaml$/u);
      expect(exportedYaml).toContain("formatVersion: 1");
      expect(exportedYaml).toContain(`novelsCount: ${library.novels.length}`);
      expect(exportedYaml).toContain(`novelId: ${novelId}`);
      expect(exportedYaml).toContain(`title: ${exportNarouTitle}`);
      expect(exportedYaml).toContain("lastReadEpisodeIndex: \"1\"");
      expect(exportedYaml).toContain("episodeIndex: \"1\"");
      expect(exportedYaml).toContain("label: E2E export bookmark");
      expect(exportedYaml).toContain("position: 12");
    } finally {
      await clearBookmarks(request, novelId);
      await putReadingState(request, novelId, null);
    }
  });
});

test("トップページ右ペインの長いあらすじを展開と折りたたみで切り替えられる", async ({ page, request }) => {
  test.skip(test.info().project.use.hasTouch === true, "モバイルではトップページに作品詳細を表示しない");
  await gotoLibrary(page);

  await openNovelDetailsByTitle(page, request, illustratedNarouTitle);

  const summary = page.locator(".novel-summary-story");
  const story = summary.locator("p");
  const collapsedStory = normalizeStoryText(await story.innerText());
  expect(Array.from(collapsedStory).length).toBeLessThanOrEqual(novelStoryPreviewLength + 1);
  expect(collapsedStory.endsWith("…")).toBeTruthy();

  const expandButton = summary.getByRole("button", { name: "すべて表示" });
  await expect(expandButton).toHaveAttribute("aria-expanded", "false");
  await expandButton.click();

  const expandedStory = normalizeStoryText(await summary.locator("p").innerText());
  expect(Array.from(expandedStory).length).toBeGreaterThan(Array.from(collapsedStory).length);
  const collapseButton = summary.getByRole("button", { name: "折りたたみ" });
  await expect(collapseButton).toHaveAttribute("aria-expanded", "true");
  await collapseButton.click();

  await expect(summary.locator("p")).toHaveText(collapsedStory);
  await expect(summary.getByRole("button", { name: "すべて表示" })).toHaveAttribute("aria-expanded", "false");
});

test("カクヨム作品の話一覧は toc.yaml の順序を保持する", async ({ page, request }) => {
  test.skip(test.info().project.use.hasTouch === true, "モバイルではトップページに作品詳細を表示しない");
  await gotoLibrary(page);
  const { toc } = await openNovelDetailsByTitle(page, request, kakuyomuTitle);

  const expectedEpisodeLabels = toc.episodes.map((episode) => buildEpisodeLabel(episode)).slice(0, 8);
  const actualEpisodeLabels = (await page.locator(".toc-item strong").allTextContents())
    .map((text) => text.trim())
    .slice(0, expectedEpisodeLabels.length);
  const actualDisplayedIndexes = (await page.locator(".toc-item .toc-index").allTextContents())
    .map((text) => text.trim())
    .slice(0, expectedEpisodeLabels.length);

  expect(actualEpisodeLabels).toEqual(expectedEpisodeLabels);
  expect(actualDisplayedIndexes).toEqual(expectedEpisodeLabels.map((_, index) => `#${index + 1}`));
});

test("カクヨム作品では長い episode ID をタイトルと通し番号で表示する", async ({ page, request }) => {
  test.skip(test.info().project.use.hasTouch === true, "モバイルではトップページに作品詳細を表示しない");
  await gotoLibrary(page);

  const { novelId, toc } = await openNovelDetailsByTitle(page, request, kakuyomuTitle);
  const firstEpisode = toc.episodes[0];

  if (!firstEpisode) {
    throw new Error("カクヨム fixture に話がありません");
  }

  await clearBookmarks(request, novelId);

  try {
    await putReadingState(request, novelId, firstEpisode.episodeIndex);
    await createBookmark(request, novelId, firstEpisode.episodeIndex, 12, "先頭付近");

    await page.goto("/");
    await gotoLibrary(page);

    const novelCard = page.locator(".library-card").filter({ hasText: kakuyomuTitle }).first();
    await expect(novelCard).toContainText(`既読: ${firstEpisode.title}`);
    await expect(novelCard).not.toContainText(firstEpisode.episodeIndex);

    await openNovelDetailsByTitle(page, request, kakuyomuTitle);
    await expect(page.locator(".novel-summary")).toContainText("最終既読");
    await expect(page.locator(`.novel-summary [data-episode-index="${firstEpisode.episodeIndex}"]`).first()).toHaveText(
      firstEpisode.title
    );
    const latestBookmarkRow = page.locator(".novel-summary dl div").filter({ hasText: "最新栞" }).first();
    await expect(latestBookmarkRow.getByRole("button")).toHaveText(firstEpisode.title);
    await page.getByRole("tab", { name: /栞/ }).click();
    const bookmarkItem = page.locator(`.bookmark-item[data-episode-index="${firstEpisode.episodeIndex}"]`).first();
    await expect(bookmarkItem).toContainText(`${firstEpisode.title} - 先頭付近`);
    await bookmarkItem.getByRole("button", { name: "開く" }).click();
    await expect(page.getByRole("button", { name: "一覧へ戻る" })).toBeVisible();
    await expect.poll(() => new URL(page.url()).searchParams.get("episode")).toBe(firstEpisode.episodeIndex);
    await expect(page.locator(".reader-page-viewport")).toBeVisible();
    await page.getByRole("button", { name: "一覧へ戻る" }).click();

    await page.getByRole("tab", { name: /話/ }).click();
    await expect(page.locator(`.toc-item[data-episode-index="${firstEpisode.episodeIndex}"] .toc-index`).first()).toHaveText(
      "#1"
    );
  } finally {
    await clearBookmarks(request, novelId);
    await putReadingState(request, novelId, null);
  }
});
