import { describe, expect, it } from "vitest";
import {
  MUTATING_CONTRACT_TESTS_ENABLED,
  REQUIRE_CONTRACT_FIXTURE,
  expectErrorShape,
  expectIsoStringOrNull,
  expectJsonResponse,
  requestJson,
} from "../harness/apiClient";
import { findFixtureEpisode } from "../harness/fixtures";

type ExtractionJobResponse = {
  jobId: string;
  requestedUpToEpisodeIndex: string;
  status: string;
  createdAt: string;
  startedAt: string | null;
  finishedAt: string | null;
};

function expectExtractionJobShape(
  value: unknown,
): asserts value is ExtractionJobResponse {
  expect(value).toEqual(
    expect.objectContaining({
      jobId: expect.any(String),
      requestedUpToEpisodeIndex: expect.any(String),
      status: expect.any(String),
      createdAt: expect.any(String),
    }),
  );
  const record = value as Record<string, unknown>;
  for (const field of [
    "profileId",
    "profileLabel",
    "generationMode",
    "generationStrategy",
    "modelId",
    "generatedCharacterCount",
    "generatedTermCount",
    "startedAt",
    "finishedAt",
    "errorMessage",
  ]) {
    expect(record).toHaveProperty(field);
  }
  expectIsoStringOrNull(record.createdAt);
  expectIsoStringOrNull(record.startedAt);
  expectIsoStringOrNull(record.finishedAt);
}

describe("extraction contract", () => {
  it("returns terms projected to a fixture episode", async () => {
    const fixture = await findFixtureEpisode("terms");
    if (!fixture) return;

    const response = await requestJson(
      `/api/library/novels/${encodeURIComponent(fixture.novelId)}/terms?upToEpisodeIndex=${encodeURIComponent(fixture.episodeIndex)}`,
    );
    expectJsonResponse(response);
    expect(response.json).toEqual(
      expect.objectContaining({
        status: expect.stringMatching(/^(ready|partial|not_generated)$/),
        novelId: fixture.novelId,
        upToEpisodeIndex: fixture.episodeIndex,
        terms: expect.any(Array),
      }),
    );
    expect(response.json).toHaveProperty("processedUpToEpisodeIndex");
    if (REQUIRE_CONTRACT_FIXTURE) {
      expect(response.json.status).toBe("ready");
    }
    for (const term of response.json.terms as unknown[]) {
      expect(term).toEqual(
        expect.objectContaining({
          term: expect.any(String),
          category: expect.stringMatching(
            /^(organization|place|item|skill|race|event|other)$/,
          ),
          description: expect.any(String),
        }),
      );
      const reading = (term as { reading?: unknown }).reading;
      expect(reading === null || typeof reading === "string").toBe(true);
    }
  });

  it("keeps terms validation and not-found errors stable", async () => {
    const missingQuery = await requestJson(
      "/api/library/novels/__api_contract_missing__/terms",
    );
    expectJsonResponse(missingQuery, 400);
    expectErrorShape(missingQuery.json);

    const missingNovel = await requestJson(
      "/api/library/novels/__api_contract_missing__/terms?upToEpisodeIndex=1",
    );
    expectJsonResponse(missingNovel, 404);
    expectErrorShape(missingNovel.json);

    const fixture = await findFixtureEpisode("terms errors");
    if (!fixture) return;
    const outOfRange = await requestJson(
      `/api/library/novels/${encodeURIComponent(fixture.novelId)}/terms?upToEpisodeIndex=999999999`,
    );
    expectJsonResponse(outOfRange, 400);
    expectErrorShape(outOfRange.json);
  });

  it("returns extraction jobs for a fixture novel", async () => {
    const fixture = await findFixtureEpisode("extraction jobs");
    if (!fixture) return;
    const response = await requestJson<{ jobs: unknown[] }>(
      `/api/library/novels/${encodeURIComponent(fixture.novelId)}/extraction-jobs`,
    );
    expectJsonResponse(response);
    expect(response.json).toEqual({ jobs: expect.any(Array) });
    if (response.json.jobs.length > 0) {
      expectExtractionJobShape(response.json.jobs[0]);
    }
  });

  it("keeps extraction job validation stable", async () => {
    const missingNovel = await requestJson(
      "/api/library/novels/__api_contract_missing__/extraction-jobs",
    );
    expectJsonResponse(missingNovel, 404);
    expectErrorShape(missingNovel.json);
    const invalid = await requestJson(
      "/api/library/novels/__api_contract_missing__/extraction-jobs",
      { method: "POST", body: {} },
    );
    expectJsonResponse(invalid, 400);
    expectErrorShape(invalid.json);
  });

  it.runIf(MUTATING_CONTRACT_TESTS_ENABLED)(
    "enqueues extraction for a fixture episode",
    async () => {
      const fixture = await findFixtureEpisode("extraction mutation");
      if (!fixture) return;
      const created = await requestJson<{ jobId: string }>(
        `/api/library/novels/${encodeURIComponent(fixture.novelId)}/extraction-jobs`,
        {
          method: "POST",
          body: { upToEpisodeIndex: fixture.episodeIndex },
        },
      );
      expect([200, 202]).toContain(created.status);
      expect(created.json).toEqual(
        expect.objectContaining({
          jobId: expect.any(String),
          requestedUpToEpisodeIndex: fixture.episodeIndex,
          status: "queued",
          message: expect.any(String),
        }),
      );
    },
  );
});
