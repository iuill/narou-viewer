import { describe, expect, it } from "vitest";
import {
  expectErrorShape,
  expectJsonResponse,
  expectNonEmptyFixtureArray,
  requestJson
} from "../harness/apiClient";

type UsageListResponse = {
  summary: {
    runCount: number;
    requestCount: number;
    inputTokens: number;
    outputTokens: number;
    totalTokens: number;
    cachedInputTokens: number;
    reasoningOutputTokens: number;
    totalCost: number;
    averageTotalTokens: number;
  };
  runs: Array<{
    runId: string;
    feature: string;
    workflowName: string;
    status: string;
    startedAt: string;
    finishedAt: string;
    elapsedMs: number;
    novelId: string | null;
    novelTitle: string | null;
    currentEpisodeIndex: string | null;
    modelId: string | null;
    profileId: string | null;
    profileLabel: string | null;
    generationMode: string;
    answerChars: number;
    requestCount: number;
    inputTokens: number;
    outputTokens: number;
    totalTokens: number;
    cachedInputTokens: number;
    reasoningOutputTokens: number;
    totalCost: number;
    toolCallCount: number;
    toolResultCount: number;
    hasSnapshot: boolean;
    errorMessage: string | null;
    requests: unknown[];
  }>;
};

function expectNullableString(value: unknown): void {
  expect(value === null || typeof value === "string").toBe(true);
}

function expectNullableNumber(value: unknown): void {
  expect(value === null || typeof value === "number").toBe(true);
}

function expectNullableObject(value: unknown): void {
  expect(value === null || (typeof value === "object" && !Array.isArray(value))).toBe(true);
}

describe("AI usage contract", () => {
  it("returns usage summary and run list shape", async () => {
    const response = await requestJson<UsageListResponse>("/api/ai-generation/usage");

    expectJsonResponse(response);
    expect(response.json).toEqual(
      expect.objectContaining({
        summary: expect.objectContaining({
          runCount: expect.any(Number),
          requestCount: expect.any(Number),
          inputTokens: expect.any(Number),
          outputTokens: expect.any(Number),
          totalTokens: expect.any(Number),
          cachedInputTokens: expect.any(Number),
          reasoningOutputTokens: expect.any(Number),
          totalCost: expect.any(Number),
          averageTotalTokens: expect.any(Number)
        }),
        runs: expect.any(Array)
      })
    );
    expectNonEmptyFixtureArray(response.json.runs, "AI usage runs");

    if (response.json.runs.length > 0) {
      expect(response.json.runs[0]).toEqual(
        expect.objectContaining({
          runId: expect.any(String),
          feature: expect.any(String),
          workflowName: expect.any(String),
          status: expect.any(String),
          startedAt: expect.any(String),
          finishedAt: expect.any(String),
          elapsedMs: expect.any(Number),
          generationMode: expect.any(String),
          answerChars: expect.any(Number),
          requestCount: expect.any(Number),
          inputTokens: expect.any(Number),
          outputTokens: expect.any(Number),
          totalTokens: expect.any(Number),
          cachedInputTokens: expect.any(Number),
          reasoningOutputTokens: expect.any(Number),
          totalCost: expect.any(Number),
          toolCallCount: expect.any(Number),
          toolResultCount: expect.any(Number),
          hasSnapshot: expect.any(Boolean),
          requests: expect.any(Array)
        })
      );
      expectNullableString(response.json.runs[0].novelId);
      expectNullableString(response.json.runs[0].novelTitle);
      expectNullableString(response.json.runs[0].currentEpisodeIndex);
      expectNullableString(response.json.runs[0].modelId);
      expectNullableString(response.json.runs[0].profileId);
      expectNullableString(response.json.runs[0].profileLabel);
      expectNullableString(response.json.runs[0].errorMessage);
      if (response.json.runs[0].requests.length > 0) {
        const request = response.json.runs[0].requests[0] as Record<string, unknown>;
        expect(request).toEqual(
          expect.objectContaining({
            requestIndex: expect.any(Number),
            kind: expect.any(String),
            toolNames: expect.any(Array),
            toolSummaries: expect.any(Array),
            inputTokens: expect.any(Number),
            outputTokens: expect.any(Number),
            totalTokens: expect.any(Number),
            cachedInputTokens: expect.any(Number),
            reasoningOutputTokens: expect.any(Number),
            cost: expect.any(Number)
          })
        );
        expectNullableNumber(request.parentRequestIndex);
      }
    }
  });

  it("returns usage run details and stable not-found errors", async () => {
    const listResponse = await requestJson<UsageListResponse>("/api/ai-generation/usage");
    expectJsonResponse(listResponse);
    expectNonEmptyFixtureArray(listResponse.json.runs, "AI usage runs");

    if (listResponse.json.runs.length > 0) {
      const detailResponse = await requestJson(`/api/ai-generation/usage/${listResponse.json.runs[0].runId}`);
      expectJsonResponse(detailResponse);
      expect(detailResponse.json).toEqual(
        expect.objectContaining({
          runId: listResponse.json.runs[0].runId,
          hasSnapshot: expect.any(Boolean),
          requests: expect.any(Array)
        })
      );
      expectNullableObject((detailResponse.json as Record<string, unknown>).snapshot);
    }

    const missingResponse = await requestJson("/api/ai-generation/usage/contract-missing-run-id");
    expectJsonResponse(missingResponse, 404);
    expectErrorShape(missingResponse.json);
  });
});
