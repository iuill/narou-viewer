import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AiJobsView } from "../src/features/ai-generation/AiJobsView";
import { fetchAiUsageRunJson } from "../src/features/ai-generation/api";
import { AiUsageView } from "../src/features/ai-generation/AiUsageView";
import type { AiUsageResponse } from "../src/features/ai-generation/types";
import { MobileStatusPanel, type MobileStatusPanelProps } from "../src/screens/library/MobileStatusPanel";

vi.mock("../src/features/ai-generation/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../src/features/ai-generation/api")>()),
  fetchAiUsageRunJson: vi.fn()
}));

function installDom(): JSDOM {
  const dom = new JSDOM('<!doctype html><html><body><div id="root"></div></body></html>', {
    url: "http://localhost/"
  });
  vi.stubGlobal("window", dom.window);
  vi.stubGlobal("document", dom.window.document);
  vi.stubGlobal("navigator", dom.window.navigator);
  vi.stubGlobal("Blob", dom.window.Blob);
  Object.defineProperty(dom.window.URL, "createObjectURL", {
    configurable: true,
    value: vi.fn(() => "blob:usage")
  });
  Object.defineProperty(dom.window.URL, "revokeObjectURL", { configurable: true, value: vi.fn() });
  vi.stubGlobal("URL", dom.window.URL);
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  return dom;
}

