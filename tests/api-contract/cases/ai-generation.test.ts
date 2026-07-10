import { describe, expect, it } from "vitest";
import {
  MUTATING_CONTRACT_TESTS_ENABLED,
  REQUIRE_CONTRACT_FIXTURE,
  expectErrorShape,
  expectJsonResponse,
  expectNonEmptyFixtureArray,
  parseNdjson,
  requestJson,
  requestRaw,
} from "../harness/apiClient";
import { findFixtureEpisode } from "../harness/fixtures";

function collectObjectKeys(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.flatMap((entry) => collectObjectKeys(entry));
  }
  if (value && typeof value === "object") {
    return Object.entries(value).flatMap(([key, child]) => [
      key,
      ...collectObjectKeys(child),
    ]);
  }
  return [];
}

function expectedEffectiveMode(
  preferredMode: "llm" | "heuristic",
  apiBaseUrlConfigured: boolean,
): "openrouter" | "heuristic" | "disabled" {
  if (preferredMode === "heuristic") {
    return "heuristic";
  }
  return apiBaseUrlConfigured ? "openrouter" : "disabled";
}

describe("AI generation settings contract", () => {
  it("returns settings metadata without exposing raw credentials", async () => {
    const response = await requestJson("/api/ai-generation/settings");

    expectJsonResponse(response);
    expect(response.json).toEqual(
      expect.objectContaining({
        apiBaseUrlConfigured: expect.any(Boolean),
        masterPassphraseConfigured: expect.any(Boolean),
        preferredMode: expect.stringMatching(/^(llm|heuristic)$/),
        effectiveGenerationMode: expect.stringMatching(
          /^(openrouter|heuristic|disabled)$/,
        ),
        settings: expect.objectContaining({
          sharedProviders: expect.objectContaining({
            openrouter: expect.objectContaining({
              hasApiKey: expect.any(Boolean),
            }),
            googleBooks: expect.objectContaining({
              hasApiKey: expect.any(Boolean),
            }),
          }),
          profiles: expect.any(Array),
        }),
      }),
    );

    const settings = (response.json as { settings: Record<string, unknown> })
      .settings;
    expect(settings).toHaveProperty("selectedProfileId");
    expectNonEmptyFixtureArray(
      (settings as { profiles: unknown[] }).profiles,
      "AI generation profiles",
    );
    expect(
      (settings.sharedProviders as { openrouter: Record<string, unknown> }).openrouter,
    ).toHaveProperty("apiKeyMasked");
    expect(
      (settings.sharedProviders as { googleBooks: Record<string, unknown> })
        .googleBooks,
    ).toHaveProperty("apiKeyMasked");
    expect(collectObjectKeys(response.json)).not.toContain("apiKey");
  });

  it("keeps AI generation settings validation errors stable", async () => {
    const invalidPreferredMode = await requestJson(
      "/api/ai-generation/settings/preferred-mode",
      {
        method: "PUT",
        body: {
          preferredMode: "invalid",
        },
      },
    );
    expectJsonResponse(invalidPreferredMode, 400);
    expectErrorShape(invalidPreferredMode.json);

    const invalidProfiles = await requestJson("/api/ai-generation/settings", {
      method: "PUT",
      body: {
        profiles: "invalid",
      },
    });
    expectJsonResponse(invalidProfiles, 400);
    expectErrorShape(invalidProfiles.json);

    const emptyProfiles = await requestJson("/api/ai-generation/settings", {
      method: "PUT",
      body: {
        profiles: [],
      },
    });
    expectJsonResponse(emptyProfiles, 400);
    expectErrorShape(emptyProfiles.json);
  });

  it("keeps AI generation playground validation and not-found errors stable", async () => {
    const missingNovelId = await requestJson(
      "/api/ai-generation/playground/extraction",
      {
        method: "POST",
        body: {
          upToEpisodeIndex: "1",
        },
      },
    );
    expectJsonResponse(missingNovelId, 400);
    expect(missingNovelId.json).toEqual(expect.objectContaining({
      error: "novelId is required.",
      code: "BAD_REQUEST",
      message: "novelId is required.",
    }));

    const missingUpToEpisodeIndex = await requestJson(
      "/api/ai-generation/playground/extraction",
      {
        method: "POST",
        body: {
          novelId: "__api_contract_missing__",
        },
      },
    );
    expectJsonResponse(missingUpToEpisodeIndex, 400);
    expect(missingUpToEpisodeIndex.json).toEqual(expect.objectContaining({
      error:
        "upToEpisodeIndex is required and must be a non-negative integer string.",
      code: "BAD_REQUEST",
      message:
        "upToEpisodeIndex is required and must be a non-negative integer string.",
    }));

    const missingNovel = await requestJson(
      "/api/ai-generation/playground/extraction",
      {
        method: "POST",
        body: {
          novelId: "__api_contract_missing__",
          upToEpisodeIndex: "1",
        },
      },
    );
    expectJsonResponse(missingNovel, 404);
    expect(missingNovel.json).toEqual(expect.objectContaining({
      error: "Novel not found.",
      code: "NOT_FOUND",
      message: "Novel not found.",
    }));

    const fixtureEpisode = await findFixtureEpisode("AI playground errors");
    if (!fixtureEpisode) {
      return;
    }

    const zeroEpisode = await requestJson(
      "/api/ai-generation/playground/extraction",
      {
        method: "POST",
        body: {
          novelId: fixtureEpisode.novelId,
          upToEpisodeIndex: 0,
        },
      },
    );
    expectJsonResponse(zeroEpisode, 400);
    expectErrorShape(zeroEpisode.json);
  });

  it("keeps AI generation playground stream preflight responses stable", async () => {
    const invalidStream = await requestJson(
      "/api/ai-generation/playground/extraction/stream",
      {
        method: "POST",
        body: {
          novelId: "__api_contract_missing__",
        },
      },
    );
    expectJsonResponse(invalidStream, 400);
    expect(invalidStream.json).toEqual(expect.objectContaining({
      error:
        "upToEpisodeIndex is required and must be a non-negative integer string.",
      code: "BAD_REQUEST",
      message:
        "upToEpisodeIndex is required and must be a non-negative integer string.",
    }));

    const fixtureEpisode = await findFixtureEpisode("AI playground stream");
    if (!fixtureEpisode) {
      return;
    }

    const outOfRangeStream = await requestJson(
      "/api/ai-generation/playground/extraction/stream",
      {
        method: "POST",
        body: {
          novelId: fixtureEpisode.novelId,
          upToEpisodeIndex: "999999999",
        },
      },
    );
    expectJsonResponse(outOfRangeStream, 400);
    expectErrorShape(outOfRangeStream.json);

    const zeroEpisodeStream = await requestJson(
      "/api/ai-generation/playground/extraction/stream",
      {
        method: "POST",
        body: {
          novelId: fixtureEpisode.novelId,
          upToEpisodeIndex: "0",
        },
      },
    );
    expectJsonResponse(zeroEpisodeStream, 400);
    expectErrorShape(zeroEpisodeStream.json);

    const streamResponse = await requestRaw(
      "/api/ai-generation/playground/extraction/stream",
      {
        method: "POST",
        body: {
          novelId: fixtureEpisode.novelId,
          upToEpisodeIndex: fixtureEpisode.episodeIndex,
        },
      },
    );
    expect(streamResponse.status).toBe(200);
    expect(streamResponse.contentType).toContain("application/x-ndjson");

    const events = parseNdjson(streamResponse.bodyText);
    expect(events.length).toBeGreaterThan(0);
    expect(events[0]).toEqual(
      expect.objectContaining({
        type: "status",
        stage: "preparing",
        step: 1,
        stepCount: 4,
      }),
    );
    expect(events).toContainEqual(
      expect.objectContaining({
        type: "status",
        stage: "loadingEpisodes",
        step: 2,
        stepCount: 4,
      }),
    );
    const lastEvent = events.at(-1);
    if (REQUIRE_CONTRACT_FIXTURE) {
      const promptPreviewIndex = events.findIndex(
        (event) => event.type === "promptPreview",
      );
      const generatingIndex = events.findIndex(
        (event) => event.type === "status" && event.stage === "generating",
      );
      const batchTimingIndex = events.findIndex(
        (event) => event.type === "batchTiming",
      );
      const buildingResponseIndex = events.findIndex(
        (event) =>
          event.type === "status" && event.stage === "buildingResponse",
      );
      const resultIndex = events.findIndex((event) => event.type === "result");
      expect([
        promptPreviewIndex,
        generatingIndex,
        batchTimingIndex,
        buildingResponseIndex,
        resultIndex,
      ]).not.toContain(-1);
      expect(promptPreviewIndex).toBeLessThan(generatingIndex);
      expect(generatingIndex).toBeLessThan(batchTimingIndex);
      expect(batchTimingIndex).toBeLessThan(buildingResponseIndex);
      expect(buildingResponseIndex).toBeLessThan(resultIndex);
      expect(events[promptPreviewIndex]).toEqual(
        expect.objectContaining({
          type: "promptPreview",
          preview: expect.objectContaining({
            systemPrompt: expect.any(String),
            batches: expect.any(Array),
          }),
        }),
      );
      expect(events[batchTimingIndex]).toEqual(
        expect.objectContaining({
          type: "batchTiming",
          batchIndex: expect.any(Number),
          batchCount: expect.any(Number),
          episodeIndexes: expect.any(Array),
          chunkCount: expect.any(Number),
        }),
      );
      expect(lastEvent).toEqual(
        expect.objectContaining({
          type: "result",
          result: expect.objectContaining({
            novelId: fixtureEpisode.novelId,
            upToEpisodeIndex: fixtureEpisode.episodeIndex,
            characters: expect.any(Array),
          }),
        }),
      );
      return;
    }

    expect(lastEvent).toEqual(
      expect.objectContaining({
        type: expect.stringMatching(/^(result|error)$/),
      }),
    );
  });

  it.runIf(MUTATING_CONTRACT_TESTS_ENABLED)(
    "persists preferred AI generation mode updates",
    async () => {
      const original = await requestJson<{
        apiBaseUrlConfigured: boolean;
        preferredMode: "llm" | "heuristic";
      }>("/api/ai-generation/settings");
      expectJsonResponse(original);

      const originalPreferredMode = original.json.preferredMode;
      const apiBaseUrlConfigured = original.json.apiBaseUrlConfigured;
      const nextPreferredMode =
        originalPreferredMode === "llm" ? "heuristic" : "llm";

      try {
        const updated = await requestJson(
          "/api/ai-generation/settings/preferred-mode",
          {
            method: "PUT",
            body: {
              preferredMode: nextPreferredMode,
            },
          },
        );
        expectJsonResponse(updated);
        expect(updated.json).toEqual(
          expect.objectContaining({
            preferredMode: nextPreferredMode,
            effectiveGenerationMode: expectedEffectiveMode(
              nextPreferredMode,
              apiBaseUrlConfigured,
            ),
          }),
        );
      } finally {
        const restored = await requestJson(
          "/api/ai-generation/settings/preferred-mode",
          {
            method: "PUT",
            body: {
              preferredMode: originalPreferredMode,
            },
          },
        );
        expectJsonResponse(restored);
        expect(restored.json).toEqual(
          expect.objectContaining({
            preferredMode: originalPreferredMode,
            effectiveGenerationMode: expectedEffectiveMode(
              originalPreferredMode,
              apiBaseUrlConfigured,
            ),
          }),
        );
      }
    },
  );

  it("returns AI generation job list shape", async () => {
    const response = await requestJson<{ jobs: unknown[] }>(
      "/api/ai-generation/jobs",
    );

    expectJsonResponse(response);
    expect(response.json).toEqual({
      jobs: expect.any(Array),
    });
    expectNonEmptyFixtureArray(response.json.jobs, "AI generation jobs");

    if (response.json.jobs.length > 0) {
      expect(response.json.jobs[0]).toEqual(
        expect.objectContaining({
          jobId: expect.any(String),
          novelId: expect.any(String),
          status: expect.any(String),
          createdAt: expect.any(String),
        }),
      );
    }
  });
});
