import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import { fetchCharacterSummary } from "../src/features/characters/api";
import type { CharacterSummaryResponse } from "../src/features/characters/types";
import {
  clearExtraction,
  fetchExtractionJobs,
  submitExtraction,
} from "../src/features/extraction/api";
import type { ExtractionJobsResponse } from "../src/features/extraction/types";
import type { TocResponse } from "../src/features/reader/types";
import { fetchTerms } from "../src/features/terms/api";
import type { TermsResponse } from "../src/features/terms/types";
import { useExtraction } from "../src/hooks/useExtraction";

vi.mock("../src/features/characters/api", () => ({
  fetchCharacterSummary: vi.fn(),
}));
vi.mock("../src/features/extraction/api", () => ({
  clearExtraction: vi.fn(),
  fetchExtractionJobs: vi.fn(),
  submitExtraction: vi.fn(),
}));
vi.mock("../src/features/terms/api", () => ({ fetchTerms: vi.fn() }));

type HookResult = ReturnType<typeof useExtraction>;

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((nextResolve, nextReject) => {
    resolve = nextResolve;
    reject = nextReject;
  });
  return { promise, reject, resolve };
}

function installDom(): JSDOM {
  const dom = new JSDOM(
    '<!doctype html><html><body><div id="root"></div></body></html>',
    {
      url: "http://localhost/",
    },
  );

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

function createToc(): TocResponse {
  return {
    novelId: "novel-a",
    fetcherWorkId: "novel-a",
    title: "作品",
    author: "著者",
    siteName: "narou",
    tocUrl: null,
    updatedAt: "2026-06-16T00:00:00.000Z",
    story: "",
    totalEpisodes: 3,
    episodes: [
      {
        episodeIndex: "1",
        title: "一",
        chapter: null,
        subchapter: null,
        sourceUrl: null,
        updatedAt: null,
        contentEtag: "toc-1",
        bodyStatus: "complete",
      },
      {
        episodeIndex: "2",
        title: "二",
        chapter: null,
        subchapter: null,
        sourceUrl: null,
        updatedAt: null,
        contentEtag: "toc-2",
        bodyStatus: "complete",
      },
      {
        episodeIndex: "3",
        title: "三",
        chapter: null,
        subchapter: null,
        sourceUrl: null,
        updatedAt: null,
        contentEtag: "toc-3",
        bodyStatus: "complete",
      },
    ],
  };
}

function createTocForNovel(novelId: string): TocResponse {
  return { ...createToc(), novelId, fetcherWorkId: novelId, title: `作品 ${novelId}` };
}

function renderSwitchableHookHarness(onRender: (result: HookResult) => void, notices: string[]) {
  const rootElement = document.getElementById("root");
  if (!rootElement) {
    throw new Error("root element is missing");
  }
  const root = createRoot(rootElement);
  function Harness({ novelId }: { novelId: string }) {
    const result = useExtraction({
      currentTocEpisodeIndex: 2,
      formatEpisodeOrderLabel: (episodeIndex) => episodeIndex,
      isOpen: true,
      onClosePanel: vi.fn(),
      onOpenPanel: vi.fn(),
      onOpenTermsPanel: vi.fn(),
      screenMode: "reader",
      selectedNovelId: novelId,
      setReaderNotice: (nextNotice) => {
        if (typeof nextNotice !== "function" && nextNotice !== null) {
          notices.push(nextNotice);
        }
      },
      toc: createTocForNovel(novelId),
    });
    onRender(result);
    return null;
  }
  return {
    render: (novelId: string) => root.render(createElement(Harness, { novelId })),
    root,
  };
}

function createReadySummary(
  upToEpisodeIndex: string,
): CharacterSummaryResponse {
  return {
    status: "ready",
    novelId: "novel-a",
    upToEpisodeIndex,
    processedUpToEpisodeIndex: upToEpisodeIndex,
    characters: [],
  };
}

function createReadyTerms(upToEpisodeIndex: string): TermsResponse {
  return {
    status: "ready",
    novelId: "novel-a",
    upToEpisodeIndex,
    processedUpToEpisodeIndex: upToEpisodeIndex,
    terms: [],
  };
}

function renderHookHarness(props: {
  currentTocEpisodeIndex?: number;
  onRender: (result: HookResult, notices: string[], openCalls: number) => void;
}): Root {
  const rootElement = document.getElementById("root");
  if (!rootElement) {
    throw new Error("root element is missing");
  }

  const notices: string[] = [];
  let openCalls = 0;

  function Harness() {
    const result = useExtraction({
      currentTocEpisodeIndex: props.currentTocEpisodeIndex ?? 1,
      formatEpisodeOrderLabel: (episodeIndex) => episodeIndex,
      isOpen: true,
      onClosePanel: vi.fn(),
      onOpenPanel: () => {
        openCalls += 1;
      },
      onOpenTermsPanel: vi.fn(),
      screenMode: "reader",
      selectedNovelId: "novel-a",
      setReaderNotice: (nextNotice) => {
        if (typeof nextNotice === "function") {
          return;
        }
        if (nextNotice !== null) {
          notices.push(nextNotice);
        }
      },
      toc: createToc(),
    });
    props.onRender(result, notices, openCalls);
    return null;
  }

  const root = createRoot(rootElement);
  root.render(createElement(Harness));
  return root;
}

describe("useExtraction", () => {
  afterEach(() => {
    vi.clearAllMocks();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("polls every two seconds while an extraction job is active", async () => {
    const dom = installDom();
    const intervalSpy = vi.spyOn(dom.window, "setInterval");
    vi.mocked(fetchExtractionJobs).mockResolvedValue({
      jobs: [
        {
          jobId: "job-running",
          requestedUpToEpisodeIndex: "2",
          generationMode: "openrouter",
          generationStrategy: "parallel_identity",
          modelId: "synthetic/model",
          status: "running",
          createdAt: "2026-07-11T00:00:00Z",
          startedAt: "2026-07-11T00:00:01Z",
          finishedAt: null,
          errorMessage: null,
        },
      ],
    });
    vi.mocked(fetchCharacterSummary).mockResolvedValue(createReadySummary("2"));
    vi.mocked(fetchTerms).mockResolvedValue(createReadyTerms("2"));

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        currentTocEpisodeIndex: 2,
        onRender: (result) => {
          latest = result;
        },
      });
      await flushAsyncWork();
    });

    expect(intervalSpy.mock.calls.at(-1)?.[1]).toBe(4000);

    await act(async () => {
      await latest?.handleOpen();
      await flushAsyncWork();
    });

    expect(intervalSpy.mock.calls.at(-1)?.[1]).toBe(2000);

    await act(async () => {
      root?.unmount();
    });
  });

  it("opens at the previous episode by default and keeps partial extraction data visible", async () => {
    installDom();
    const jobs: ExtractionJobsResponse = { jobs: [] };
    vi.mocked(fetchExtractionJobs).mockResolvedValue(jobs);
    vi.mocked(fetchTerms).mockResolvedValue(createReadyTerms("2"));
    vi.mocked(fetchCharacterSummary).mockResolvedValue({
      status: "partial",
      novelId: "novel-a",
      upToEpisodeIndex: "2",
      processedUpToEpisodeIndex: "1",
      characters: [],
    });

    let latest: HookResult | null = null;
    let openCalls = 0;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        currentTocEpisodeIndex: 2,
        onRender: (result, _notices, nextOpenCalls) => {
          latest = result;
          openCalls = nextOpenCalls;
        },
      });
      await flushAsyncWork();
    });

    await act(async () => {
      await latest?.handleOpen();
      await flushAsyncWork();
    });

    expect(openCalls).toBe(1);
    expect(fetchCharacterSummary).toHaveBeenCalledWith("novel-a", "2");
    expect(fetchCharacterSummary).toHaveBeenCalledTimes(1);
    expect(latest?.data?.status).toBe("partial");
    expect(latest?.notice).toContain(
      "第1話時点までの生成済み人物一覧を表示しています。",
    );

    await act(async () => {
      root?.unmount();
    });
  });

  it("submits a character summary job and refreshes the panel", async () => {
    installDom();
    vi.mocked(fetchExtractionJobs).mockResolvedValue({ jobs: [] });
    vi.mocked(fetchCharacterSummary).mockResolvedValue(createReadySummary("2"));
    vi.mocked(fetchTerms).mockResolvedValue(createReadyTerms("2"));
    vi.mocked(submitExtraction).mockResolvedValue({
      jobId: "job-a",
      generationStrategy: "parallel_identity",
      message: "生成を開始しました。",
      requestedUpToEpisodeIndex: "2",
      status: "queued",
    });

    let latest: HookResult | null = null;
    let latestNotices: string[] = [];
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        currentTocEpisodeIndex: 2,
        onRender: (result, notices) => {
          latest = result;
          latestNotices = notices;
        },
      });
      await flushAsyncWork();
    });

    await act(async () => {
      latest?.setRequestedUpToEpisodeIndex("2");
      await flushAsyncWork();
      await latest?.handleGenerate();
      await flushAsyncWork();
    });

    expect(submitExtraction).toHaveBeenCalledWith("novel-a", {
      generationStrategy: "parallel_identity",
      upToEpisodeIndex: "2",
    });
    expect(fetchCharacterSummary).toHaveBeenCalledWith("novel-a", "2");
    expect(latestNotices).toContain("生成を開始しました。");

    await act(async () => {
      root?.unmount();
    });
  });

  it("reports a partial term boundary without replacing it with another request", async () => {
    installDom();
    vi.mocked(fetchExtractionJobs).mockResolvedValue({ jobs: [] });
    vi.mocked(fetchCharacterSummary).mockResolvedValue(createReadySummary("2"));
    vi.mocked(fetchTerms).mockResolvedValue({
      status: "partial",
      novelId: "novel-a",
      upToEpisodeIndex: "2",
      processedUpToEpisodeIndex: "1",
      terms: [],
    });

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        currentTocEpisodeIndex: 2,
        onRender: (result) => {
          latest = result;
        },
      });
      await flushAsyncWork();
    });
    await act(async () => {
      await latest?.handleOpen();
      await flushAsyncWork();
    });

    expect(latest?.data?.upToEpisodeIndex).toBe("2");
    expect(latest?.termsData?.upToEpisodeIndex).toBe("2");
    expect(latest?.termsData?.status).toBe("partial");
    expect(latest?.notice).toContain(
      "第1話時点までの生成済み用語一覧を表示しています。",
    );
    expect(vi.mocked(fetchTerms).mock.calls.every(([, episodeIndex]) => episodeIndex === "2")).toBe(true);
    await act(async () => root?.unmount());
  });

  it("shows an actionable notice for legacy character-only state", async () => {
    installDom();
    vi.mocked(fetchExtractionJobs).mockResolvedValue({ jobs: [] });
    vi.mocked(fetchCharacterSummary).mockResolvedValue(createReadySummary("2"));
    vi.mocked(fetchTerms).mockResolvedValue({
      status: "not_generated",
      novelId: "novel-a",
      upToEpisodeIndex: "2",
      processedUpToEpisodeIndex: null,
      terms: [],
    });

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        currentTocEpisodeIndex: 2,
        onRender: (result) => {
          latest = result;
        },
      });
      await flushAsyncWork();
    });
    await act(async () => {
      await latest?.handleOpen();
      await flushAsyncWork();
    });
    expect(latest?.notice).toContain("旧生成データには用語が含まれないため");
    expect(latest?.canClear).toBe(true);
    await act(async () => root?.unmount());
  });

  it("switches the extraction boundary between the current and previous episode", async () => {
    installDom();
    vi.mocked(fetchExtractionJobs).mockResolvedValue({ jobs: [] });
    vi.mocked(fetchCharacterSummary).mockImplementation(async (_novelId, episodeIndex) => createReadySummary(episodeIndex));
    vi.mocked(fetchTerms).mockImplementation(async (_novelId, episodeIndex) => createReadyTerms(episodeIndex));

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        currentTocEpisodeIndex: 2,
        onRender: (result) => {
          latest = result;
        },
      });
      await flushAsyncWork();
    });

    expect(latest?.includeCurrentEpisode).toBe(false);
    expect(latest?.defaultUpToEpisodeIndex).toBe("2");
    await act(async () => {
      latest?.setIncludeCurrentEpisode(true);
      await flushAsyncWork();
    });
    expect(latest?.includeCurrentEpisode).toBe(true);
    expect(latest?.defaultUpToEpisodeIndex).toBe("3");
    expect(latest?.requestedUpToEpisodeIndex).toBe("3");
    expect(fetchCharacterSummary).toHaveBeenCalledWith("novel-a", "3");

    await act(async () => root?.unmount());
  });

  it("does not write an old generation result after switching away and back", async () => {
    installDom();
    const submitA = deferred<Awaited<ReturnType<typeof submitExtraction>>>();
    vi.mocked(submitExtraction).mockReturnValue(submitA.promise);
    vi.mocked(fetchExtractionJobs).mockResolvedValue({ jobs: [] });
    vi.mocked(fetchCharacterSummary).mockImplementation(async (novelId, episodeIndex) => ({
      ...createReadySummary(episodeIndex),
      novelId,
    }));
    vi.mocked(fetchTerms).mockImplementation(async (novelId, episodeIndex) => ({
      ...createReadyTerms(episodeIndex),
      novelId,
    }));

    let latest: HookResult | null = null;
    const notices: string[] = [];
    const harness = renderSwitchableHookHarness((result) => {
      latest = result;
    }, notices);
    await act(async () => {
      harness.render("novel-a");
      await flushAsyncWork();
      latest?.setRequestedUpToEpisodeIndex("2");
      await flushAsyncWork();
    });
    let oldOperation!: Promise<void>;
    await act(async () => {
      oldOperation = latest?.handleGenerate() ?? Promise.resolve();
      await flushAsyncWork();
    });
    await act(async () => {
      harness.render("novel-b");
      await flushAsyncWork();
    });
    await act(async () => {
      harness.render("novel-a");
      await flushAsyncWork();
    });
    await act(async () => {
      await latest?.handleOpen();
      await flushAsyncWork();
    });
    const refreshCallsBeforeOldResult = vi.mocked(fetchCharacterSummary).mock.calls.length;
    await act(async () => {
      submitA.resolve({
        jobId: "old-job-a",
        generationStrategy: "parallel_identity",
        message: "古い生成結果",
        requestedUpToEpisodeIndex: "2",
        status: "queued",
      });
      await oldOperation;
      await flushAsyncWork();
    });

    expect(latest?.data?.novelId).toBe("novel-a");
    expect(vi.mocked(fetchCharacterSummary)).toHaveBeenCalledTimes(refreshCallsBeforeOldResult);
    expect(notices).not.toContain("古い生成結果");
    expect(latest?.isSubmitting).toBe(false);
    await act(async () => harness.root.unmount());
  });

  it("does not write an old clear result into the newly selected novel", async () => {
    installDom();
    const clearA = deferred<Awaited<ReturnType<typeof clearExtraction>>>();
    vi.mocked(clearExtraction).mockReturnValue(clearA.promise);
    vi.mocked(fetchExtractionJobs).mockResolvedValue({ jobs: [] });
    vi.mocked(fetchCharacterSummary).mockImplementation(async (novelId, episodeIndex) => ({
      ...createReadySummary(episodeIndex),
      novelId,
    }));
    vi.mocked(fetchTerms).mockImplementation(async (novelId, episodeIndex) => ({
      ...createReadyTerms(episodeIndex),
      novelId,
    }));

    let latest: HookResult | null = null;
    const notices: string[] = [];
    const harness = renderSwitchableHookHarness((result) => {
      latest = result;
    }, notices);
    await act(async () => {
      harness.render("novel-a");
      await flushAsyncWork();
      await latest?.handleOpen();
      await flushAsyncWork();
    });
    let oldOperation!: Promise<void>;
    await act(async () => {
      oldOperation = latest?.handleClear() ?? Promise.resolve();
      await flushAsyncWork();
    });
    await act(async () => {
      harness.render("novel-b");
      await flushAsyncWork();
    });
    await act(async () => {
      await latest?.handleOpen();
      await flushAsyncWork();
    });
    const refreshCallsBeforeOldResult = vi.mocked(fetchCharacterSummary).mock.calls.length;
    await act(async () => {
      clearA.resolve({
        message: "古いクリア結果",
        characterProfileDeleted: true,
        characterEventsDeleted: true,
        termProfileDeleted: true,
        extractionJobsDeleted: 1,
        extractionJobIndexDeleted: true,
        extractionCheckpointsDeleted: 0,
      });
      await oldOperation;
      await flushAsyncWork();
    });

    expect(latest?.data?.novelId).toBe("novel-b");
    expect(vi.mocked(fetchCharacterSummary)).toHaveBeenCalledTimes(refreshCallsBeforeOldResult);
    expect(notices).not.toContain("古いクリア結果");
    expect(latest?.isClearing).toBe(false);
    await act(async () => harness.root.unmount());
  });
});
