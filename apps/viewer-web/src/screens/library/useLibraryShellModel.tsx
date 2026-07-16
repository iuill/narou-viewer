import type { Dispatch, SetStateAction } from "react";
import type { useNovelPublications } from "../../hooks/useNovelPublications";
import type { ReaderRouteControllerProps } from "../../routes/ReaderRouteController";
import { formatDate } from "../../shared/date";
import type { LibraryScreenShellProps, LibraryShellProps } from "../LibraryShell";
import type { useFetcherLibraryModel } from "./useFetcherLibraryModel";
import type { useLibraryShellPanels } from "./useLibraryShellPanels";

type LibraryPanelProps = LibraryScreenShellProps["libraryPanelProps"];
type TocPanelProps = NonNullable<LibraryScreenShellProps["tocPanelProps"]>;
type MobileLibraryPanel = LibraryScreenShellProps["activeMobileLibraryPanel"];
type MobileHomeTab = LibraryScreenShellProps["mobileHomeTab"];
type FetcherLibraryModel = ReturnType<typeof useFetcherLibraryModel>;
type LibraryShellPanels = ReturnType<typeof useLibraryShellPanels>;
type NovelPublicationsModel = ReturnType<typeof useNovelPublications>;

type UseLibraryShellModelInput = {
  activeMobileLibraryPanel: MobileLibraryPanel;
  aiGeneration: Pick<LibraryShellProps, "aiGeneration"> & {
    workspaceProps: LibraryScreenShellProps["aiGenerationWorkspaceProps"];
    workspaceRef: LibraryScreenShellProps["aiGenerationWorkspaceRef"];
  };
  clientUpdateRequired: LibraryShellProps["status"]["clientUpdateRequired"];
  currentNovel: TocPanelProps["currentNovel"];
  error: LibraryShellProps["status"]["error"];
  episodeDisplayLookup: TocPanelProps["episodeDisplayLookup"];
  fetcher: FetcherLibraryModel;
  filteredNovelsCount: number;
  googleBooksConfigNotice: string | null;
  isInitialLoading: boolean;
  isMobileLibraryViewport: boolean;
  isNovelLoading: boolean;
  isShowingAllBookmarks: boolean;
  isTocStoryExpanded: boolean;
  libraryFilterQuery: LibraryPanelProps["libraryFilterQuery"];
  libraryNotice: LibraryPanelProps["libraryNotice"];
  libraryPagination: LibraryPanelProps["libraryPagination"];
  mobileDetailInitialSection: TocPanelProps["initialSection"];
  mobileHomeTab: MobileHomeTab;
  novelPublications: NovelPublicationsModel;
  novelsCount: number;
  onBackToLibrary: () => void;
  onClearLibraryFilter: LibraryPanelProps["onClearLibraryFilter"];
  onDeleteBookmark: TocPanelProps["onDeleteBookmark"];
  onLibraryFilterQueryChange: LibraryPanelProps["onLibraryFilterQueryChange"];
  onLibraryPageChange: LibraryPanelProps["onLibraryPageChange"];
  onMobileHomeTabChange: LibraryScreenShellProps["onMobileHomeTabChange"];
  onOpenBookmark: TocPanelProps["onOpenBookmark"];
  onOpenEpisode: TocPanelProps["onOpenEpisode"];
  onOpenNovelPublications: (novelId: string) => void;
  onSelectNovel: ReaderRouteControllerProps["onSelectNovel"];
  panels: LibraryShellPanels;
  readerBookmarks: Pick<TocPanelProps, "bookmarks" | "latestBookmark" | "pendingBookmarkId" | "visibleBookmarks">;
  readerState: TocPanelProps["readerState"];
  readerToc: Pick<
    TocPanelProps,
    | "lastReadEpisodeIndex"
    | "preferFriendlyEpisodeLabels"
    | "selectedEpisodeIndex"
    | "toc"
    | "tocPagination"
    | "tocStoryText"
    | "visibleTocEpisodes"
  > & {
    isTocStoryTruncated: boolean;
  };
  runtimeStatus: LibraryShellProps["status"]["runtimeStatus"];
  runtimeStatusLabel: string;
  setIsShowingAllBookmarks: Dispatch<SetStateAction<boolean>>;
  setIsTocStoryExpanded: Dispatch<SetStateAction<boolean>>;
  setMobileHomeTab: Dispatch<SetStateAction<MobileHomeTab>>;
  setTocPage: TocPanelProps["onTocPageChange"];
  selectedNovelId: string | null;
  viewerBuildCommitDate: string;
  viewerBuildSummary: string;
  visibleLibraryNovels: LibraryPanelProps["visibleLibraryNovels"];
};

