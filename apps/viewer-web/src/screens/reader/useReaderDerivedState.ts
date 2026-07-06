import { useCallback, useMemo } from "react";
import type { NovelSummary } from "../../features/library/types";
import { paginateItems } from "../../features/library/pagination";
import { createStoryPreview, normalizeStoryText } from "../../features/library/text";
import {
  canMoveReaderPage,
  getReaderEdgeTapPageMoveDirections,
  getReaderPageMoveDirections
} from "../../features/reader/gestureNavigation";
import { calculateImageViewerWidth } from "../../features/reader/imageViewer";
import {
  buildEpisodeDisplayLookup,
  buildEpisodeLabel,
  type EpisodeDisplayReference,
  formatEpisodeOrderLabel,
  formatEpisodeReferenceLabel,
  shouldUseFriendlyEpisodeLabels
} from "../../features/reader/episodeLabels";
import { createReadingStateKey } from "../../features/reader/readerStateKey";
import type { EpisodeIndex, EpisodeResponse, ReaderState, TocEpisode, TocResponse } from "../../features/reader/types";
import type { ReaderSyncConflict } from "../../hooks/useReaderState";
import type { ImageViewerState } from "../../ReaderImageViewer";
import { renderReaderDocumentHtml } from "../../readerDocument";
import type { ReadingMode } from "../../readerPreferences";
import { formatDate } from "../../shared/date";

const NOVEL_STORY_PREVIEW_LENGTH = 120;
const TOC_PAGE_SIZE = 50;

type ScreenMode = "library" | "reader";

function isTocEpisodeFetched(episode: TocEpisode | null | undefined): episode is TocEpisode {
  return Boolean(episode && (!episode.bodyStatus || episode.bodyStatus === "complete"));
}

type UseReaderDerivedStateOptions = {
  currentNovel: NovelSummary | null;
  currentPageIndex: number;
  episode: EpisodeResponse | null;
  imageViewer: ImageViewerState | null;
  imageViewerZoomPercent: number;
  isEpisodeLayoutReady: boolean;
  isEpisodeLoading: boolean;
  isReaderModalOpen: boolean;
  isTocStoryExpanded: boolean;
  readerState: ReaderState | null;
  readerSyncConflict: ReaderSyncConflict | null;
  readingMode: ReadingMode;
  reverseTapPageNavigation: boolean;
  screenMode: ScreenMode;
  selectedEpisodeIndex: EpisodeIndex | null;
  selectedNovelId: string | null;
  toc: TocResponse | null;
  tocPage: number;
  totalPages: number;
};

