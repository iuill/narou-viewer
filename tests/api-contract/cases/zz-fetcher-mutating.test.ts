import { describe, expect, it } from "vitest";
import {
  API_BASE_URL,
  DESTRUCTIVE_FETCHER_CONTRACT_TESTS_ENABLED,
  expectJsonResponse,
  requestJson,
} from "../harness/apiClient";
import { findFixtureEpisodeForNovelID } from "../harness/fixtures";

type CommandResponse = {
  message: string;
  novelIds?: string[];
  fetcherWorkIds?: string[];
  taskIds?: string[];
};

type RemoveResponse = CommandResponse & {
  withFiles: boolean;
  viewerStateCleanupStatus?: string;
  viewerStateCleanup?: {
    readingStatesDeleted: number;
    bookmarksDeleted: number;
  };
};

type CancelResponse = {
  message?: string;
  taskId?: string;
  status?: string;
  requestedAction?: string;
  changed?: boolean;
  cancelled?: boolean;
};

type TaskPayload = {
  taskId?: string;
  status?: string;
};

type TaskSummaryResponse = {
  current?: TaskPayload | null;
  queued?: TaskPayload[];
  paused?: TaskPayload[];
  interrupted?: TaskPayload[];
  recentCompleted?: TaskPayload[];
  recentFailed?: TaskPayload[];
};

async function waitForCanceledTask(taskId: string): Promise<void> {
  const deadline = Date.now() + 5_000;
  let lastStatus = "not found";

  while (Date.now() < deadline) {
    const summary = await requestJson<TaskSummaryResponse>("/api/fetcher/tasks/summary");
    expectJsonResponse(summary);
    const tasks = [
      ...(summary.json.current ? [summary.json.current] : []),
      ...(summary.json.queued ?? []),
      ...(summary.json.paused ?? []),
      ...(summary.json.interrupted ?? []),
      ...(summary.json.recentCompleted ?? []),
      ...(summary.json.recentFailed ?? []),
    ];
    const task = tasks.find((candidate) => candidate.taskId === taskId);
    lastStatus = task?.status ?? "not found";
    if (lastStatus === "canceled") {
      return;
    }
    await new Promise<void>((resolve) => setTimeout(resolve, 50));
  }

  throw new Error(`Task ${taskId} did not reach canceled status; last status: ${lastStatus}`);
}

function requireSafeDestructiveFetcherTarget(): string {
  const url = new URL(API_BASE_URL);
  const safeHost =
    url.hostname === "viewer-api-e2e" ||
    url.hostname === "localhost" ||
    url.hostname === "127.0.0.1" ||
    url.hostname === "::1" ||
    url.hostname === "[::1]";
  const safePort = url.hostname === "viewer-api-e2e" || url.port === "18080";
  expect(
    safeHost && safePort,
    `Refusing destructive fetcher contract against ${API_BASE_URL}. Use viewer-api-e2e or localhost:18080.`,
  ).toBe(true);

  const targetNovelID =
    process.env.API_CONTRACT_DESTRUCTIVE_FETCHER_TARGET_NOVEL_ID?.trim();
  if (!targetNovelID) {
    throw new Error(
      "API_CONTRACT_DESTRUCTIVE_FETCHER_TARGET_NOVEL_ID is required for destructive fetcher contract tests.",
    );
  }
  return targetNovelID;
}

