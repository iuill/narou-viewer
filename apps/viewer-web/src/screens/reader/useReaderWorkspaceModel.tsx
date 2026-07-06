import { type Dispatch, type SetStateAction, useEffect, useMemo, useRef, useState } from "react";
import type { ApiClientUpdateRequiredEventDetail } from "../../api/contract";
import type { NovelSummary } from "../../features/library/types";
import { useReaderSession } from "../../features/reader/useReaderSession";
import { useReaderSessionCommands } from "../../features/reader/useReaderSessionCommands";
import type { EpisodeIndex, TocEpisode } from "../../features/reader/types";
import { detectWebKitEngine } from "../../features/reader/verticalPagination";
import { useAutoClearedNotice } from "../../hooks/useAutoClearedNotice";
import { useCharacterSummary } from "../../hooks/useCharacterSummary";
import { useTouchDevice } from "../../hooks/useMediaQuery";
import { useReaderBookmarks } from "../../hooks/useReaderBookmarks";
import { useReaderControlsLayout } from "../../hooks/useReaderControlsLayout";
import { useReaderFullscreen } from "../../hooks/useReaderFullscreen";
import { useReaderImageViewer } from "../../hooks/useReaderImageViewer";
import { useReaderPagingState } from "../../hooks/useReaderPagingState";
import { useReaderPanels } from "../../hooks/useReaderPanels";
import { useReaderPreferences } from "../../hooks/useReaderPreferences";
import { useReaderSpeech } from "../../hooks/useReaderSpeech";
import { useReaderSyncConflictResolution } from "../../hooks/useReaderSyncConflictResolution";
import {
  createEmptyReaderAiAssistantState,
  type ReaderAiAssistantState
} from "../../ReaderAiAssistantPanel";
import { loadReaderLocalPreferences } from "../../readerPreferences";
import type { AppliedReaderStateAutoSaveGuard } from "../../readerStateAutoSaveGuard";
import { formatDate } from "../../shared/date";
import type { ReaderScreenModel } from "../ReaderShell";
import { useReaderCommands } from "./useReaderCommands";
import { useReaderDerivedState } from "./useReaderDerivedState";
import { useReaderEffects } from "./useReaderEffects";
import { useReaderPagingHelpers } from "./useReaderPagingHelpers";
import { useReaderRefs } from "./useReaderRefs";
import { useReaderScreenModel } from "./useReaderScreenModel";

type ScreenMode = "library" | "reader";

type InitialReaderSelection = {
  episodeIndex: EpisodeIndex | null;
  position: number | null;
  screenMode: ScreenMode;
};

type UseReaderWorkspaceModelOptions = {
  clientUpdateRequired: ApiClientUpdateRequiredEventDetail | null;
  currentNovel: NovelSummary | null;
  error: string | null;
  initialSelection: InitialReaderSelection;
  libraryReloadKey: number;
  selectedNovelId: string | null;
  setError: Dispatch<SetStateAction<string | null>>;
  setNovels: Dispatch<SetStateAction<NovelSummary[]>>;
  setSelectedNovelId: Dispatch<SetStateAction<string | null>>;
};