export function useLibraryShellModel({
  activeMobileLibraryPanel,
  aiGeneration,
  clientUpdateRequired,
  currentNovel,
  error,
  episodeDisplayLookup,
  fetcher,
  filteredNovelsCount,
  googleBooksConfigNotice,
  isInitialLoading,
  isMobileLibraryViewport,
  isNovelLoading,
  isShowingAllBookmarks,
  isTocStoryExpanded,
  libraryFilterQuery,
  libraryNotice,
  libraryPagination,
  mobileDetailInitialSection,
  mobileHomeTab,
  novelPublications,
  novelsCount,
  onBackToLibrary,
  onClearLibraryFilter,
  onDeleteBookmark,
  onLibraryFilterQueryChange,
  onLibraryPageChange,
  onMobileHomeTabChange,
  onOpenBookmark,
  onOpenEpisode,
  onOpenNovelPublications,
  onSelectNovel,
  panels,
  readerBookmarks,
  readerState,
  readerToc,
  runtimeStatus,
  runtimeStatusLabel,
  selectedNovelId,
  setIsShowingAllBookmarks,
  setIsTocStoryExpanded,
  setMobileHomeTab,
  setTocPage,
  viewerBuildCommitDate,
  viewerBuildSummary,
  visibleLibraryNovels
}: UseLibraryShellModelInput): LibraryShellProps {
  function handleScrollToQueueProgress() {
    setMobileHomeTab("download");
    window.requestAnimationFrame(() => {
      document.getElementById("library-queue-progress")?.scrollIntoView({
        behavior: "smooth",
        block: "start"
      });
    });
    panels.queue.setIsOpen(false);
  }

  return {
    aiGeneration: aiGeneration.aiGeneration,
    isMobileLibraryViewport,
    libraryScreenProps: {
      activeMobileLibraryPanel,
      aiGenerationWorkspaceRef: aiGeneration.workspaceRef,
      aiGenerationWorkspaceProps: aiGeneration.workspaceProps,
      isInitialLoading,
      isMobileLibraryViewport,
      mobileHomeTab,
      onMobileHomeTabChange,
      libraryPanelProps: {
        activeFetcherTaskEntries: fetcher.activeFetcherTaskEntries,
        activeFetcherTasksCount: fetcher.activeFetcherTasks.length,
        cancelingFetcherTaskIds: fetcher.cancelingFetcherTaskIds,
        controllingFetcherTaskIds: fetcher.controllingFetcherTaskIds,
        downloadForce: fetcher.downloadForce,
        downloadTarget: fetcher.downloadTarget,
        filteredNovelsCount,
        isDownloadComposerOpen: fetcher.isDownloadComposerOpen,
        isDownloadDropActive: fetcher.isDownloadDropActive,
        isDownloadSubmitting: fetcher.isDownloadSubmitting,
        isLibraryExporting: fetcher.isLibraryExporting,
        libraryFilterQuery,
        libraryNotice,
        libraryPagination,
        fetcherQueueRunning: fetcher.fetcherQueue?.running ?? false,
        fetcherStatusCheckedAt: fetcher.fetcherStatusCheckedAt,
        fetcherStatusError: fetcher.fetcherStatusError,
        fetcherTaskEntries: fetcher.fetcherTaskEntries,
        mobileHomeTab: isMobileLibraryViewport ? (mobileHomeTab === "download" ? "download" : "library") : undefined,
        novelsCount,
        onClearLibraryFilter,
        onCloseDownloadComposer: () => {
          fetcher.setIsDownloadComposerOpen(false);
          fetcher.setIsDownloadDropActive(false);
        },
        onDownloadDragEnter: (event) => {
          event.preventDefault();
          fetcher.setIsDownloadDropActive(true);
        },
        onDownloadDragLeave: (event) => {
          event.preventDefault();
          if (event.currentTarget === event.target) {
            fetcher.setIsDownloadDropActive(false);
          }
        },
        onDownloadDragOver: (event) => {
          event.preventDefault();
          fetcher.setIsDownloadDropActive(true);
        },
        onDownloadDrop: fetcher.handleDownloadDrop,
        onDownloadForceChange: fetcher.setDownloadForce,
        onDownloadSubmit: fetcher.handleDownloadSubmit,
        onDownloadTargetChange: fetcher.setDownloadTarget,
        onExportLibrary: fetcher.handleExportLibrary,
        onCancelFetcherTask: fetcher.handleCancelFetcherTask,
        onPauseFetcherTask: fetcher.handlePauseFetcherTask,
        onResumeFetcherTask: fetcher.handleResumeFetcherTask,
        onLibraryFilterQueryChange,
        onLibraryPageChange,
        onOpenNovelPublications: isMobileLibraryViewport ? onOpenNovelPublications : undefined,
        onResumeNovel: fetcher.handleResumeNovel,
        onSelectNovel,
        onToggleDownloadComposer: () => {
          if (isMobileLibraryViewport) {
            setMobileHomeTab("download");
            fetcher.setIsDownloadDropActive(false);
            return;
          }
          fetcher.setIsDownloadComposerOpen((current) => !current);
          fetcher.setIsDownloadDropActive(false);
        },
        queueStatusLabel: fetcher.queueStatusLabel,
        resumingNovelIds: fetcher.fetcherActionBusyNovelIds,
        resumableNovels: fetcher.resumableNovels,
        selectedNovelId,
        showStoryAction: isMobileLibraryViewport,
        onUpdateNovel: fetcher.handleUpdateNovel,
        updatableNovels: fetcher.updatableNovels,
        updatingNovelIds: fetcher.fetcherActionBusyNovelIds,
        visibleLibraryNovels
      },
      tocPanelProps:
        isMobileLibraryViewport && activeMobileLibraryPanel !== "details"
          ? null
          : {
              bookmarks: readerBookmarks.bookmarks,
              currentNovel,
              episodeDisplayLookup,
              initialSection: isMobileLibraryViewport ? mobileDetailInitialSection : "episodes",
              isMobileLibraryViewport,
              isNovelActionSubmitting:
                fetcher.isNovelActionSubmitting || (currentNovel ? fetcher.fetcherActionBusyNovelIds.has(currentNovel.novelId) : false),
              isNovelLoading,
              publicationProps: {
                displayCoverEntryId: novelPublications.displayCoverEntryId,
                entries: novelPublications.entries,
                isLoading: novelPublications.isLoading,
                onClear: (entry) => novelPublications.saveEntry(entry.id, { kind: entry.kind, mode: "none" }),
                onCreateISBN: (kind, isbn13) => novelPublications.createEntry({ kind, mode: "isbn", isbn13 }),
                onDisable: (entry) => novelPublications.saveEntry(entry.id, { kind: entry.kind, mode: "disabled" }),
                onRedisplay: (entry) => novelPublications.saveEntry(entry.id, { kind: entry.kind, mode: "visible" }),
                onSaveISBN: (entryId, isbn13) => {
                  const entry = novelPublications.entries.find((candidate) => candidate.id === entryId);
                  return novelPublications.saveEntry(entryId, { kind: entry?.kind, mode: "isbn", isbn13 });
                },
                onSetDisplayCover: (entryId) => novelPublications.setDisplayCover({ entryId }),
                savingEntryId: novelPublications.savingEntryId
              },
              isResumeSubmitting: currentNovel ? fetcher.fetcherActionBusyNovelIds.has(currentNovel.novelId) : false,
              isShowingAllBookmarks,
              isTocStoryExpanded,
              isTocStoryTruncated: readerToc.isTocStoryTruncated,
              lastReadEpisodeIndex: readerToc.lastReadEpisodeIndex,
              latestBookmark: readerBookmarks.latestBookmark,
              novelsCount,
              onBackToLibrary,
              onDeleteBookmark,
              onOpenBookmark,
              onOpenEpisode,
              onRemoveNovel: fetcher.handleRemoveCurrentNovel,
              onResumeNovel: fetcher.handleResumeNovel,
              onTocPageChange: setTocPage,
              onToggleShowingAllBookmarks: () => setIsShowingAllBookmarks((current) => !current),
              onToggleStoryExpanded: () => setIsTocStoryExpanded((current) => !current),
              onUpdateNovel: fetcher.handleUpdateCurrentNovel,
              pendingBookmarkId: readerBookmarks.pendingBookmarkId,
              preferFriendlyEpisodeLabels: readerToc.preferFriendlyEpisodeLabels,
              readerState,
              selectedEpisodeIndex: readerToc.selectedEpisodeIndex,
              toc: readerToc.toc,
              tocPagination: readerToc.tocPagination,
              tocStoryText: readerToc.tocStoryText,
              visibleBookmarks: readerBookmarks.visibleBookmarks,
              visibleTocEpisodes: readerToc.visibleTocEpisodes
            }
    },
    queue: {
      currentFetcherTask: fetcher.currentFetcherTask,
      fetcherQueue: fetcher.fetcherQueue,
      fetcherStatusCheckedAt: fetcher.fetcherStatusCheckedAt,
      fetcherStatusError: fetcher.fetcherStatusError,
      fetcherTasksFailedCount: fetcher.fetcherTasks?.failedCount ?? 0,
      fetcherTasksPausedCount: fetcher.fetcherTasks?.pausedCount ?? 0,
      fetcherTasksInterruptedCount: fetcher.fetcherTasks?.interruptedCount ?? 0,
      fetcherUpdateNotice: fetcher.fetcherUpdateNotice,
      hasActiveFetcherTasks: fetcher.hasFetcherTaskActivity,
      hasFetcherStatus: fetcher.hasFetcherStatus,
      isOpen: panels.queue.isOpen,
      onScrollToQueueProgress: handleScrollToQueueProgress,
      panelRef: panels.queue.panelRef,
      queuedTaskPreviewEntries: fetcher.queuedTaskPreviewEntries,
      queueStatusLabel: fetcher.queueStatusLabel,
      pausedFetcherTaskPreviewEntries: fetcher.pausedFetcherTaskPreviewEntries,
      interruptedFetcherTaskPreviewEntries: fetcher.interruptedFetcherTaskPreviewEntries,
      recentFailedFetcherTaskPreviewEntries: fetcher.recentFailedFetcherTaskPreviewEntries,
      setIsOpen: panels.queue.setIsOpen
    },
    status: {
      clientUpdateRequired,
      error,
      formatDate,
      googleBooksConfigNotice,
      isOpen: panels.status.isOpen,
      panelRef: panels.status.panelRef,
      runtimeStatus,
      runtimeStatusLabel,
      setIsOpen: panels.status.setIsOpen,
      viewerBuildCommitDate,
      viewerBuildSummary
    }
  };
}