describe("fetcher mutating flow contract", () => {
  it.runIf(DESTRUCTIVE_FETCHER_CONTRACT_TESTS_ENABLED)(
    "updates, cancels, and removes a fixture work through canonical fetcher routes",
    async () => {
      const targetNovelID = requireSafeDestructiveFetcherTarget();
      const fixtureEpisode = await findFixtureEpisodeForNovelID(
        "fetcher mutating flow",
        targetNovelID,
      );
      if (!fixtureEpisode) {
        return;
      }

      const originalReaderState = await requestJson<{
        stateVersion: number;
      }>(`/api/reader/state?novelId=${encodeURIComponent(fixtureEpisode.novelId)}`);
      expectJsonResponse(originalReaderState);

      const readerState = await requestJson("/api/reader/state", {
        method: "PUT",
        body: {
          novelId: fixtureEpisode.novelId,
          lastReadEpisodeIndex: fixtureEpisode.episodeIndex,
          position: 7,
          clientId: "api-contract-fetcher-mutating",
          expectedStateVersion: originalReaderState.json.stateVersion,
        },
      });
      expectJsonResponse(readerState);

      const bookmark = await requestJson<{ id: string }>("/api/bookmarks", {
        method: "POST",
        body: {
          novelId: fixtureEpisode.novelId,
          episodeIndex: fixtureEpisode.episodeIndex,
          position: 3,
          label: "fetcher-mutating-cleanup",
        },
      });
      expectJsonResponse(bookmark, 201);

      const update = await requestJson<CommandResponse>(
        "/api/fetcher/works/update",
        {
          method: "POST",
          body: {
            novelIds: [fixtureEpisode.novelId],
            skipUnchanged: true,
          },
        },
      );
      expectJsonResponse(update, 202);
      expect(update.json).toEqual(
        expect.objectContaining({
          message: expect.any(String),
          novelIds: [fixtureEpisode.novelId],
          fetcherWorkIds: expect.any(Array),
        }),
      );

      const taskId = update.json.taskIds?.[0];
      expect(taskId, "update response should include a taskId for cancel contract").toEqual(
        expect.any(String),
      );
      expect(taskId).not.toBe("");
      if (!taskId) {
        return;
      }

      const cancel = await requestJson<CancelResponse>(
        `/api/fetcher/tasks/${encodeURIComponent(taskId)}/cancel`,
        {
          method: "POST",
        },
      );
      expect([200, 404, 409]).toContain(cancel.status);
      expect(cancel.contentType).toContain("application/json");
      if (cancel.status === 200) {
        expect(cancel.json).toEqual(expect.objectContaining({ taskId, changed: true }));
        const cancellationRequested =
          cancel.json.status === "running" && cancel.json.requestedAction === "cancel";
        const cancellationCompleted =
          cancel.json.status === "canceled" && cancel.json.cancelled === true;
        expect(cancellationRequested || cancellationCompleted).toBe(true);
        await waitForCanceledTask(taskId);
      }

      const remove = await requestJson<RemoveResponse>(
        "/api/fetcher/works/remove",
        {
          method: "POST",
          body: {
            novelIds: [fixtureEpisode.novelId],
            withFiles: true,
          },
        },
      );
      expectJsonResponse(remove, 202);
      expect(remove.json).toEqual(
        expect.objectContaining({
          message: expect.any(String),
          novelIds: [fixtureEpisode.novelId],
          fetcherWorkIds: expect.any(Array),
          withFiles: true,
          viewerStateCleanupStatus: "ok",
        }),
      );
      expect(remove.json.viewerStateCleanup).toEqual(
        expect.objectContaining({
          readingStatesDeleted: expect.any(Number),
          bookmarksDeleted: expect.any(Number),
        }),
      );
      expect(remove.json.viewerStateCleanup?.readingStatesDeleted).toBeGreaterThanOrEqual(1);
      expect(remove.json.viewerStateCleanup?.bookmarksDeleted).toBeGreaterThanOrEqual(1);

      const library = await requestJson<{ novels: Array<{ novelId: string }> }>(
        "/api/library/novels",
      );
      expectJsonResponse(library);
      expect(library.json.novels.some((novel) => novel.novelId === fixtureEpisode.novelId)).toBe(false);

      const prunedReaderState = await requestJson<{
        lastReadEpisodeIndex: string | null;
        position: number;
      }>(`/api/reader/state?novelId=${encodeURIComponent(fixtureEpisode.novelId)}`);
      expectJsonResponse(prunedReaderState);
      expect(prunedReaderState.json.lastReadEpisodeIndex).toBeNull();
      expect(prunedReaderState.json.position).toBe(0);

      const prunedBookmarks = await requestJson<{ bookmarks: unknown[] }>(
        `/api/bookmarks?novelId=${encodeURIComponent(fixtureEpisode.novelId)}`,
      );
      expectJsonResponse(prunedBookmarks);
      expect(prunedBookmarks.json.bookmarks).toHaveLength(0);
    },
  );
});
