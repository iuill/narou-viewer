import { describe, expect, it } from "vitest";
import { expectErrorShape, expectJsonResponse, requestJson } from "../harness/apiClient";

function expectCanonicalTaskSummaryShape(value: unknown): void {
  expect(value).toEqual(
    expect.objectContaining({
      queued: expect.any(Array),
      paused: expect.any(Array),
      interrupted: expect.any(Array),
      recentCompleted: expect.any(Array),
      recentFailed: expect.any(Array),
      completedCount: expect.any(Number),
      failedCount: expect.any(Number),
      canceledCount: expect.any(Number),
      pausedCount: expect.any(Number),
      interruptedCount: expect.any(Number),
      convertQueued: expect.any(Array),
    }),
  );
  const record = value as Record<string, unknown>;
  expect(record).toHaveProperty("current");
  expect(record).toHaveProperty("convertCurrent");
  expect(record).not.toHaveProperty("paused_count");
  expect(record).not.toHaveProperty("interrupted_count");
  expect(record).not.toHaveProperty("recent_completed");
  expect(record).not.toHaveProperty("recent_failed");
  expect(record).not.toHaveProperty("completed_count");
  expect(record).not.toHaveProperty("failed_count");
  expect(record).not.toHaveProperty("convert_current");
  expect(record).not.toHaveProperty("convert_queued");
}

describe("fetcher canonical API contract", () => {
  it("returns canonical fetcher status and task summary shapes", async () => {
    const status = await requestJson("/api/fetcher/status");
    expectJsonResponse(status);
    expect(status.json).toEqual(
      expect.objectContaining({
        version: expect.any(Object),
        queue: expect.any(Object),
        tasks: expect.any(Object),
        checkedAt: expect.any(String),
      }),
    );
    expectCanonicalTaskSummaryShape((status.json as { tasks: unknown }).tasks);

    const queue = await requestJson("/api/fetcher/queue");
    expectJsonResponse(queue);
    expect(queue.json).toEqual(
      expect.objectContaining({
        total: expect.any(Number),
        queued: expect.any(Number),
        webWorker: expect.any(Number),
        worker: expect.any(Number),
        running: expect.any(Boolean),
        paused: expect.any(Number),
        interrupted: expect.any(Number),
      }),
    );

    const tasks = await requestJson("/api/fetcher/tasks/summary");
    expectJsonResponse(tasks);
    expectCanonicalTaskSummaryShape(tasks.json);
  });

  it("keeps canonical fetcher command validation errors stable", async () => {
    const missingTargets = await requestJson("/api/fetcher/works/download", {
      method: "POST",
      body: {},
    });
    expectJsonResponse(missingTargets, 400);
    expect(missingTargets.json).toEqual(expect.objectContaining({
      error: "targets must be a non-empty string array.",
      code: "BAD_REQUEST",
      message: "targets must be a non-empty string array.",
    }));

    const missingNovelIds = await requestJson("/api/fetcher/works/update", {
      method: "POST",
      body: {},
    });
    expectJsonResponse(missingNovelIds, 400);
    expect(missingNovelIds.json).toEqual(expect.objectContaining({
      error: "novelIds must be a non-empty string array.",
      code: "BAD_REQUEST",
      message: "novelIds must be a non-empty string array.",
    }));

    const missingNovel = await requestJson("/api/fetcher/works/remove", {
      method: "POST",
      body: {
        novelIds: ["__api_contract_missing__"],
      },
    });
    expectJsonResponse(missingNovel, 404);
    expect(missingNovel.json).toEqual(expect.objectContaining({
      error: "Some novelIds were not found in the local library.",
      code: "NOVELS_NOT_FOUND",
      message: "Some novelIds were not found in the local library.",
      details: {
        missingNovelIds: ["__api_contract_missing__"],
      },
      missingNovelIds: ["__api_contract_missing__"],
    }));

    const blankTaskId = await requestJson("/api/fetcher/tasks/%20/cancel", {
      method: "POST",
    });
    expectJsonResponse(blankTaskId, 400);
    expect(blankTaskId.json).toEqual(expect.objectContaining({
      error: "taskId is required.",
      code: "BAD_REQUEST",
      message: "taskId is required.",
    }));

    const missingTask = await requestJson("/api/fetcher/tasks/__api_contract_missing__/cancel", {
      method: "POST",
    });
    expect([404, 502]).toContain(missingTask.status);
    expect(missingTask.contentType).toContain("application/json");
    expectErrorShape(missingTask.json);

    const missingPauseTask = await requestJson("/api/fetcher/tasks/__api_contract_missing__/pause", {
      method: "POST",
    });
    expect([404, 502]).toContain(missingPauseTask.status);
    expect(missingPauseTask.contentType).toContain("application/json");
    expectErrorShape(missingPauseTask.json);

    const missingResumeTask = await requestJson("/api/fetcher/tasks/__api_contract_missing__/resume", {
      method: "POST",
    });
    expect([404, 502]).toContain(missingResumeTask.status);
    expect(missingResumeTask.contentType).toContain("application/json");
    expectErrorShape(missingResumeTask.json);
  });
});