function renderIntoRoot(root: Root, element: React.ReactElement) {
  return act(async () => {
    root.render(element);
    await Promise.resolve();
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function richAiUsage(): AiUsageResponse {
  return {
    summary: {
      averageTotalTokens: 1500,
      cachedInputTokens: 200,
      inputTokens: 2400,
      outputTokens: 600,
      reasoningOutputTokens: 50,
      requestCount: 2,
      runCount: 1,
      totalCost: 0.0123,
      totalTokens: 3000
    },
    runs: [
      {
        answerChars: 120,
        cachedInputTokens: 200,
        currentEpisodeIndex: "3",
        elapsedMs: 1234,
        errorMessage: "一部失敗",
        feature: "reader-assistant",
        finishedAt: "2026-07-01T00:00:01.000Z",
        generationMode: "openrouter",
        hasSnapshot: true,
        inputTokens: 2400,
        modelId: "openai/gpt-test",
        novelId: "n1",
        novelTitle: "テスト作品",
        outputTokens: 600,
        profileLabel: "高速",
        reasoningOutputTokens: 50,
        requestCount: 2,
        requests: [
          {
            cachedInputTokens: 100,
            cost: 0.01,
            inputTokens: 1200,
            kind: "tool_call",
            outputTokens: 200,
            parentRequestIndex: null,
            reasoningOutputTokens: 0,
            requestIndex: 1,
            toolNames: ["search"],
            toolSummaries: [],
            totalTokens: 1400
          },
          {
            cachedInputTokens: 100,
            cost: 0.002,
            inputTokens: 1200,
            kind: "final_answer",
            outputTokens: 400,
            parentRequestIndex: 1,
            reasoningOutputTokens: 50,
            requestIndex: 2,
            toolNames: [],
            toolSummaries: ["まとめ"],
            totalTokens: 1600
          }
        ],
        runId: "run-abcdefghijklmnopqrstuvwxyz",
        startedAt: "2026-07-01T00:00:00.000Z",
        status: "failed",
        toolCallCount: 1,
        toolResultCount: 1,
        totalCost: 0.0123,
        totalTokens: 3000,
        workflowName: "reader"
      }
    ]
  };
}

function mobileStatusProps(overrides: Partial<MobileStatusPanelProps> = {}): MobileStatusPanelProps {
  return {
    aiGeneration: {
      activeJobsCount: 2,
      activeSettingsProfileUpdatedAt: null,
      failedJobsCount: 1,
      isOpen: false,
      jobsError: "履歴取得に失敗",
      onOpenView: vi.fn(),
      panelRef: { current: null },
      runtimeErrorDetail: "ランタイム未接続",
      setIsOpen: vi.fn(),
      settingsError: "設定保存に失敗",
      summaryLabel: "AI注意",
      triggerStatus: "warning"
    },
    queue: {
      currentFetcherTask: {
        completedAt: null,
        currentStep: 1,
        elapsedTime: null,
        errorMessage: null,
        failedEpisodeId: null,
        id: "task-1",
        message: "本文取得中",
        novelAuthor: "著者",
        novelId: "n1",
        novelIds: [],
        novelTitle: "テスト作品",
        progress: 50,
        resumeEpisodeId: null,
        savedEpisodeCount: null,
        startedAt: "2026-07-01T00:00:00.000Z",
        status: "running",
        target: "テスト作品",
        totalSteps: 2,
        type: "download",
        warnings: []
      },
      fetcherQueue: { available: true, running: true, total: 3, webWorker: 1, worker: 2 },
      fetcherStatusCheckedAt: "2026-07-01T00:00:00.000Z",
      fetcherStatusError: "取得状態に警告",
      fetcherTasksFailedCount: 1,
      fetcherUpdateNotice: "更新があります",
      hasActiveFetcherTasks: true,
      hasFetcherStatus: true,
      isOpen: false,
      onScrollToQueueProgress: vi.fn(),
      panelRef: { current: null },
      queuedTaskPreviewEntries: [],
      queueStatusLabel: "取得中",
      recentFailedFetcherTaskPreviewEntries: [],
      setIsOpen: vi.fn()
    },
    status: {
      clientUpdateRequired: null,
      error: null,
      formatDate: (value) => value ?? "確認中",
      googleBooksConfigNotice: null,
      isOpen: false,
      panelRef: { current: null },
      runtimeStatus: {
        checkedAt: "2026-07-01T00:00:00.000Z",
        services: [
          {
            detail: "API ready",
            id: "api",
            label: "API",
            status: "ok",
            summary: "ok"
          }
        ],
        status: "ok"
      },
      runtimeStatusLabel: "正常",
      setIsOpen: vi.fn(),
      viewerBuildCommitDate: "2026-07-01",
      viewerBuildSummary: "test"
    },
    ...overrides
  };
}

describe("AI generation and mobile status views", () => {
  let root: Root | null = null;

  afterEach(async () => {
    await act(async () => {
      root?.unmount();
      await Promise.resolve();
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    root = null;
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("renders AI job list branches and filter actions", async () => {
    const dom = installDom();
    root = createRoot(dom.window.document.getElementById("root") as HTMLElement);
    const onSetFilter = vi.fn();
    const onOpenNovel = vi.fn();

    await renderIntoRoot(
      root,
      createElement(AiJobsView, {
        aiGenerationActiveJobsCount: 1,
        aiGenerationCompletedJobsCount: 1,
        aiGenerationFailedJobsCount: 1,
        aiGenerationJobFilter: "active",
        aiGenerationJobsError: "履歴エラー",
        hasAiGenerationJobs: true,
        isAiGenerationJobsLoading: true,
        onOpenNovelFromJob: onOpenNovel,
        onSetAiGenerationJobFilter: onSetFilter,
        visibleAiGenerationJobs: [
          {
            completedAt: null,
            currentStep: 1,
            elapsedTime: null,
            errorMessage: "生成失敗",
            finishedAt: "2026-07-01T00:01:00.000Z",
            generationMode: "openrouter",
            generationStrategy: "strict",
            id: "job-1",
            jobId: "job-1",
            message: null,
            modelId: "openai/gpt-test",
            novelAuthor: null,
            novelId: "n1",
            novelTitle: "作品A",
            profileId: "profile-1",
            profileLabel: "標準",
            progress: 100,
            requestedUpToEpisodeIndex: "3",
            startedAt: "2026-07-01T00:00:00.000Z",
            status: "failed",
            totalSteps: 1
          }
        ]
      })
    );

    expect(dom.window.document.body.textContent).toContain("履歴エラー");
    expect(dom.window.document.body.textContent).toContain("作品A");
    expect(dom.window.document.body.textContent).toContain("生成失敗");

    const buttons = [...dom.window.document.querySelectorAll<HTMLButtonElement>("button")];
    await act(async () => {
      buttons.find((button) => button.textContent?.includes("失敗"))?.click();
      buttons.find((button) => button.textContent?.includes("完了"))?.click();
      buttons.find((button) => button.textContent === "作品を開く")?.click();
    });
    expect(onSetFilter).toHaveBeenCalledWith("failed");
    expect(onSetFilter).toHaveBeenCalledWith("completed");
    expect(onOpenNovel).toHaveBeenCalledWith("n1");

    await renderIntoRoot(
      root,
      createElement(AiJobsView, {
        aiGenerationActiveJobsCount: 0,
        aiGenerationCompletedJobsCount: 0,
        aiGenerationFailedJobsCount: 1,
        aiGenerationJobFilter: "failed",
        aiGenerationJobsError: null,
        hasAiGenerationJobs: true,
        isAiGenerationJobsLoading: false,
        onOpenNovelFromJob: onOpenNovel,
        onSetAiGenerationJobFilter: onSetFilter,
        visibleAiGenerationJobs: [
          {
            createdAt: "",
            errorMessage: "抽出チェックポイントが現在の build と互換性がないため自動再開を停止しました。",
            finishedAt: null,
            generationMode: null,
            generationStrategy: null,
            jobId: "incompatible-job",
            modelId: null,
            novelAuthor: null,
            novelId: "n1",
            novelTitle: "作品A",
            profileId: null,
            profileLabel: null,
            requestedUpToEpisodeIndex: "",
            startedAt: null,
            status: "incompatible"
          }
        ]
      })
    );
    expect(dom.window.document.body.textContent).toContain("互換性なし・要復旧");
    expect(dom.window.document.body.textContent).toContain("対象範囲不明");
    expect(dom.window.document.body.textContent).toContain("生成方式不明");
    expect(dom.window.document.body.textContent).toContain("自動再開を停止しました");

    await renderIntoRoot(
      root,
      createElement(AiJobsView, {
        aiGenerationActiveJobsCount: 0,
        aiGenerationCompletedJobsCount: 0,
        aiGenerationFailedJobsCount: 0,
        aiGenerationJobFilter: "active",
        aiGenerationJobsError: null,
        hasAiGenerationJobs: false,
        isAiGenerationJobsLoading: true,
        onOpenNovelFromJob: onOpenNovel,
        onSetAiGenerationJobFilter: onSetFilter,
        visibleAiGenerationJobs: []
      })
    );
    expect(dom.window.document.body.textContent).toContain("読み込み中");
    expect(dom.window.document.body.textContent).toContain("進行中のキャラ生成はありません");

    await renderIntoRoot(
      root,
      createElement(AiJobsView, {
        aiGenerationActiveJobsCount: 0,
        aiGenerationCompletedJobsCount: 0,
        aiGenerationFailedJobsCount: 0,
        aiGenerationJobFilter: "failed",
        aiGenerationJobsError: null,
        hasAiGenerationJobs: false,
        isAiGenerationJobsLoading: false,
        onOpenNovelFromJob: onOpenNovel,
        onSetAiGenerationJobFilter: onSetFilter,
        visibleAiGenerationJobs: []
      })
    );
    expect(dom.window.document.body.textContent).toContain("失敗したキャラ生成はありません");

    await renderIntoRoot(
      root,
      createElement(AiJobsView, {
        aiGenerationActiveJobsCount: 0,
        aiGenerationCompletedJobsCount: 0,
        aiGenerationFailedJobsCount: 0,
        aiGenerationJobFilter: "completed",
        aiGenerationJobsError: null,
        hasAiGenerationJobs: false,
        isAiGenerationJobsLoading: false,
        onOpenNovelFromJob: onOpenNovel,
        onSetAiGenerationJobFilter: onSetFilter,
        visibleAiGenerationJobs: []
      })
    );
    expect(dom.window.document.body.textContent).toContain("完了済みのキャラ生成はまだありません");
  });

  it("renders AI usage branches and downloads saved JSON", async () => {
    const dom = installDom();
    root = createRoot(dom.window.document.getElementById("root") as HTMLElement);
    vi.mocked(fetchAiUsageRunJson).mockResolvedValue({ ok: true });

    await renderIntoRoot(
      root,
      createElement(AiUsageView, {
        aiUsage: null,
        aiUsageError: "usage error",
        isAiUsageLoading: true
      })
    );
    expect(dom.window.document.body.textContent).toContain("usage error");
    expect(dom.window.document.body.textContent).toContain("AI使用量を読み込み中");

    await renderIntoRoot(
      root,
      createElement(AiUsageView, {
        aiUsage: { summary: { ...richAiUsage().summary, requestCount: 0, runCount: 0 }, runs: [] },
        aiUsageError: null,
        isAiUsageLoading: false
      })
    );
    expect(dom.window.document.body.textContent).toContain("まだ読書AIの使用量は記録されていません");

    await renderIntoRoot(
      root,
      createElement(AiUsageView, {
        aiUsage: richAiUsage(),
        aiUsageError: null,
        isAiUsageLoading: false
      })
    );

    expect(dom.window.document.body.textContent).toContain("テスト作品");
    expect(dom.window.document.body.textContent).toContain("reasoning 50");
    expect(dom.window.document.body.textContent).toContain("一部失敗");
    expect(dom.window.document.body.textContent).toContain("tool call");
    expect(dom.window.document.body.textContent).toContain("final answer");

    await act(async () => {
      dom.window.document.querySelector<HTMLButtonElement>(".ai-usage-json-button")?.click();
      await Promise.resolve();
    });
    expect(fetchAiUsageRunJson).toHaveBeenCalledWith("run-abcdefghijklmnopqrstuvwxyz");
    expect(URL.createObjectURL).toHaveBeenCalled();
    expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:usage");
  });

  it("renders mobile status branches for active and pending states", async () => {
    const dom = installDom();
    root = createRoot(dom.window.document.getElementById("root") as HTMLElement);
    const onScrollToQueueProgress = vi.fn();
    const onOpenView = vi.fn();

    await renderIntoRoot(
      root,
      createElement(MobileStatusPanel, {
        ...mobileStatusProps(),
        aiGeneration: { ...mobileStatusProps().aiGeneration, onOpenView },
        queue: { ...mobileStatusProps().queue, onScrollToQueueProgress }
      })
    );
    expect(dom.window.document.body.textContent).toContain("更新があります");
    expect(dom.window.document.body.textContent).toContain("API ready");
    expect(dom.window.document.body.textContent).toContain("実行中: yes");
    expect(dom.window.document.body.textContent).toContain("50%");
    expect(dom.window.document.body.textContent).toContain("設定保存に失敗");
    expect(dom.window.document.body.textContent).toContain("ランタイム未接続");
    expect(dom.window.document.body.textContent).toContain("履歴取得に失敗");

    await act(async () => {
      dom.window.document.querySelector<HTMLButtonElement>(".queue-detail-button")?.click();
      dom.window.document.querySelector<HTMLButtonElement>(".ai-generation-nav-button")?.click();
    });
    expect(onScrollToQueueProgress).toHaveBeenCalledTimes(1);
    expect(onOpenView).toHaveBeenCalled();

    await renderIntoRoot(
      root,
      createElement(MobileStatusPanel, {
        ...mobileStatusProps({
          aiGeneration: {
            ...mobileStatusProps().aiGeneration,
            activeJobsCount: 0,
            failedJobsCount: 0,
            jobsError: null,
            runtimeErrorDetail: null,
            settingsError: null,
            summaryLabel: "AI OK"
          },
          queue: {
            ...mobileStatusProps().queue,
            currentFetcherTask: null,
            fetcherQueue: null,
            fetcherStatusCheckedAt: null,
            fetcherStatusError: null,
            fetcherUpdateNotice: null
          },
          status: {
            ...mobileStatusProps().status,
            runtimeStatus: null,
            runtimeStatusLabel: "確認中"
          }
        })
      })
    );
    expect(dom.window.document.body.textContent).toContain("動作状況を確認中");
    expect(dom.window.document.body.textContent).toContain("実行中: no");
    expect(dom.window.document.body.textContent).toContain("進行中: 0 件");
  });
});
