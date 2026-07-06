import { describe, expect, it } from "vitest";
import {
  MUTATING_CONTRACT_TESTS_ENABLED,
  expectErrorShape,
  expectJsonResponse,
  expectNonEmptyFixtureArray,
  requestJson,
  requestRaw,
} from "../harness/apiClient";
import { findFixtureEpisode } from "../harness/fixtures";
import { expectNovelSummaryShape } from "../harness/shapes";

type ReaderStateContract = {
  novelId: string;
  lastReadEpisodeIndex: string | null;
  position: number;
  scroll: { type: "ratio"; value: number } | null;
  stateVersion: number;
  updatedAt: string | null;
  updatedByClientId: string | null;
};

describe("library and episode read contract", () => {
  it("returns a stable novel list shape", async () => {
    const response = await requestJson<{ novels: unknown[] }>(
      "/api/library/novels",
    );

    expectJsonResponse(response);
    expect(response.json).toEqual({
      novels: expect.any(Array),
    });
    expectNonEmptyFixtureArray(response.json.novels, "novels");

    if (response.json.novels.length > 0) {
      expectNovelSummaryShape(response.json.novels[0]);
    }
  });

  it("keeps library not-found and validation error shapes stable", async () => {
    const tocNotFound = await requestJson(
      "/api/library/novels/__api_contract_missing__/toc",
    );
    expectJsonResponse(tocNotFound, 404);
    expectErrorShape(tocNotFound.json);

    const invalidEpisodeIndex = await requestJson(
      "/api/library/novels/__api_contract_missing__/episodes/not-a-number",
    );
    expectJsonResponse(invalidEpisodeIndex, 400);
    expectErrorShape(invalidEpisodeIndex.json);

    const fixtureEpisode = await findFixtureEpisode("library error contract");
    if (!fixtureEpisode) {
      return;
    }

    const episodeNotFound = await requestJson(
      `/api/library/novels/${encodeURIComponent(fixtureEpisode.novelId)}/episodes/999999999`,
    );
    expectJsonResponse(episodeNotFound, 404);
    expectErrorShape(episodeNotFound.json);

    const assetNotFound = await requestJson(
      `/api/library/novels/${encodeURIComponent(fixtureEpisode.novelId)}/assets/__missing_asset__.png`,
    );
    expectJsonResponse(assetNotFound, 404);
    expectErrorShape(assetNotFound.json);
  });

  it("returns toc and episode shapes when the fixture has novels", async () => {
    const novelsResponse = await requestJson<{
      novels: Array<{ novelId: string; totalEpisodes: number }>;
    }>("/api/library/novels");
    expectJsonResponse(novelsResponse);

    const novelsWithEpisodes = novelsResponse.json.novels.filter(
      (candidate) => candidate.totalEpisodes > 0,
    );
    expectNonEmptyFixtureArray(novelsWithEpisodes, "novels with episodes");

    const fixtureEpisode = await findFixtureEpisode("library reader");
    if (!fixtureEpisode) {
      return;
    }

    const tocResponse = await requestJson<{
      episodes: Array<{ episodeIndex: string }>;
    }>(`/api/library/novels/${encodeURIComponent(fixtureEpisode.novelId)}/toc`);
    expectJsonResponse(tocResponse);
    expectNonEmptyFixtureArray(tocResponse.json.episodes, "toc episodes");
    expect(tocResponse.json).toEqual(
      expect.objectContaining({
        novelId: fixtureEpisode.novelId,
        title: expect.any(String),
        author: expect.any(String),
        episodes: expect.any(Array),
      }),
    );
    expectNovelSummaryShape(tocResponse.json);
    expect(tocResponse.json.episodes.length).toBeGreaterThan(0);
    expect(tocResponse.json.episodes[0]).toEqual(
      expect.objectContaining({
        episodeIndex: expect.any(String),
        title: expect.any(String),
      }),
    );

    const firstEpisodeIndex = fixtureEpisode.episodeIndex;
    const episodeResponse = await requestJson(
      `/api/library/novels/${encodeURIComponent(fixtureEpisode.novelId)}/episodes/${encodeURIComponent(firstEpisodeIndex)}`,
    );
    expectJsonResponse(episodeResponse);
    expect(episodeResponse.etag).toBeTruthy();
    expect(episodeResponse.json).toEqual(
      expect.objectContaining({
        novelId: fixtureEpisode.novelId,
        episodeIndex: firstEpisodeIndex,
        title: expect.any(String),
        html: expect.any(String),
        readerDocument: expect.any(Object),
        plainTextLength: expect.any(Number),
        updatedAt: expect.anything(),
        contentEtag: expect.any(String),
      }),
    );

    const cachedEpisodeResponse = await requestRaw(
      `/api/library/novels/${encodeURIComponent(fixtureEpisode.novelId)}/episodes/${encodeURIComponent(firstEpisodeIndex)}`,
      {
        headers: {
          "if-none-match": episodeResponse.etag ?? "",
        },
      },
    );
    expect(cachedEpisodeResponse.status).toBe(304);
    expect(cachedEpisodeResponse.bodyText).toBe("");
  });

  it.runIf(MUTATING_CONTRACT_TESTS_ENABLED)(
    "uses reader state as the same lastActivityAt in list and toc responses",
    async () => {
      const fixtureEpisode = await findFixtureEpisode(
        "library last activity contract",
      );
      if (!fixtureEpisode) {
        return;
      }

      const stateUrl = `/api/reader/state?novelId=${encodeURIComponent(fixtureEpisode.novelId)}`;
      const original = await requestJson<ReaderStateContract>(stateUrl);
      expectJsonResponse(original);

      try {
        const updated = await requestJson<ReaderStateContract>(
          "/api/reader/state",
          {
            method: "PUT",
            body: {
              novelId: fixtureEpisode.novelId,
              lastReadEpisodeIndex: fixtureEpisode.episodeIndex,
              position: original.json.position === 23 ? 24 : 23,
              scroll: null,
              clientId: "api-contract-last-activity",
              expectedStateVersion: original.json.stateVersion,
            },
          },
        );
        expectJsonResponse(updated);
        expect(updated.json.updatedAt).toEqual(expect.any(String));

        const novelsResponse = await requestJson<{
          novels: Array<{ novelId: string; lastActivityAt: string | null }>;
        }>("/api/library/novels");
        expectJsonResponse(novelsResponse);
        const novel = novelsResponse.json.novels.find(
          (entry) => entry.novelId === fixtureEpisode.novelId,
        );
        expect(novel).toBeTruthy();
        expectNovelSummaryShape(novel);
        expect(novel?.lastActivityAt).toBe(updated.json.updatedAt);

        const tocResponse = await requestJson<{
          novelId: string;
          lastActivityAt: string | null;
        }>(`/api/library/novels/${encodeURIComponent(fixtureEpisode.novelId)}/toc`);
        expectJsonResponse(tocResponse);
        expectNovelSummaryShape(tocResponse.json);
        expect(tocResponse.json.lastActivityAt).toBe(updated.json.updatedAt);
      } finally {
        const current = await requestJson<ReaderStateContract>(stateUrl);
        expectJsonResponse(current);
        const restored = await requestJson("/api/reader/state", {
          method: "PUT",
          body: {
            novelId: fixtureEpisode.novelId,
            lastReadEpisodeIndex: original.json.lastReadEpisodeIndex,
            position: original.json.position,
            scroll: original.json.scroll,
            clientId: original.json.updatedByClientId,
            expectedStateVersion: current.json.stateVersion,
          },
        });
        expectJsonResponse(restored);
      }
    },
  );
});
