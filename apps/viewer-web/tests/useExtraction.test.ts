import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import { fetchCharacterSummary } from "../src/features/characters/api";
import type { CharacterSummaryResponse } from "../src/features/characters/types";
import {
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
    ],
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
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("opens with fallback summary notice when target is not generated yet", async () => {
    installDom();
    const jobs: ExtractionJobsResponse = { jobs: [] };
    vi.mocked(fetchExtractionJobs).mockResolvedValue(jobs);
    vi.mocked(fetchTerms).mockResolvedValue(createReadyTerms("2"));
    vi.mocked(fetchCharacterSummary)
      .mockResolvedValueOnce({
        status: "not_generated",
        novelId: "novel-a",
        upToEpisodeIndex: "2",
        processedUpToEpisodeIndex: "1",
        characters: [],
      })
      .mockResolvedValueOnce(createReadySummary("1"));

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
    expect(fetchCharacterSummary).toHaveBeenNthCalledWith(1, "novel-a", "2");
    expect(fetchCharacterSummary).toHaveBeenNthCalledWith(2, "novel-a", "1");
    expect(latest?.data?.status).toBe("ready");
    expect(latest?.notice).toContain(
      "第1話時点までの生成済み一覧を表示しています。",
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

  it("resolves character and term fallbacks independently", async () => {
    installDom();
    vi.mocked(fetchExtractionJobs).mockResolvedValue({ jobs: [] });
    vi.mocked(fetchCharacterSummary).mockResolvedValue(createReadySummary("2"));
    vi.mocked(fetchTerms)
      .mockResolvedValueOnce({
        status: "not_generated",
        novelId: "novel-a",
        upToEpisodeIndex: "2",
        processedUpToEpisodeIndex: "1",
        terms: [],
      })
      .mockResolvedValueOnce(createReadyTerms("1"));

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
    expect(latest?.termsData?.upToEpisodeIndex).toBe("1");
    expect(latest?.notice).toContain(
      "第2話時点の用語一覧はまだ生成されていません。",
    );
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
});
