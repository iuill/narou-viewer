import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import type { FetcherStatusSnapshot } from "../src/features/fetcher/types";
import { fetchFetcherStatus } from "../src/features/fetcher/api";
import { useFetcherStatus } from "../src/hooks/useFetcherStatus";

vi.mock("../src/features/fetcher/api", () => ({
  fetchFetcherStatus: vi.fn()
}));

type HookResult = ReturnType<typeof useFetcherStatus>;

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

  return dom;
}

async function flushAsyncWork(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
}

function renderHookHarness(props: {
  refreshKey?: unknown;
  isPaused?: boolean;
  onQueueSettled?: () => void;
  onRender: (result: HookResult) => void;
}): Root {
  const rootElement = document.getElementById("root");
  if (!rootElement) {
    throw new Error("root element is missing");
  }

  function Harness() {
    const result = useFetcherStatus({
      isPaused: props.isPaused ?? false,
      refreshKey: props.refreshKey ?? 0,
      onQueueSettled: props.onQueueSettled ?? vi.fn()
    });
    props.onRender(result);
    return null;
  }

  const root = createRoot(rootElement);
  root.render(createElement(Harness));
  return root;
}

function createStatus(overrides: Partial<FetcherStatusSnapshot> = {}): FetcherStatusSnapshot {
  return {
    queue: {
      total: 0,
      webWorker: 0,
      worker: 0,
      running: false
    },
    tasks: {
      current: null,
      queued: [],
      recentCompleted: [],
      recentFailed: [],
      completedCount: 0,
      failedCount: 0,
      convertCurrent: null,
      convertQueued: []
    },
    error: null,
    didUpdate: true,
    ...overrides
  };
}

describe("useFetcherStatus", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("keeps partially successful status responses", async () => {
    installDom();
    vi.mocked(fetchFetcherStatus).mockResolvedValueOnce(
      createStatus({
        queue: {
          total: 1,
          webWorker: 0,
          worker: 1,
          running: true
        },
        tasks: null,
        error: "タスク状態の取得に失敗しました。"
      })
    );

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    expect(latest?.queue?.running).toBe(true);
    expect(latest?.tasks).toBeNull();
    expect(latest?.error).toBe("タスク状態の取得に失敗しました。");
    expect(latest?.checkedAt).toEqual(expect.any(String));
    expect(latest?.isLoading).toBe(false);

    await act(async () => {
      root?.unmount();
    });
  });

  it("notifies when active fetcher work settles", async () => {
    installDom();
    vi.useFakeTimers();
    const onQueueSettled = vi.fn();
    vi.mocked(fetchFetcherStatus)
      .mockResolvedValueOnce(
        createStatus({
          queue: {
            total: 1,
            webWorker: 0,
            worker: 1,
            running: true
          },
          tasks: {
            current: {
              id: "task-1",
              type: "download",
              status: "running"
            },
            queued: [],
            recentCompleted: [],
            recentFailed: [],
            completedCount: 0,
            failedCount: 0,
            convertCurrent: null,
            convertQueued: []
          }
        })
      )
      .mockResolvedValueOnce(
        createStatus({
          queue: {
            total: 0,
            webWorker: 0,
            worker: 0,
            running: false
          },
          tasks: {
            current: null,
            queued: [],
            recentCompleted: [],
            recentFailed: [],
            completedCount: 1,
            failedCount: 0,
            convertCurrent: null,
            convertQueued: []
          }
        })
      );

    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onQueueSettled,
        onRender: () => {}
      });
      await flushAsyncWork();
    });

    expect(onQueueSettled).not.toHaveBeenCalled();

    await act(async () => {
      vi.advanceTimersByTime(2000);
      await flushAsyncWork();
    });

    expect(onQueueSettled).toHaveBeenCalledTimes(1);

    await act(async () => {
      root?.unmount();
    });
  });

  it("applies slow status responses instead of starting overlapping polls", async () => {
    installDom();
    vi.useFakeTimers();
    const releaseStatusResponses: Array<(status: FetcherStatusSnapshot) => void> = [];
    vi.mocked(fetchFetcherStatus).mockImplementation(
      () =>
        new Promise<FetcherStatusSnapshot>((resolve) => {
          releaseStatusResponses.push(resolve);
        })
    );

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    await act(async () => {
      vi.advanceTimersByTime(4000);
      await flushAsyncWork();
    });
    expect(releaseStatusResponses.length).toBeGreaterThan(0);
    expect(latest?.tasks).toBeNull();

    await act(async () => {
      releaseStatusResponses[0]?.(
        createStatus({
          tasks: {
            current: {
              id: "update-task-1",
              type: "update",
              novelIds: ["101"],
              status: "running"
            },
            queued: [],
            recentCompleted: [],
            recentFailed: [],
            completedCount: 0,
            failedCount: 0,
            convertCurrent: null,
            convertQueued: []
          }
        })
      );
      await flushAsyncWork();
    });
    expect(latest?.tasks?.current?.id).toBe("update-task-1");

    await act(async () => {
      root?.unmount();
    });
  });
});
