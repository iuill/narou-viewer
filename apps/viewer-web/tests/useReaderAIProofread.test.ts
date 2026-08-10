import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import {
  deleteReaderAIProofread,
  fetchReaderAIProofread,
  generateReaderAIProofread
} from "../src/features/reader/api";
import type { EpisodeResponse, ReaderAIProofreadResponse } from "../src/features/reader/types";
import { useReaderAIProofread } from "../src/hooks/useReaderAIProofread";

vi.mock("../src/features/reader/api", () => ({
  deleteReaderAIProofread: vi.fn(),
  fetchReaderAIProofread: vi.fn(),
  generateReaderAIProofread: vi.fn()
}));

type HookResult = ReturnType<typeof useReaderAIProofread>;

function installDom(): void {
  const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
    url: "http://localhost/"
  });
  vi.stubGlobal("window", dom.window);
  vi.stubGlobal("document", dom.window.document);
  vi.stubGlobal("navigator", dom.window.navigator);
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
}

async function flushAsyncWork(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
}

function createEpisode(episodeIndex = "1"): EpisodeResponse {
  return {
    novelId: "novel-a",
    episodeIndex,
    title: `第${episodeIndex}話`,
    chapter: null,
    subchapter: null,
    html: "",
    readerDocument: {
      version: 1,
      blocks: [{ type: "paragraph", section: "body", inlines: [{ type: "text", text: "原文です。" }] }]
    },
    plainTextLength: 5,
    updatedAt: "2026-07-30T00:00:00Z",
    contentEtag: `etag-${episodeIndex}`
  };
}

function createReadyResult(episode = createEpisode()): ReaderAIProofreadResponse {
  return {
    status: "ready",
    novelId: episode.novelId,
    episodeIndex: episode.episodeIndex,
    sourceEtag: episode.contentEtag,
    generatedAt: "2026-07-30T00:01:00Z",
    modelId: "synthetic-model",
    readerDocument: {
      version: 1,
      blocks: [{ type: "paragraph", section: "body", inlines: [{ type: "text", text: "校正版です。" }] }]
    }
  };
}

function createTestRoot(): Root {
  const rootElement = document.getElementById("root");
  if (!rootElement) {
    throw new Error("root element is missing");
  }
  return createRoot(rootElement);
}

function HookHarness({
  episode,
  onError,
  onRender
}: {
  episode: EpisodeResponse | null;
  onError: (message: string | null) => void;
  onRender: (result: HookResult) => void;
}) {
  const result = useReaderAIProofread(episode, onError);
  onRender(result);
  return null;
}

function renderHarness(
  root: Root,
  episode: EpisodeResponse | null,
  onError: (message: string | null) => void,
  onRender: (result: HookResult) => void
): void {
  root.render(createElement(HookHarness, { episode, onError, onRender }));
}

