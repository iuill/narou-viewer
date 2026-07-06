import { describe, expect, it } from "vitest";
import {
  MUTATING_CONTRACT_TESTS_ENABLED,
  REQUIRE_CONTRACT_FIXTURE,
  expectErrorShape,
  expectIsoStringOrNull,
  expectJsonResponse,
  expectNonEmptyFixtureArray,
  requestJson,
} from "../harness/apiClient";
import { findFixtureEpisode, type FixtureEpisode } from "../harness/fixtures";

type CharacterSummaryResponse = {
  status: "ready" | "not_generated";
  novelId: string;
  upToEpisodeIndex: string;
  processedUpToEpisodeIndex: string | null;
  characters: unknown[];
};

type CharacterJobResponse = {
  jobId: string;
  requestedUpToEpisodeIndex: string;
  status: string;
  createdAt: string;
  startedAt: string | null;
  finishedAt: string | null;
  errorMessage: string | null;
};

function expectCharacterSummaryResponseShape(
  value: unknown,
  fixtureEpisode: FixtureEpisode,
): asserts value is CharacterSummaryResponse {
  expect(value).toEqual(
    expect.objectContaining({
      status: expect.stringMatching(/^(ready|not_generated)$/),
      novelId: fixtureEpisode.novelId,
      upToEpisodeIndex: fixtureEpisode.episodeIndex,
      characters: expect.any(Array),
    }),
  );
  const record = value as Record<string, unknown>;
  expect(record).toHaveProperty("processedUpToEpisodeIndex");
  if (record.processedUpToEpisodeIndex !== null) {
    expect(record.processedUpToEpisodeIndex).toEqual(expect.any(String));
  }
}

function expectCharacterShape(value: unknown): void {
  expect(value).toEqual(
    expect.objectContaining({
      characterId: expect.any(String),
      canonicalName: expect.any(String),
      firstAppearanceEpisodeIndex: expect.any(String),
      aliases: expect.any(Array),
    }),
  );
  const record = value as Record<string, unknown>;
  expect(record).toHaveProperty("fullName");
  expect(record).toHaveProperty("gender");
  expect(record).toHaveProperty("appearance");
  expect(record).toHaveProperty("personality");
  expect(record).toHaveProperty("summary");
  expect(record).toHaveProperty("importance");
}

function expectCharacterJobShape(
  value: unknown,
): asserts value is CharacterJobResponse {
  expect(value).toEqual(
    expect.objectContaining({
      jobId: expect.any(String),
      requestedUpToEpisodeIndex: expect.any(String),
      status: expect.any(String),
      createdAt: expect.any(String),
    }),
  );
  const record = value as Record<string, unknown>;
  expect(record).toHaveProperty("profileId");
  expect(record).toHaveProperty("profileLabel");
  expect(record).toHaveProperty("generationMode");
  expect(record).toHaveProperty("modelId");
  expect(record).toHaveProperty("startedAt");
  expect(record).toHaveProperty("finishedAt");
  expect(record).toHaveProperty("errorMessage");
  expectIsoStringOrNull(record.createdAt);
  expectIsoStringOrNull(record.startedAt);
  expectIsoStringOrNull(record.finishedAt);
}

