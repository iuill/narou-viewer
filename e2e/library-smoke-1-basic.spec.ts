import { expect, test } from "@playwright/test";
import {
  appTitle,
  findFirstReadableEpisodeIndex,
  getLibraryCardInfo,
  gotoLibrary,
  illustratedNarouTitle,
  kakuyomuTitle,
  libraryPageSize,
  loadLibrary,
  openNovelDetailsByTitle,
  openNovelByTitle,
  setupLibrarySmokeSuite
} from "./library-smoke.helpers";

setupLibrarySmokeSuite(test);

test.describe("pc-xga 代表の fetcher validation", () => {
  test.skip(({ hasTouch }) => hasTouch === true, "request-only の配線確認は pc-xga で代表する");

  test("fetcher download API validates targets before proxying", async ({ request }) => {
    const response = await request.post("/api/fetcher/works/download", {
      data: {
        targets: []
      }
    });

    expect(response.status()).toBe(400);
    const payload = await response.json();
    expect(payload).toMatchObject({
      error: "targets must be a non-empty string array."
    });
  });
});

test("library screen opens the fetcher download composer from the add button", async ({ page }) => {
  await gotoLibrary(page);

  if (test.info().project.use.hasTouch === true) {
    await expect(page.locator(".toc-panel")).toHaveCount(0);
    await expect(page.getByText("作品詳細")).toHaveCount(0);
  }

  const libraryPanel = page.locator(".library-panel");
  await libraryPanel.getByRole("button", { name: "小説を追加" }).click();

  const composer = libraryPanel.locator(".library-download-composer");
  await expect(composer).toBeVisible();
  const submitButton = composer.getByRole("button", { name: "ダウンロード" });
  await expect(submitButton).toBeDisabled();

  await composer.getByRole("textbox").fill("n9669bk");
  await expect(submitButton).toBeEnabled();
});

test("novel detail shows update and remove actions", async ({ page, request }) => {
  test.skip(test.info().project.use.hasTouch === true, "モバイルではトップページに作品詳細を表示しない");
  await gotoLibrary(page);
  await openNovelDetailsByTitle(page, request, illustratedNarouTitle);

  const actions = page.locator(".novel-summary-actions");
  await expect(actions.getByRole("button", { name: "更新" })).toBeVisible();
  await expect(actions.getByRole("button", { name: "削除" })).toBeVisible();
});

test("タッチ端末ではライブラリカードのあらすじを開いても本文へ遷移しない", async ({ page, request }) => {
  test.skip(test.info().project.use.hasTouch !== true, "タッチ端末専用");
  await gotoLibrary(page);

  const library = await loadLibrary(request);
  const targetNovel = library.novels.find((novel) => (novel.story?.trim().length ?? 0) > 0);
  if (!targetNovel?.story) {
    throw new Error("あらすじ付きの作品が library API から取得できませんでした");
  }

  const card = page.locator(".library-card").filter({ hasText: targetNovel.title }).first();
  await expect(card).toBeVisible();
  await card.getByRole("button", { name: "あらすじ" }).click();

  await expect(card.locator(".library-card-summary")).toContainText(targetNovel.story.replace(/\s+/g, " ").trim().slice(0, 20));
  await expect(page.locator(".reader-shell")).toHaveCount(0);
  await expect.poll(() => new URL(page.url()).searchParams.get("episode")).toBeNull();
});

test.describe("pc-xga 代表の deep link", () => {
  test.skip(({ hasTouch }) => hasTouch === true, "DOM ロジック中心の確認は pc-xga で代表する");

  test("deep link で指定した小説を初回ロード後も維持する", async ({ page, request }) => {
    const library = await loadLibrary(request);
    expect(library.novels.length, "library should include at least two novels").toBeGreaterThan(1);

    const targetNovel = library.novels.at(1);
    if (!targetNovel) {
      throw new Error("library should include a second novel");
    }

    await page.goto(`/?novelId=${encodeURIComponent(targetNovel.novelId)}`);

    await expect(page.getByRole("heading", { name: appTitle })).toBeVisible();
    await expect
      .poll(() => new URL(page.url()).searchParams.get("novelId"), {
        message: "deep-linked novelId should remain selected after the initial library load"
      })
      .toBe(targetNovel.novelId);
    await expect(page.locator(".toc-panel .panel-header h2")).toHaveText(targetNovel.title);
  });
});

test("トップページに複数小説が表示され、PC は話一覧経由で、タッチ端末は本文へ遷移できる", async ({ page, request }, testInfo) => {
  const useTouch = testInfo.project.use.hasTouch === true;
  const library = await loadLibrary(request);
  await gotoLibrary(page, { checkChrome: true });

  const libraryCards = page.locator(".library-card");
  await expect(libraryCards).toHaveCount(Math.min(library.novels.length, libraryPageSize));

  const sites: string[] = [];
  for (let index = 0; index < Math.min(library.novels.length, libraryPageSize); index += 1) {
    const cardInfo = await getLibraryCardInfo(libraryCards.nth(index));
    sites.push(cardInfo.siteName);
    expect(cardInfo.text).toMatch(/話数:\s*\d+/);
    expect(cardInfo.text).toMatch(/既読:/);
    expect(cardInfo.text).toMatch(/栞:\s*\d+/);
    expect(cardInfo.text).toMatch(/更新:/);
  }
  expect(sites).toEqual(expect.arrayContaining(["カクヨム", "小説家になろう"]));

  if (!useTouch) {
    const kakuyomu = await openNovelByTitle(page, request, kakuyomuTitle);
    expect(kakuyomu.totalEpisodes).toBeGreaterThan(0);
  }

  const openedNovel = await openNovelByTitle(page, request, illustratedNarouTitle);

  if (useTouch) {
    await expect(page.getByRole("button", { name: "一覧へ戻る" })).toBeVisible();
    await expect(page.locator(".reader-page-indicator")).toContainText("/");
    await expect
      .poll(() => new URL(page.url()).searchParams.get("episode"), {
        message: "touch projects should navigate directly to the reader"
      })
      .not.toBeNull();
    return;
  }

  const firstEpisodeIndex = await findFirstReadableEpisodeIndex(request, openedNovel.novelId);
  const firstEpisode = page.locator(`.toc-item[data-episode-index="${firstEpisodeIndex}"]`).first();

  await firstEpisode.click();

  await expect(page.getByRole("button", { name: "一覧へ戻る" })).toBeVisible();
  await expect(page.locator(".reader-page-indicator")).toContainText("/");
  await expect.poll(() => new URL(page.url()).searchParams.get("episode")).toBe(firstEpisodeIndex);
});
