import { useCallback, useEffect, useRef, useState, type Dispatch, type MutableRefObject, type SetStateAction } from "react";
import { fetchEpisode, fetchNovelContext } from "../features/reader/api";
import type { NovelSummary } from "../features/library/types";
import type {
  Bookmark,
  EpisodeIndex,
  EpisodeResponse,
  NovelReaderSettingsResponse,
  ReaderState,
  TocEpisode,
  TocResponse
} from "../features/reader/types";
import { useReaderStateSync, type PutReaderStateInput, type ReaderSyncConflict, type ReaderSyncConflictResolutionState, type ScreenMode } from "./useReaderStateSync";

export type { ReaderSyncConflict, ReaderSyncConflictResolutionState, ScreenMode };

type UseReaderStateOptions = {
  initialEpisodeIndex: EpisodeIndex | null;
  initialPosition: number | null;
  initialScreenMode: ScreenMode;
  libraryReloadKey: number;
  onError: (message: string | null) => void;
  readerClientId: string;
  selectedNovelId: string | null;
  setNovels: Dispatch<SetStateAction<NovelSummary[]>>;
};

export type UseReaderStateResult = {
  bookmarks: Bookmark[];
  episode: EpisodeResponse | null;
  isEpisodeLoading: boolean;
  isNovelLoading: boolean;
  isReaderLoadingOverlayVisible: boolean;
  readerSyncConflictResolutionState: ReaderSyncConflictResolutionState;
  openSelectedNovelInReaderRef: MutableRefObject<boolean>;
  pendingReadingStateKeyRef: MutableRefObject<string | null>;
  putReaderState: (input: PutReaderStateInput) => Promise<ReaderState>;
  resetReaderStateCache: (novelId: string) => void;
  readerState: ReaderState | null;
  readerSettings: NovelReaderSettingsResponse | null;
  readerSyncConflict: ReaderSyncConflict | null;
  screenMode: ScreenMode;
  screenModeRef: MutableRefObject<ScreenMode>;
  selectedEpisodeIndex: EpisodeIndex | null;
  selectedEpisodeIndexRef: MutableRefObject<EpisodeIndex | null>;
  selectedPosition: number | null;
  selectedPositionRef: MutableRefObject<number | null>;
  setBookmarks: Dispatch<SetStateAction<Bookmark[]>>;
  setReaderSyncConflictResolutionState: Dispatch<SetStateAction<ReaderSyncConflictResolutionState>>;
  setReaderState: Dispatch<SetStateAction<ReaderState | null>>;
  setReaderSettings: Dispatch<SetStateAction<NovelReaderSettingsResponse | null>>;
  setReaderSyncConflict: Dispatch<SetStateAction<ReaderSyncConflict | null>>;
  setScreenMode: Dispatch<SetStateAction<ScreenMode>>;
  setSelectedEpisodeIndex: Dispatch<SetStateAction<EpisodeIndex | null>>;
  setSelectedPosition: Dispatch<SetStateAction<number | null>>;
  toc: TocResponse | null;
};

const READER_LOADING_OVERLAY_MIN_VISIBLE_MS = 1000;

function isTocEpisodeFetched(episode: TocEpisode | null | undefined): episode is TocEpisode {
  return Boolean(episode && (!episode.bodyStatus || episode.bodyStatus === "complete"));
}

function findFetchedTocEpisode(episodes: TocEpisode[], episodeIndex: EpisodeIndex | null): TocEpisode | null {
  if (episodeIndex === null) {
    return null;
  }

  const episode = episodes.find((tocEpisode) => tocEpisode.episodeIndex === episodeIndex) ?? null;
  return isTocEpisodeFetched(episode) ? episode : null;
}

function toErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Unknown error";
}