describe("character summaries and jobs contract", () => {
  it("returns character summaries for a fixture episode", async () => {
    const fixtureEpisode = await findFixtureEpisode("character summaries");
    if (!fixtureEpisode) {
      return;
    }

    const response = await requestJson<CharacterSummaryResponse>(
      `/api/library/novels/${encodeURIComponent(fixtureEpisode.novelId)}/characters?upToEpisodeIndex=${encodeURIComponent(
        fixtureEpisode.episodeIndex,
      )}`,
    );
    expectJsonResponse(response);
    expectCharacterSummaryResponseShape(response.json, fixtureEpisode);

    if (REQUIRE_CONTRACT_FIXTURE) {
      expect(response.json.status).toBe("ready");
      expectNonEmptyFixtureArray(
        response.json.characters,
        "character summaries",
      );
    }
    if (response.json.characters.length > 0) {
      expectCharacterShape(response.json.characters[0]);
    }
  });

  it("keeps character summary validation and not-found errors stable", async () => {
    const missingQuery = await requestJson(
      "/api/library/novels/__api_contract_missing__/characters",
    );
    expectJsonResponse(missingQuery, 400);
    expectErrorShape(missingQuery.json);

    const missingNovel = await requestJson(
      "/api/library/novels/__api_contract_missing__/characters?upToEpisodeIndex=1",
    );
    expectJsonResponse(missingNovel, 404);
    expectErrorShape(missingNovel.json);

    const fixtureEpisode = await findFixtureEpisode("character summary errors");
    if (!fixtureEpisode) {
      return;
    }

    const outOfRangeEpisode = await requestJson(
      `/api/library/novels/${encodeURIComponent(fixtureEpisode.novelId)}/characters?upToEpisodeIndex=999999999`,
    );
    expectJsonResponse(outOfRangeEpisode, 400);
    expectErrorShape(outOfRangeEpisode.json);

    const zeroEpisode = await requestJson(
      `/api/library/novels/${encodeURIComponent(fixtureEpisode.novelId)}/characters?upToEpisodeIndex=0`,
    );
    expectJsonResponse(zeroEpisode, 400);
    expectErrorShape(zeroEpisode.json);
  });

  it("returns character job list shape for a fixture novel", async () => {
    const fixtureEpisode = await findFixtureEpisode("character jobs");
    if (!fixtureEpisode) {
      return;
    }

    const response = await requestJson<{ jobs: unknown[] }>(
      `/api/library/novels/${encodeURIComponent(fixtureEpisode.novelId)}/character-jobs`,
    );
    expectJsonResponse(response);
    expect(response.json).toEqual({
      jobs: expect.any(Array),
    });

    if (response.json.jobs.length > 0) {
      expectCharacterJobShape(response.json.jobs[0]);
    }
  });

  it("keeps character job validation and not-found errors stable", async () => {
    const missingNovel = await requestJson(
      "/api/library/novels/__api_contract_missing__/character-jobs",
    );
    expectJsonResponse(missingNovel, 404);
    expectErrorShape(missingNovel.json);

    const missingUpToEpisodeIndex = await requestJson(
      "/api/library/novels/__api_contract_missing__/character-jobs",
      {
        method: "POST",
        body: {},
      },
    );
    expectJsonResponse(missingUpToEpisodeIndex, 400);
    expectErrorShape(missingUpToEpisodeIndex.json);

    const postMissingNovel = await requestJson(
      "/api/library/novels/__api_contract_missing__/character-jobs",
      {
        method: "POST",
        body: {
          upToEpisodeIndex: "1",
        },
      },
    );
    expectJsonResponse(postMissingNovel, 404);
    expectErrorShape(postMissingNovel.json);

    const fixtureEpisode = await findFixtureEpisode("character job errors");
    if (!fixtureEpisode) {
      return;
    }

    const outOfRangeJob = await requestJson(
      `/api/library/novels/${encodeURIComponent(fixtureEpisode.novelId)}/character-jobs`,
      {
        method: "POST",
        body: {
          upToEpisodeIndex: "999999999",
        },
      },
    );
    expectJsonResponse(outOfRangeJob, 400);
    expectErrorShape(outOfRangeJob.json);

    const zeroJob = await requestJson(
      `/api/library/novels/${encodeURIComponent(fixtureEpisode.novelId)}/character-jobs`,
      {
        method: "POST",
        body: {
          upToEpisodeIndex: 0,
        },
      },
    );
    expectJsonResponse(zeroJob, 400);
    expectErrorShape(zeroJob.json);
  });

  it.runIf(MUTATING_CONTRACT_TESTS_ENABLED)(
    "enqueues a character job for a fixture episode",
    async () => {
      const fixtureEpisode = await findFixtureEpisode("character job mutation");
      if (!fixtureEpisode) {
        return;
      }

      const created = await requestJson<{ jobId: string }>(
        `/api/library/novels/${encodeURIComponent(fixtureEpisode.novelId)}/character-jobs`,
        {
          method: "POST",
          body: {
            upToEpisodeIndex: fixtureEpisode.episodeIndex,
          },
        },
      );
      expect([200, 202]).toContain(created.status);
      expect(created.contentType).toContain("application/json");
      expect(created.json).toEqual(
        expect.objectContaining({
          jobId: expect.any(String),
          requestedUpToEpisodeIndex: fixtureEpisode.episodeIndex,
          status: "queued",
          message: expect.any(String),
        }),
      );

      const jobs = await requestJson<{ jobs: unknown[] }>(
        `/api/library/novels/${encodeURIComponent(fixtureEpisode.novelId)}/character-jobs`,
      );
      expectJsonResponse(jobs);
      const createdJob = jobs.json.jobs.find(
        (job) => (job as { jobId?: unknown }).jobId === created.json.jobId,
      );
      expect(createdJob).toBeDefined();
      expectCharacterJobShape(createdJob);
      const createdJobRecord = createdJob as CharacterJobResponse;
      if (createdJobRecord.status === "queued") {
        expect(createdJobRecord.startedAt).toBeNull();
        expect(createdJobRecord.finishedAt).toBeNull();
      }
    },
  );
});
