import { useMemo, useState } from "react";
import {
  formatViewerBuildCommitDate,
  formatViewerBuildSummary,
  viewerBuildInfo
} from "../../buildInfo";
import type { ApiClientUpdateRequiredEventDetail } from "../../api/contract";
import type { NovelSummary } from "../../features/library/types";
import { useAutoClearedNotice } from "../../hooks/useAutoClearedNotice";
import { useNovelPublications } from "../../hooks/useNovelPublications";
import { findRuntimeService } from "../../hooks/useRuntimeStatus";
import type { RuntimeStatusResponse, RuntimeStatusService } from "../../features/runtime/types";
import type { ReaderRouteControllerProps } from "../../routes/ReaderRouteController";
import { formatDate } from "../../shared/date";
import { useAiGenerationWorkspaceModel } from "../ai-generation/useAiGenerationWorkspaceModel";
import type { LibraryShellProps } from "../LibraryShell";
import type { ReaderScreenModel } from "../ReaderShell";
import type { ReaderWorkspaceModel } from "../reader/useReaderWorkspaceModel";
import { useFetcherLibraryModel } from "./useFetcherLibraryModel";
import { useLibraryShellModel } from "./useLibraryShellModel";
import { useLibraryShellPanels } from "./useLibraryShellPanels";

type MobileLibraryPanel = "library" | "details";
type MobileHomeTab = "library" | "download" | "status";

type UseLibraryWorkspaceModelInput = {
  aiGenerationRuntimeService: RuntimeStatusService | null;
  clientUpdateRequired: ApiClientUpdateRequiredEventDetail | null;
  currentNovel: NovelSummary | null;
  error: string | null;
  fetcherRuntimeService: RuntimeStatusService | null;
  filteredNovels: NovelSummary[];
  initialNovelId: string | null;
  isInitialLoading: boolean;
  isMobileLibraryViewport: boolean;
  libraryFilterQuery: string;
  libraryPagination: LibraryShellProps["libraryScreenProps"]["libraryPanelProps"]["libraryPagination"];
  libraryReloadKey: number;
  novels: NovelSummary[];
  onClearLibraryFilter: () => void;
  onLibraryFilterQueryChange: (value: string) => void;
  onLibraryPageChange: (page: number) => void;
  readerWorkspace: ReaderWorkspaceModel;
  refreshRuntimeStatus: () => Promise<RuntimeStatusResponse | null>;
  requestLibraryReload: () => void;
  runtimeStatus: RuntimeStatusResponse | null;
  runtimeStatusLabel: string;
  selectedNovelId: string | null;
  setError: (message: string | null) => void;
  visibleLibraryNovels: NovelSummary[];
};

export type LibraryWorkspaceModel = {
  reader: ReaderScreenModel;
  routeController: ReaderRouteControllerProps;
  shell: LibraryShellProps;
};

type ReaderAiAssistantAvailability = {
  isAvailable: boolean;
  unavailableMessage: string | null;
};

function applyReaderAiAssistantAvailability(
  reader: ReaderScreenModel,
  availability: ReaderAiAssistantAvailability
): ReaderScreenModel {
  const updateReaderAiAction = (actions: ReaderScreenModel["state"]["readerVisibleActions"]) =>
    actions.map((action) =>
      action.id === "reader-ai"
        ? {
            ...action,
            disabled: !availability.isAvailable,
            title: availability.unavailableMessage ?? "読書AI"
          }
        : action
    );

  return {
    ...reader,
    state: {
      ...reader.state,
      readerAiAssistantUnavailableMessage: availability.unavailableMessage,
      readerOverflowActions: updateReaderAiAction(reader.state.readerOverflowActions),
      readerVisibleActions: updateReaderAiAction(reader.state.readerVisibleActions)
    }
  };
}