function createReaderClientId(currentCrypto: Crypto | undefined = typeof crypto === "undefined" ? undefined : crypto): string {
  if (currentCrypto && typeof currentCrypto.randomUUID === "function") {
    return currentCrypto.randomUUID();
  }

  return `reader-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}

export function useReaderWorkspaceModel({
  clientUpdateRequired,
  currentNovel,
  error,
  initialSelection,
  libraryReloadKey,
  selectedNovelId,
  setError,
  setNovels,
  setSelectedNovelId
}: UseReaderWorkspaceModelOptions) {
  const initialReaderLocalPreferences = useMemo(() => loadReaderLocalPreferences(), []);
  const readerClientId = useMemo(() => createReaderClientId(), []);
  const [tocPage, setTocPage] = useState(1);
  const {
    bookmarks,
    episode,
    isEpisodeLoading,
    isNovelLoading,
    isReaderLoadingOverlayVisible,
    openSelectedNovelInReaderRef,
    pendingReadingStateKeyRef,
    readerSyncConflictResolutionState,
    readerState,
    readerSyncConflict,
    screenMode,
    selectedEpisodeIndex,
    selectedEpisodeIndexRef,
    selectedPosition,
    selectedPositionRef,
    toc,
    activeReaderSettings,
    commands: readerSessionCommands,
    isReaderCorrectionUnavailable
  } = useReaderSession({
    initialEpisodeIndex: initialSelection.episodeIndex,
    initialPosition: initialSelection.position,
    initialScreenMode: initialSelection.screenMode,
    libraryReloadKey,
    onError: setError,
    readerClientId,
    selectedNovelId,
    setSelectedNovelId,
    setNovels
  });
  const [readerAiAssistantState, setReaderAiAssistantState] = useState<ReaderAiAssistantState>(() =>
    createEmptyReaderAiAssistantState()
  );
  const [readerNotice, setReaderNotice] = useAutoClearedNotice(2400);
  const [pendingNextEpisodeConfirmation, setPendingNextEpisodeConfirmation] = useState<TocEpisode | null>(null);
  const appliedReaderStateAutoSaveGuardRef = useRef<AppliedReaderStateAutoSaveGuard | null>(null);
  const isNextEpisodeConfirmationOpen = pendingNextEpisodeConfirmation !== null;
  const isReaderModalOpen = isNextEpisodeConfirmationOpen || readerSyncConflict !== null;
  const {
    debugPageOverflow,
    handleResetReaderPreferences,
    handleRetryReaderExperimentalFontLoad,
    readerArticleFontFamilyCss,
    readerArticleFontWeight,
    readerExperimentalFontId,
    readerExperimentalFontLayoutVersion,
    readerExperimentalFontLoadStatus,
    readerExperimentalFontWeight,
    readerFontFamily,
    readerFontSizePx,
    readerLetterSpacingEm,
    readerTheme,
    readingMode,
    reverseTapPageNavigation,
    setDebugPageOverflow,
    setReaderExperimentalFontId,
    setReaderExperimentalFontWeight,
    setReaderFontFamily,
    setReaderFontSizePx,
    setReaderLetterSpacingEm,
    setReaderTheme,
    setReadingMode,
    setReverseTapPageNavigation
  } = useReaderPreferences({
    initialLocalPreferences: initialReaderLocalPreferences,
    setError,
    setReaderNotice
  });
  const {
    imageViewerStageRef,
    nextEpisodeConfirmPrimaryButtonRef,
    nextEpisodeConfirmReturnFocusRef,
    readerControlsRef,
    readerOverflowRef,
    readerPageIndicatorRef,
    readerPanelRef,
    readerShellRef,
    readerViewportRef
  } = useReaderRefs();
  const {
    imageViewer,
    imageViewerZoomPercent,
    isImageViewerDragging,
    isImageViewerInfoOpen,
    closeImageViewer,
    openImageViewer,
    setImageViewerZoomPercent,
    setIsImageViewerInfoOpen,
    handleImageViewerPointerDown,
    handleImageViewerPointerMove,
    handleImageViewerPointerUp
  } = useReaderImageViewer();
  const {
    currentPageIndex,
    isEpisodeLayoutReady,
    layoutAnchorPositionRef,
    movePage,
    resetPageIndex,
    setCurrentPageIndex,
    setIsEpisodeLayoutReady,
    setTotalPages,
    setVerticalLastPageReservePx,
    shouldCapturePageAnchorRef,
    totalPages,
    verticalLastPageReservePx
  } = useReaderPagingState({
    initialPosition: initialSelection.position,
    onSelectedPositionChange: readerSessionCommands.updateSelectedPosition
  });
  const readerCommands = useReaderSessionCommands({
    currentEpisodeIndex: episode?.episodeIndex ?? null,
    layoutAnchorPositionRef,
    onError: setError,
    openSelectedNovelInReaderRef,
    sessionCommands: readerSessionCommands,
    setIsEpisodeLayoutReady,
    shouldCapturePageAnchorRef,
    toc
  });
  const {
    handleReturnToLibrary,
    handleToggleReaderFullscreen,
    isReaderFullscreen,
    isReaderPseudoFullscreen
  } = useReaderFullscreen({
    onReturnToLibrary: readerCommands.returnToLibrary,
    readerShellRef,
    screenMode,
    setReaderNotice
  });

  // biome-ignore lint/correctness/useExhaustiveDependencies: reader context changes should dismiss the pending episode confirmation.
  useEffect(() => {
    setPendingNextEpisodeConfirmation(null);
  }, [currentPageIndex, screenMode, selectedEpisodeIndex, selectedNovelId, toc]);

  const {
    activeReaderPanel,
    closeActiveReaderPanel,
    closeCharacterSummaryPanel,
    closeReaderPanel,
    isCharacterSummaryOpen,
    isReaderAiAssistantOpen,
    isReaderBookmarksOpen,
    isReaderExperimentalFontOpen,
    isReaderInfoOpen,
    isReaderSettingsOpen,
    isReaderSpeechOpen,
    isReaderTocOpen,
    isReaderOverflowOpen,
    openCharacterSummaryPanel,
    setIsReaderOverflowOpen,
    toggleReaderPanel
  } = useReaderPanels({
    closeImageViewer,
    readerControlsRef,
    readerOverflowRef,
    readerPanelRef,
    screenMode
  });
  const { readerControlViewportWidth, readerPageIndicatorWidth } = useReaderControlsLayout({
    currentPageIndex,
    episode,
    readerPageIndicatorRef,
    readerShellRef,
    readerViewportRef,
    screenMode,
    totalPages
  });
  const isWebKitEngine = useMemo(() => detectWebKitEngine(), []);
  const isTouchDevice = useTouchDevice();
  const {
    getCurrentPageIndexFromViewport,
    getCurrentReaderViewportPosition,
    getPagingMetrics,
    measureVerticalPages,
    scrollToPage,
    verticalPagingCacheRef
  } = useReaderPagingHelpers({
    currentPageIndex,
    episode,
    readerArticleFontFamilyCss,
    readerArticleFontWeight,
    readerExperimentalFontLayoutVersion,
    readerFontSizePx,
    readerLetterSpacingEm,
    readerViewportRef,
    readingMode,
    verticalLastPageReservePx
  });
  const isReaderKeyboardPagingBlocked =
    imageViewer !== null || activeReaderPanel !== null || isReaderOverflowOpen || readerSyncConflict !== null;
  const [isShowingAllBookmarks, setIsShowingAllBookmarks] = useState(false);
  const [isTocStoryExpanded, setIsTocStoryExpanded] = useState(false);
  const {
    canMoveForwardReaderPage,
    canOpenNextEpisode,
    canOpenPreviousEpisode,
    canUseNextPageButton,
    canUsePreviousPageButton,
    canUseReaderPageActions,
    currentTocEpisodeIndex,
    edgeTapPageMoveDirections,
    episodeDisplayLookup,
    formatCharacterSummaryEpisodeOrder,
    hasStableReaderEpisode,
    hasUnlistedEpisodes,
    imageViewerWidth,
    lastReadEpisodeIndex,
    nextEpisode,
    nextPageActionLabel,
    pageMoveDirections,
    preferFriendlyEpisodeLabels,
    previousEpisode,
    previousPageActionLabel,
    readerForwardPageDirection,
    readerInfoEpisodeReferenceLabel,
    readerInfoEpisodeTitle,
    readerInfoPageLabel,
    readerInfoSourceUrl,
    readerInfoUpdatedAtLabel,
    readerLoadingEpisodeTitle,
    readerSyncConflictApplyDisabledReason,
    readerSyncConflictEpisodeLabel,
    renderedEpisodeHtml,
    savedReadingStateKey,
    selectedTocPage,
    tocPagination,
    tocStoryPreview,
    tocStoryText,
    visibleTocEpisodes
  } = useReaderDerivedState({
    currentNovel,
    currentPageIndex,
    episode,
    imageViewer,
    imageViewerZoomPercent,
    isEpisodeLayoutReady,
    isEpisodeLoading,
    isReaderModalOpen,
    isTocStoryExpanded,
    readerState,
    readerSyncConflict,
    readingMode,
    reverseTapPageNavigation,
    screenMode,
    selectedEpisodeIndex,
    selectedNovelId,
    toc,
    tocPage,
    totalPages
  });
  const {
    handleReaderSpeechPause,
    handleReaderSpeechPlay,
    handleReaderSpeechResume,
    isReaderSpeechPaused,
    isReaderSpeechPlaying,
    isReaderSpeechProgressAutoScrollSuppressed,
    isReaderSpeechSupported,
    logReaderSpeechDebugEvent,
    readerSpeechActiveChunkIndex,
    readerSpeechChunks,
    readerSpeechDebugHighlight,
    readerSpeechEnabled,
    readerSpeechPreferRubyText,
    readerSpeechRate,
    readerSpeechState,
    readerSpeechVoiceUri,
    readerSpeechVoices,
    setReaderSpeechDebugHighlight,
    setReaderSpeechEnabled,
    setReaderSpeechPreferRubyText,
    setReaderSpeechRate,
    setReaderSpeechVoiceUri,
    shouldShowReaderSpeechControls,
    stopReaderSpeech
  } = useReaderSpeech({
    currentPageIndex,
    episode,
    getCurrentPageIndexFromViewport,
    getCurrentReaderViewportPosition,
    getPagingMetrics,
    initialDebugHighlight: initialReaderLocalPreferences.speechDebugHighlight,
    initialEnabled: initialReaderLocalPreferences.speechEnabled,
    initialPreferRubyText: initialReaderLocalPreferences.speechPreferRubyText,
    initialRate: initialReaderLocalPreferences.speechRate,
    initialVoiceUri: initialReaderLocalPreferences.speechVoiceUri,
    readerViewportRef,
    readingMode,
    renderedEpisodeHtml,
    screenMode,
    scrollToPage,
    selectedEpisodeIndex,
    selectedEpisodeIndexRef,
    selectedPositionRef,
    setCurrentPageIndex,
    setError,
    setIsReaderOverflowOpen,
    setReaderNotice,
    setSelectedPosition: readerSessionCommands.updateSelectedPosition
  });
  // biome-ignore lint/correctness/useExhaustiveDependencies: bookmark expansion resets when the selected novel changes.
  useEffect(() => {
    setIsShowingAllBookmarks(false);
  }, [selectedNovelId]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: TOC story expansion resets when the selected novel changes.
  useEffect(() => {
    setIsTocStoryExpanded(false);
  }, [selectedNovelId]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: reader assistant draft resets when the selected novel changes.
  useEffect(() => {
    setReaderAiAssistantState(createEmptyReaderAiAssistantState());
  }, [selectedNovelId]);

  useEffect(() => {
    if (tocPage !== tocPagination.currentPage) {
      setTocPage(tocPagination.currentPage);
    }
  }, [tocPage, tocPagination.currentPage]);

  useEffect(() => {
    setTocPage((currentPage) => (currentPage === selectedTocPage ? currentPage : selectedTocPage));
  }, [selectedTocPage]);

  useEffect(() => {
    if (!isReaderTocOpen) {
      return;
    }

    setTocPage((currentPage) => (currentPage === selectedTocPage ? currentPage : selectedTocPage));
  }, [isReaderTocOpen, selectedTocPage]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: visible entries changing should re-run the selected TOC scroll.
  useEffect(() => {
    if (!isReaderTocOpen || selectedEpisodeIndex === null) {
      return;
    }

    const selectedTocEntry = Array.from(
      readerPanelRef.current?.querySelectorAll<HTMLElement>('[data-reader-panel-item="toc-episode"]') ?? []
    ).find((entry) => entry.dataset.episodeIndex === selectedEpisodeIndex);

    selectedTocEntry?.scrollIntoView({ block: "center", inline: "nearest" });
  }, [isReaderTocOpen, selectedEpisodeIndex, visibleTocEpisodes]);

  const {
    createCurrentBookmark: handleCreateBookmark,
    deleteBookmarkById: handleDeleteBookmark,
    isBookmarkSaving,
    latestBookmark,
    pendingBookmarkId,
    visibleBookmarks
  } = useReaderBookmarks({
    bookmarks,
    episodeDisplayLookup,
    isShowingAllBookmarks,
    preferFriendlyEpisodeLabels,
    readerViewportRef,
    readingMode,
    selectedEpisodeIndex,
    selectedNovelId,
    setBookmarks: readerSessionCommands.updateBookmarks,
    setError,
    setNovels,
    setReaderNotice,
    setSelectedPosition: readerSessionCommands.updateSelectedPosition
  });
  const {
    activeJobs: characterSummaryActiveJobs,
    canClear: characterSummaryCanClear,
    canGenerate: characterSummaryCanGenerate,
    completedJobs: characterSummaryCompletedJobs,
    data: characterSummaryData,
    defaultUpToEpisodeIndex: characterSummaryDefaultUpToEpisodeIndex,
    error: characterSummaryError,
    handleClear: handleClearCharacterSummary,
    handleGenerate: handleGenerateCharacterSummary,
    handleOpen: handleOpenCharacterSummary,
    isClearing: isCharacterSummaryClearing,
    isLoading: isCharacterSummaryLoading,
    isSubmitting: isCharacterSummarySubmitting,
    notice: characterSummaryNotice,
    requestedGenerationStrategy: characterSummaryGenerationStrategy,
    requestedUpToEpisodeIndex: characterSummaryUpToEpisodeIndex,
    setRequestedGenerationStrategy: setCharacterSummaryGenerationStrategy,
    setRequestedUpToEpisodeIndex: setCharacterSummaryUpToEpisodeIndex
  } = useCharacterSummary({
    currentTocEpisodeIndex,
    formatEpisodeOrderLabel: formatCharacterSummaryEpisodeOrder,
    isOpen: isCharacterSummaryOpen,
    onClosePanel: closeCharacterSummaryPanel,
    onOpenPanel: openCharacterSummaryPanel,
    screenMode,
    selectedNovelId,
    setReaderNotice,
    toc
  });
  const {
    canApplyReaderSyncConflict,
    handleApplyReaderSyncConflict,
    handleOverwriteReaderSyncConflict,
    readerSyncConflictResolutionError
  } = useReaderSyncConflictResolution({
    appliedReaderStateAutoSaveGuardRef,
    applyDisabledReason: readerSyncConflictApplyDisabledReason,
    closeActiveReaderPanel,
    closeImageViewer,
    episode,
    getCurrentReaderViewportPosition,
    handleOpenEpisode: readerCommands.openEpisode,
    hasStableReaderEpisode,
    isEpisodeLoading,
    pendingReadingStateKeyRef,
    savePosition: readerSessionCommands.savePosition,
    readerSyncConflict,
    readerSyncConflictResolutionState,
    selectedEpisodeIndex,
    selectedNovelId,
    selectedPosition,
    setError,
    setIsReaderOverflowOpen,
    setPendingNextEpisodeConfirmation,
    setReaderNotice,
    applyServerState: readerSessionCommands.applyServerState,
    clearReaderSyncConflict: readerSessionCommands.clearReaderSyncConflict,
    markReaderSyncConflictResolution: readerSessionCommands.markReaderSyncConflictResolution,
    stopReaderSpeech
  });

  useReaderEffects({
    appliedReaderStateAutoSaveGuardRef,
    currentPageIndex,
    debugPageOverflow,
    episode,
    getCurrentPageIndexFromViewport,
    getCurrentReaderViewportPosition,
    getPagingMetrics,
    hasStableReaderEpisode,
    isEpisodeLayoutReady,
    isEpisodeLoading,
    isReaderFullscreen,
    isReaderSpeechProgressAutoScrollSuppressed,
    layoutAnchorPositionRef,
    logReaderSpeechDebugEvent,
    measureVerticalPages,
    openImageViewer,
    pendingReadingStateKeyRef,
    readerArticleFontFamilyCss,
    readerArticleFontWeight,
    readerExperimentalFontLayoutVersion,
    readerFontSizePx,
    readerLetterSpacingEm,
    readerSessionCommands,
    readerSpeechState,
    readerStateStateVersion: readerState?.stateVersion,
    readerSyncConflict,
    readerSyncConflictResolutionState,
    readerViewportRef,
    readingMode,
    resetPageIndex,
    savedReadingStateKey,
    screenMode,
    scrollToPage,
    selectedEpisodeIndex,
    selectedEpisodeIndexRef,
    selectedNovelId,
    selectedPosition,
    setCurrentPageIndex,
    setError,
    setIsEpisodeLayoutReady,
    setTotalPages,
    setVerticalLastPageReservePx,
    shouldCapturePageAnchorRef,
    totalPages,
    verticalLastPageReservePx,
    verticalPagingCacheRef
  });

  const {
    handleConfirmNextEpisode,
    handleNextEpisodeConfirmationBackdropClick,
    handleNextEpisodeConfirmationCloseClick,
    handleNextEpisodeConfirmationKeyDown,
    handleOpenBookmark,
    handleResetReaderSpeechPreferences,
    handleViewportClick,
    handleViewportTouchCancel,
    handleViewportTouchEnd,
    handleViewportTouchStart,
    readerOverflowActions,
    readerVisibleActions
  } = useReaderCommands({
    canMoveForwardReaderPage,
    canOpenNextEpisode,
    canOpenPreviousEpisode,
    canUseNextPageButton,
    canUsePreviousPageButton,
    canUseReaderPageActions,
    closeReaderPanel,
    edgeTapPageMoveDirections,
    episode,
    episodeContentEtag: episode?.contentEtag ?? null,
    handleCreateBookmark,
    handleOpenCharacterSummary,
    handleReturnToLibrary,
    handleToggleReaderFullscreen,
    hasUnlistedEpisodes,
    isBookmarkSaving,
    isCharacterSummaryOpen,
    isEpisodeLoading,
    isReaderAiAssistantAvailable: false,
    isReaderAiAssistantOpen,
    isReaderAiAssistantUnavailableMessage: null,
    isReaderBookmarksOpen,
    isReaderExperimentalFontOpen,
    isReaderFullscreen,
    isReaderInfoOpen,
    isReaderKeyboardPagingBlocked,
    isReaderModalOpen,
    isReaderSettingsOpen,
    isReaderSpeechOpen,
    isReaderSpeechPaused,
    isReaderSpeechPlaying,
    isReaderTocOpen,
    isTouchDevice,
    movePage,
    nextEpisode,
    nextEpisodeConfirmPrimaryButtonRef,
    nextEpisodeConfirmReturnFocusRef,
    nextPageActionLabel,
    pageMoveDirections,
    pendingNextEpisodeConfirmation,
    previousEpisode,
    previousPageActionLabel,
    readerCommands,
    readerControlViewportWidth,
    readerForwardPageDirection,
    readerPageIndicatorWidth,
    readerViewportRef,
    screenMode,
    setIsReaderOverflowOpen,
    setPendingNextEpisodeConfirmation,
    setReaderNotice,
    setReaderSpeechDebugHighlight,
    setReaderSpeechEnabled,
    setReaderSpeechPreferRubyText,
    setReaderSpeechRate,
    setReaderSpeechVoiceUri,
    shouldShowReaderSpeechControls,
    stopReaderSpeech,
    toggleReaderPanel
  });

  function selectNovelFromLibrary(novelId: string, options: { openInReaderOnSelect: boolean }) {
    if (options.openInReaderOnSelect && novelId === selectedNovelId) {
      const resolvedEpisodeIndex = selectedEpisodeIndex ?? readerState?.lastReadEpisodeIndex ?? toc?.episodes[0]?.episodeIndex ?? null;
      if (resolvedEpisodeIndex !== null) {
        const resolvedPosition =
          readerState?.lastReadEpisodeIndex === resolvedEpisodeIndex ? readerState.position : null;
        return readerCommands.openEpisode(resolvedEpisodeIndex, resolvedPosition)
          ? "opened-reader"
          : "open-failed";
      }
    }

    readerCommands.selectNovel(novelId, { openInReader: options.openInReaderOnSelect });
    return "selected-library" as const;
  }

  const displayedPageNumber = currentPageIndex + 1;
  const model: ReaderScreenModel = useReaderScreenModel({
    activeReaderSettings,
    bookmarks,
    canApplyReaderSyncConflict,
    characterSummaryActiveJobs,
    characterSummaryCanClear,
    characterSummaryCanGenerate,
    characterSummaryCompletedJobs,
    characterSummaryData,
    characterSummaryDefaultUpToEpisodeIndex,
    characterSummaryError,
    characterSummaryGenerationStrategy,
    characterSummaryNotice,
    characterSummaryUpToEpisodeIndex,
    clientUpdateRequired,
    currentNovel,
    debugPageOverflow,
    displayedPageNumber,
    episode,
    episodeDisplayLookup,
    error,
    hasStableReaderEpisode,
    imageViewer,
    imageViewerWidth,
    imageViewerZoomPercent,
    isBookmarkSaving,
    isCharacterSummaryClearing,
    isCharacterSummaryLoading,
    isCharacterSummaryOpen,
    isCharacterSummarySubmitting,
    isEpisodeLoading,
    isImageViewerDragging,
    isImageViewerInfoOpen,
    isReaderAiAssistantOpen,
    isReaderBookmarksOpen,
    isReaderCorrectionUnavailable,
    isReaderExperimentalFontOpen,
    isReaderFullscreen,
    isReaderInfoOpen,
    isReaderLoadingOverlayVisible,
    isReaderOverflowOpen,
    isReaderPseudoFullscreen,
    isReaderSettingsOpen,
    isReaderSpeechOpen,
    isReaderSpeechPaused,
    isReaderSpeechPlaying,
    isReaderSpeechSupported,
    isReaderTocOpen,
    isShowingAllBookmarks,
    isTouchDevice,
    isWebKitEngine,
    pendingBookmarkId,
    pendingNextEpisodeConfirmation,
    preferFriendlyEpisodeLabels,
    readerAiAssistantState,
    readerAiAssistantUnavailableMessage: null,
    readerArticleFontFamilyCss,
    readerArticleFontWeight,
    readerExperimentalFontId,
    readerExperimentalFontLoadStatus,
    readerExperimentalFontWeight,
    readerFontFamily,
    readerFontSizePx,
    readerInfoEpisodeReferenceLabel,
    readerInfoEpisodeTitle,
    readerInfoPageLabel,
    readerInfoSourceUrl,
    readerInfoUpdatedAtLabel,
    readerLetterSpacingEm,
    readerLoadingEpisodeTitle,
    readerNotice,
    readerOverflowActions,
    readerSpeechActiveChunkIndex,
    readerSpeechChunks,
    readerSpeechDebugHighlight,
    readerSpeechEnabled,
    readerSpeechPreferRubyText,
    readerSpeechRate,
    readerSpeechVoiceUri,
    readerSpeechVoices,
    readerSyncConflict,
    readerSyncConflictApplyDisabledReason,
    readerSyncConflictEpisodeLabel,
    readerSyncConflictResolutionError,
    readerSyncConflictResolutionState,
    readerTheme,
    readerVisibleActions,
    readingMode,
    renderedEpisodeHtml,
    reverseTapPageNavigation,
    selectedEpisodeIndex,
    selectedNovelId,
    sourceNovelTitle: currentNovel?.title ?? toc?.title ?? "小説",
    toc,
    tocPagination,
    totalPages,
    verticalLastPageReservePx,
    visibleBookmarks,
    visibleTocEpisodes,
    closeActiveReaderPanel,
    closeImageViewer,
    closeReaderPanel,
    formatCharacterSummaryEpisodeOrder,
    formatDate,
    getCurrentReaderViewportPosition,
    handleApplyReaderSyncConflict,
    handleClearCharacterSummary,
    handleConfirmNextEpisode,
    handleCreateBookmark,
    handleDeleteBookmark,
    handleGenerateCharacterSummary,
    handleImageViewerPointerDown,
    handleImageViewerPointerMove,
    handleImageViewerPointerUp,
    handleNextEpisodeConfirmationBackdropClick,
    handleNextEpisodeConfirmationCloseClick,
    handleNextEpisodeConfirmationKeyDown,
    handleOpenBookmark,
    handleOverwriteReaderSyncConflict,
    handleReaderSpeechPause,
    handleReaderSpeechPlay,
    handleReaderSpeechResume,
    handleResetReaderPreferences,
    handleResetReaderSpeechPreferences,
    handleRetryReaderExperimentalFontLoad,
    handleViewportClick,
    handleViewportTouchCancel,
    handleViewportTouchEnd,
    handleViewportTouchStart,
    readerCommands,
    readerSessionCommands,
    setCharacterSummaryGenerationStrategy,
    setCharacterSummaryUpToEpisodeIndex,
    setDebugPageOverflow,
    setError,
    setImageViewerZoomPercent,
    setIsImageViewerInfoOpen,
    setIsReaderOverflowOpen,
    setIsShowingAllBookmarks,
    setReaderAiAssistantState,
    setReaderExperimentalFontId,
    setReaderExperimentalFontWeight,
    setReaderFontFamily,
    setReaderFontSizePx,
    setReaderLetterSpacingEm,
    setReaderSpeechDebugHighlight,
    setReaderSpeechEnabled,
    setReaderSpeechPreferRubyText,
    setReaderSpeechRate,
    setReaderSpeechVoiceUri,
    setReaderTheme,
    setReadingMode,
    setReverseTapPageNavigation,
    setTocPage,
    stopReaderSpeech,
    imageViewerStageRef,
    nextEpisodeConfirmPrimaryButtonRef,
    readerControlsRef,
    readerOverflowRef,
    readerPageIndicatorRef,
    readerPanelRef,
    readerShellRef,
    readerViewportRef,
    selectedPositionRef
  });

  return {
    model,
    screenMode,
    commands: {
      handleDeleteBookmark,
      handleOpenBookmark,
      readerCommands,
      readerSessionCommands,
      selectNovelFromLibrary
    },
    metadata: {
      episodeDisplayLookup,
      isNovelLoading,
      isShowingAllBookmarks,
      isTocStoryExpanded,
      lastReadEpisodeIndex,
      latestBookmark,
      pendingBookmarkId,
      preferFriendlyEpisodeLabels,
      setIsShowingAllBookmarks,
      setIsTocStoryExpanded,
      setTocPage,
      tocPagination,
      tocStoryPreview,
      tocStoryText,
      visibleBookmarks,
      visibleTocEpisodes
    },
    selection: {
      bookmarks,
      readerState,
      selectedEpisodeIndex,
      selectedPosition,
      toc
    }
  };
}

export type ReaderWorkspaceModel = ReturnType<typeof useReaderWorkspaceModel>;
