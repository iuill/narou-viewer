import { expect, test, type APIRequestContext, type Locator, type Page } from "@playwright/test";
import { clickLibraryCardPrimaryAction, disableReaderStateSave } from "./library-smoke.helpers";

type RuntimeStatusResponse = {
  status: "ok" | "warn" | "error";
  services: Array<{
    id: string;
    label: string;
    status: "ok" | "warn" | "error";
  }>;
};

type FetcherStatusResponse = {
  checkedAt: string;
  version: {
    current: string | null;
    latest: string | null;
  };
  queue: {
    total: number;
    queued: number;
    webWorker: number;
    worker: number;
    paused: number;
    interrupted: number;
    running: boolean;
  };
  tasks: {
    queued: unknown[];
    paused: unknown[];
    interrupted: unknown[];
    recentCompleted: unknown[];
    recentFailed: unknown[];
    completedCount: number;
    failedCount: number;
    canceledCount: number;
    pausedCount: number;
    interruptedCount: number;
    convertQueued: unknown[];
  };
};

type FetcherTasksSummaryResponse = {
  current: unknown | null;
  queued: unknown[];
  paused: unknown[];
  interrupted: unknown[];
  recentCompleted: unknown[];
  recentFailed: unknown[];
  completedCount: number;
  failedCount: number;
  canceledCount: number;
  pausedCount: number;
  interruptedCount: number;
  convertCurrent: unknown | null;
  convertQueued: unknown[];
};

type NovelSummary = {
  novelId: string;
  title: string;
  siteName: string;
  story?: string | null;
  totalEpisodes: number;
};

type LibraryResponse = {
  novels: NovelSummary[];
};

type EpisodeIndex = string;

type TocResponse = {
  title: string;
  episodes: Array<{
    episodeIndex: EpisodeIndex;
    title: string;
    contentEtag: string;
  }>;
};

type EpisodeResponse = {
  episodeIndex: EpisodeIndex;
  title: string;
  html: string;
  plainTextLength: number;
  contentEtag: string;
};

type NovelReaderSettingsResponse = {
  correction: {
    quoteNormalization: boolean;
    hyphenDashNormalization: boolean;
    parenthesisNormalization: boolean;
    halfwidthAlnumPunctuationNormalization: boolean;
  };
};

const appTitle = "Web小説ビューア";
const libraryPageSize = 12;
const tocPageSize = 50;

async function loadLibrary(request: APIRequestContext) {
  const response = await request.get("/api/library/novels");
  expect(response.ok(), "failed to load library").toBeTruthy();
  return (await response.json()) as LibraryResponse;
}

async function loadFirstNovelContext(request: APIRequestContext) {
  const library = await loadLibrary(request);
  expect(library.novels.length, "library should include at least one novel").toBeGreaterThan(0);

  for (const novel of library.novels) {
    const tocResponse = await request.get(`/api/library/novels/${encodeURIComponent(novel.novelId)}/toc`);
    expect(tocResponse.ok(), "failed to load toc").toBeTruthy();

    const toc = (await tocResponse.json()) as TocResponse;
    expect(toc.episodes.length, "toc should include at least one episode").toBeGreaterThan(0);

    for (const episode of toc.episodes) {
      const episodeResponse = await request.get(
        `/api/library/novels/${encodeURIComponent(novel.novelId)}/episodes/${encodeURIComponent(episode.episodeIndex)}`
      );
      if (episodeResponse.ok()) {
        return {
          library,
          novel,
          toc,
          firstEpisode: episode
        };
      }
    }
  }

  throw new Error("episode API で読める fixture が見つかりませんでした。");
}