describe("useReaderAIProofread", () => {
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

  it("stays ungenerated without an episode and ignores generation commands", async () => {
    installDom();
    const testRoot = createTestRoot();
    root = testRoot;
    let latest: HookResult | null = null;
    const onError = vi.fn();

    await act(async () => {
      renderHarness(testRoot, null, onError, (result) => {
        latest = result;
      });
      await flushAsyncWork();
    });
    expect(latest?.state).toBe("not_generated");
    expect(latest?.displayedEpisode).toBeNull();

    await act(async () => {
      await latest?.generate();
      await latest?.remove();
    });
    expect(generateReaderAIProofread).not.toHaveBeenCalled();
    expect(deleteReaderAIProofread).not.toHaveBeenCalled();
  });

  it("loads, displays, and removes a saved proofread document", async () => {
    installDom();
    const episode = createEpisode();
    const ready = createReadyResult(episode);
    vi.mocked(fetchReaderAIProofread).mockResolvedValue(ready);
    vi.mocked(deleteReaderAIProofread).mockResolvedValue();
    const testRoot = createTestRoot();
    root = testRoot;
    let latest: HookResult | null = null;
    const onError = vi.fn();

    await act(async () => {
      renderHarness(testRoot, episode, onError, (result) => {
        latest = result;
      });
      await flushAsyncWork();
    });
    expect(latest?.state).toBe("ready");
    expect(latest?.displayedEpisode).toBe(episode);

    await act(async () => {
      latest?.setIsShowingProofread(true);
      await flushAsyncWork();
    });
    expect(latest?.isShowingProofread).toBe(true);
    expect(latest?.displayedEpisode?.readerDocument).toBe(ready.readerDocument);

    const episodeWithUpdatedCorrections = { ...episode, contentEtag: `${episode.contentEtag}-corrections` };
    await act(async () => {
      renderHarness(testRoot, episodeWithUpdatedCorrections, onError, (result) => {
        latest = result;
      });
      await flushAsyncWork();
    });
    expect(latest?.isShowingProofread).toBe(true);
    expect(latest?.displayedEpisode?.readerDocument).toBe(ready.readerDocument);

    await act(async () => {
      await latest?.remove();
      await flushAsyncWork();
    });
    expect(deleteReaderAIProofread).toHaveBeenCalledWith("novel-a", "1");
    expect(latest?.state).toBe("not_generated");
    expect(latest?.result).toBeNull();
    expect(latest?.isShowingProofread).toBe(false);
  });

  it("generates a proofread document and prevents duplicate generation", async () => {
    installDom();
    const episode = createEpisode();
    const ready = createReadyResult(episode);
    vi.mocked(fetchReaderAIProofread).mockResolvedValue({
      status: "not_generated",
      novelId: episode.novelId,
      episodeIndex: episode.episodeIndex,
      sourceEtag: episode.contentEtag,
      generatedAt: null,
      modelId: null
    });
    let resolveGeneration: ((value: ReaderAIProofreadResponse) => void) | null = null;
    vi.mocked(generateReaderAIProofread).mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveGeneration = resolve;
        })
    );
    const testRoot = createTestRoot();
    root = testRoot;
    let latest: HookResult | null = null;
    const onError = vi.fn();

    await act(async () => {
      renderHarness(testRoot, episode, onError, (result) => {
        latest = result;
      });
      await flushAsyncWork();
    });
    let firstGeneration: Promise<void> | undefined;
    await act(async () => {
      firstGeneration = latest?.generate();
      await flushAsyncWork();
    });
    expect(latest?.state).toBe("generating");

    await act(async () => {
      await latest?.generate();
      await latest?.remove();
    });
    expect(generateReaderAIProofread).toHaveBeenCalledTimes(1);
    expect(deleteReaderAIProofread).not.toHaveBeenCalled();

    await act(async () => {
      resolveGeneration?.(ready);
      await firstGeneration;
      await flushAsyncWork();
    });
    expect(latest?.state).toBe("ready");
    expect(latest?.result).toBe(ready);
    expect(latest?.isShowingProofread).toBe(true);
  });

  it("reports load, generation, and deletion failures", async () => {
    installDom();
    const episode = createEpisode();
    const onError = vi.fn();
    vi.mocked(fetchReaderAIProofread).mockRejectedValue("load failed");
    const testRoot = createTestRoot();
    root = testRoot;
    let latest: HookResult | null = null;

    await act(async () => {
      renderHarness(testRoot, episode, onError, (result) => {
        latest = result;
      });
      await flushAsyncWork();
    });
    expect(latest?.state).toBe("error");
    expect(onError).toHaveBeenLastCalledWith("AI校正結果の取得に失敗しました。");

    vi.mocked(generateReaderAIProofread).mockRejectedValue(new Error("generate failed"));
    await act(async () => {
      await latest?.generate();
      await flushAsyncWork();
    });
    expect(latest?.state).toBe("error");
    expect(onError).toHaveBeenLastCalledWith("generate failed");

    vi.mocked(generateReaderAIProofread).mockRejectedValue("generate failed");
    await act(async () => {
      await latest?.generate();
      await flushAsyncWork();
    });
    expect(onError).toHaveBeenLastCalledWith("AI校正に失敗しました。");

    vi.mocked(deleteReaderAIProofread).mockRejectedValue("delete failed");
    await act(async () => {
      await latest?.remove();
      await flushAsyncWork();
    });
    expect(latest?.state).toBe("error");
    expect(onError).toHaveBeenLastCalledWith("AI校正版の削除に失敗しました。");

    vi.mocked(deleteReaderAIProofread).mockRejectedValue(new Error("delete failed"));
    await act(async () => {
      await latest?.remove();
      await flushAsyncWork();
    });
    expect(onError).toHaveBeenLastCalledWith("delete failed");
  });

  it("ignores a load result after the episode changes", async () => {
    installDom();
    const firstEpisode = createEpisode("1");
    const secondEpisode = createEpisode("2");
    let resolveFirst: ((value: ReaderAIProofreadResponse) => void) | null = null;
    vi.mocked(fetchReaderAIProofread)
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolveFirst = resolve;
          })
      )
      .mockResolvedValueOnce({
        status: "not_generated",
        novelId: secondEpisode.novelId,
        episodeIndex: secondEpisode.episodeIndex,
        sourceEtag: secondEpisode.contentEtag,
        generatedAt: null,
        modelId: null
      });
    const testRoot = createTestRoot();
    root = testRoot;
    let latest: HookResult | null = null;
    const onError = vi.fn();

    await act(async () => {
      renderHarness(testRoot, firstEpisode, onError, (result) => {
        latest = result;
      });
      await flushAsyncWork();
    });
    await act(async () => {
      renderHarness(testRoot, secondEpisode, onError, (result) => {
        latest = result;
      });
      await flushAsyncWork();
      resolveFirst?.(createReadyResult(firstEpisode));
      await flushAsyncWork();
    });
    expect(latest?.state).toBe("not_generated");
    expect(latest?.result?.episodeIndex).toBe("2");
    expect(latest?.isShowingProofread).toBe(false);
  });

  it("ignores a generation result after the episode changes", async () => {
    installDom();
    const firstEpisode = createEpisode("1");
    const secondEpisode = createEpisode("2");
    let resolveGeneration: ((value: ReaderAIProofreadResponse) => void) | null = null;
    vi.mocked(fetchReaderAIProofread).mockResolvedValue({
      status: "not_generated",
      novelId: firstEpisode.novelId,
      episodeIndex: firstEpisode.episodeIndex,
      sourceEtag: firstEpisode.contentEtag,
      generatedAt: null,
      modelId: null
    });
    vi.mocked(generateReaderAIProofread).mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveGeneration = resolve;
        })
    );
    const testRoot = createTestRoot();
    root = testRoot;
    let latest: HookResult | null = null;
    const onError = vi.fn();

    await act(async () => {
      renderHarness(testRoot, firstEpisode, onError, (result) => {
        latest = result;
      });
      await flushAsyncWork();
    });
    let generation: Promise<void> | undefined;
    await act(async () => {
      generation = latest?.generate();
      await flushAsyncWork();
      renderHarness(testRoot, secondEpisode, onError, (result) => {
        latest = result;
      });
      await flushAsyncWork();
      resolveGeneration?.(createReadyResult(firstEpisode));
      await generation;
      await flushAsyncWork();
    });

    expect(latest?.state).toBe("not_generated");
    expect(latest?.displayedEpisode).toBe(secondEpisode);
    expect(latest?.isShowingProofread).toBe(false);
  });

  it("ignores deletion completion after the episode changes", async () => {
    installDom();
    const firstEpisode = createEpisode("1");
    const secondEpisode = createEpisode("2");
    let resolveDeletion: (() => void) | null = null;
    vi.mocked(fetchReaderAIProofread)
      .mockResolvedValueOnce(createReadyResult(firstEpisode))
      .mockResolvedValueOnce(createReadyResult(secondEpisode));
    vi.mocked(deleteReaderAIProofread).mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveDeletion = resolve;
        })
    );
    const testRoot = createTestRoot();
    root = testRoot;
    let latest: HookResult | null = null;
    const onError = vi.fn();

    await act(async () => {
      renderHarness(testRoot, firstEpisode, onError, (result) => {
        latest = result;
      });
      await flushAsyncWork();
    });
    let deletion: Promise<void> | undefined;
    await act(async () => {
      deletion = latest?.remove();
      await flushAsyncWork();
      renderHarness(testRoot, secondEpisode, onError, (result) => {
        latest = result;
      });
      await flushAsyncWork();
      resolveDeletion?.();
      await deletion;
      await flushAsyncWork();
    });

    expect(latest?.state).toBe("ready");
    expect(latest?.result?.episodeIndex).toBe("2");
  });

  it("ignores successful and failed loads after unmounting", async () => {
    installDom();
    const episode = createEpisode();
    const onError = vi.fn();
    let resolveLoad: ((value: ReaderAIProofreadResponse) => void) | null = null;
    vi.mocked(fetchReaderAIProofread).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveLoad = resolve;
        })
    );
    const successRoot = createTestRoot();
    root = successRoot;

    await act(async () => {
      renderHarness(successRoot, episode, onError, () => undefined);
      await flushAsyncWork();
    });
    await act(async () => {
      successRoot.unmount();
      root = null;
      await flushAsyncWork();
    });
    await act(async () => {
      resolveLoad?.(createReadyResult(episode));
      await flushAsyncWork();
    });
    expect(onError).not.toHaveBeenCalled();

    let rejectLoad: ((reason: unknown) => void) | null = null;
    vi.mocked(fetchReaderAIProofread).mockImplementationOnce(
      () =>
        new Promise((_, reject) => {
          rejectLoad = reject;
        })
    );
    const failureRoot = createTestRoot();
    root = failureRoot;
    await act(async () => {
      renderHarness(failureRoot, episode, onError, () => undefined);
      await flushAsyncWork();
    });
    await act(async () => {
      failureRoot.unmount();
      root = null;
      await flushAsyncWork();
    });
    await act(async () => {
      rejectLoad?.(new Error("late failure"));
      await flushAsyncWork();
    });
    expect(onError).not.toHaveBeenCalled();
  });
});
