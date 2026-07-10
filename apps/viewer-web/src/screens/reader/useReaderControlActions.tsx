import { type Dispatch, type SetStateAction, useEffect } from "react";
import type { ReaderControlAction } from "../../ReaderBottomControls";
import { getReaderControlCapacity } from "../../features/reader/controlsLayout";
import type { ReaderPageMoveDirections } from "../../features/reader/gestureNavigation";
import { useMediaQuery } from "../../hooks/useMediaQuery";
import type { ReaderPanelView } from "../../hooks/useReaderPanels";

const READER_CONTROL_INDICATOR_CLEARANCE_PX = 16;
const READER_CONTROL_COMPACT_BREAKPOINT_PX = 720;
const READER_CONTROL_TABLET_MIN_WIDTH_PX = 561;
const READER_CONTROL_TABLET_MAX_WIDTH_PX = 1100;
const READER_CONTROL_TABLET_BUTTON_SIZE_PX = 52;

type UseReaderControlActionsOptions = {
  canOpenNextEpisode: boolean;
  canOpenPreviousEpisode: boolean;
  canUseNextPageButton: boolean;
  canUsePreviousPageButton: boolean;
  closeReaderPanel: () => void;
  episode: unknown;
  handleCreateBookmark: () => Promise<void>;
  handleEpisodeMove: (direction: -1 | 1) => void;
  handleOpenCharacterSummary: () => void | Promise<void>;
  handleOpenTerms: () => void | Promise<void>;
  handlePageMove: (direction: -1 | 1) => void;
  handleReturnToLibrary: () => void | Promise<void>;
  handleToggleReaderFullscreen: () => void | Promise<void>;
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
  isReaderModalOpen: boolean;
  isReaderSettingsOpen: boolean;
  isReaderSpeechOpen: boolean;
  isReaderSpeechPaused: boolean;
  isReaderSpeechPlaying: boolean;
  isReaderTocOpen: boolean;
  isTouchDevice: boolean;
  nextPageActionLabel: string;
  pageMoveDirections: ReaderPageMoveDirections;
  previousPageActionLabel: string;
  readerControlViewportWidth: number;
  readerPageIndicatorWidth: number;
  setIsReaderOverflowOpen: Dispatch<SetStateAction<boolean>>;
  shouldShowReaderSpeechControls: boolean;
  toggleReaderPanel: (panel: Exclude<ReaderPanelView, "characters">) => void;
};

