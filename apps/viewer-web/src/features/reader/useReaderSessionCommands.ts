import { useCallback, useMemo, type Dispatch, type MutableRefObject, type SetStateAction } from "react";
import type { EpisodeIndex, TocResponse } from "./types";

type TocEpisodeLike = TocResponse["episodes"][number];

type ReaderSelectionSessionCommands = {
  clearSelection: (options?: { clearNovel?: boolean }) => void;
  openEpisodeSelection: (episodeIndex: EpisodeIndex, position: number | null) => void;
  returnToLibrary: () => void;
  selectNovel: (novelId: string, options?: { openInReader?: boolean }) => void;
  updateSelectedPosition: Dispatch<SetStateAction<number | null>>;
};

type UseReaderSessionCommandsOptions = {
  currentEpisodeIndex: EpisodeIndex | null;
  layoutAnchorPositionRef: MutableRefObject<number | null>;
  onError: (message: string | null) => void;
  openSelectedNovelInReaderRef: MutableRefObject<boolean>;
  sessionCommands: ReaderSelectionSessionCommands;
  setIsEpisodeLayoutReady: Dispatch<SetStateAction<boolean>>;
  shouldCapturePageAnchorRef: MutableRefObject<boolean>;
  toc: TocResponse | null;
};

export type ReaderSessionCommands = {
  clearSelection: (options?: { clearNovel?: boolean }) => void;
  openEpisode: (episodeIndex: EpisodeIndex, position?: number | null) => boolean;
  returnToLibrary: () => void;
  selectNovel: (novelId: string, options?: { openInReader?: boolean }) => void;
  updateSelectedPosition: Dispatch<SetStateAction<number | null>>;
};

function isTocEpisodeFetched(episode: TocEpisodeLike | null | undefined): episode is TocEpisodeLike {
  return Boolean(episode && (!episode.bodyStatus || episode.bodyStatus === "complete"));
}

export function useReaderSessionCommands({
  currentEpisodeIndex,
  layoutAnchorPositionRef,
  onError,
  openSelectedNovelInReaderRef,
  sessionCommands,
  setIsEpisodeLayoutReady,
  shouldCapturePageAnchorRef,
  toc
}: UseReaderSessionCommandsOptions): ReaderSessionCommands {
  const openEpisode = useCallback(
    (episodeIndex: EpisodeIndex, position: number | null = null): boolean => {
      const targetEpisode = toc?.episodes.find((tocEpisode) => tocEpisode.episodeIndex === episodeIndex);
      if (toc && !targetEpisode) {
        onError("対象の話が目次にありません。");
        return false;
      }
      if (targetEpisode && !isTocEpisodeFetched(targetEpisode)) {
        onError("この話はまだ本文が取得されていません。再開して取得してください。");
        return false;
      }

      layoutAnchorPositionRef.current = position;
      shouldCapturePageAnchorRef.current = false;
      openSelectedNovelInReaderRef.current = false;
      if (currentEpisodeIndex !== episodeIndex) {
        setIsEpisodeLayoutReady(false);
      }
      onError(null);
      sessionCommands.openEpisodeSelection(episodeIndex, position);
      return true;
    },
    [
      currentEpisodeIndex,
      layoutAnchorPositionRef,
      onError,
      openSelectedNovelInReaderRef,
      sessionCommands,
      setIsEpisodeLayoutReady,
      shouldCapturePageAnchorRef,
      toc
    ]
  );

  const clearSelection = useCallback((options: { clearNovel?: boolean } = {}) => {
    layoutAnchorPositionRef.current = null;
    openSelectedNovelInReaderRef.current = false;
    shouldCapturePageAnchorRef.current = false;
    sessionCommands.clearSelection(options);
  }, [layoutAnchorPositionRef, openSelectedNovelInReaderRef, sessionCommands, shouldCapturePageAnchorRef]);

  const returnToLibrary = useCallback(() => {
    sessionCommands.returnToLibrary();
  }, [sessionCommands]);

  const selectNovel = useCallback(
    (novelId: string, options: { openInReader?: boolean } = {}) => {
      layoutAnchorPositionRef.current = null;
      shouldCapturePageAnchorRef.current = false;
      openSelectedNovelInReaderRef.current = options.openInReader ?? false;
      sessionCommands.selectNovel(novelId, options);
    },
    [layoutAnchorPositionRef, openSelectedNovelInReaderRef, sessionCommands, shouldCapturePageAnchorRef]
  );

  return useMemo(
    () => ({
      clearSelection,
      openEpisode,
      returnToLibrary,
      selectNovel,
      updateSelectedPosition: sessionCommands.updateSelectedPosition
    }),
    [clearSelection, openEpisode, returnToLibrary, selectNovel, sessionCommands.updateSelectedPosition]
  );
}
