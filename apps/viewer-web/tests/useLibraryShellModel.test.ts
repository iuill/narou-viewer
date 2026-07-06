import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useLibraryShellModel } from "../src/screens/library/useLibraryShellModel";

type HookResult = ReturnType<typeof useLibraryShellModel>;

function installDom(): JSDOM {
  const dom = new JSDOM('<!doctype html><html><body><div id="root"></div></body></html>', {
    url: "http://localhost/"
  });
  Object.defineProperty(dom.window, "requestAnimationFrame", {
    configurable: true,
    value: (callback: FrameRequestCallback) => callback(0)
  });
  vi.stubGlobal("window", dom.window);
  vi.stubGlobal("document", dom.window.document);
  vi.stubGlobal("navigator", dom.window.navigator);
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  return dom;
}

async function renderHookHarness(input: Parameters<typeof useLibraryShellModel>[0]): Promise<{ model: HookResult; root: Root }> {
  let model: HookResult | null = null;

  function Harness() {
    model = useLibraryShellModel(input);
    return null;
  }

  const rootElement = document.getElementById("root");
  if (!rootElement) {
    throw new Error("root element not found");
  }

  const root = createRoot(rootElement);
  await act(async () => {
    root.render(createElement(Harness));
    await Promise.resolve();
  });

  if (!model) {
    throw new Error("hook result was not rendered");
  }
  return { model, root };
}

function createFetcherModel() {
  return {
    activeFetcherTaskEntries: [],
    activeFetcherTasks: [],
    cancelingFetcherTaskIds: new Set<string>(),
    currentFetcherTask: null,
    downloadForce: false,
    downloadTarget: "",
    fetcherActionBusyNovelIds: new Set<string>(),
    fetcherQueue: { available: true, running: false, total: 0, worker: 0, webWorker: 0 },
    fetcherStatusCheckedAt: "2026-07-01T00:00:00.000Z",
    fetcherStatusError: null,
    fetcherTasks: null,
    fetcherUpdateNotice: null,
    handleCancelFetcherTask: vi.fn(),
    handleDownloadDrop: vi.fn(),
    handleDownloadSubmit: vi.fn(),
    handleExportLibrary: vi.fn(),
    handleRemoveCurrentNovel: vi.fn(),
    handleResumeNovel: vi.fn(),
    handleUpdateCurrentNovel: vi.fn(),
    handleUpdateNovel: vi.fn(),
    hasActiveFetcherTasks: false,
    hasFetcherStatus: true,
    isDownloadComposerOpen: false,
    isDownloadDropActive: false,
    isDownloadSubmitting: false,
    isLibraryExporting: false,
    isNovelActionSubmitting: false,
    queuedTaskPreviewEntries: [],
    queueStatusLabel: "待機中",
    recentFailedFetcherTaskPreviewEntries: [],
    resumableNovels: [],
    setDownloadForce: vi.fn(),
    setDownloadTarget: vi.fn(),
    setIsDownloadComposerOpen: vi.fn(),
    setIsDownloadDropActive: vi.fn(),
    updatableNovels: []
  };
}