async function gotoLibrary(page: Page, expectedCardCount: number): Promise<Locator | null> {
  await page.goto("/");

  await expect(page.getByRole("heading", { name: appTitle })).toBeVisible();
  const statusButton = page.getByRole("button", { name: /動作状況/ });
  if (await statusButton.isVisible()) {
    await expect(statusButton).toContainText(/正常|一部制限あり|要確認/);
  } else {
    await expect(page.getByRole("button", { name: "状況" })).toBeVisible();
  }
  await expect.poll(async () => page.locator(".library-card").count()).toBe(Math.min(expectedCardCount, libraryPageSize));
  return (await statusButton.isVisible()) ? statusButton : null;
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

test.describe("pc-xga 代表の runtime APIs", () => {
  test.skip(({ hasTouch }) => hasTouch === true, "API-only の確認は pc-xga で代表する");

  test("runtime APIs return healthy responses", async ({ request }) => {
    const healthResponse = await request.get("/api/health");
    expect(healthResponse.ok(), "health endpoint should respond successfully").toBeTruthy();
    const health = await healthResponse.json();
    expect(health).toMatchObject({
      status: "ok",
      service: "viewer-api"
    });

    const statusResponse = await request.get("/api/system/status");
    expect(statusResponse.ok(), "system status endpoint should respond successfully").toBeTruthy();
    const status = (await statusResponse.json()) as RuntimeStatusResponse;

    expect(["ok", "warn"]).toContain(status.status);
    expect(status.services.map((service) => service.id)).toEqual([
      "viewer-api",
      "novel-fetcher",
      "go-internal-ai",
      "google-books",
      "library"
    ]);
    expect(status.services.every((service) => ["ok", "warn"].includes(service.status))).toBeTruthy();
  });

  test("fetcher API exposes v2 status and task summary", async ({ request }) => {
    const response = await request.get("/api/fetcher/status");
    expect(response.ok(), "fetcher status endpoint should respond successfully").toBeTruthy();

    const status = (await response.json()) as FetcherStatusResponse;
    expect(status.checkedAt.length).toBeGreaterThan(0);
    expect(status.version.current).toMatch(/\d+\.\d+\.\d+/);
    expect(status.queue.total).toBeGreaterThanOrEqual(0);
    expect(status.queue.queued).toBeGreaterThanOrEqual(0);
    expect(status.queue.paused).toBeGreaterThanOrEqual(0);
    expect(status.queue.interrupted).toBeGreaterThanOrEqual(0);
    expect(typeof status.queue.running).toBe("boolean");
    expect(Array.isArray(status.tasks.queued)).toBeTruthy();
    expect(Array.isArray(status.tasks.paused)).toBeTruthy();
    expect(Array.isArray(status.tasks.interrupted)).toBeTruthy();
    expect(Array.isArray(status.tasks.recentCompleted)).toBeTruthy();
    expect(Array.isArray(status.tasks.recentFailed)).toBeTruthy();
    expect(Array.isArray(status.tasks.convertQueued)).toBeTruthy();
    expect(status.tasks.completedCount).toBeGreaterThanOrEqual(0);
    expect(status.tasks.failedCount).toBeGreaterThanOrEqual(0);
    expect(status.tasks.canceledCount).toBeGreaterThanOrEqual(0);
    expect(status.tasks.pausedCount).toBeGreaterThanOrEqual(0);
    expect(status.tasks.interruptedCount).toBeGreaterThanOrEqual(0);
  });

  test("fetcher task summary API exposes canonical camelCase fields", async ({ request }) => {
    const response = await request.get("/api/fetcher/tasks/summary");
    expect(response.ok(), "fetcher task summary endpoint should respond successfully").toBeTruthy();

    const summary = (await response.json()) as FetcherTasksSummaryResponse;
    expect(Array.isArray(summary.queued)).toBeTruthy();
    expect(Array.isArray(summary.paused)).toBeTruthy();
    expect(Array.isArray(summary.interrupted)).toBeTruthy();
    expect(Array.isArray(summary.recentCompleted)).toBeTruthy();
    expect(Array.isArray(summary.recentFailed)).toBeTruthy();
    expect(Array.isArray(summary.convertQueued)).toBeTruthy();
    expect(summary.completedCount).toBeGreaterThanOrEqual(0);
    expect(summary.failedCount).toBeGreaterThanOrEqual(0);
    expect(summary.canceledCount).toBeGreaterThanOrEqual(0);
    expect(summary.pausedCount).toBeGreaterThanOrEqual(0);
    expect(summary.interruptedCount).toBeGreaterThanOrEqual(0);
    expect(summary).not.toHaveProperty("recent_completed");
    expect(summary).not.toHaveProperty("completed_count");
  });

  test("the first episode API returns content and supports conditional GET", async ({ request }) => {
    const { novel, firstEpisode } = await loadFirstNovelContext(request);
    const episodeUrl = `/api/library/novels/${encodeURIComponent(novel.novelId)}/episodes/${encodeURIComponent(
      firstEpisode.episodeIndex
    )}`;
    const settingsResponse = await request.get(
      `/api/library/novels/${encodeURIComponent(novel.novelId)}/reader-settings`
    );
    expect(settingsResponse.ok(), "reader settings endpoint should respond successfully").toBeTruthy();
    const settings = (await settingsResponse.json()) as NovelReaderSettingsResponse;
    const correctionSuffix = [
      `q${settings.correction.quoteNormalization ? 1 : 0}`,
      `h${settings.correction.hyphenDashNormalization ? 1 : 0}`,
      `p${settings.correction.parenthesisNormalization ? 1 : 0}`,
      `a${settings.correction.halfwidthAlnumPunctuationNormalization ? 1 : 0}`
    ].join("");

    const firstResponse = await request.get(episodeUrl);
    expect(firstResponse.ok(), "episode endpoint should respond successfully").toBeTruthy();
    const etag = firstResponse.headers().etag;
    const episode = (await firstResponse.json()) as EpisodeResponse;

    expect(episode.episodeIndex).toBe(firstEpisode.episodeIndex);
    expect(episode.title.length).toBeGreaterThan(0);
    expect(episode.html.length).toBeGreaterThan(0);
    expect(episode.plainTextLength).toBeGreaterThan(0);
    expect(episode.contentEtag).toBe(`${firstEpisode.contentEtag}-reader-corrections-${correctionSuffix}`);
    expect(etag).toBe(`"${episode.contentEtag}"`);

    const notModifiedResponse = await request.get(episodeUrl, {
      headers: {
        "if-none-match": etag ?? ""
      }
    });
    expect(notModifiedResponse.status(), "episode endpoint should return 304 for matching ETag").toBe(304);
    expect(notModifiedResponse.headers().etag, "304 response should keep the matching ETag").toBe(etag);
  });
});

test("top page shows the library and opens the selected novel surface", async ({ page, request }, testInfo) => {
  const useTouch = testInfo.project.use.hasTouch === true;
  const { library, novel, toc } = await loadFirstNovelContext(request);
  await disableReaderStateSave(page);
  const statusButton = await gotoLibrary(page, library.novels.length);

  if (statusButton) {
    await statusButton.click();
    const statusPopover = page.getByLabel("サービスの動作状況");
    await expect(statusPopover).toBeVisible();
    await expect(statusPopover.getByText("viewer-api", { exact: true })).toBeVisible();
    await expect(statusPopover.getByText("novel-fetcher", { exact: true })).toBeVisible();
    await expect(statusPopover.getByText("Go internal AI", { exact: true })).toBeVisible();
    await expect(statusPopover.getByText("ローカルライブラリ", { exact: true })).toBeVisible();
    await statusButton.click();
    await expect(statusPopover).toBeHidden();
  } else {
    await page.getByRole("button", { name: "状況" }).click();
    const statusPanel = page.locator(".mobile-status-panel");
    await expect(statusPanel).toBeVisible();
    await expect(statusPanel.getByText("viewer-api", { exact: true })).toBeVisible();
    await expect(statusPanel.getByText("novel-fetcher", { exact: true })).toBeVisible();
    await expect(statusPanel.getByText("Go internal AI", { exact: true })).toBeVisible();
    await expect(statusPanel.getByText("ローカルライブラリ", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "ライブラリ" }).click();
    await expect(page.locator(".library-panel")).toBeVisible();
  }

  const targetCard = page.locator(".library-card").filter({ hasText: novel.title }).first();
  await expect(targetCard).toContainText(novel.title);
  await expect(targetCard).toContainText(novel.siteName);
  await expect(targetCard).toContainText(`話数: ${novel.totalEpisodes}`);
  expect(novel.totalEpisodes).toBe(toc.episodes.length);

  await clickLibraryCardPrimaryAction(targetCard);
  expect(await getSelectedNovelId(page)).toBe(novel.novelId);

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

  await expect(page.locator(".toc-panel .panel-header h2")).toHaveText(novel.title);
  await expect.poll(async () => page.locator(".toc-item").count()).toBe(Math.min(toc.episodes.length, tocPageSize));
});