export function useReaderDerivedState({
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
}: UseReaderDerivedStateOptions) {
  const imageViewerWidth = imageViewer ? calculateImageViewerWidth(imageViewer, imageViewerZoomPercent) : null;
  const renderedEpisodeHtml = useMemo(
    () =>
      episode
        ? renderReaderDocumentHtml(episode.readerDocument, {
            enableVisibilityFragments: readingMode === "vertical"
          })
        : "",
    [episode, readingMode]
  );
  const tocPagination = useMemo(() => paginateItems(toc?.episodes ?? [], tocPage, TOC_PAGE_SIZE), [toc?.episodes, tocPage]);
  const visibleTocEpisodes = tocPagination.items;
  const pageMoveDirections = useMemo(() => getReaderPageMoveDirections(readingMode), [readingMode]);
  const edgeTapPageMoveDirections = useMemo(
    () => getReaderEdgeTapPageMoveDirections(pageMoveDirections, reverseTapPageNavigation),
    [pageMoveDirections, reverseTapPageNavigation]
  );
  const previousPageActionLabel = readingMode === "vertical" ? "次のページへ進む" : "前のページへ戻る";
  const nextPageActionLabel = readingMode === "vertical" ? "前のページへ戻る" : "次のページへ進む";
  const currentTocEpisodeIndex = useMemo(() => {
    if (!toc || selectedEpisodeIndex === null) {
      return -1;
    }

    return toc.episodes.findIndex((tocEpisode) => tocEpisode.episodeIndex === selectedEpisodeIndex);
  }, [selectedEpisodeIndex, toc]);
  const currentTocEpisode =
    currentTocEpisodeIndex >= 0 ? toc?.episodes[currentTocEpisodeIndex] ?? null : null;
  const previousEpisode = currentTocEpisodeIndex > 0 ? toc?.episodes[currentTocEpisodeIndex - 1] ?? null : null;
  const nextEpisode =
    currentTocEpisodeIndex >= 0 && currentTocEpisodeIndex < (toc?.episodes.length ?? 0) - 1
      ? toc?.episodes[currentTocEpisodeIndex + 1] ?? null
      : null;
  const hasUnlistedEpisodes =
    toc !== null && currentTocEpisodeIndex === toc.episodes.length - 1 && toc.totalEpisodes > toc.episodes.length;
  const canOpenPreviousEpisode = isTocEpisodeFetched(previousEpisode);
  const canOpenNextEpisode = isTocEpisodeFetched(nextEpisode);
  const readerForwardPageDirection = readingMode === "vertical" ? pageMoveDirections.previous : pageMoveDirections.next;
  const canMoveToPreviousReaderPage = canMoveReaderPage(currentPageIndex, totalPages, pageMoveDirections.previous);
  const canMoveToNextReaderPage = canMoveReaderPage(currentPageIndex, totalPages, pageMoveDirections.next);
  const canMoveForwardReaderPage = canMoveReaderPage(currentPageIndex, totalPages, readerForwardPageDirection);
  const hasStableReaderEpisode =
    screenMode === "reader" &&
    episode !== null &&
    episode.novelId === selectedNovelId &&
    episode.episodeIndex === selectedEpisodeIndex &&
    !isEpisodeLoading &&
    isEpisodeLayoutReady;
  const canUseReaderPageActions = hasStableReaderEpisode && !isReaderModalOpen;
  const canUseForwardPageAction = canUseReaderPageActions;
  const canUsePreviousPageButton =
    canUseReaderPageActions &&
    (pageMoveDirections.previous === readerForwardPageDirection ? canUseForwardPageAction : canMoveToPreviousReaderPage);
  const canUseNextPageButton =
    canUseReaderPageActions &&
    (pageMoveDirections.next === readerForwardPageDirection ? canUseForwardPageAction : canMoveToNextReaderPage);
  const selectedTocPage = useMemo(
    () => (toc ? resolveTocPageForEpisode(toc.episodes, selectedEpisodeIndex) : 1),
    [selectedEpisodeIndex, toc]
  );
  const lastReadEpisodeIndex = readerState?.lastReadEpisodeIndex ?? null;
  const normalizedTocStory = toc ? normalizeStoryText(toc.story) : "";
  const tocStoryPreview = createStoryPreview(normalizedTocStory, NOVEL_STORY_PREVIEW_LENGTH);
  const tocStoryText =
    normalizedTocStory.length === 0
      ? "あらすじはありません。"
      : tocStoryPreview.isTruncated && !isTocStoryExpanded
        ? tocStoryPreview.text
        : normalizedTocStory;
  const episodeDisplayLookup = useMemo(
    () => (toc ? buildEpisodeDisplayLookup(toc.episodes) : new Map<EpisodeIndex, EpisodeDisplayReference>()),
    [toc]
  );
  const formatCharacterSummaryEpisodeOrder = useCallback(
    (episodeIndex: string) => formatEpisodeOrderLabel(episodeIndex, episodeDisplayLookup),
    [episodeDisplayLookup]
  );
  const preferFriendlyEpisodeLabels = useMemo(
    () => shouldUseFriendlyEpisodeLabels(toc?.siteName ?? currentNovel?.siteName, toc?.tocUrl ?? currentNovel?.tocUrl),
    [currentNovel?.siteName, currentNovel?.tocUrl, toc?.siteName, toc?.tocUrl]
  );
  const savedReadingStateKey = createReadingStateKey(
    readerState?.novelId ?? null,
    readerState?.lastReadEpisodeIndex ?? null,
    readerState?.position ?? null
  );
  const readerSyncConflictServerState = readerSyncConflict?.serverState ?? null;
  const readerSyncConflictEpisodeLabel =
    readerSyncConflictServerState !== null && readerSyncConflictServerState.lastReadEpisodeIndex !== null
      ? formatEpisodeReferenceLabel(
          readerSyncConflictServerState.lastReadEpisodeIndex,
          episodeDisplayLookup,
          preferFriendlyEpisodeLabels
        )
      : null;
  const readerSyncConflictApplyDisabledReason = (() => {
    if (!readerSyncConflictServerState) {
      return null;
    }
    const targetEpisodeIndex = readerSyncConflictServerState.lastReadEpisodeIndex;
    if (targetEpisodeIndex === null) {
      return "反映できる既読位置がありません。";
    }
    const targetEpisode = toc?.episodes.find((tocEpisode) => tocEpisode.episodeIndex === targetEpisodeIndex) ?? null;
    if (!targetEpisode) {
      return "対象の話が目次にありません。";
    }
    if (!isTocEpisodeFetched(targetEpisode)) {
      return "対象の話はまだ本文が取得されていません。";
    }
    return null;
  })();
  const readerInfoEpisodeReferenceLabel =
    selectedEpisodeIndex !== null
      ? formatEpisodeReferenceLabel(selectedEpisodeIndex, episodeDisplayLookup, preferFriendlyEpisodeLabels)
      : "未取得";
  const readerInfoEpisodeTitle = episode ? buildEpisodeLabel(episode) : "未取得";
  const hasCurrentNovelToc = toc?.novelId === selectedNovelId;
  const readerLoadingEpisode =
    (hasCurrentNovelToc ? currentTocEpisode : null) ??
    (episode?.novelId === selectedNovelId && episode.episodeIndex === selectedEpisodeIndex ? episode : null);
  const readerLoadingEpisodeTitle = readerLoadingEpisode
    ? buildEpisodeLabel(readerLoadingEpisode)
    : selectedEpisodeIndex !== null
      ? `#${selectedEpisodeIndex}`
      : "未取得";
  const readerInfoPageLabel = totalPages > 0 ? `${currentPageIndex + 1} / ${totalPages}` : "未取得";
  const readerInfoUpdatedAtLabel = currentNovel?.updatedAt ? formatDate(currentNovel.updatedAt) : "未取得";
  const readerInfoSourceUrl = currentTocEpisode?.sourceUrl ?? episode?.sourceUrl ?? currentNovel?.tocUrl ?? null;

  return {
    canMoveForwardReaderPage,
    canOpenNextEpisode,
    canOpenPreviousEpisode,
    canUseNextPageButton,
    canUsePreviousPageButton,
    canUseReaderPageActions,
    currentTocEpisodeIndex,
    currentTocEpisode,
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
  };
}

function resolveTocPageForEpisode(episodes: TocEpisode[], episodeIndex: EpisodeIndex | null): number {
  if (episodeIndex === null) {
    return 1;
  }

  const tocEpisodeIndex = episodes.findIndex((tocEpisode) => tocEpisode.episodeIndex === episodeIndex);
  if (tocEpisodeIndex < 0) {
    return 1;
  }

  return Math.floor(tocEpisodeIndex / TOC_PAGE_SIZE) + 1;
}
