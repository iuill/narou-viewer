import type {
  Dispatch,
  KeyboardEvent as ReactKeyboardEvent,
  MouseEvent as ReactMouseEvent,
  MutableRefObject,
  PointerEvent as ReactPointerEvent,
  RefObject,
  SetStateAction,
  TouchEvent as ReactTouchEvent
} from "react";
import { ListPaginationControls } from "../../ListPaginationControls";
import { ReaderAiAssistantPanel, type ReaderAiAssistantState } from "../../ReaderAiAssistantPanel";
import { ReaderBookmarkPanel } from "../../ReaderBookmarkPanel";
import { ReaderBottomControls, type ReaderControlAction } from "../../ReaderBottomControls";
import { ReaderCharacterSummaryPanel } from "../../ReaderCharacterSummaryPanel";
import { ReaderTermListPanel } from "../../ReaderTermListPanel";
import { ReaderExperimentalFontPanel } from "../../ReaderExperimentalFontPanel";
import { ReaderFloatingPanel } from "../../ReaderFloatingPanel";
import { ReaderImageViewer, type ImageViewerState } from "../../ReaderImageViewer";
import { ReaderInfoPanel } from "../../ReaderInfoPanel";
import { ReaderPager } from "../../ReaderPager";
import { ReaderSettingsPanel } from "../../ReaderSettingsPanel";
import { ReaderSpeechPanel } from "../../ReaderSpeechPanel";
import { ReaderSyncConflictPanel } from "../../ReaderSyncConflictPanel";
import type { ApiClientUpdateRequiredEventDetail } from "../../api/contract";
import type { CharacterSummaryResponse } from "../characters/types";
import type { ExtractionGenerationStrategy, ExtractionJobSummary } from "../extraction/types";
import type { TermsResponse } from "../terms/types";
import type { NovelSummary } from "../library/types";
import type { ReaderSpeechChunk, ReaderSpeechVoiceOption } from "../../readerSpeech";
import type { ReaderExperimentalFontId, ReaderExperimentalFontWeight, ReadingMode } from "../../readerPreferences";
import type { ReaderSyncConflict, ReaderSyncConflictResolutionState } from "../../hooks/useReaderState";
import {
  buildEpisodeLabel,
  formatEpisodeIndexLabel,
  type EpisodeDisplayReference
} from "./episodeLabels";
import type { PaginationResult } from "../library/pagination";
import { ReaderShell } from "./ReaderShell";
import type {
  Bookmark,
  EpisodeIndex,
  EpisodeResponse,
  NovelReaderSettingsResponse,
  ReaderFontFamily,
  ReaderTheme,
  TocEpisode,
  TocResponse
} from "./types";
import type { ReaderSessionCommands as ReaderSelectionCommands } from "./useReaderSessionCommands";
import type { ReaderSessionCommands as ReaderStateCommands } from "./useReaderSession";

type ReaderExperimentalFontLoadStatus = "idle" | "loading" | "ready" | "error";