export function useReaderState({
  initialEpisodeIndex,
  initialPosition,
  initialScreenMode,
  libraryReloadKey,
  onError,
  readerClientId,
  selectedNovelId,
  setNovels
}: UseReaderStateOptions): UseReaderStateResult {
  const [toc, setToc] = useState<TocResponse | null>(null);
  const [selectedEpisodeIndex, setSelectedEpisodeIndex] = useState<EpisodeIndex | null>(initialEpisodeIndex);
  const [selectedPosition, setSelectedPosition] = useState<number | null>(initialPosition);
  const [episode, setEpisode] = useState<EpisodeResponse | null>(null);
  const [readerSettings, setReaderSettings] = useState<NovelReaderSettingsResponse | null>(null);
  const readerSettingsMutationSeqRef = useRef(0);
  const setReaderSettingsFromOutside = useCallback<Dispatch<SetStateAction<NovelReaderSettingsResponse | null>>>((nextSettings) => {
    readerSettingsMutationSeqRef.current += 1;
    setReaderSettings(nextSettings);
  }, []);
  const [bookmarks, setBookmarks] = useState<Bookmark[]>([]);
  const [screenMode, setScreenMode] = useState<ScreenMode>(initialScreenMode);
  const [isNovelLoading, setIsNovelLoading] = useState(false);
  const [isEpisodeLoading, setIsEpisodeLoading] = useState(false);
  const [isReaderLoadingOverlayVisible, setIsReaderLoadingOverlayVisible] = useState(false);
  const selectedEpisodeIndexRef = useRef<EpisodeIndex | null>(initialEpisodeIndex);
  const selectedPositionRef = useRef<number | null>(initialPosition);
  const selectedNovelIdRef = useRef<string | null>(selectedNovelId);
  const screenModeRef = useRef<ScreenMode>(initialScreenMode);
  const openSelectedNovelInReaderRef = useRef(false);
  const pendingReadingStateKeyRef = useRef<string | null>(null);
  const readerLoadingOverlayStartedAtRef = useRef<number | null>(null);

  selectedEpisodeIndexRef.current = selectedEpisodeIndex;
  selectedPositionRef.current = selectedPosition;
  selectedNovelIdRef.current = selectedNovelId;
  screenModeRef.current = screenMode;

  const {
    getReaderStateGeneration,
    putReaderState,
    readerState,
    readerSyncConflict,
    readerSyncConflictResolutionState,
    reconcileIncomingReaderState,
    resetReaderStateCache,
    setReaderState: setReaderStateFromOutside,
    setReaderSyncConflict: setReaderSyncConflictFromOutside,
    setReaderSyncConflictResolutionState
  } = useReaderStateSync({
    pendingReadingStateKeyRef,
    readerClientId,
    screenMode,
    screenModeRef,
    selectedEpisodeIndexRef,
    selectedNovelId,
    selectedNovelIdRef,
    selectedPositionRef,
    setNovels
  });

  useEffect(() => {
    if (screenMode !== "reader") {
      readerLoadingOverlayStartedAtRef.current = null;
      setIsReaderLoadingOverlayVisible(false);
      return;
    }

    if (isEpisodeLoading) {
      if (!isReaderLoadingOverlayVisible) {
        readerLoadingOverlayStartedAtRef.current = Date.now();
        setIsReaderLoadingOverlayVisible(true);
      }
      return;
    }

    if (!isReaderLoadingOverlayVisible) {
      readerLoadingOverlayStartedAtRef.current = null;
      return;
    }

    const startedAt = readerLoadingOverlayStartedAtRef.current;
    const elapsedMs = startedAt === null ? READER_LOADING_OVERLAY_MIN_VISIBLE_MS : Date.now() - startedAt;

    if (elapsedMs >= READER_LOADING_OVERLAY_MIN_VISIBLE_MS) {
      setIsReaderLoadingOverlayVisible(false);
      readerLoadingOverlayStartedAtRef.current = null;
      return;
    }

    const timeoutId = window.setTimeout(() => {
      setIsReaderLoadingOverlayVisible(false);
      readerLoadingOverlayStartedAtRef.current = null;
    }, READER_LOADING_OVERLAY_MIN_VISIBLE_MS - elapsedMs);

    return () => {
      window.clearTimeout(timeoutId);
    };
  }, [isEpisodeLoading, isReaderLoadingOverlayVisible, screenMode]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: libraryReloadKey intentionally reloads the selected novel context.
  useEffect(() => {
    if (!selectedNovelId) {
      openSelectedNovelInReaderRef.current = false;
      setToc(null);
      setEpisode(null);
      setReaderStateFromOutside(null);
      pendingReadingStateKeyRef.current = null;
      setReaderSettings(null);
      setBookmarks([]);
      setSelectedEpisodeIndex(null);
      setSelectedPosition(null);
      setScreenMode("library");
      return;
    }

    const novelId = selectedNovelId;
    let cancelled = false;
    const contextGeneration = getReaderStateGeneration(novelId);
    const readerSettingsLoadSeq = readerSettingsMutationSeqRef.current;

    async function loadNovelContext() {
      setIsNovelLoading(true);
      onError(null);
      setEpisode(null);

      try {
        const {
          toc: nextToc,
          readerState: nextReaderState,
          bookmarks: nextBookmarks,
          readerSettings: nextReaderSettings
        } = await fetchNovelContext(novelId);

        if (cancelled || contextGeneration !== getReaderStateGeneration(novelId)) {
          return;
        }

        setToc(nextToc);
        const currentSelectedEpisodeIndex = selectedEpisodeIndexRef.current;
        const currentSelectedPosition = selectedPositionRef.current;
        const currentScreenMode = screenModeRef.current;
        const { activeReaderState } = reconcileIncomingReaderState(nextReaderState);
        if (readerSettingsLoadSeq === readerSettingsMutationSeqRef.current) {
          setReaderSettings(nextReaderSettings);
        }
        setBookmarks(nextBookmarks);
        const selectedFetchedEpisode = findFetchedTocEpisode(nextToc.episodes, currentSelectedEpisodeIndex);
        const lastReadFetchedEpisode = findFetchedTocEpisode(nextToc.episodes, activeReaderState.lastReadEpisodeIndex);
        const firstFetchedEpisode = nextToc.episodes.find(isTocEpisodeFetched) ?? null;
        const resolvedEpisodeIndex =
          selectedFetchedEpisode?.episodeIndex ?? lastReadFetchedEpisode?.episodeIndex ?? firstFetchedEpisode?.episodeIndex ?? null;
        const resolvedFromSelectedEpisode =
          currentSelectedEpisodeIndex !== null && resolvedEpisodeIndex === selectedFetchedEpisode?.episodeIndex;
        const shouldRestoreSavedPositionForCurrentEpisode =
          resolvedFromSelectedEpisode &&
          currentSelectedPosition === null &&
          currentScreenMode === "reader" &&
          currentSelectedEpisodeIndex === activeReaderState.lastReadEpisodeIndex;
        const resolvedPosition = resolvedFromSelectedEpisode
          ? shouldRestoreSavedPositionForCurrentEpisode
            ? activeReaderState.position
            : currentSelectedPosition
          : resolvedEpisodeIndex !== null && resolvedEpisodeIndex === activeReaderState.lastReadEpisodeIndex
            ? activeReaderState.position
            : null;

        setSelectedEpisodeIndex(resolvedEpisodeIndex);
        setSelectedPosition(resolvedPosition);
        if (openSelectedNovelInReaderRef.current && resolvedEpisodeIndex !== null) {
          openSelectedNovelInReaderRef.current = false;
          setScreenMode("reader");
        }
      } catch (loadError) {
        if (!cancelled) {
          onError(toErrorMessage(loadError));
        }
      } finally {
        if (!cancelled) {
          setIsNovelLoading(false);
        }
      }
    }

    void loadNovelContext();

    return () => {
      cancelled = true;
    };
  }, [
    getReaderStateGeneration,
    libraryReloadKey,
    onError,
    reconcileIncomingReaderState,
    setReaderStateFromOutside,
    selectedNovelId
  ]);

  const readerSettingsNovelId = readerSettings?.novelId ?? null;
  const readerSettingsCorrection = readerSettings?.correction ?? null;

  // biome-ignore lint/correctness/useExhaustiveDependencies: libraryReloadKey intentionally reloads the current episode content.
  useEffect(() => {
    if (screenMode !== "reader" || !selectedNovelId || selectedEpisodeIndex === null) {
      setEpisode(null);
      return;
    }
    if (!toc || readerSettingsNovelId !== selectedNovelId) {
      return;
    }
    const tocEpisode = toc.episodes.find((episodeSummary) => episodeSummary.episodeIndex === selectedEpisodeIndex);
    if (tocEpisode && !isTocEpisodeFetched(tocEpisode)) {
      setEpisode(null);
      setIsEpisodeLoading(false);
      onError("この話はまだ本文が取得されていません。再開して取得してください。");
      return;
    }

    const novelId = selectedNovelId;
    const episodeIndex = selectedEpisodeIndex;
    let cancelled = false;

    async function loadEpisode() {
      setIsEpisodeLoading(true);
      onError(null);

      try {
        const nextEpisode = await fetchEpisode(novelId, episodeIndex);
        if (cancelled) {
          return;
        }

        setEpisode(nextEpisode);
        setNovels((current) =>
          current.map((novel) =>
            novel.novelId === novelId
              ? {
                  ...novel,
                  lastReadEpisodeIndex: episodeIndex,
                  lastReadEpisodeTitle: nextEpisode.title
                }
              : novel
          )
        );
      } catch (loadError) {
        if (!cancelled) {
          onError(toErrorMessage(loadError));
        }
      } finally {
        if (!cancelled) {
          setIsEpisodeLoading(false);
        }
      }
    }

    void loadEpisode();

    return () => {
      cancelled = true;
    };
  }, [
    libraryReloadKey,
    onError,
    readerSettingsCorrection?.hyphenDashNormalization,
    readerSettingsCorrection?.halfwidthAlnumPunctuationNormalization,
    readerSettingsCorrection?.parenthesisNormalization,
    readerSettingsCorrection?.quoteNormalization,
    readerSettingsNovelId,
    screenMode,
    selectedEpisodeIndex,
    selectedNovelId,
    setNovels,
    toc
  ]);

  return {
    bookmarks,
    episode,
    isEpisodeLoading,
    isNovelLoading,
    isReaderLoadingOverlayVisible,
    readerSyncConflictResolutionState,
    openSelectedNovelInReaderRef,
    pendingReadingStateKeyRef,
    putReaderState,
    resetReaderStateCache,
    readerState,
    readerSettings,
    readerSyncConflict,
    screenMode,
    screenModeRef,
    selectedEpisodeIndex,
    selectedEpisodeIndexRef,
    selectedPosition,
    selectedPositionRef,
    setBookmarks,
    setReaderSyncConflictResolutionState,
    setReaderState: setReaderStateFromOutside,
    setReaderSettings: setReaderSettingsFromOutside,
    setReaderSyncConflict: setReaderSyncConflictFromOutside,
    setScreenMode,
    setSelectedEpisodeIndex,
    setSelectedPosition,
    toc
  };
}
