import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import {
  fetchAiGenerationJobs,
  fetchAiGenerationSettings,
  fetchAiUsage,
  putAiGenerationPreferredMode,
  readAiGenerationPlaygroundStream
} from "../src/features/ai-generation/api";
import type { NovelSummary } from "../src/features/library/types";
import type { RuntimeStatusResponse } from "../src/features/runtime/types";
import type { AiGenerationJobsResponse, AiGenerationSettingsResponse } from "../src/features/ai-generation/types";
import { useAiGeneration } from "../src/hooks/useAiGeneration";

vi.mock("../src/features/ai-generation/api", () => ({
  fetchAiGenerationJobs: vi.fn(),
  fetchAiGenerationSettings: vi.fn(),
  fetchAiUsage: vi.fn(),
  postAiGenerationPlaygroundStream: vi.fn(),
  putAiGenerationPreferredMode: vi.fn(),
  putAiGenerationSettings: vi.fn(),
  readAiGenerationPlaygroundStream: vi.fn()
}));

type HookResult = ReturnType<typeof useAiGeneration>;

function installDom(): JSDOM {
  const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
    url: "http://localhost/"
  });

  Object.defineProperty(dom.window.document, "hidden", {
    configurable: true,
    value: false
  });
  vi.stubGlobal("window", dom.window);
  vi.stubGlobal("document", dom.window.document);
  vi.stubGlobal("navigator", dom.window.navigator);
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);

  return dom;
}

async function flushAsyncWork(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
}

function createRuntimeStatus(): RuntimeStatusResponse {
  return {
    status: "ok",
    checkedAt: "2026-06-15T00:00:00.000Z",
    services: []
  };
}

function createNovel(novelId: string): NovelSummary {
  return {
    novelId,
    fetcherWorkId: novelId,
    title: "作品",
    author: "著者",
    siteName: "narou",
    tocUrl: null,
    totalEpisodes: 5,
    lastReadEpisodeIndex: null,
    lastReadEpisodeTitle: null,
    latestBookmarkEpisodeIndex: null,
    bookmarkCount: 0,
    updatedAt: null
  };
}

function createSettings(): AiGenerationSettingsResponse {
  return {
    apiBaseUrlConfigured: true,
    preferredMode: "llm",
    effectiveGenerationMode: "openrouter",
    masterPassphraseConfigured: true,
    settings: {
      selectedProfileId: "profile-a",
      sharedProviders: {
        openrouter: {
          hasApiKey: true,
          apiKeyMasked: "sk-***",
          updatedAt: "2026-06-15T00:00:00.000Z"
        }
      },
      profiles: [
        {
          id: "profile-a",
          label: "Default",
          provider: "openrouter",
          credentials: {
            source: "shared",
            hasApiKey: true,
            apiKeyMasked: null,
            updatedAt: null
          },
          modelId: "openai/gpt",
          providerOrder: [],
          allowFallbacks: true,
          requireParameters: false,
          updatedAt: "2026-06-15T00:00:00.000Z"
        }
      ],
      extractionStrategyModels: {
        nameDiscoveryModelId: "openai/gpt-5-nano"
      },
      extractionRuntime: {
        parallelRequestConcurrency: 4
      }
    }
  };
}

function createUsage() {
  return {
    summary: {
      averageTotalTokens: 0,
      cachedInputTokens: 0,
      inputTokens: 0,
      outputTokens: 0,
      reasoningOutputTokens: 0,
      requestCount: 0,
      runCount: 0,
      totalCost: 0,
      totalTokens: 0
    },
    runs: []
  };
}

function renderHookHarness(props: {
  onRender: (result: HookResult) => void;
  refreshRuntimeStatus?: () => Promise<RuntimeStatusResponse | null>;
}): Root {
  const rootElement = document.getElementById("root");
  if (!rootElement) {
    throw new Error("root element is missing");
  }

  function Harness() {
    const result = useAiGeneration({
      isPaused: false,
      libraryReloadKey: 0,
      novels: [createNovel("novel-a")],
      refreshRuntimeStatus: props.refreshRuntimeStatus ?? vi.fn().mockResolvedValue(createRuntimeStatus()),
      runtimeStatus: createRuntimeStatus(),
      selectedNovelId: "novel-a"
    });
    props.onRender(result);
    return null;
  }

  const root = createRoot(rootElement);
  root.render(createElement(Harness));
  return root;
}

