import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import { fetchCharacterJobs, fetchCharacterSummary, submitCharacterJob } from "../src/features/characters/api";
import type { CharacterJobsResponse, CharacterSummaryResponse } from "../src/features/characters/types";
import type { TocResponse } from "../src/features/reader/types";
import { useCharacterSummary } from "../src/hooks/useCharacterSummary";

vi.mock("../src/features/characters/api", () => ({
  fetchCharacterJobs: vi.fn(),
  fetchCharacterSummary: vi.fn(),
  submitCharacterJob: vi.fn()
}));

type HookResult = ReturnType<typeof useCharacterSummary>;

function installDom(): JSDOM {
  const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
    url: "http://localhost/"
  });

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
        bodyStatus: "complete"
      },
      {
        episodeIndex: "2",
        title: "二",
        chapter: null,
        subchapter: null,
        sourceUrl: null,
        updatedAt: null,
        contentEtag: "toc-2",
        bodyStatus: "complete"
      }
    ]
  };
}

function createReadySummary(upToEpisodeIndex: string): CharacterSummaryResponse {
  return {
    status: "ready",
    novelId: "novel-a",
    upToEpisodeIndex,
    processedUpToEpisodeIndex: upToEpisodeIndex,
    characters: []
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
    const result = useCharacterSummary({
      currentTocEpisodeIndex: props.currentTocEpisodeIndex ?? 1,
      formatEpisodeOrderLabel: (episodeIndex) => episodeIndex,
      isOpen: true,
      onClosePanel: vi.fn(),
      onOpenPanel: () => {
        openCalls += 1;
      },
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
      toc: createToc()
    });
    props.onRender(result, notices, openCalls);
    return null;
  }

  const root = createRoot(rootElement);
  root.render(createElement(Harness));
  return root;
}

describe("useCharacterSummary", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("opens with fallback summary notice when target is not generated yet", async () => {
    installDom();
    const jobs: CharacterJobsResponse = { jobs: [] };
    vi.mocked(fetchCharacterJobs).mockResolvedValue(jobs);
    vi.mocked(fetchCharacterSummary)
      .mockResolvedValueOnce({
        status: "not_generated",
        novelId: "novel-a",
        upToEpisodeIndex: "2",
        processedUpToEpisodeIndex: "1",
        characters: []
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
        }
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
    expect(latest?.notice).toContain("第1話時点までの生成済み一覧を表示しています。");

    await act(async () => {
      root?.unmount();
    });
  });

  it("submits a character summary job and refreshes the panel", async () => {
    installDom();
    vi.mocked(fetchCharacterJobs).mockResolvedValue({ jobs: [] });
    vi.mocked(fetchCharacterSummary).mockResolvedValue(createReadySummary("2"));
    vi.mocked(submitCharacterJob).mockResolvedValue({
      jobId: "job-a",
      generationStrategy: "parallel_identity",
      message: "生成を開始しました。",
      requestedUpToEpisodeIndex: "2",
      status: "queued"
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
        }
      });
      await flushAsyncWork();
    });

    await act(async () => {
      latest?.setRequestedUpToEpisodeIndex("2");
      await flushAsyncWork();
      await latest?.handleGenerate();
      await flushAsyncWork();
    });

    expect(submitCharacterJob).toHaveBeenCalledWith("novel-a", {
      generationStrategy: "parallel_identity",
      upToEpisodeIndex: "2"
    });
    expect(fetchCharacterSummary).toHaveBeenCalledWith("novel-a", "2");
    expect(latestNotices).toContain("生成を開始しました。");

    await act(async () => {
      root?.unmount();
    });
  });
});