export type ReaderScreenState = {
  activeReaderSettings: NovelReaderSettingsResponse | null;
  bookmarks: Bookmark[];
  canApplyReaderSyncConflict: boolean;
  characterSummaryActiveJobs: ExtractionJobSummary[];
  characterSummaryCanClear: boolean;
  characterSummaryCanGenerate: boolean;
  characterSummaryCompletedJobs: ExtractionJobSummary[];
  characterSummaryData: CharacterSummaryResponse | null;
  characterSummaryDefaultUpToEpisodeIndex: EpisodeIndex | null;
  characterSummaryError: string | null;
  characterSummaryNotice: string | null;
  characterSummaryGenerationStrategy: ExtractionGenerationStrategy;
  characterSummaryUpToEpisodeIndex: string;
  clientUpdateRequired: ApiClientUpdateRequiredEventDetail | null;
  currentNovel: NovelSummary | null;
  debugPageOverflow: boolean;
  displayedPageNumber: number;
  episode: EpisodeResponse | null;
  episodeDisplayLookup: Map<EpisodeIndex, EpisodeDisplayReference>;
  error: string | null;
  hasStableReaderEpisode: boolean;
  imageViewer: ImageViewerState | null;
  imageViewerWidth: number | null;
  imageViewerZoomPercent: number;
  isBookmarkSaving: boolean;
  isCharacterSummaryClearing: boolean;
  isCharacterSummaryLoading: boolean;
  isCharacterSummaryOpen: boolean;
  isCharacterSummarySubmitting: boolean;
  isTermsOpen: boolean;
  isEpisodeLoading: boolean;
  isImageViewerDragging: boolean;
  isImageViewerInfoOpen: boolean;
  isReaderAiAssistantOpen: boolean;
  isReaderBookmarksOpen: boolean;
  isReaderCorrectionUnavailable: boolean;
  isReaderExperimentalFontOpen: boolean;
  isReaderFullscreen: boolean;
  isReaderInfoOpen: boolean;
  isReaderLoadingOverlayVisible: boolean;
  isReaderOverflowOpen: boolean;
  isReaderPseudoFullscreen: boolean;
  isReaderSettingsOpen: boolean;
  isReaderSpeechOpen: boolean;
  isReaderSpeechPaused: boolean;
  isReaderSpeechPlaying: boolean;
  isReaderSpeechSupported: boolean;
  isReaderTocOpen: boolean;
  isShowingAllBookmarks: boolean;
  isTouchDevice: boolean;
  isWebKitEngine: boolean;
  pendingBookmarkId: string | null;
  pendingNextEpisodeConfirmation: TocEpisode | null;
  preferFriendlyEpisodeLabels: boolean;
  readerAiAssistantState: ReaderAiAssistantState;
  readerAiAssistantUnavailableMessage: string | null;
  readerArticleFontFamilyCss: string;
  readerArticleFontWeight: ReaderExperimentalFontWeight | null;
  readerExperimentalFontId: ReaderExperimentalFontId;
  readerExperimentalFontLoadStatus: ReaderExperimentalFontLoadStatus;
  readerExperimentalFontWeight: ReaderExperimentalFontWeight;
  readerFontFamily: ReaderFontFamily;
  readerFontSizePx: number;
  readerInfoEpisodeReferenceLabel: string;
  readerInfoEpisodeTitle: string;
  readerInfoPageLabel: string;
  readerInfoSourceUrl: string | null;
  readerInfoUpdatedAtLabel: string;
  readerLetterSpacingEm: number;
  readerLoadingEpisodeTitle: string;
  readerNotice: string | null;
  readerOverflowActions: ReaderControlAction[];
  readerSpeechActiveChunkIndex: number | null;
  readerSpeechChunks: ReaderSpeechChunk[];
  readerSpeechDebugHighlight: boolean;
  readerSpeechEnabled: boolean;
  readerSpeechPreferRubyText: boolean;
  readerSpeechRate: number;
  readerSpeechVoiceUri: string | null;
  readerSpeechVoices: ReaderSpeechVoiceOption[];
  readerSyncConflict: ReaderSyncConflict | null;
  readerSyncConflictApplyDisabledReason: string | null;
  readerSyncConflictEpisodeLabel: string | null;
  readerSyncConflictResolutionError: string | null;
  readerSyncConflictResolutionState: ReaderSyncConflictResolutionState;
  readerTheme: ReaderTheme;
  readerVisibleActions: ReaderControlAction[];
  readingMode: ReadingMode;
  renderedEpisodeHtml: string;
  reverseTapPageNavigation: boolean;
  selectedEpisodeIndex: EpisodeIndex | null;
  selectedNovelId: string | null;
  sourceNovelTitle: string;
  toc: TocResponse | null;
  termsData: TermsResponse | null;
  tocPagination: PaginationResult<TocEpisode>;
  totalPages: number;
  verticalLastPageReservePx: number;
  visibleBookmarks: Bookmark[];
  visibleTocEpisodes: TocEpisode[];
};

