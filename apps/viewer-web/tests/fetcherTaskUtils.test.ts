import { describe, expect, it } from "vitest";
import {
  formatFetcherTask,
  formatFetcherTaskFailure,
  formatFetcherTaskProgress,
  formatFetcherTaskStepProgress,
  getFetcherTaskTargetLabel,
  getFetcherActiveTaskEntries,
  getFetcherActiveTasks,
  getFetcherBusyFetcherWorkIds,
  getFetcherQueuedTasks,
  getFetcherResumableTaskEntries,
  getFetcherResumableTaskWorkIds,
  getFetcherTaskProgressValue,
  getFetcherTaskListEntries,
  hasFetcherTaskDeterminateProgress,
  toFetcherTaskSummary,
} from "../src/features/fetcher/model";
import type { FetcherTaskSummaryResponse } from "../src/features/fetcher/types";

function expectCurrentTask(summary: ReturnType<typeof toFetcherTaskSummary>) {
  expect(summary.current).not.toBeNull();
  if (summary.current === null) {
    throw new Error("Expected current fetcher task");
  }
  return summary.current;
}

describe("fetcherTaskUtils", () => {
  it("normalizes canonical task summary payloads from fetcher BFF", () => {
    const summary = toFetcherTaskSummary({
      current: {
        taskId: "task-1",
        type: "download",
        targets: ["https://ncode.syosetu.com/n1234ab/"],
        novelId: 42,
        novelIds: [42, "43"],
        novelTitle: "長編サンプル",
        novelAuthor: "作者A",
        status: "running",
        message: "ダウンロード中...",
        warnings: ["同名または近いタイトルの作品が別サイトにあります: 長編サンプル（小説家になろう）"],
        error: "network timeout",
        createdAt: "2026-03-16T00:00:00.000Z",
        startedAt: "2026-03-16T00:00:05.000Z",
        finishedAt: "2026-03-16T00:00:35.000Z",
        elapsedTime: 18.4,
        progress: 37.5,
        totalSteps: 120,
        currentStep: 45,
      },
      queued: [
        {
          id: "task-2",
          type: "update",
          novelTitle: "続編",
          status: "queued",
          progress: 0,
          totalSteps: "12",
          currentStep: "0",
        },
      ],
      recentCompleted: [],
      recentFailed: [],
      completedCount: 3,
      failedCount: 1,
      convertCurrent: null,
      convertQueued: [],
    } satisfies FetcherTaskSummaryResponse);

    expect(summary.current).toMatchObject({
      id: "task-1",
      type: "download",
      target: "https://ncode.syosetu.com/n1234ab/",
      novelId: "42",
      novelIds: ["42", "43"],
      novelTitle: "長編サンプル",
      novelAuthor: "作者A",
      warnings: ["同名または近いタイトルの作品が別サイトにあります: 長編サンプル（小説家になろう）"],
      errorMessage: "network timeout",
      completedAt: "2026-03-16T00:00:35.000Z",
      progress: 37.5,
      totalSteps: 120,
      currentStep: 45,
    });
    const currentTask = expectCurrentTask(summary);
    expect(getFetcherTaskTargetLabel(currentTask)).toBe("長編サンプル");
    expect(formatFetcherTaskFailure(currentTask)).toBe("network timeout");
    expect(summary.queued[0]).toMatchObject({
      id: "task-2",
      type: "update",
      status: "queued",
      totalSteps: 12,
      currentStep: 0,
    });
    expect(summary.completedCount).toBe(3);
    expect(summary.failedCount).toBe(1);
  });

  it("collects current and queued active tasks in display order", () => {
    const summary = toFetcherTaskSummary({
      current: { id: "download-current", type: "download", status: "running" },
      queued: [{ id: "download-queued", type: "download", status: "queued" }],
      recentCompleted: [],
      recentFailed: [],
      completedCount: 0,
      failedCount: 0,
      convertCurrent: { id: "convert-current", type: "convert", status: "running" },
      convertQueued: [{ id: "convert-queued", type: "convert", status: "queued" }],
    } satisfies FetcherTaskSummaryResponse);

    expect(getFetcherActiveTasks(summary).map((task) => task.id)).toEqual([
      "download-current",
      "convert-current",
      "download-queued",
      "convert-queued",
    ]);
  });

  it("collects busy fetcher work IDs for active fetcher tasks", () => {
    const summary = toFetcherTaskSummary({
      current: { id: "update-current", type: "update", novelIds: ["101"], status: "running" },
      queued: [
        { id: "resume-queued", type: "resume", novelIds: ["102"], status: "queued" },
        { id: "download-queued", type: "download", novelIds: ["103"], status: "queued" },
      ],
      recentCompleted: [{ id: "update-completed", type: "update", novelIds: ["104"], status: "completed" }],
      recentFailed: [{ id: "resume-failed", type: "resume", novelIds: ["105"], status: "failed" }],
      completedCount: 1,
      failedCount: 1,
      convertCurrent: { id: "convert-current", type: "convert", novelIds: ["106"], status: "running" },
      convertQueued: [],
    } satisfies FetcherTaskSummaryResponse);

    expect([...getFetcherBusyFetcherWorkIds(summary)].sort()).toEqual(["101", "102", "103"]);
  });

  it("keeps queued tasks in preview order even when status is missing", () => {
    const summary = toFetcherTaskSummary({
      current: null,
      queued: [
        { id: "download-queued", type: "download", novelTitle: "連載A" },
        { id: "update-queued", type: "update", status: "queued", novelTitle: "連載B" },
      ],
      recentCompleted: [],
      recentFailed: [],
      completedCount: 0,
      failedCount: 0,
      convertCurrent: null,
      convertQueued: [{ id: "convert-queued", type: "convert", novelTitle: "連載C" }],
    } satisfies FetcherTaskSummaryResponse);

    expect(getFetcherQueuedTasks(summary).map((task) => task.id)).toEqual([
      "download-queued",
      "update-queued",
      "convert-queued",
    ]);
  });

  it("excludes canceled history from resumable tasks and owned work IDs", () => {
    const summary = toFetcherTaskSummary({
      current: null,
      queued: [],
      paused: [{ id: "paused", type: "update", novelIds: ["101"], status: "paused", canResume: true }],
      interrupted: [],
      recentCompleted: [],
      recentFailed: [
        { id: "failed", type: "resume", novelIds: ["102"], status: "failed", canResume: true },
        { id: "canceled", type: "resume", novelIds: ["103"], status: "canceled", canResume: false }
      ],
      completedCount: 0,
      failedCount: 2,
      convertCurrent: null,
      convertQueued: []
    } satisfies FetcherTaskSummaryResponse);

    expect(getFetcherResumableTaskEntries(summary).map((entry) => entry.task.id)).toEqual(["paused", "failed"]);
    expect([...getFetcherResumableTaskWorkIds(summary)]).toEqual(["101", "102"]);
  });

  it("preserves current task payload fields", () => {
    const summary = toFetcherTaskSummary({
      current: {
        type: "download",
        state: "running",
        title: "作品タイトル",
        novelAuthor: "作者A",
        createdAt: "2026-03-16T00:00:00.000Z",
        totalSteps: 8,
        currentStep: 3,
      },
      queued: [],
      recentCompleted: [],
      recentFailed: [],
      completedCount: 0,
      failedCount: 0,
      convertCurrent: null,
      convertQueued: [],
    } satisfies FetcherTaskSummaryResponse);

    expect(summary.current).toMatchObject({
      type: "download",
      status: "running",
      novelTitle: "作品タイトル",
      novelAuthor: "作者A",
      createdAt: "2026-03-16T00:00:00.000Z",
      totalSteps: 8,
      currentStep: 3,
    });
    expect(formatFetcherTask(summary.current)).toBe("追加 / 実行中 / 作品タイトル");
  });

  it("formats progress labels from task steps when available", () => {
    const summary = toFetcherTaskSummary({
      current: {
        id: "task-1",
        type: "download",
        novelTitle: "長編サンプル",
        status: "running",
        totalSteps: 100,
        currentStep: 25,
      },
      queued: [],
      recentCompleted: [],
      recentFailed: [],
      completedCount: 0,
      failedCount: 0,
      convertCurrent: null,
      convertQueued: [],
    } satisfies FetcherTaskSummaryResponse);

    const task = expectCurrentTask(summary);
    expect(getFetcherTaskProgressValue(task)).toBe(25);
    expect(formatFetcherTaskProgress(task)).toBe("25%");
    expect(formatFetcherTaskStepProgress(task)).toBe("25 / 100 話");
    expect(formatFetcherTask(task)).toBe("追加 / 実行中 / 長編サンプル");
  });

  it("does not invent step progress when only total steps are reported", () => {
    const summary = toFetcherTaskSummary({
      current: {
        id: "task-1",
        type: "download",
        status: "running",
        totalSteps: 100,
        message: "総話数を確認中",
      },
      queued: [],
      recentCompleted: [],
      recentFailed: [],
      completedCount: 0,
      failedCount: 0,
      convertCurrent: null,
      convertQueued: [],
    } satisfies FetcherTaskSummaryResponse);

    const task = expectCurrentTask(summary);
    expect(hasFetcherTaskDeterminateProgress(task)).toBe(false);
    expect(formatFetcherTaskProgress(task)).toBe("進行中");
    expect(formatFetcherTaskStepProgress(task)).toBeNull();
  });

  it("treats running download tasks without progress fields as indeterminate", () => {
    const summary = toFetcherTaskSummary({
      current: {
        id: "task-1",
        type: "download",
        novelTitle: "新規作品",
        status: "running",
        message: "実行中...",
      },
      queued: [],
      recentCompleted: [],
      recentFailed: [],
      completedCount: 0,
      failedCount: 0,
      convertCurrent: null,
      convertQueued: [],
    } satisfies FetcherTaskSummaryResponse);

    const task = expectCurrentTask(summary);
    expect(hasFetcherTaskDeterminateProgress(task)).toBe(false);
    expect(getFetcherTaskProgressValue(task)).toBe(0);
    expect(formatFetcherTaskProgress(task)).toBe("進行中");
    expect(formatFetcherTaskStepProgress(task)).toBeNull();
  });

  it("builds unique render keys for active tasks even when fallback ids collide", () => {
    const summary = toFetcherTaskSummary({
      current: null,
      queued: [
        { type: "download", novelTitle: "重複タスク" },
        { type: "download", novelTitle: "重複タスク" },
      ],
      recentCompleted: [],
      recentFailed: [],
      completedCount: 0,
      failedCount: 0,
      convertCurrent: null,
      convertQueued: [],
    } satisfies FetcherTaskSummaryResponse);

    const entries = getFetcherActiveTaskEntries(summary);
    expect(entries).toHaveLength(2);
    expect(new Set(entries.map((entry) => entry.key)).size).toBe(2);
    expect(entries[0]?.task.id).toBe(entries[1]?.task.id);
  });

  it("builds unique render keys for queued and failed task lists when fallback ids collide", () => {
    const summary = toFetcherTaskSummary({
      current: null,
      queued: [
        { type: "download", novelTitle: "重複タスク" },
        { type: "download", novelTitle: "重複タスク" },
      ],
      recentCompleted: [],
      recentFailed: [
        { type: "resume", status: "failed", novelTitle: "重複失敗", message: "timeout" },
        { type: "resume", status: "failed", novelTitle: "重複失敗", message: "timeout" },
      ],
      completedCount: 0,
      failedCount: 2,
      convertCurrent: null,
      convertQueued: [],
    } satisfies FetcherTaskSummaryResponse);

    const queuedEntries = getFetcherTaskListEntries(getFetcherQueuedTasks(summary));
    const failedEntries = getFetcherTaskListEntries(summary.recentFailed);

    expect(new Set(queuedEntries.map((entry) => entry.key)).size).toBe(2);
    expect(new Set(failedEntries.map((entry) => entry.key)).size).toBe(2);
    expect(queuedEntries[0]?.task.id).toBe(queuedEntries[1]?.task.id);
    expect(failedEntries[0]?.task.id).toBe(failedEntries[1]?.task.id);
  });
});
