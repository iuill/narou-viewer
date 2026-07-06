import { describe, expect, it } from "vitest";
import {
  expectErrorShape,
  expectJsonResponse,
  parseNdjson,
  requestJson,
  requestRaw,
  REQUIRE_CONTRACT_FIXTURE,
} from "../harness/apiClient";
import { findFixtureEpisode } from "../harness/fixtures";

describe("reader assistant contract", () => {
  it("keeps chat validation and not-found errors stable", async () => {
    const missingMessage = await requestJson(
      "/api/library/novels/__api_contract_missing__/reader-assistant/chat",
      {
        method: "POST",
        body: {
          currentEpisodeIndex: "1",
          position: 0,
        },
      },
    );
    expectJsonResponse(missingMessage, 400);
    expect(missingMessage.json).toEqual(expect.objectContaining({
      error: "message is required.",
      code: "BAD_REQUEST",
      message: "message is required.",
    }));

    const invalidEpisodeIndex = await requestJson(
      "/api/library/novels/__api_contract_missing__/reader-assistant/chat",
      {
        method: "POST",
        body: {
          message: "契約テスト",
          currentEpisodeIndex: "not-a-number",
          position: 0,
        },
      },
    );
    expectJsonResponse(invalidEpisodeIndex, 400);
    expect(invalidEpisodeIndex.json).toEqual(expect.objectContaining({
      error: "currentEpisodeIndex must be a non-negative integer string.",
      code: "BAD_REQUEST",
      message: "currentEpisodeIndex must be a non-negative integer string.",
    }));

    const invalidPosition = await requestJson(
      "/api/library/novels/__api_contract_missing__/reader-assistant/chat",
      {
        method: "POST",
        body: {
          message: "契約テスト",
          currentEpisodeIndex: "1",
          position: -1,
        },
      },
    );
    expectJsonResponse(invalidPosition, 400);
    expect(invalidPosition.json).toEqual(expect.objectContaining({
      error: "position must be a non-negative integer or null.",
      code: "BAD_REQUEST",
      message: "position must be a non-negative integer or null.",
    }));

    const missingNovel = await requestJson(
      "/api/library/novels/__api_contract_missing__/reader-assistant/chat",
      {
        method: "POST",
        body: {
          message: "契約テスト",
          currentEpisodeIndex: "1",
          position: 0,
        },
      },
    );
    expectJsonResponse(missingNovel, 404);
    expect(missingNovel.json).toEqual(expect.objectContaining({
      error: "Novel not found.",
      code: "NOT_FOUND",
      message: "Novel not found.",
    }));
  });

  it("keeps chat stream preflight and disabled-LLM NDJSON response shape stable", async () => {
    const missingMessage = await requestJson(
      "/api/library/novels/__api_contract_missing__/reader-assistant/chat/stream",
      {
        method: "POST",
        body: {
          currentEpisodeIndex: "1",
          position: 0,
        },
      },
    );
    expectJsonResponse(missingMessage, 400);
    expect(missingMessage.json).toEqual(expect.objectContaining({
      error: "message is required.",
      code: "BAD_REQUEST",
      message: "message is required.",
    }));

    const missingNovel = await requestJson(
      "/api/library/novels/__api_contract_missing__/reader-assistant/chat/stream",
      {
        method: "POST",
        body: {
          message: "契約テスト",
          currentEpisodeIndex: "1",
          position: 0,
        },
      },
    );
    expectJsonResponse(missingNovel, 404);
    expect(missingNovel.json).toEqual(expect.objectContaining({
      error: "Novel not found.",
      code: "NOT_FOUND",
      message: "Novel not found.",
    }));

    const fixtureEpisode = await findFixtureEpisode("reader assistant stream");
    if (!fixtureEpisode) {
      return;
    }

    const streamResponse = await requestRaw(
      `/api/library/novels/${encodeURIComponent(fixtureEpisode.novelId)}/reader-assistant/chat/stream`,
      {
        method: "POST",
        body: {
          message: "契約テスト",
          currentEpisodeIndex: fixtureEpisode.episodeIndex,
          position: 0,
        },
      },
    );
    expect(streamResponse.status).toBe(200);
    expect(streamResponse.contentType).toContain("application/x-ndjson");

    const events = parseNdjson(streamResponse.bodyText);
    expect(events.length).toBeGreaterThan(0);
    expect(events[0]).toEqual(
      expect.objectContaining({
        type: expect.stringMatching(/^(status|error)$/),
      }),
    );
    for (const event of events) {
      expect(event).toEqual(
        expect.objectContaining({
          type: expect.any(String),
        }),
      );
    }
    const lastEvent = events.at(-1);
    if ((lastEvent as { type?: unknown }).type === "result") {
      expect(events[0]).toEqual(
        expect.objectContaining({
          type: "status",
        }),
      );
      expect(events[1]).toEqual(
        expect.objectContaining({
          type: "status",
        }),
      );
      const firstToolCallIndex = events.findIndex(
        (event) => (event as { type?: unknown }).type === "tool_call",
      );
      const firstToolResultIndex = events.findIndex(
        (event) => (event as { type?: unknown }).type === "tool_result",
      );
      expect(firstToolCallIndex).toBeGreaterThanOrEqual(2);
      expect(firstToolResultIndex).toBeGreaterThan(firstToolCallIndex);
      expect(events.at(-1)).toEqual(
        expect.objectContaining({
          type: "result",
        }),
      );
    }
    if (REQUIRE_CONTRACT_FIXTURE) {
      if ((lastEvent as { type?: unknown }).type === "error") {
        expect(lastEvent).toEqual({
          type: "error",
          error:
            "読書AIはLLM連携が未設定のため利用できません。AI機能の設定でOpenRouter APIキーとモデルを設定してください。",
        });
        return;
      }
      expect(lastEvent).toEqual(
        expect.objectContaining({
          type: "result",
          response: expect.objectContaining({
            novelId: fixtureEpisode.novelId,
            maxEpisodeIndex: fixtureEpisode.episodeIndex,
            generationMode: expect.stringMatching(/^(local|remote)$/),
            toolRequests: expect.any(Array),
            toolResults: expect.any(Array),
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

  it("keeps fixture episode boundary validation stable when a novel fixture is available", async () => {
    const fixtureEpisode = await findFixtureEpisode("reader assistant boundary");
    if (!fixtureEpisode) {
      return;
    }

    const response = await requestJson(
      `/api/library/novels/${encodeURIComponent(fixtureEpisode.novelId)}/reader-assistant/chat`,
      {
        method: "POST",
        body: {
          message: "契約テスト",
          currentEpisodeIndex: "999999999",
          position: 0,
        },
      },
    );
    expectJsonResponse(response, 400);
    expectErrorShape(response.json);
  });
});