export type ReaderScreenCommands = {
  closeActiveReaderPanel: () => void;
  closeImageViewer: () => void;
  closeReaderPanel: () => void;
  formatCharacterSummaryEpisodeOrder: (episodeIndex: EpisodeIndex) => string;
  formatDate: (value: string | null) => string;
  getCurrentReaderViewportPosition: () => number | null;
  handleApplyReaderSyncConflict: () => Promise<void>;
  handleClearCharacterSummary: () => void | Promise<void>;
  handleConfirmNextEpisode: () => void;
  handleCreateBookmark: () => Promise<void>;
  handleDeleteBookmark: (bookmarkId: string) => Promise<void>;
  handleGenerateCharacterSummary: () => void | Promise<void>;
  handleImageViewerPointerDown: (event: ReactPointerEvent<HTMLDivElement>) => void;
  handleImageViewerPointerMove: (event: ReactPointerEvent<HTMLDivElement>) => void;
  handleImageViewerPointerUp: (event: ReactPointerEvent<HTMLDivElement>) => void;
  handleNextEpisodeConfirmationBackdropClick: () => void;
  handleNextEpisodeConfirmationCloseClick: () => void;
  handleNextEpisodeConfirmationKeyDown: (event: ReactKeyboardEvent<HTMLElement>) => void;
  handleOpenBookmark: (bookmark: Bookmark) => void;
  handleOverwriteReaderSyncConflict: () => Promise<void>;
  handleReaderSpeechPause: () => Promise<void>;
  handleReaderSpeechPlay: () => Promise<void>;
  handleReaderSpeechResume: () => Promise<void>;
  handleResetReaderPreferences: () => void;
  handleResetReaderSpeechPreferences: () => void;
  handleRetryReaderExperimentalFontLoad: () => void;
  handleViewportClick: (event: ReactMouseEvent<HTMLDivElement>) => void;
  handleViewportTouchCancel: () => void;
  handleViewportTouchEnd: (event: ReactTouchEvent<HTMLDivElement>) => void;
  handleViewportTouchStart: (event: ReactTouchEvent<HTMLDivElement>) => void;
  readerCommands: ReaderSelectionCommands;
  readerSessionCommands: ReaderStateCommands;
  setCharacterSummaryGenerationStrategy: Dispatch<SetStateAction<ExtractionGenerationStrategy>>;
  setCharacterSummaryUpToEpisodeIndex: Dispatch<SetStateAction<string>>;
  setDebugPageOverflow: Dispatch<SetStateAction<boolean>>;
  setError: Dispatch<SetStateAction<string | null>>;
  setImageViewerZoomPercent: Dispatch<SetStateAction<number>>;
  setIsImageViewerInfoOpen: Dispatch<SetStateAction<boolean>>;
  setIsReaderOverflowOpen: Dispatch<SetStateAction<boolean>>;
  setIsShowingAllBookmarks: Dispatch<SetStateAction<boolean>>;
  setReaderAiAssistantState: Dispatch<SetStateAction<ReaderAiAssistantState>>;
  setReaderExperimentalFontId: Dispatch<SetStateAction<ReaderExperimentalFontId>>;
  setReaderExperimentalFontWeight: Dispatch<SetStateAction<ReaderExperimentalFontWeight>>;
  setReaderFontFamily: Dispatch<SetStateAction<ReaderFontFamily>>;
  setReaderFontSizePx: Dispatch<SetStateAction<number>>;
  setReaderLetterSpacingEm: Dispatch<SetStateAction<number>>;
  setReaderSpeechDebugHighlight: Dispatch<SetStateAction<boolean>>;
  setReaderSpeechEnabled: Dispatch<SetStateAction<boolean>>;
  setReaderSpeechPreferRubyText: Dispatch<SetStateAction<boolean>>;
  setReaderSpeechRate: Dispatch<SetStateAction<number>>;
  setReaderSpeechVoiceUri: Dispatch<SetStateAction<string | null>>;
  setReaderTheme: Dispatch<SetStateAction<ReaderTheme>>;
  setReadingMode: Dispatch<SetStateAction<ReadingMode>>;
  setReverseTapPageNavigation: Dispatch<SetStateAction<boolean>>;
  setTocPage: Dispatch<SetStateAction<number>>;
  stopReaderSpeech: (options?: { notice?: string | null }) => Promise<void>;
};

