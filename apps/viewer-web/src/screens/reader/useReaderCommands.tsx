import {
  type Dispatch,
  type MouseEvent as ReactMouseEvent,
  type KeyboardEvent as ReactKeyboardEvent,
  type RefObject,
  type SetStateAction,
  type TouchEvent as ReactTouchEvent,
  useCallback,
  useEffect,
  useRef
} from "react";
import {
  getReaderTouchGestureStart,
  hasActiveTextSelection,
  READER_EDGE_TAP_MAX_DURATION_MS,
  type ReaderPageMoveDirections,
  type ReaderTouchGesture,
  resolveReaderEdgeNavigationDirection,
  resolveReaderTouchNavigationDirection
} from "../../features/reader/gestureNavigation";
import type { Bookmark, TocEpisode } from "../../features/reader/types";
import type { ReaderSessionCommands } from "../../features/reader/useReaderSessionCommands";
import type { ReaderPanelView } from "../../hooks/useReaderPanels";
import { DEFAULT_READER_LOCAL_PREFERENCES } from "../../readerPreferences";
import { useReaderControlActions } from "./useReaderControlActions";

type ScreenMode = "library" | "reader";

function isTocEpisodeFetched(episode: TocEpisode | null | undefined): episode is TocEpisode {
  return Boolean(episode && (!episode.bodyStatus || episode.bodyStatus === "complete"));
}

type UseReaderCommandsOptions = {
  canMoveForwardReaderPage: boolean;
  canOpenNextEpisode: boolean;
  canOpenPreviousEpisode: boolean;
  canUseNextPageButton: boolean;
  canUsePreviousPageButton: boolean;
  canUseReaderPageActions: boolean;
  closeReaderPanel: () => void;
  edgeTapPageMoveDirections: ReaderPageMoveDirections;
  episode: unknown;
  episodeContentEtag: string | null;
  handleCreateBookmark: () => Promise<void>;
  handleOpenCharacterSummary: () => void | Promise<void>;
  handleOpenTerms: () => void | Promise<void>;
  handleReturnToLibrary: () => void | Promise<void>;
  handleToggleReaderFullscreen: () => void | Promise<void>;
  hasUnlistedEpisodes: boolean;
  isBookmarkSaving: boolean;
  isCharacterSummaryOpen: boolean;
  isTermsOpen: boolean;
  isEpisodeLoading: boolean;
  isReaderAiAssistantAvailable: boolean;
  isReaderAiAssistantOpen: boolean;
  isReaderAiAssistantUnavailableMessage: string | null;
  isReaderBookmarksOpen: boolean;
  isReaderExperimentalFontOpen: boolean;
  isReaderFullscreen: boolean;
  isReaderInfoOpen: boolean;
  isReaderKeyboardPagingBlocked: boolean;
  isReaderModalOpen: boolean;
  isReaderSettingsOpen: boolean;
  isReaderSpeechOpen: boolean;
  isReaderSpeechPaused: boolean;
  isReaderSpeechPlaying: boolean;
  isReaderTocOpen: boolean;
  isTouchDevice: boolean;
  movePage: (direction: -1 | 1) => void;
  nextEpisode: TocEpisode | null;
  nextEpisodeConfirmPrimaryButtonRef: RefObject<HTMLButtonElement | null>;
  nextEpisodeConfirmReturnFocusRef: RefObject<HTMLElement | null>;
  nextPageActionLabel: string;
  pageMoveDirections: ReaderPageMoveDirections;
  pendingNextEpisodeConfirmation: TocEpisode | null;
  previousEpisode: TocEpisode | null;
  previousPageActionLabel: string;
  readerCommands: ReaderSessionCommands;
  readerControlViewportWidth: number;
  readerForwardPageDirection: -1 | 1;
  readerPageIndicatorWidth: number;
  readerViewportRef: RefObject<HTMLDivElement | null>;
  screenMode: ScreenMode;
  setIsReaderOverflowOpen: Dispatch<SetStateAction<boolean>>;
  setPendingNextEpisodeConfirmation: Dispatch<SetStateAction<TocEpisode | null>>;
  setReaderNotice: Dispatch<SetStateAction<string | null>>;
  setReaderSpeechDebugHighlight: Dispatch<SetStateAction<boolean>>;
  setReaderSpeechEnabled: Dispatch<SetStateAction<boolean>>;
  setReaderSpeechPreferRubyText: Dispatch<SetStateAction<boolean>>;
  setReaderSpeechRate: Dispatch<SetStateAction<number>>;
  setReaderSpeechVoiceUri: Dispatch<SetStateAction<string | null>>;
  shouldShowReaderSpeechControls: boolean;
  stopReaderSpeech: (options?: { notice?: string | null }) => Promise<void>;
  toggleReaderPanel: (panel: Exclude<ReaderPanelView, "characters">) => void;
};