export function useLibraryWorkspaceModel({
  aiGenerationRuntimeService,
  clientUpdateRequired,
  currentNovel,
  error,
  fetcherRuntimeService,
  filteredNovels,
  initialNovelId,
  isInitialLoading,
  isMobileLibraryViewport,
  libraryFilterQuery,
  libraryPagination,
  libraryReloadKey,
  novels,
  onClearLibraryFilter,
  onLibraryFilterQueryChange,
  onLibraryPageChange,
  readerWorkspace,
  refreshRuntimeStatus,
  requestLibraryReload,
  runtimeStatus,
  runtimeStatusLabel,
  selectedNovelId,
  setError,
  visibleLibraryNovels
}: UseLibraryWorkspaceModelInput): LibraryWorkspaceModel {
  const libraryShellPanels = useLibraryShellPanels();
  const [libraryNotice, setLibraryNotice] = useAutoClearedNotice(5000);
  const [mobileLibraryPanel, setMobileLibraryPanel] = useState<MobileLibraryPanel>(() =>
    initialNovelId ? "details" : "library"
  );
  const [mobileDetailInitialSection, setMobileDetailInitialSection] = useState<"episodes" | "publications" | "bookmarks">(
    "episodes"
  );
  const [mobileHomeTab, setMobileHomeTab] = useState<MobileHomeTab>("library");

  const fetcher = useFetcherLibraryModel({
    currentNovel,
    fetcherRuntimeService,
    libraryReloadKey,
    novels,
    onError: setError,
    readerCommands: readerWorkspace.commands.readerCommands,
    readerSessionCommands: readerWorkspace.commands.readerSessionCommands,
    requestLibraryReload,
    screenMode: readerWorkspace.screenMode,
    setLibraryNotice
  });
  const aiGeneration = useAiGenerationWorkspaceModel({
    isMobileLibraryViewport,
    isPanelOpen: libraryShellPanels.aiGeneration.isOpen,
    isPaused: readerWorkspace.screenMode === "reader",
    libraryReloadKey,
    novels,
    onClosePanel: () => libraryShellPanels.aiGeneration.setIsOpen(false),
    onOpenNovelFromJob: (novelId) => {
      handleSelectNovel(novelId);
      setMobileLibraryPanel("library");
    },
    panelRef: libraryShellPanels.aiGeneration.panelRef,
    refreshRuntimeStatus,
    runtimeService: aiGenerationRuntimeService,
    runtimeStatus,
    selectedNovelId,
    setIsPanelOpen: libraryShellPanels.aiGeneration.setIsOpen
  });
  const reader = useMemo(
    () => applyReaderAiAssistantAvailability(readerWorkspace.model, aiGeneration.readerAiAssistant),
    [aiGeneration.readerAiAssistant, readerWorkspace.model]
  );
  const novelPublications = useNovelPublications({
    novelId: selectedNovelId,
    onError: setError,
    onSaved: requestLibraryReload
  });
  const googleBooksRuntimeService = useMemo(() => findRuntimeService(runtimeStatus, "google-books"), [runtimeStatus]);
  const googleBooksConfigNotice =
    googleBooksRuntimeService?.status === "warn" ? googleBooksRuntimeService.detail : null;
  const viewerBuildSummary = formatViewerBuildSummary(viewerBuildInfo);
  const viewerBuildCommitDate = formatViewerBuildCommitDate(viewerBuildInfo, formatDate);
  const activeMobileLibraryPanel: MobileLibraryPanel =
    selectedNovelId && mobileLibraryPanel === "details" ? "details" : "library";

  function handleSelectNovel(novelId: string) {
    const result = readerWorkspace.commands.selectNovelFromLibrary(novelId, {
      openInReaderOnSelect: isMobileLibraryViewport
    });
    if (isMobileLibraryViewport && result === "selected-library") {
      setMobileLibraryPanel("library");
    }
  }

  function handleOpenNovelPublications(novelId: string) {
    readerWorkspace.commands.readerCommands.selectNovel(novelId, { openInReader: false });
    setMobileDetailInitialSection("publications");
    setMobileLibraryPanel("details");
    setMobileHomeTab("library");
  }

  function handleMobileHomeTabChange(tab: MobileHomeTab) {
    setMobileHomeTab(tab);
    if (tab === "library") {
      setMobileLibraryPanel("library");
    }
    if (tab !== "download") {
      fetcher.setIsDownloadComposerOpen(false);
      fetcher.setIsDownloadDropActive(false);
    }
  }

  const routeController: ReaderRouteControllerProps = {
    filteredNovels,
    isInitialLoading,
    isMobileLibraryViewport,
    onClearSelection: () => readerWorkspace.commands.readerCommands.clearSelection({ clearNovel: true }),
    onSelectNovel: handleSelectNovel,
    onShowMobileLibraryPanel: () => setMobileLibraryPanel("library"),
    selectedNovelId
  };

  const shell = useLibraryShellModel({
    activeMobileLibraryPanel,
    aiGeneration: {
      aiGeneration: aiGeneration.menu,
      workspaceProps: aiGeneration.workspaceProps,
      workspaceRef: aiGeneration.workspaceRef
    },
    clientUpdateRequired,
    currentNovel,
    error,
    episodeDisplayLookup: readerWorkspace.metadata.episodeDisplayLookup,
    fetcher,
    filteredNovelsCount: filteredNovels.length,
    googleBooksConfigNotice,
    isInitialLoading,
    isMobileLibraryViewport,
    isNovelLoading: readerWorkspace.metadata.isNovelLoading,
    isShowingAllBookmarks: readerWorkspace.metadata.isShowingAllBookmarks,
    isTocStoryExpanded: readerWorkspace.metadata.isTocStoryExpanded,
    libraryFilterQuery,
    libraryNotice,
    libraryPagination,
    mobileDetailInitialSection,
    mobileHomeTab,
    novelPublications,
    novelsCount: novels.length,
    onBackToLibrary: () => setMobileLibraryPanel("library"),
    onClearLibraryFilter,
    onDeleteBookmark: readerWorkspace.commands.handleDeleteBookmark,
    onLibraryFilterQueryChange,
    onLibraryPageChange,
    onMobileHomeTabChange: handleMobileHomeTabChange,
    onOpenBookmark: readerWorkspace.commands.handleOpenBookmark,
    onOpenEpisode: readerWorkspace.commands.readerCommands.openEpisode,
    onOpenNovelPublications: handleOpenNovelPublications,
    onSelectNovel: handleSelectNovel,
    panels: libraryShellPanels,
    readerBookmarks: {
      bookmarks: readerWorkspace.selection.bookmarks,
      latestBookmark: readerWorkspace.metadata.latestBookmark,
      pendingBookmarkId: readerWorkspace.metadata.pendingBookmarkId,
      visibleBookmarks: readerWorkspace.metadata.visibleBookmarks
    },
    readerState: readerWorkspace.selection.readerState,
    readerToc: {
      isTocStoryTruncated: readerWorkspace.metadata.tocStoryPreview.isTruncated,
      lastReadEpisodeIndex: readerWorkspace.metadata.lastReadEpisodeIndex,
      preferFriendlyEpisodeLabels: readerWorkspace.metadata.preferFriendlyEpisodeLabels,
      selectedEpisodeIndex: readerWorkspace.selection.selectedEpisodeIndex,
      toc: readerWorkspace.selection.toc,
      tocPagination: readerWorkspace.metadata.tocPagination,
      tocStoryText: readerWorkspace.metadata.tocStoryText,
      visibleTocEpisodes: readerWorkspace.metadata.visibleTocEpisodes
    },
    runtimeStatus,
    runtimeStatusLabel,
    selectedNovelId,
    setIsShowingAllBookmarks: readerWorkspace.metadata.setIsShowingAllBookmarks,
    setIsTocStoryExpanded: readerWorkspace.metadata.setIsTocStoryExpanded,
    setMobileHomeTab,
    setTocPage: readerWorkspace.metadata.setTocPage,
    viewerBuildCommitDate,
    viewerBuildSummary,
    visibleLibraryNovels
  });

  return {
    reader,
    routeController,
    shell
  };
}
