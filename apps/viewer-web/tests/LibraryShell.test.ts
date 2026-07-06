import { act, createElement, type ReactNode, useRef, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { LibraryShellProps } from "../src/screens/LibraryShell";
import { LibraryShell } from "../src/screens/LibraryShell";
import { QueuePanel } from "../src/screens/library/QueuePanel";

vi.mock("../src/LibraryScreen", () => ({
  LibraryScreen: ({ mobileStatusPanel }: { mobileStatusPanel: ReactNode }) =>
    createElement("section", { "data-component": "library-screen" }, mobileStatusPanel)
}));

function installDom() {
  const dom = new JSDOM('<!doctype html><html><body><div id="root"></div></body></html>', {
    url: "http://localhost/"
  });
  vi.stubGlobal("window", dom.window);
  vi.stubGlobal("document", dom.window.document);
  vi.stubGlobal("navigator", dom.window.navigator);
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  return dom;
}

function createBaseProps(overrides: Partial<LibraryShellProps> = {}): LibraryShellProps {
  return {
    aiGeneration: {
      activeJobsCount: 0,
      activeSettingsProfileUpdatedAt: null,
      failedJobsCount: 0,
      isOpen: false,
      jobsError: null,
      onOpenView: vi.fn(),
      panelRef: { current: null },
      runtimeErrorDetail: null,
      setIsOpen: vi.fn(),
      settingsError: null,
      summaryLabel: "AI OK",
      triggerStatus: "ok"
    },
    isMobileLibraryViewport: false,
    libraryScreenProps: {} as LibraryShellProps["libraryScreenProps"],
    queue: {
      currentFetcherTask: null,
      fetcherQueue: { available: true, running: false, total: 0, webWorker: 0, worker: 0 },
      fetcherStatusCheckedAt: null,
      fetcherStatusError: null,
      fetcherTasksFailedCount: 0,
      fetcherUpdateNotice: null,
      hasActiveFetcherTasks: false,
      hasFetcherStatus: true,
      isOpen: false,
      onScrollToQueueProgress: vi.fn(),
      panelRef: { current: null },
      queuedTaskPreviewEntries: [],
      queueStatusLabel: "待機中",
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
        checkedAt: "2026-01-01T00:00:00Z",
        services: [],
        status: "ok"
      },
      runtimeStatusLabel: "正常",
      setIsOpen: vi.fn(),
      viewerBuildCommitDate: "2026-01-01",
      viewerBuildSummary: "test-build"
    },
    ...overrides
  };
}

function StatefulLibraryShell() {
  const [isStatusOpen, setIsStatusOpen] = useState(false);
  const [isQueueOpen, setIsQueueOpen] = useState(false);
  const [isAiOpen, setIsAiOpen] = useState(false);
  const statusRef = useRef<HTMLDivElement | null>(null);
  const queueRef = useRef<HTMLDivElement | null>(null);
  const aiRef = useRef<HTMLDivElement | null>(null);
  const props = createBaseProps({
    aiGeneration: {
      ...createBaseProps().aiGeneration,
      isOpen: isAiOpen,
      panelRef: aiRef,
      setIsOpen: setIsAiOpen
    },
    queue: {
      ...createBaseProps().queue,
      isOpen: isQueueOpen,
      panelRef: queueRef,
      setIsOpen: setIsQueueOpen
    },
    status: {
      ...createBaseProps().status,
      isOpen: isStatusOpen,
      panelRef: statusRef,
      setIsOpen: setIsStatusOpen
    }
  });

  return createElement(LibraryShell, props);
}