export function useReaderCommands({
  canMoveForwardReaderPage,
  canOpenNextEpisode,
  canOpenPreviousEpisode,
  canUseNextPageButton,
  canUsePreviousPageButton,
  canUseReaderPageActions,
  closeReaderPanel,
  edgeTapPageMoveDirections,
  episode,
  episodeContentEtag,
  handleCreateBookmark,
  handleOpenCharacterSummary,
  handleOpenTerms,
  handleReturnToLibrary,
  handleToggleReaderFullscreen,
  hasUnlistedEpisodes,
  isBookmarkSaving,
  isCharacterSummaryOpen,
  isTermsOpen,
  isEpisodeLoading,
  isReaderAiAssistantAvailable,
  isReaderAiAssistantOpen,
  isReaderAiAssistantUnavailableMessage,
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
}: UseReaderCommandsOptions) {
  const readerTouchGestureRef = useRef<ReaderTouchGesture | null>(null);
  const readerTouchTextSelectionGestureRef = useRef(false);
  const pendingReaderTouchNavigationRef = useRef<number | null>(null);
  const pendingReaderTouchSelectionCandidateRef = useRef(false);
  const closeNextEpisodeConfirmation = useCallback((options?: { restoreFocus?: boolean }) => {
    setPendingNextEpisodeConfirmation(null);
    if (options?.restoreFocus) {
      window.requestAnimationFrame(() => {
        nextEpisodeConfirmReturnFocusRef.current?.focus();
      });
    }
  }, [nextEpisodeConfirmReturnFocusRef, setPendingNextEpisodeConfirmation]);

  const openNextEpisodeConfirmation = useCallback((targetEpisode: TocEpisode) => {
    nextEpisodeConfirmReturnFocusRef.current =
      document.activeElement instanceof HTMLElement ? document.activeElement : null;
    closeReaderPanel();
    setIsReaderOverflowOpen(false);
    setPendingNextEpisodeConfirmation(targetEpisode);
  }, [closeReaderPanel, nextEpisodeConfirmReturnFocusRef, setIsReaderOverflowOpen, setPendingNextEpisodeConfirmation]);

  const handleConfirmNextEpisode = useCallback(() => {
    const targetEpisode = pendingNextEpisodeConfirmation;
    if (!targetEpisode) {
      return;
    }
    setPendingNextEpisodeConfirmation(null);
    if (!isTocEpisodeFetched(targetEpisode)) {
      setReaderNotice("次の話はまだ本文が取得されていません。再開して取得してください。");
      return;
    }
    setIsReaderOverflowOpen(false);
    readerCommands.openEpisode(targetEpisode.episodeIndex);
  }, [pendingNextEpisodeConfirmation, readerCommands, setIsReaderOverflowOpen, setPendingNextEpisodeConfirmation, setReaderNotice]);

  const handleNextEpisodeConfirmationBackdropClick = useCallback(() => {
    closeNextEpisodeConfirmation({ restoreFocus: true });
  }, [closeNextEpisodeConfirmation]);

  const handleNextEpisodeConfirmationCloseClick = useCallback(() => {
    closeNextEpisodeConfirmation({ restoreFocus: true });
  }, [closeNextEpisodeConfirmation]);

  const handleNextEpisodeConfirmationKeyDown = useCallback(
    (event: ReactKeyboardEvent<HTMLElement>) => {
      if (event.key === "Escape") {
        event.preventDefault();
        closeNextEpisodeConfirmation({ restoreFocus: true });
        return;
      }

      if (event.key !== "Tab") {
        return;
      }

      const focusableElements = Array.from(
        event.currentTarget.querySelectorAll<HTMLElement>(
          'button:not(:disabled), [href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])'
        )
      );
      if (focusableElements.length === 0) {
        return;
      }

      const firstElement = focusableElements[0];
      const lastElement = focusableElements[focusableElements.length - 1];
      if (event.shiftKey && document.activeElement === firstElement) {
        event.preventDefault();
        lastElement.focus();
      } else if (!event.shiftKey && document.activeElement === lastElement) {
        event.preventDefault();
        firstElement.focus();
      }
    },
    [closeNextEpisodeConfirmation]
  );

  useEffect(() => {
    if (!pendingNextEpisodeConfirmation) {
      return;
    }

    const frameId = window.requestAnimationFrame(() => {
      nextEpisodeConfirmPrimaryButtonRef.current?.focus();
    });
    return () => {
      window.cancelAnimationFrame(frameId);
    };
  }, [nextEpisodeConfirmPrimaryButtonRef, pendingNextEpisodeConfirmation]);

  const handleOpenBookmark = useCallback((bookmark: Bookmark) => {
    closeReaderPanel();
    readerCommands.openEpisode(bookmark.episodeIndex, bookmark.position);
  }, [closeReaderPanel, readerCommands]);

  const handleEpisodeMove = useCallback((direction: -1 | 1) => {
    if (isReaderModalOpen) {
      return;
    }

    const targetEpisode = direction < 0 ? previousEpisode : nextEpisode;
    if (!isTocEpisodeFetched(targetEpisode)) {
      return;
    }

    setIsReaderOverflowOpen(false);
    readerCommands.openEpisode(targetEpisode.episodeIndex);
  }, [isReaderModalOpen, nextEpisode, previousEpisode, readerCommands, setIsReaderOverflowOpen]);

  const handlePageMove = useCallback((direction: -1 | 1) => {
    if (!canUseReaderPageActions) {
      return;
    }

    if (direction === readerForwardPageDirection && !canMoveForwardReaderPage) {
      if (nextEpisode && !canOpenNextEpisode) {
        setReaderNotice("次の話はまだ本文が取得されていません。再開して取得してください。");
        return;
      }

      if (nextEpisode) {
        openNextEpisodeConfirmation(nextEpisode);
        return;
      }

      if (hasUnlistedEpisodes) {
        setReaderNotice("続きの話はまだ取得されていません。再開して取得してください。");
        return;
      }

      setReaderNotice("最終話の最終ページに到達しました。");
      return;
    }

    movePage(direction);
  }, [
    canMoveForwardReaderPage,
    canOpenNextEpisode,
    canUseReaderPageActions,
    hasUnlistedEpisodes,
    movePage,
    nextEpisode,
    openNextEpisodeConfirmation,
    readerForwardPageDirection,
    setReaderNotice
  ]);

  const hasReaderViewportTextSelection = useCallback(
    (viewport: HTMLDivElement): boolean => hasActiveTextSelection(document.getSelection(), viewport),
    []
  );

  const handleEdgeNavigation = useCallback(
    (target: HTMLDivElement, clientX: number) => {
      if (hasReaderViewportTextSelection(target)) {
        return;
      }

      const rect = target.getBoundingClientRect();
      const direction = resolveReaderEdgeNavigationDirection({
        viewportLeft: rect.left,
        viewportWidth: rect.width,
        clientX,
        pageMoveDirections: edgeTapPageMoveDirections
      });

      if (direction !== null) {
        handlePageMove(direction);
      }
    },
    [edgeTapPageMoveDirections, handlePageMove, hasReaderViewportTextSelection]
  );

  const handleViewportClick = useCallback(
    (event: ReactMouseEvent<HTMLDivElement>) => {
      handleEdgeNavigation(event.currentTarget, event.clientX);
    },
    [handleEdgeNavigation]
  );

  const cancelPendingReaderTouchNavigation = useCallback(() => {
    const pendingId = pendingReaderTouchNavigationRef.current;
    if (pendingId !== null) {
      window.cancelAnimationFrame(pendingId);
      pendingReaderTouchNavigationRef.current = null;
    }
    pendingReaderTouchSelectionCandidateRef.current = false;
  }, []);

  const markReaderTouchTextSelectionGesture = useCallback(() => {
    const touchGesture = readerTouchGestureRef.current;
    const isCurrentLongPress =
      touchGesture !== null && Date.now() - touchGesture.startTimeMs >= READER_EDGE_TAP_MAX_DURATION_MS;
    const isPendingLongPress =
      pendingReaderTouchNavigationRef.current !== null && pendingReaderTouchSelectionCandidateRef.current;

    if (isCurrentLongPress || isPendingLongPress) {
      readerTouchTextSelectionGestureRef.current = true;
    }
  }, []);

  // biome-ignore lint/correctness/useExhaustiveDependencies: touch selection listeners intentionally follow reader touch/content state.
  useEffect(() => {
    if (screenMode !== "reader" || !isTouchDevice) {
      return;
    }

    const viewport = readerViewportRef.current;
    if (!viewport) {
      return;
    }

    const onSelectionChange = () => {
      if (
        (readerTouchGestureRef.current || pendingReaderTouchNavigationRef.current !== null) &&
        hasReaderViewportTextSelection(viewport)
      ) {
        readerTouchTextSelectionGestureRef.current = true;
      }
    };

    viewport.addEventListener("selectstart", markReaderTouchTextSelectionGesture);
    document.addEventListener("selectionchange", onSelectionChange);

    return () => {
      cancelPendingReaderTouchNavigation();
      viewport.removeEventListener("selectstart", markReaderTouchTextSelectionGesture);
      document.removeEventListener("selectionchange", onSelectionChange);
    };
  }, [screenMode, isTouchDevice, episodeContentEtag]);

  const handleViewportTouchStart = useCallback((event: ReactTouchEvent<HTMLDivElement>) => {
    cancelPendingReaderTouchNavigation();
    readerTouchGestureRef.current = getReaderTouchGestureStart({
      firstTouch: event.touches[0],
      touchCount: event.touches.length
    });
    readerTouchTextSelectionGestureRef.current = false;
  }, [cancelPendingReaderTouchNavigation]);

  const handleViewportTouchEnd = useCallback(
    (event: ReactTouchEvent<HTMLDivElement>) => {
      const touchGesture = readerTouchGestureRef.current;
      readerTouchGestureRef.current = null;
      if (!touchGesture) {
        readerTouchTextSelectionGestureRef.current = false;
        return;
      }
      if (hasReaderViewportTextSelection(event.currentTarget)) {
        readerTouchTextSelectionGestureRef.current = false;
        return;
      }
      const changedTouch = event.changedTouches[0];
      if (!changedTouch) {
        readerTouchTextSelectionGestureRef.current = false;
        return;
      }
      const rect = event.currentTarget.getBoundingClientRect();
      const viewport = event.currentTarget;
      const endTouch = {
        clientX: changedTouch.clientX,
        clientY: changedTouch.clientY
      };
      const touchEndTimeMs = Date.now();

      cancelPendingReaderTouchNavigation();
      pendingReaderTouchSelectionCandidateRef.current =
        touchEndTimeMs - touchGesture.startTimeMs >= READER_EDGE_TAP_MAX_DURATION_MS;
      const pendingId = window.requestAnimationFrame(() => {
        if (pendingReaderTouchNavigationRef.current !== pendingId) {
          return;
        }
        pendingReaderTouchNavigationRef.current = null;
        pendingReaderTouchSelectionCandidateRef.current = false;
        const isTextSelectionGesture =
          readerTouchTextSelectionGestureRef.current || hasReaderViewportTextSelection(viewport);
        readerTouchTextSelectionGestureRef.current = false;
        const direction = resolveReaderTouchNavigationDirection({
          touchGesture,
          endTouch,
          isTextSelectionGesture,
          nowMs: touchEndTimeMs,
          viewportLeft: rect.left,
          viewportWidth: rect.width,
          swipePageMoveDirections: pageMoveDirections,
          tapPageMoveDirections: edgeTapPageMoveDirections
        });

        if (direction !== null) {
          handlePageMove(direction);
        }
      });
      pendingReaderTouchNavigationRef.current = pendingId;
    },
    [
      cancelPendingReaderTouchNavigation,
      edgeTapPageMoveDirections,
      handlePageMove,
      hasReaderViewportTextSelection,
      pageMoveDirections
    ]
  );

  const handleViewportTouchCancel = useCallback(() => {
    cancelPendingReaderTouchNavigation();
    readerTouchGestureRef.current = null;
    readerTouchTextSelectionGestureRef.current = false;
  }, [cancelPendingReaderTouchNavigation]);

  useEffect(() => {
    if (screenMode !== "reader" || isTouchDevice || isReaderKeyboardPagingBlocked || isReaderModalOpen) {
      return;
    }

    const onKeyDown = (event: KeyboardEvent) => {
      const target = event.target;
      if (
        event.defaultPrevented ||
        (target instanceof Element &&
          target.closest('input, select, textarea, button, a, [contenteditable="true"], [role="slider"]'))
      ) {
        return;
      }

      if (event.key === "ArrowLeft") {
        event.preventDefault();
        handlePageMove(pageMoveDirections.previous);
      } else if (event.key === "ArrowRight") {
        event.preventDefault();
        handlePageMove(pageMoveDirections.next);
      }
    };

    window.addEventListener("keydown", onKeyDown);
    return () => {
      window.removeEventListener("keydown", onKeyDown);
    };
  }, [screenMode, isTouchDevice, isReaderKeyboardPagingBlocked, isReaderModalOpen, pageMoveDirections, handlePageMove]);

  const handleResetReaderSpeechPreferences = useCallback(() => {
    setReaderSpeechEnabled(DEFAULT_READER_LOCAL_PREFERENCES.speechEnabled);
    setReaderSpeechRate(DEFAULT_READER_LOCAL_PREFERENCES.speechRate);
    setReaderSpeechVoiceUri(DEFAULT_READER_LOCAL_PREFERENCES.speechVoiceUri);
    setReaderSpeechPreferRubyText(DEFAULT_READER_LOCAL_PREFERENCES.speechPreferRubyText);
    setReaderSpeechDebugHighlight(DEFAULT_READER_LOCAL_PREFERENCES.speechDebugHighlight);
    void stopReaderSpeech({ notice: "読み上げ設定を初期化しました。" });
  }, [
    setReaderSpeechDebugHighlight,
    setReaderSpeechEnabled,
    setReaderSpeechPreferRubyText,
    setReaderSpeechRate,
    setReaderSpeechVoiceUri,
    stopReaderSpeech
  ]);

  const { readerOverflowActions, readerVisibleActions } = useReaderControlActions({
    canOpenNextEpisode,
    canOpenPreviousEpisode,
    canUseNextPageButton,
    canUsePreviousPageButton,
    closeReaderPanel,
    episode,
    handleCreateBookmark,
    handleEpisodeMove,
    handleOpenCharacterSummary,
    handleOpenTerms,
    handlePageMove,
    handleReturnToLibrary,
    handleToggleReaderFullscreen,
    isBookmarkSaving,
    isCharacterSummaryOpen,
    isTermsOpen,
    isEpisodeLoading,
    isReaderAiAssistantAvailable,
    isReaderAiAssistantOpen,
    isReaderAiAssistantUnavailableMessage,
    isReaderBookmarksOpen,
    isReaderExperimentalFontOpen,
    isReaderFullscreen,
    isReaderInfoOpen,
    isReaderModalOpen,
    isReaderSettingsOpen,
    isReaderSpeechOpen,
    isReaderSpeechPaused,
    isReaderSpeechPlaying,
    isReaderTocOpen,
    isTouchDevice,
    nextPageActionLabel,
    pageMoveDirections,
    previousPageActionLabel,
    readerControlViewportWidth,
    readerPageIndicatorWidth,
    setIsReaderOverflowOpen,
    shouldShowReaderSpeechControls,
    toggleReaderPanel
  });

  return {
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
  };
}