function createInput(overrides: Record<string, unknown> = {}) {
  const fetcher = createFetcherModel();
  return {
    activeMobileLibraryPanel: "details",
    aiGeneration: {
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
      workspaceProps: null,
      workspaceRef: { current: null }
    },
    clientUpdateRequired: null,
    currentNovel: { novelId: "novel-a", title: "作品" },
    error: null,
    episodeDisplayLookup: new Map(),
    fetcher,
    filteredNovelsCount: 1,
    googleBooksConfigNotice: null,
    isInitialLoading: false,
    isMobileLibraryViewport: false,
    isNovelLoading: false,
    isShowingAllBookmarks: false,
    isTocStoryExpanded: false,
    libraryFilterQuery: "",
    libraryNotice: null,
    libraryPagination: { currentPage: 1, totalPages: 1, totalItems: 1, startItemNumber: 1, endItemNumber: 1 },
    mobileDetailInitialSection: "episodes",
    mobileHomeTab: "library",
    novelPublications: {
      displayCoverEntryId: null,
      entries: [],
      isLoading: false,
      saveEntry: vi.fn(),
      createEntry: vi.fn(),
      setDisplayCover: vi.fn(),
      savingEntryId: null
    },
    novelsCount: 1,
    onBackToLibrary: vi.fn(),
    onClearLibraryFilter: vi.fn(),
    onDeleteBookmark: vi.fn(),
    onLibraryFilterQueryChange: vi.fn(),
    onLibraryPageChange: vi.fn(),
    onMobileHomeTabChange: vi.fn(),
    onOpenBookmark: vi.fn(),
    onOpenEpisode: vi.fn(),
    onOpenNovelPublications: vi.fn(),
    onSelectNovel: vi.fn(),
    panels: {
      queue: { isOpen: true, panelRef: { current: null }, setIsOpen: vi.fn() },
      status: { isOpen: false, panelRef: { current: null }, setIsOpen: vi.fn() }
    },
    readerBookmarks: {
      bookmarks: [],
      latestBookmark: null,
      pendingBookmarkId: null,
      visibleBookmarks: []
    },
    readerState: null,
    readerToc: {
      isTocStoryTruncated: false,
      lastReadEpisodeIndex: null,
      preferFriendlyEpisodeLabels: false,
      selectedEpisodeIndex: null,
      toc: null,
      tocPagination: { currentPage: 1, totalPages: 1, totalItems: 0, startItemNumber: 0, endItemNumber: 0 },
      tocStoryText: "",
      visibleTocEpisodes: []
    },
    runtimeStatus: { checkedAt: "2026-07-01T00:00:00.000Z", services: [], status: "ok" },
    runtimeStatusLabel: "正常",
    selectedNovelId: "novel-a",
    setIsShowingAllBookmarks: vi.fn(),
    setIsTocStoryExpanded: vi.fn(),
    setMobileHomeTab: vi.fn(),
    setTocPage: vi.fn(),
    viewerBuildCommitDate: "2026-07-01",
    viewerBuildSummary: "build",
    visibleLibraryNovels: [],
    ...overrides
  } as Parameters<typeof useLibraryShellModel>[0] & { fetcher: ReturnType<typeof createFetcherModel> };
}

describe("useLibraryShellModel", () => {
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

  it("assembles desktop props and scrolls queue progress from the status popover action", async () => {
    const dom = installDom();
    const input = createInput();
    const scrollIntoView = vi.fn();
    const queueProgress = dom.window.document.createElement("div");
    queueProgress.id = "library-queue-progress";
    Object.defineProperty(queueProgress, "scrollIntoView", { configurable: true, value: scrollIntoView });
    dom.window.document.body.append(queueProgress);

    const rendered = await renderHookHarness(input);
    root = rendered.root;
    const model = rendered.model;
    expect(model.libraryScreenProps.tocPanelProps?.initialSection).toBe("episodes");
    expect(model.libraryScreenProps.libraryPanelProps.onOpenNovelPublications).toBeUndefined();

    model.queue.onScrollToQueueProgress();
    expect(input.setMobileHomeTab).toHaveBeenCalledWith("download");
    expect(scrollIntoView).toHaveBeenCalledWith({ behavior: "smooth", block: "start" });
    expect(input.panels.queue.setIsOpen).toHaveBeenCalledWith(false);
  });

  it("uses mobile tabs and hides details when another mobile panel is active", async () => {
    installDom();
    const input = createInput({
      activeMobileLibraryPanel: "home",
      isMobileLibraryViewport: true,
      mobileHomeTab: "download"
    });
    const rendered = await renderHookHarness(input);
    root = rendered.root;
    const model = rendered.model;

    expect(model.libraryScreenProps.tocPanelProps).toBeNull();
    expect(model.libraryScreenProps.libraryPanelProps.mobileHomeTab).toBe("download");
    expect(model.libraryScreenProps.libraryPanelProps.onOpenNovelPublications).toBe(input.onOpenNovelPublications);

    model.libraryScreenProps.libraryPanelProps.onToggleDownloadComposer();
    expect(input.setMobileHomeTab).toHaveBeenCalledWith("download");
    expect(input.fetcher.setIsDownloadDropActive).toHaveBeenCalledWith(false);
  });
});