describe("useAiGeneration", () => {
  let root: Root | null = null;

  afterEach(async () => {
    await act(async () => {
      root?.unmount();
      root = null;
      await flushAsyncWork();
    });
    vi.unstubAllGlobals();
    vi.clearAllMocks();
    vi.restoreAllMocks();
  });

  it("loads settings and jobs into draft state", async () => {
    installDom();
    const jobs: AiGenerationJobsResponse = {
      jobs: [
        {
          jobId: "job-a",
          novelId: "novel-a",
          novelTitle: "作品",
          novelAuthor: "著者",
          profileId: "profile-a",
          profileLabel: "Default",
          requestedUpToEpisodeIndex: "2",
          generationMode: "openrouter",
          modelId: "openai/gpt",
          status: "running",
          createdAt: "2026-06-15T00:00:00.000Z",
          startedAt: "2026-06-15T00:00:00.000Z",
          finishedAt: null,
          errorMessage: null
        }
      ]
    };
    vi.mocked(fetchAiGenerationSettings).mockResolvedValue(createSettings());
    vi.mocked(fetchAiGenerationJobs).mockResolvedValue(jobs);
    vi.mocked(readAiGenerationPlaygroundStream).mockResolvedValue({
      novelId: "novel-a",
      novelTitle: "作品",
      upToEpisodeIndex: "2",
      processedUpToEpisodeIndex: "2",
      profileId: null,
      profileLabel: null,
      generationMode: "heuristic",
      modelId: null,
      characters: []
    });

    let latest: HookResult | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    expect(fetchAiGenerationSettings).toHaveBeenCalledTimes(1);
    expect(fetchAiGenerationJobs).toHaveBeenCalledTimes(1);
    expect(latest?.preferredMode).toBe("llm");
    expect(latest?.selectedProfileId).toBe("profile-a");
    expect(latest?.profileDrafts).toHaveLength(1);
    expect(latest?.extractionStrategyModelsDraft.nameDiscoveryModelId).toBe("openai/gpt-5-nano");
    expect(latest?.extractionRuntimeDraft.parallelRequestConcurrency).toBe(4);
    expect(latest?.activeJobs).toHaveLength(1);
    expect(latest?.playgroundNovelId).toBe("novel-a");

  });

  it("saves preferred mode and refreshes runtime status", async () => {
    installDom();
    vi.mocked(fetchAiGenerationSettings).mockResolvedValue(createSettings());
    vi.mocked(fetchAiGenerationJobs).mockResolvedValue({ jobs: [] });
    vi.mocked(readAiGenerationPlaygroundStream).mockResolvedValue({
      novelId: "novel-a",
      novelTitle: "作品",
      upToEpisodeIndex: "2",
      processedUpToEpisodeIndex: "2",
      profileId: null,
      profileLabel: null,
      generationMode: "heuristic",
      modelId: null,
      characters: []
    });
    vi.mocked(putAiGenerationPreferredMode).mockResolvedValue({
      preferredMode: "heuristic",
      effectiveGenerationMode: "heuristic"
    });
    const refreshRuntimeStatus = vi.fn().mockResolvedValue(createRuntimeStatus());

    let latest: HookResult | null = null;
    await act(async () => {
      root = renderHookHarness({
        refreshRuntimeStatus,
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    await act(async () => {
      await latest?.changePreferredMode("heuristic");
      await flushAsyncWork();
    });

    expect(putAiGenerationPreferredMode).toHaveBeenCalledWith("heuristic");
    expect(refreshRuntimeStatus).toHaveBeenCalledTimes(1);
    expect(latest?.preferredMode).toBe("heuristic");
    expect(latest?.notice).toBe("連携モードを更新しました。");

  });

  it("reports preferred mode save failures without changing the selected mode", async () => {
    installDom();
    vi.mocked(fetchAiGenerationSettings).mockResolvedValue(createSettings());
    vi.mocked(fetchAiGenerationJobs).mockResolvedValue({ jobs: [] });
    vi.mocked(readAiGenerationPlaygroundStream).mockResolvedValue({
      novelId: "novel-a",
      novelTitle: "作品",
      upToEpisodeIndex: "2",
      processedUpToEpisodeIndex: "2",
      profileId: null,
      profileLabel: null,
      generationMode: "heuristic",
      modelId: null,
      characters: []
    });
    vi.mocked(putAiGenerationPreferredMode).mockRejectedValue(new Error("mode failed"));

    let latest: HookResult | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    await act(async () => {
      await latest?.changePreferredMode("heuristic");
      await flushAsyncWork();
    });

    expect(latest?.preferredMode).toBe("llm");
    expect(latest?.settingsError).toBe("mode failed");

  });

  it("opens usage view and surfaces usage load failures", async () => {
    installDom();
    vi.mocked(fetchAiGenerationSettings).mockResolvedValue(createSettings());
    vi.mocked(fetchAiGenerationJobs).mockResolvedValue({ jobs: [] });
    vi.mocked(fetchAiUsage).mockRejectedValue(new Error("usage failed"));
    vi.mocked(readAiGenerationPlaygroundStream).mockResolvedValue({
      novelId: "novel-a",
      novelTitle: "作品",
      upToEpisodeIndex: "2",
      processedUpToEpisodeIndex: "2",
      profileId: null,
      profileLabel: null,
      generationMode: "heuristic",
      modelId: null,
      characters: []
    });

    let latest: HookResult | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    await act(async () => {
      await latest?.openView("usage");
      await flushAsyncWork();
    });

    expect(latest?.activeView).toBe("usage");
    expect(latest?.usage).toBeNull();
    expect(latest?.usageError).toBe("usage failed");

  });

  it("opens playground with background refreshes for settings, jobs, and usage", async () => {
    installDom();
    vi.mocked(fetchAiGenerationSettings).mockResolvedValue(createSettings());
    vi.mocked(fetchAiGenerationJobs).mockResolvedValue({ jobs: [] });
    vi.mocked(fetchAiUsage).mockResolvedValue(createUsage());
    vi.mocked(readAiGenerationPlaygroundStream).mockResolvedValue({
      novelId: "novel-a",
      novelTitle: "作品",
      upToEpisodeIndex: "2",
      processedUpToEpisodeIndex: "2",
      profileId: null,
      profileLabel: null,
      generationMode: "heuristic",
      modelId: null,
      characters: []
    });

    let latest: HookResult | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    vi.mocked(fetchAiGenerationSettings).mockClear();
    vi.mocked(fetchAiGenerationJobs).mockClear();
    vi.mocked(fetchAiUsage).mockClear();
    await act(async () => {
      await latest?.openView("playground");
      await flushAsyncWork();
    });

    expect(latest?.activeView).toBe("playground");
    expect(fetchAiGenerationSettings).toHaveBeenCalledTimes(1);
    expect(fetchAiGenerationJobs).toHaveBeenCalledTimes(1);
    expect(fetchAiUsage).toHaveBeenCalledTimes(1);
    expect(latest?.usage?.runs).toEqual([]);

  });
});
