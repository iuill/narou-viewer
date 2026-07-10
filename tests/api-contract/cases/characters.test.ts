import { describe, expect, it } from "vitest";
import {
  REQUIRE_CONTRACT_FIXTURE,
  expectErrorShape,
  expectJsonResponse,
  expectNonEmptyFixtureArray,
  requestJson,
} from "../harness/apiClient";
import { findFixtureEpisode, type FixtureEpisode } from "../harness/fixtures";

type CharacterSummaryResponse = {
  status: "ready" | "partial" | "not_generated";
  novelId: string;
  upToEpisodeIndex: string;
  processedUpToEpisodeIndex: string | null;
  characters: unknown[];
};

function expectCharacterSummaryResponseShape(
  value: unknown,
  fixtureEpisode: FixtureEpisode,
): asserts value is CharacterSummaryResponse {
  expect(value).toEqual(
    expect.objectContaining({
      status: expect.stringMatching(/^(ready|partial|not_generated)$/),
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

describe("character summaries contract", () => {
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
});