export type ReaderScreenRefs = {
  imageViewerStageRef: RefObject<HTMLDivElement | null>;
  nextEpisodeConfirmPrimaryButtonRef: RefObject<HTMLButtonElement | null>;
  readerControlsRef: RefObject<HTMLDivElement | null>;
  readerOverflowRef: RefObject<HTMLDivElement | null>;
  readerPageIndicatorRef: RefObject<HTMLParagraphElement | null>;
  readerPanelRef: RefObject<HTMLElement | null>;
  readerShellRef: RefObject<HTMLElement | null>;
  readerViewportRef: RefObject<HTMLDivElement | null>;
  selectedPositionRef: MutableRefObject<number | null>;
};

type ReaderScreenProps = {
  commands: ReaderScreenCommands;
  refs: ReaderScreenRefs;
  state: ReaderScreenState;
};

export function ReaderScreen(props: ReaderScreenProps) {
  const flatProps = { ...props.state, ...props.commands, ...props.refs };
  const {
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
    closeActiveReaderPanel,
    closeImageViewer,
    closeReaderPanel,
    currentNovel,
    debugPageOverflow,
    displayedPageNumber,
    episode,
    episodeDisplayLookup,
    error,
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
    hasStableReaderEpisode,
    imageViewer,
    imageViewerStageRef,
    imageViewerWidth,
    imageViewerZoomPercent,
    isBookmarkSaving,
    isCharacterSummaryClearing,
    isCharacterSummaryLoading,
    isCharacterSummaryOpen,
    isCharacterSummarySubmitting,
    isTermsOpen,
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
    nextEpisodeConfirmPrimaryButtonRef,
    pendingBookmarkId,
    pendingNextEpisodeConfirmation,
    preferFriendlyEpisodeLabels,
    readerAiAssistantState,
    readerAiAssistantUnavailableMessage,
    readerArticleFontFamilyCss,
    readerArticleFontWeight,
    readerCommands,
    readerControlsRef,
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
    readerOverflowRef,
    readerPageIndicatorRef,
    readerPanelRef,
    readerSessionCommands,
    readerShellRef,
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
    readerViewportRef,
    readerVisibleActions,
    readingMode,
    renderedEpisodeHtml,
    reverseTapPageNavigation,
    selectedEpisodeIndex,
    selectedNovelId,
    selectedPositionRef,
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
    sourceNovelTitle,
    stopReaderSpeech,
    toc,
    termsData,
    tocPagination,
    totalPages,
    verticalLastPageReservePx,
    visibleBookmarks,
    visibleTocEpisodes,
  } = flatProps;

  return (
    <ReaderShell
      isFullscreen={isReaderFullscreen}
      isPseudoFullscreen={isReaderPseudoFullscreen}
      isWebKitEngine={isWebKitEngine}
      ref={readerShellRef}
      theme={readerTheme}
    >
      <section className="reader-stage reader-stage-minimal">
        <ReaderBottomControls
          controlsRef={readerControlsRef}
          isOverflowOpen={isReaderOverflowOpen}
          onToggleOverflow={() => {
            setIsReaderOverflowOpen((current: boolean) => !current);
            closeActiveReaderPanel();
          }}
          overflowActions={readerOverflowActions}
          overflowRef={readerOverflowRef}
          visibleActions={readerVisibleActions}
        />
        {isReaderTocOpen ? (
          <ReaderFloatingPanel
            ariaLabel="本文画面の目次"
            className="reader-toc-panel reader-overlay-panel--toc"
            description={toc ? `${toc.title} / ${tocPagination.totalItems} 話` : "作品情報を読み込み中..."}
            onClose={closeReaderPanel}
            ref={readerPanelRef}
            title="目次"
          >
            {toc ? (
              <>
                <ListPaginationControls
                  currentPage={tocPagination.currentPage}
                  endItemNumber={tocPagination.endItemNumber}
                  label="本文画面の話一覧"
                  onPageChange={setTocPage}
                  startItemNumber={tocPagination.startItemNumber}
                  totalItems={tocPagination.totalItems}
                  totalPages={tocPagination.totalPages}
                />
                {tocPagination.totalItems === 0 ? (
                  <p className="message">話データがありません。</p>
                ) : (
                  <div className="toc-list reader-toc-list">
                    {visibleTocEpisodes.map((tocEpisode: (typeof visibleTocEpisodes)[number]) => {
                      const isFetched = !tocEpisode.bodyStatus || tocEpisode.bodyStatus === "complete";
                      const statusLabel = isFetched ? (tocEpisode.updatedAt ? formatDate(tocEpisode.updatedAt) : "更新日未取得") : "未取得";

                      return (
                        <button
                          data-reader-panel-item="toc-episode"
                          data-episode-index={tocEpisode.episodeIndex}
                          key={`${tocEpisode.episodeIndex}-${tocEpisode.contentEtag}`}
                          className={`reader-panel-list-item reader-toc-entry ${
                            selectedEpisodeIndex === tocEpisode.episodeIndex ? "selected" : ""
                          } ${isFetched ? "" : "unfetched"}`}
                          disabled={!isFetched}
                          onClick={() => {
                            closeReaderPanel();
                            readerCommands.openEpisode(tocEpisode.episodeIndex);
                          }}
                          type="button"
                        >
                          <span className="reader-panel-list-item-meta reader-toc-index">
                            {formatEpisodeIndexLabel(tocEpisode.episodeIndex, episodeDisplayLookup, preferFriendlyEpisodeLabels)}
                          </span>
                          <strong className="reader-panel-list-item-title">{buildEpisodeLabel(tocEpisode)}</strong>
                          <span className="reader-panel-list-item-meta reader-toc-status">{statusLabel}</span>
                        </button>
                      );
                    })}
                  </div>
                )}
              </>
            ) : (
              <p className="message">目次を読み込み中...</p>
            )}
          </ReaderFloatingPanel>
        ) : null}
        {isReaderBookmarksOpen ? (
          <ReaderBookmarkPanel
            bookmarks={bookmarks}
            currentEpisodeLabel={readerInfoEpisodeReferenceLabel}
            currentEpisodeTitle={readerInfoEpisodeTitle}
            episodeDisplayLookup={episodeDisplayLookup}
            isBookmarkSaving={isBookmarkSaving}
            isEpisodeLoaded={Boolean(episode)}
            isShowingAllBookmarks={isShowingAllBookmarks}
            onClose={closeReaderPanel}
            onCreateBookmark={() => {
              void handleCreateBookmark();
            }}
            onDeleteBookmark={(bookmarkId) => {
              void handleDeleteBookmark(bookmarkId);
            }}
            onOpenBookmark={handleOpenBookmark}
            onToggleAllBookmarks={() => setIsShowingAllBookmarks((current: boolean) => !current)}
            pendingBookmarkId={pendingBookmarkId}
            preferFriendlyEpisodeLabels={preferFriendlyEpisodeLabels}
            ref={readerPanelRef}
            visibleBookmarks={visibleBookmarks}
          />
        ) : null}
        {isReaderSpeechOpen ? (
          <section ref={readerPanelRef}>
            <ReaderSpeechPanel
              hasActiveChunk={readerSpeechActiveChunkIndex !== null}
              hasSpeechContent={readerSpeechChunks.length > 0}
              isPaused={isReaderSpeechPaused}
              isPlaying={isReaderSpeechPlaying}
              isSpeechSupported={isReaderSpeechSupported}
              onClose={closeReaderPanel}
              onPause={() => {
                void handleReaderSpeechPause();
              }}
              onPlay={() => {
                void handleReaderSpeechPlay();
              }}
              onReset={handleResetReaderSpeechPreferences}
              onResume={() => {
                void handleReaderSpeechResume();
              }}
              onSpeechDebugHighlightChange={setReaderSpeechDebugHighlight}
              onSpeechEnabledChange={setReaderSpeechEnabled}
              onSpeechPreferRubyTextChange={setReaderSpeechPreferRubyText}
              onSpeechRateChange={setReaderSpeechRate}
              onSpeechVoiceUriChange={setReaderSpeechVoiceUri}
              onStop={() => {
                void stopReaderSpeech();
              }}
              speechDebugHighlight={readerSpeechDebugHighlight}
              speechEnabled={readerSpeechEnabled}
              speechPreferRubyText={readerSpeechPreferRubyText}
              speechRate={readerSpeechRate}
              speechVoiceUri={readerSpeechVoiceUri}
              speechVoices={readerSpeechVoices}
            />
          </section>
        ) : null}
        {isReaderSettingsOpen ? (
          <section ref={readerPanelRef}>
            <ReaderSettingsPanel
              onClose={closeReaderPanel}
              onDebugPageOverflowChange={setDebugPageOverflow}
              onHalfwidthAlnumPunctuationNormalizationChange={(enabled) =>
                readerSessionCommands.changeNovelReaderCorrection({
                  halfwidthAlnumPunctuationNormalization: enabled
                })
              }
              onHyphenDashNormalizationChange={(enabled) =>
                readerSessionCommands.changeNovelReaderCorrection({
                  hyphenDashNormalization: enabled
                })
              }
              onParenthesisNormalizationChange={(enabled) =>
                readerSessionCommands.changeNovelReaderCorrection({
                  parenthesisNormalization: enabled
                })
              }
              onQuoteNormalizationChange={(enabled) =>
                readerSessionCommands.changeNovelReaderCorrection({
                  quoteNormalization: enabled
                })
              }
              onReaderFontFamilyChange={setReaderFontFamily}
              onReaderFontSizeChange={setReaderFontSizePx}
              onReaderLetterSpacingChange={setReaderLetterSpacingEm}
              onReaderThemeChange={setReaderTheme}
              onReadingModeChange={setReadingMode}
              onReset={() => {
                if (isReaderCorrectionUnavailable) {
                  return;
                }
                handleResetReaderPreferences();
                readerSessionCommands.changeNovelReaderCorrection({
                  quoteNormalization: true,
                  hyphenDashNormalization: true,
                  parenthesisNormalization: true,
                  halfwidthAlnumPunctuationNormalization: true
                });
              }}
              onReverseTapPageNavigationChange={setReverseTapPageNavigation}
              debugPageOverflow={debugPageOverflow}
              halfwidthAlnumPunctuationNormalizationEnabled={
                activeReaderSettings?.correction.halfwidthAlnumPunctuationNormalization ?? true
              }
              hyphenDashNormalizationEnabled={activeReaderSettings?.correction.hyphenDashNormalization ?? true}
              isReaderCorrectionSaving={isReaderCorrectionUnavailable}
              parenthesisNormalizationEnabled={activeReaderSettings?.correction.parenthesisNormalization ?? true}
              quoteNormalizationEnabled={activeReaderSettings?.correction.quoteNormalization ?? true}
              readerFontFamily={readerFontFamily}
              readerFontSizePx={readerFontSizePx}
              readerLetterSpacingEm={readerLetterSpacingEm}
              readerTheme={readerTheme}
              readingMode={readingMode}
              reverseTapPageNavigation={reverseTapPageNavigation}
            />
          </section>
        ) : null}
        {isReaderExperimentalFontOpen ? (
          <section ref={readerPanelRef}>
            <ReaderExperimentalFontPanel
              loadStatus={readerExperimentalFontLoadStatus}
              onClose={closeReaderPanel}
              onReaderExperimentalFontChange={setReaderExperimentalFontId}
              onReaderExperimentalFontWeightChange={setReaderExperimentalFontWeight}
              onRetryRemoteFontLoad={handleRetryReaderExperimentalFontLoad}
              previewFontFamilyCss={readerArticleFontFamilyCss}
              previewFontWeight={readerArticleFontWeight}
              readerExperimentalFontId={readerExperimentalFontId}
              readerExperimentalFontWeight={readerExperimentalFontWeight}
            />
          </section>
        ) : null}
        {isReaderInfoOpen ? (
          <ReaderInfoPanel
            currentNovel={currentNovel}
            episodeReferenceLabel={readerInfoEpisodeReferenceLabel}
            episodeTitle={readerInfoEpisodeTitle}
            onClose={closeReaderPanel}
            pageLabel={readerInfoPageLabel}
            ref={readerPanelRef}
            sourceUrl={readerInfoSourceUrl}
            updatedAtLabel={readerInfoUpdatedAtLabel}
          />
        ) : null}
        {isCharacterSummaryOpen ? (
          <section ref={readerPanelRef}>
            <ReaderCharacterSummaryPanel
              activeJobs={characterSummaryActiveJobs}
              canClear={characterSummaryCanClear}
              canGenerate={characterSummaryCanGenerate}
              completedJobs={characterSummaryCompletedJobs}
              data={characterSummaryData}
              defaultUpToEpisodeIndex={characterSummaryDefaultUpToEpisodeIndex}
              error={characterSummaryError}
              formatEpisodeOrderLabel={formatCharacterSummaryEpisodeOrder}
              isClearing={isCharacterSummaryClearing}
              isLoading={isCharacterSummaryLoading}
              isSubmitting={isCharacterSummarySubmitting}
              notice={characterSummaryNotice}
              onClear={handleClearCharacterSummary}
              onClose={closeReaderPanel}
              onRequestedGenerationStrategyChange={setCharacterSummaryGenerationStrategy}
              onRequestedUpToEpisodeIndexChange={setCharacterSummaryUpToEpisodeIndex}
              onSubmit={handleGenerateCharacterSummary}
              requestedGenerationStrategy={characterSummaryGenerationStrategy}
              requestedUpToEpisodeIndex={characterSummaryUpToEpisodeIndex}
            />
          </section>
        ) : null}
        {isTermsOpen ? (
          <section ref={readerPanelRef}>
            <ReaderTermListPanel
              data={termsData}
              error={characterSummaryError}
              formatEpisodeOrderLabel={formatCharacterSummaryEpisodeOrder}
              isLoading={isCharacterSummaryLoading}
              notice={characterSummaryNotice}
              onClose={closeReaderPanel}
            />
          </section>
        ) : null}
        {isReaderAiAssistantOpen ? (
          <section ref={readerPanelRef}>
            <ReaderAiAssistantPanel
              assistantState={readerAiAssistantState}
              currentEpisodeIndex={selectedEpisodeIndex}
              disabledReason={readerAiAssistantUnavailableMessage}
              formatEpisodeOrderLabel={formatCharacterSummaryEpisodeOrder}
              getCurrentPosition={() => getCurrentReaderViewportPosition() ?? selectedPositionRef.current ?? 0}
              novelId={selectedNovelId}
              onAssistantStateChange={setReaderAiAssistantState}
              onClose={closeReaderPanel}
            />
          </section>
        ) : null}
        {readerNotice ? <p className="reader-notice">{readerNotice}</p> : null}
        {clientUpdateRequired ? (
          <div className="client-update-required-backdrop">
            <section aria-label="アプリの更新が必要です" aria-modal="true" className="client-update-required" role="alertdialog">
              <div className="client-update-required-header">
                <p className="client-update-required-title">アプリの更新が必要です</p>
              </div>
              <p className="client-update-required-body">バージョンアップしました。アプリを再読み込みしてください。</p>
              <div className="reader-actions client-update-required-actions">
                <button onClick={() => window.location.reload()} type="button">
                  再読み込み
                </button>
              </div>
            </section>
          </div>
        ) : null}
        {pendingNextEpisodeConfirmation && !readerSyncConflict ? (
          // biome-ignore lint/a11y/noStaticElementInteractions: backdrop is pointer-only; keyboard users have the visible close button and Escape.
          // biome-ignore lint/a11y/useKeyWithClickEvents: backdrop is pointer-only; keyboard users have the visible close button and Escape.
          <div className="reader-next-episode-confirm-backdrop" onClick={handleNextEpisodeConfirmationBackdropClick}>
            <section
              aria-label="次の話へ移動"
              aria-live="polite"
              aria-modal="true"
              className="reader-next-episode-confirm"
              onClick={(event) => event.stopPropagation()}
              onKeyDown={handleNextEpisodeConfirmationKeyDown}
              role="alertdialog"
              style={{ position: "relative", zIndex: 1 }}
            >
              <div className="reader-next-episode-confirm-header">
                <p className="reader-next-episode-confirm-title">次の話へ進みますか？</p>
                <button
                  aria-label="次の話への移動を閉じる"
                  className="reader-next-episode-confirm-close"
                  onClick={handleNextEpisodeConfirmationCloseClick}
                  type="button"
                >
                  ×
                </button>
              </div>
              <p className="reader-next-episode-confirm-body">「{pendingNextEpisodeConfirmation.title}」を開きます。</p>
              <div className="reader-actions reader-next-episode-confirm-actions">
                <button ref={nextEpisodeConfirmPrimaryButtonRef} onClick={handleConfirmNextEpisode} type="button">
                  進む
                </button>
              </div>
            </section>
          </div>
        ) : null}
        {readerSyncConflict ? (
          <ReaderSyncConflictPanel
            applyDisabledReason={readerSyncConflictApplyDisabledReason}
            canApply={canApplyReaderSyncConflict}
            canOverwrite={hasStableReaderEpisode}
            episodeLabel={readerSyncConflictEpisodeLabel}
            formatDate={formatDate}
            onApply={handleApplyReaderSyncConflict}
            onOverwrite={() => {
              void handleOverwriteReaderSyncConflict();
            }}
            overwriteDisabledReason={hasStableReaderEpisode ? null : "本文の読み込み完了後に上書きできます。"}
            resolutionError={readerSyncConflictResolutionError}
            resolutionState={readerSyncConflictResolutionState}
            serverState={readerSyncConflict.serverState}
          />
        ) : null}
        {error ? (
          <section aria-label="本文画面のエラー" className="reader-error-alert" data-reader-panel-interactive role="alert">
            <p>{error}</p>
            <button aria-label="エラーを閉じる" onClick={() => setError(null)} type="button">
              ×
            </button>
          </section>
        ) : null}

        <ReaderPager
          articleFontFamilyCss={readerArticleFontFamilyCss}
          articleFontWeight={readerArticleFontWeight}
          displayedPageNumber={displayedPageNumber}
          episode={episode}
          isEpisodeLoading={isEpisodeLoading}
          isFullscreen={isReaderFullscreen}
          isLoadingOverlayVisible={isReaderLoadingOverlayVisible}
          isTouchDevice={isTouchDevice}
          letterSpacingEm={readerLetterSpacingEm}
          loadingEpisodeTitle={readerLoadingEpisodeTitle}
          loadingNovelTitle={sourceNovelTitle}
          onViewportClick={handleViewportClick}
          onViewportTouchCancel={handleViewportTouchCancel}
          onViewportTouchEnd={handleViewportTouchEnd}
          onViewportTouchStart={handleViewportTouchStart}
          pageIndicatorRef={readerPageIndicatorRef}
          readerFontSizePx={readerFontSizePx}
          readingMode={readingMode}
          renderedEpisodeHtml={renderedEpisodeHtml}
          totalPages={totalPages}
          verticalLastPageReservePx={verticalLastPageReservePx}
          viewportRef={readerViewportRef}
        />
        {imageViewer ? (
          <ReaderImageViewer
            imageViewer={imageViewer}
            imageViewerWidth={imageViewerWidth}
            isDragging={isImageViewerDragging}
            isInfoOpen={isImageViewerInfoOpen}
            onClose={closeImageViewer}
            onInfoOpenChange={setIsImageViewerInfoOpen}
            onPointerDown={handleImageViewerPointerDown}
            onPointerMove={handleImageViewerPointerMove}
            onPointerUp={handleImageViewerPointerUp}
            onZoomPercentChange={setImageViewerZoomPercent}
            stageRef={imageViewerStageRef}
            zoomPercent={imageViewerZoomPercent}
          />
        ) : null}
      </section>
    </ReaderShell>
  );
}