export function useReaderControlActions({
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
}: UseReaderControlActionsOptions) {
  const isReaderControlCompactViewport = useMediaQuery(`(max-width: ${READER_CONTROL_COMPACT_BREAKPOINT_PX}px)`);
  const isReaderControlTabletViewport = useMediaQuery(
    `(min-width: ${READER_CONTROL_TABLET_MIN_WIDTH_PX}px) and (max-width: ${READER_CONTROL_TABLET_MAX_WIDTH_PX}px)`
  );
  const readerControlButtonSizePx = isReaderControlTabletViewport ? READER_CONTROL_TABLET_BUTTON_SIZE_PX : undefined;
  const shouldReserveReaderIndicatorWidth = isReaderControlCompactViewport || isReaderControlTabletViewport;

  const readerControlActions: ReaderControlAction[] = [
    {
      id: "home",
      label: "一覧へ戻る",
      title: "一覧へ戻る",
      className: "reader-home-button",
      disabled: isReaderModalOpen,
      onClick: () => {
        setIsReaderOverflowOpen(false);
        void handleReturnToLibrary();
      },
      icon: (
        <svg aria-hidden="true" className="reader-home-icon" viewBox="0 0 24 24">
          <path d="M12 3 3 10h2v10h5v-6h4v6h5V10h2z" />
        </svg>
      )
    },
    {
      id: "episode-next",
      label: "次の話へ進む",
      title: "次の話へ進む",
      className: "reader-episode-next-button",
      disabled: !canOpenNextEpisode || isEpisodeLoading || isReaderModalOpen,
      onClick: () => {
        handleEpisodeMove(1);
      },
      icon: (
        <svg aria-hidden="true" className="reader-episode-next-icon" viewBox="0 0 24 24">
          <path d="m12.3 6.4-1.4-1.4-7 7 7 7 1.4-1.4L6.7 12Z" />
          <path d="m18.3 6.4-1.4-1.4-7 7 7 7 1.4-1.4-5.6-5.6Z" />
        </svg>
      )
    },
    {
      id: "page-previous",
      label: previousPageActionLabel,
      title: previousPageActionLabel,
      className: "reader-page-previous-button",
      disabled: !canUsePreviousPageButton,
      onClick: () => {
        setIsReaderOverflowOpen(false);
        handlePageMove(pageMoveDirections.previous);
      },
      icon: (
        <svg aria-hidden="true" className="reader-page-previous-icon" viewBox="0 0 24 24">
          <path d="M14.7 5.3 8 12l6.7 6.7 1.4-1.4L10.8 12l5.3-5.3Z" />
        </svg>
      )
    },
    {
      id: "page-next",
      label: nextPageActionLabel,
      title: nextPageActionLabel,
      className: "reader-page-next-button",
      disabled: !canUseNextPageButton,
      onClick: () => {
        setIsReaderOverflowOpen(false);
        handlePageMove(pageMoveDirections.next);
      },
      icon: (
        <svg aria-hidden="true" className="reader-page-next-icon" viewBox="0 0 24 24">
          <path d="m9.3 5.3-1.4 1.4 5.3 5.3-5.3 5.3 1.4 1.4L16 12Z" />
        </svg>
      )
    },
    {
      id: "episode-previous",
      label: "前の話へ戻る",
      title: "前の話へ戻る",
      className: "reader-episode-previous-button",
      disabled: !canOpenPreviousEpisode || isEpisodeLoading || isReaderModalOpen,
      onClick: () => {
        handleEpisodeMove(-1);
      },
      icon: (
        <svg aria-hidden="true" className="reader-episode-previous-icon" viewBox="0 0 24 24">
          <path d="m11.7 6.4 1.4-1.4 7 7-7 7-1.4-1.4 5.6-5.6Z" />
          <path d="m5.7 6.4 1.4-1.4 7 7-7 7-1.4-1.4 5.6-5.6Z" />
        </svg>
      )
    },
    {
      id: "toc",
      label: "目次",
      title: "目次",
      className: "reader-toc-button",
      ariaExpanded: isReaderTocOpen,
      onClick: () => toggleReaderPanel("toc"),
      icon: (
        <svg aria-hidden="true" className="reader-toc-icon" viewBox="0 0 24 24">
          <path d="M4 5.5A1.5 1.5 0 0 1 5.5 4h1A1.5 1.5 0 0 1 8 5.5v1A1.5 1.5 0 0 1 6.5 8h-1A1.5 1.5 0 0 1 4 6.5Zm4.8-.5h11.2v2H8.8Zm0 5h11.2v2H8.8ZM4 11.5A1.5 1.5 0 0 1 5.5 10h1A1.5 1.5 0 0 1 8 11.5v1A1.5 1.5 0 0 1 6.5 14h-1A1.5 1.5 0 0 1 4 12.5Zm0 6A1.5 1.5 0 0 1 5.5 16h1A1.5 1.5 0 0 1 8 17.5v1A1.5 1.5 0 0 1 6.5 20h-1A1.5 1.5 0 0 1 4 18.5Zm4.8-.5h11.2v2H8.8Z" />
        </svg>
      )
    },
    {
      id: "bookmark",
      label: isTouchDevice ? "栞" : "栞を追加",
      title: isTouchDevice ? "栞一覧と追加" : "栞を追加",
      className: "reader-bookmark-button",
      ariaExpanded: isTouchDevice ? isReaderBookmarksOpen : undefined,
      disabled: !isTouchDevice && (!episode || isBookmarkSaving),
      onClick: () => {
        setIsReaderOverflowOpen(false);
        if (isTouchDevice) {
          toggleReaderPanel("bookmarks");
          return;
        }

        void handleCreateBookmark();
      },
      icon: (
        <svg aria-hidden="true" className="reader-bookmark-icon" viewBox="0 0 24 24">
          <path d="M6 3.5A2.5 2.5 0 0 1 8.5 1h7A2.5 2.5 0 0 1 18 3.5V23l-6-4.25L6 23Z" />
        </svg>
      )
    },
    ...(shouldShowReaderSpeechControls
      ? [
          {
            id: "speech",
            label: "読み上げ",
            title: "読み上げ",
            className: `reader-speech-button${isReaderSpeechOpen || isReaderSpeechPlaying || isReaderSpeechPaused ? " is-active" : ""}`,
            ariaExpanded: isReaderSpeechOpen,
            onClick: () => {
              toggleReaderPanel("speech");
            },
            icon: (
              <svg aria-hidden="true" className="reader-speech-icon" viewBox="0 0 24 24">
                <path d="M5.5 9A1.5 1.5 0 0 1 7 7.5h2.34l4.43-3.54A1 1 0 0 1 15.4 4.7v14.6a1 1 0 0 1-1.63.78L9.34 16.5H7A1.5 1.5 0 0 1 5.5 15Zm10.9-1.43a1 1 0 0 1 1.4-.15 6.32 6.32 0 0 1 0 9.16 1 1 0 1 1-1.26-1.55 4.32 4.32 0 0 0 0-6.06 1 1 0 0 1-.14-1.4Zm2.95-2.92a1 1 0 0 1 1.39-.11 10.17 10.17 0 0 1 0 14.92 1 1 0 0 1-1.28-1.54 8.17 8.17 0 0 0 0-11.84 1 1 0 0 1-.11-1.43Z" />
              </svg>
            )
          }
        ]
      : []),
    {
      id: "settings",
      label: "読書設定",
      title: "読書設定",
      className: "reader-settings-button",
      ariaExpanded: isReaderSettingsOpen,
      onClick: () => toggleReaderPanel("settings"),
      icon: (
        <svg aria-hidden="true" className="reader-settings-icon" viewBox="0 0 24 24">
          <path d="m19.14 12.94.02-.94-.02-.94 1.86-1.46a.6.6 0 0 0 .15-.77l-1.77-3.06a.6.6 0 0 0-.73-.26l-2.2.89a7.6 7.6 0 0 0-1.62-.94L14.5 3.1a.6.6 0 0 0-.59-.5h-3.54a.6.6 0 0 0-.59.5l-.33 2.36c-.57.23-1.11.54-1.62.94l-2.2-.89a.6.6 0 0 0-.73.26L3.13 8.83a.6.6 0 0 0 .15.77l1.86 1.46-.02.94.02.94-1.86 1.46a.6.6 0 0 0-.15.77l1.77 3.06c.15.26.46.37.73.26l2.2-.89c.5.4 1.05.71 1.62.94l.33 2.36c.04.29.29.5.59.5h3.54c.3 0 .55-.21.59-.5l.33-2.36c.57-.23 1.11-.54 1.62-.94l2.2.89a.6.6 0 0 0 .73-.26l1.77-3.06a.6.6 0 0 0-.15-.77zm-6.99 2.56a3.5 3.5 0 1 1 0-7 3.5 3.5 0 0 1 0 7z" />
        </svg>
      )
    },
    {
      id: "experimental-font",
      label: "実験フォント",
      title: "実験フォント",
      className: "reader-experimental-font-button",
      ariaExpanded: isReaderExperimentalFontOpen,
      onClick: () => toggleReaderPanel("experimental-font"),
      icon: (
        <svg aria-hidden="true" className="reader-experimental-font-icon" viewBox="0 0 24 24">
          <path d="M6 19h2.4l1.24-3.4h4.72L15.6 19H18L13.2 6h-2.4Zm4.39-5.33L12 9.24l1.61 4.43Z" />
        </svg>
      )
    },
    {
      id: "characters",
      label: "キャラクター一覧",
      title: "キャラクター一覧",
      className: "reader-characters-button",
      ariaExpanded: isCharacterSummaryOpen,
      onClick: () => {
        setIsReaderOverflowOpen(false);
        if (isCharacterSummaryOpen) {
          closeReaderPanel();
          return;
        }

        void handleOpenCharacterSummary();
      },
      icon: (
        <svg aria-hidden="true" className="reader-characters-icon" viewBox="0 0 24 24">
          <path d="M8.5 11.5a3.25 3.25 0 1 1 0-6.5 3.25 3.25 0 0 1 0 6.5Zm7 0a2.75 2.75 0 1 1 0-5.5 2.75 2.75 0 0 1 0 5.5Zm-7 2c3.06 0 5.5 1.64 5.5 3.67V20H3v-2.83c0-2.03 2.44-3.67 5.5-3.67Zm7 1.1c2.48 0 4.5 1.23 4.5 2.74V20h-4.9v-2.96c0-.57-.14-1.1-.41-1.54.24-.04.5-.06.81-.06Z" />
        </svg>
      )
    },
    {
      id: "terms",
      label: "用語一覧",
      title: "用語一覧",
      className: "reader-terms-button",
      ariaExpanded: isTermsOpen,
      onClick: () => {
        setIsReaderOverflowOpen(false);
        if (isTermsOpen) {
          closeReaderPanel();
          return;
        }
        void handleOpenTerms();
      },
      icon: (
        <svg aria-hidden="true" className="reader-terms-icon" viewBox="0 0 24 24">
          <path d="M5 3.5h11.5A2.5 2.5 0 0 1 19 6v14.5H7A3 3 0 0 1 4 17.5v-11A3 3 0 0 1 7 3.5Zm2 2A1 1 0 0 0 6 6.5v8.17c.31-.11.65-.17 1-.17h10V6a.5.5 0 0 0-.5-.5Zm0 11a1 1 0 0 0 0 2h10v-2Zm2-8h6v1.5H9Zm0 3h5v1.5H9Z" />
        </svg>
      )
    },
    {
      id: "reader-ai",
      label: "読書AI",
      title: isReaderAiAssistantUnavailableMessage ?? "読書AI",
      className: "reader-ai-button",
      ariaExpanded: isReaderAiAssistantOpen,
      disabled: !isReaderAiAssistantAvailable,
      onClick: () => toggleReaderPanel("reader-ai"),
      icon: (
        <svg aria-hidden="true" className="reader-ai-icon" viewBox="0 0 24 24">
          <path d="M18.15 2.05 19 3.9l1.85.85L19 5.6l-.85 1.85-.85-1.85-1.85-.85 1.85-.85ZM5.7 8.4h1.35v5.2H5.7a2.6 2.6 0 0 1 0-5.2Zm11.25 0h1.35a2.6 2.6 0 0 1 0 5.2h-1.35Z" />
          <path
            clipRule="evenodd"
            d="M11 3h2v2.05h-2ZM8.2 5.65h7.6a3.4 3.4 0 0 1 3.4 3.4v5.4a3.4 3.4 0 0 1-3.4 3.4h-1.05l-2.2 2.45a.75.75 0 0 1-1.1 0l-2.2-2.45H8.2a3.4 3.4 0 0 1-3.4-3.4v-5.4a3.4 3.4 0 0 1 3.4-3.4Zm.3 2.15a1.55 1.55 0 0 0-1.55 1.55v4.8A1.55 1.55 0 0 0 8.5 15.7h7a1.55 1.55 0 0 0 1.55-1.55v-4.8A1.55 1.55 0 0 0 15.5 7.8Zm.55 3.25a1.25 1.25 0 1 1 2.5 0 1.25 1.25 0 0 1-2.5 0Zm5.9 0a1.25 1.25 0 1 0-2.5 0 1.25 1.25 0 0 0 2.5 0Zm-5.35 3.15h4.8v1.35H9.6Z"
            fillRule="evenodd"
          />
        </svg>
      )
    },
    {
      id: "info",
      label: "情報",
      title: "情報",
      className: "reader-info-button",
      ariaExpanded: isReaderInfoOpen,
      onClick: () => toggleReaderPanel("info"),
      icon: (
        <svg aria-hidden="true" className="reader-info-icon" viewBox="0 0 24 24">
          <path d="M12 2a10 10 0 1 0 10 10A10 10 0 0 0 12 2Zm0 4.2a1.3 1.3 0 1 1-1.3 1.3A1.3 1.3 0 0 1 12 6.2Zm1.7 11.6h-3.4v-1.7h.85v-4.25h-.85v-1.7h2.55v5.95h.85Z" />
        </svg>
      )
    },
    {
      id: "fullscreen",
      label: isReaderFullscreen ? "フルスクリーン解除" : "フルスクリーン表示",
      title: isReaderFullscreen ? "フルスクリーン解除" : "フルスクリーン表示",
      className: "reader-fullscreen-button",
      onClick: () => {
        setIsReaderOverflowOpen(false);
        void handleToggleReaderFullscreen();
      },
      icon: (
        <svg aria-hidden="true" className="reader-fullscreen-icon" viewBox="0 0 24 24">
          {isReaderFullscreen ? (
            <path d="M6.7 5.3 12 10.6l5.3-5.3 1.4 1.4-5.3 5.3 5.3 5.3-1.4 1.4-5.3-5.3-5.3 5.3-1.4-1.4 5.3-5.3-5.3-5.3Z" />
          ) : (
            <path d="M5 5h6v2H7v4H5Zm12 0v6h-2V7h-4V5ZM5 13h2v4h4v2H5Zm12 4v-4h2v6h-6v-2Z" />
          )}
        </svg>
      )
    }
  ];
  const readerControlCapacity = Math.max(
    1,
    Math.min(
      getReaderControlCapacity(
        readerControlViewportWidth,
        isReaderControlCompactViewport,
        shouldReserveReaderIndicatorWidth ? readerPageIndicatorWidth + READER_CONTROL_INDICATOR_CLEARANCE_PX : 0,
        readerControlButtonSizePx
      ),
      readerControlActions.length
    )
  );
  const hasReaderOverflow = readerControlCapacity < readerControlActions.length;
  const readerVisibleActions = hasReaderOverflow
    ? readerControlActions.slice(0, Math.max(readerControlCapacity - 1, 1))
    : readerControlActions;
  const readerOverflowActions = hasReaderOverflow
    ? readerControlActions.slice(Math.max(readerControlCapacity - 1, 1))
    : [];

  useEffect(() => {
    if (readerOverflowActions.length === 0) {
      setIsReaderOverflowOpen(false);
    }
  }, [readerOverflowActions.length, setIsReaderOverflowOpen]);

  return {
    readerOverflowActions,
    readerVisibleActions
  };
}