describe("LibraryShell", () => {
  let root: Root | null = null;

  afterEach(async () => {
    await act(async () => {
      root?.unmount();
      await Promise.resolve();
    });
    root = null;
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("keeps the status, queue, and AI popovers mutually exclusive", async () => {
    const dom = installDom();
    const rootElement = dom.window.document.getElementById("root");
    if (!rootElement) {
      throw new Error("root element not found");
    }
    root = createRoot(rootElement);

    await act(async () => {
      root?.render(createElement(StatefulLibraryShell));
    });

    const statusButton = dom.window.document.querySelector<HTMLButtonElement>(".hero-status-group .status-trigger");
    const queueButton = dom.window.document.querySelector<HTMLButtonElement>(".queue-panel .status-trigger");
    const aiButton = dom.window.document.querySelector<HTMLButtonElement>(".ai-generation-panel .status-trigger");
    expect(statusButton).toBeTruthy();
    expect(queueButton).toBeTruthy();
    expect(aiButton).toBeTruthy();

    await act(async () => {
      statusButton?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
    });
    expect(dom.window.document.querySelector(".status-popover")).toBeTruthy();
    expect(dom.window.document.querySelector(".queue-popover")).toBeNull();

    await act(async () => {
      queueButton?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
    });
    expect(dom.window.document.querySelector(".status-popover")).toBeNull();
    expect(dom.window.document.querySelector(".queue-popover:not(.ai-generation-popover)")).toBeTruthy();
    expect(dom.window.document.querySelector(".ai-generation-popover")).toBeNull();

    await act(async () => {
      aiButton?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
    });
    expect(dom.window.document.querySelector(".status-popover")).toBeNull();
    expect(dom.window.document.querySelector(".queue-popover:not(.ai-generation-popover)")).toBeNull();
    expect(dom.window.document.querySelector(".ai-generation-popover")).toBeTruthy();
  });

  it("renders queue detail branches for empty, active, and failed tasks", async () => {
    const dom = installDom();
    const rootElement = dom.window.document.getElementById("root");
    if (!rootElement) {
      throw new Error("root element not found");
    }
    const onScrollToQueueProgress = vi.fn();
    root = createRoot(rootElement);

    await act(async () => {
      root?.render(
        createElement(QueuePanel, {
          formatDate: (value: string | null) => value ?? "確認中",
          onToggle: vi.fn(),
          queue: {
            currentFetcherTask: {
              completedAt: null,
              currentStep: 1,
              elapsedTime: null,
              errorMessage: null,
              failedEpisodeId: null,
              id: "current",
              novelAuthor: null,
              novelId: null,
              novelIds: [],
              novelTitle: null,
              message: "取得中",
              progress: 50,
              resumeEpisodeId: null,
              savedEpisodeCount: null,
              startedAt: null,
              status: "running",
              target: "作品",
              totalSteps: 2,
              type: "download",
              warnings: []
            },
            fetcherQueue: { available: true, running: true, total: 2, webWorker: 0, worker: 2 },
            fetcherStatusCheckedAt: "2026-07-01T00:00:00.000Z",
            fetcherStatusError: "一部取得失敗",
            fetcherTasksFailedCount: 1,
            fetcherUpdateNotice: null,
            hasActiveFetcherTasks: true,
            hasFetcherStatus: true,
            isOpen: true,
            onScrollToQueueProgress,
            panelRef: { current: null },
            queuedTaskPreviewEntries: [
              {
                key: "queued",
                task: {
                  completedAt: null,
                  currentStep: null,
                  elapsedTime: null,
                  errorMessage: null,
                  failedEpisodeId: null,
                  id: "queued",
                  message: null,
                  novelAuthor: null,
                  novelId: null,
                  novelIds: ["1"],
                  novelTitle: null,
                  progress: null,
                  resumeEpisodeId: null,
                  savedEpisodeCount: null,
                  startedAt: null,
                  status: "queued",
                  target: "待機作品",
                  totalSteps: null,
                  type: "resume",
                  warnings: []
                }
              }
            ],
            queueStatusLabel: "稼働中",
            recentFailedFetcherTaskPreviewEntries: [
              {
                key: "failed",
                task: {
                  completedAt: null,
                  currentStep: null,
                  elapsedTime: null,
                  errorMessage: "boom",
                  failedEpisodeId: "2",
                  id: "failed",
                  message: null,
                  novelAuthor: null,
                  novelId: null,
                  novelIds: [],
                  novelTitle: null,
                  progress: null,
                  resumeEpisodeId: "2",
                  savedEpisodeCount: null,
                  startedAt: null,
                  status: "failed",
                  target: "失敗作品",
                  totalSteps: null,
                  type: "update",
                  warnings: []
                }
              }
            ],
            setIsOpen: vi.fn()
          }
        })
      );
    });

    expect(rootElement.textContent).toContain("一部取得失敗");
    expect(rootElement.textContent).toContain("作品");
    expect(rootElement.textContent).toContain("待機作品");
    expect(rootElement.textContent).toContain("失敗作品");

    const detailButton = rootElement.querySelector<HTMLButtonElement>(".queue-detail-button");
    await act(async () => {
      detailButton?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
    });
    expect(onScrollToQueueProgress).toHaveBeenCalled();
  });
});
