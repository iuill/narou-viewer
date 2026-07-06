import { useCallback, useEffect, useMemo, useRef, useState, type Dispatch, type SetStateAction } from "react";
import type { NovelSummary } from "../library/types";
import type { Bookmark, EpisodeIndex, NovelReaderCorrectionPatch, ReaderState } from "./types";
import { useReaderRouteSync } from "../../hooks/useReaderRouteSync";
import { useReaderState, type ScreenMode, type UseReaderStateResult } from "../../hooks/useReaderState";
import { putNovelReaderSettings } from "./api";
import type { PutReaderStateInput, ReaderSyncConflictResolutionState } from "../../hooks/useReaderStateSync";

type ReaderPositionUpdate = number | null | ((currentPosition: number | null) => number | null);

type UseReaderSessionOptions = {
  initialEpisodeIndex: EpisodeIndex | null;
  initialPosition: number | null;
  initialScreenMode: ScreenMode;
  libraryReloadKey: number;
  onError: (message: string | null) => void;
  readerClientId: string;
  selectedNovelId: string | null;
  setNovels: Dispatch<SetStateAction<NovelSummary[]>>;
  setSelectedNovelId: Dispatch<SetStateAction<string | null>>;
};

export type ReaderSessionCommands = {
  applyServerState: (readerState: ReaderState) => void;
  changeNovelReaderCorrection: (correction: NovelReaderCorrectionPatch) => void;
  clearSelection: (options?: { clearNovel?: boolean }) => void;
  clearReaderSyncConflict: () => void;
  closeReader: () => void;
  forgetReaderStateCache: (novelId: string) => void;
  markReaderSyncConflictResolution: (state: ReaderSyncConflictResolutionState) => void;
  openEpisodeSelection: (episodeIndex: EpisodeIndex, position: number | null) => void;
  returnToLibrary: () => void;
  savePosition: (input: PutReaderStateInput) => Promise<ReaderState>;
  selectNovel: (novelId: string, options?: { openInReader?: boolean }) => void;
  updateBookmarks: Dispatch<SetStateAction<Bookmark[]>>;
  updateSelectedPosition: (nextPosition: ReaderPositionUpdate) => void;
};

type UseReaderSessionResult = Omit<
  UseReaderStateResult,
  | "putReaderState"
  | "resetReaderStateCache"
  | "setBookmarks"
  | "setReaderSettings"
  | "setReaderState"
  | "setReaderSyncConflict"
  | "setReaderSyncConflictResolutionState"
  | "setScreenMode"
  | "setSelectedEpisodeIndex"
  | "setSelectedPosition"
> & {
  activeReaderSettings: UseReaderStateResult["readerSettings"] | null;
  commands: ReaderSessionCommands;
  isReaderCorrectionSaving: boolean;
  isReaderCorrectionUnavailable: boolean;
};

export function useReaderSession({
  initialEpisodeIndex,
  initialPosition,
  initialScreenMode,
  libraryReloadKey,
  onError,
  readerClientId,
  selectedNovelId,
  setNovels,
  setSelectedNovelId
}: UseReaderSessionOptions): UseReaderSessionResult {
  const readerState = useReaderState({
    initialEpisodeIndex,
    initialPosition,
    initialScreenMode,
    libraryReloadKey,
    onError,
    readerClientId,
    selectedNovelId,
    setNovels
  });

  useReaderRouteSync({
    screenMode: readerState.screenMode,
    selectedEpisodeIndex: readerState.selectedEpisodeIndex,
    selectedNovelId,
    selectedPosition: readerState.selectedPosition,
    setScreenMode: readerState.setScreenMode,
    setSelectedEpisodeIndex: readerState.setSelectedEpisodeIndex,
    setSelectedNovelId,
    setSelectedPosition: readerState.setSelectedPosition
  });

  const [isReaderCorrectionSaving, setIsReaderCorrectionSaving] = useState(false);
  const readerCorrectionSaveSeqRef = useRef(0);
  const readerCorrectionSaveSeqByNovelIdRef = useRef(new Map<string, number>());
  const selectedNovelIdRef = useRef(selectedNovelId);

  useEffect(() => {
    selectedNovelIdRef.current = selectedNovelId;
    setIsReaderCorrectionSaving(false);
  }, [selectedNovelId]);

  const activeReaderSettings = readerState.readerSettings?.novelId === selectedNovelId ? readerState.readerSettings : null;
  const isReaderCorrectionUnavailable = isReaderCorrectionSaving || readerState.isNovelLoading || activeReaderSettings === null;

  const changeNovelReaderCorrection = useCallback(
    (correction: NovelReaderCorrectionPatch) => {
      if (!selectedNovelId || isReaderCorrectionUnavailable) {
        return;
      }
      const requestSeq = ++readerCorrectionSaveSeqRef.current;
      const novelId = selectedNovelId;
      readerCorrectionSaveSeqByNovelIdRef.current.set(novelId, requestSeq);
      setIsReaderCorrectionSaving(true);
      void (async () => {
        try {
          const savedSettings = await putNovelReaderSettings(novelId, { correction });
          if (
            readerCorrectionSaveSeqByNovelIdRef.current.get(novelId) === requestSeq &&
            selectedNovelIdRef.current === novelId
          ) {
            readerState.setReaderSettings(savedSettings);
          }
        } catch (saveError) {
          if (
            readerCorrectionSaveSeqByNovelIdRef.current.get(novelId) === requestSeq &&
            selectedNovelIdRef.current === novelId
          ) {
            onError(saveError instanceof Error ? saveError.message : "Unknown error");
          }
        } finally {
          if (
            readerCorrectionSaveSeqByNovelIdRef.current.get(novelId) === requestSeq &&
            selectedNovelIdRef.current === novelId
          ) {
            setIsReaderCorrectionSaving(false);
          }
        }
      })();
    },
    [isReaderCorrectionUnavailable, onError, readerState.setReaderSettings, selectedNovelId]
  );

  const closeReader = useCallback(() => {
    readerState.setScreenMode("library");
  }, [readerState.setScreenMode]);

  const clearSelection = useCallback(
    (options: { clearNovel?: boolean } = {}) => {
      if (options.clearNovel) {
        setSelectedNovelId(null);
      }
      readerState.setSelectedEpisodeIndex(null);
      readerState.setSelectedPosition(null);
      readerState.setScreenMode("library");
    },
    [readerState.setScreenMode, readerState.setSelectedEpisodeIndex, readerState.setSelectedPosition, setSelectedNovelId]
  );

  const openEpisodeSelection = useCallback(
    (episodeIndex: EpisodeIndex, position: number | null) => {
      readerState.setSelectedEpisodeIndex(episodeIndex);
      readerState.setSelectedPosition(position);
      readerState.setScreenMode("reader");
    },
    [readerState.setScreenMode, readerState.setSelectedEpisodeIndex, readerState.setSelectedPosition]
  );

  const returnToLibrary = useCallback(() => {
    readerState.setScreenMode("library");
  }, [readerState.setScreenMode]);

  const selectNovel = useCallback(
    (novelId: string, options: { openInReader?: boolean } = {}) => {
      readerState.openSelectedNovelInReaderRef.current = options.openInReader ?? false;
      setSelectedNovelId(novelId);
      readerState.setSelectedEpisodeIndex(null);
      readerState.setSelectedPosition(null);
      readerState.setScreenMode("library");
    },
    [
      readerState.openSelectedNovelInReaderRef,
      readerState.setScreenMode,
      readerState.setSelectedEpisodeIndex,
      readerState.setSelectedPosition,
      setSelectedNovelId
    ]
  );

  const updateSelectedPosition = useCallback(
    (nextPosition: ReaderPositionUpdate) => {
      readerState.setSelectedPosition(nextPosition);
    },
    [readerState.setSelectedPosition]
  );

  const {
    putReaderState: _putReaderState,
    resetReaderStateCache: _resetReaderStateCache,
    setBookmarks: _setBookmarks,
    setReaderState: _setReaderState,
    setReaderSettings: _setReaderSettings,
    setReaderSyncConflict: _setReaderSyncConflict,
    setReaderSyncConflictResolutionState: _setReaderSyncConflictResolutionState,
    setScreenMode: _setScreenMode,
    setSelectedEpisodeIndex: _setSelectedEpisodeIndex,
    setSelectedPosition: _setSelectedPosition,
    ...publicReaderState
  } = readerState;
  const commands = useMemo<ReaderSessionCommands>(
    () => ({
      applyServerState: readerState.setReaderState,
      changeNovelReaderCorrection,
      clearSelection,
      clearReaderSyncConflict: () => readerState.setReaderSyncConflict(null),
      closeReader,
      forgetReaderStateCache: readerState.resetReaderStateCache,
      markReaderSyncConflictResolution: readerState.setReaderSyncConflictResolutionState,
      openEpisodeSelection,
      returnToLibrary,
      savePosition: readerState.putReaderState,
      selectNovel,
      updateBookmarks: readerState.setBookmarks,
      updateSelectedPosition
    }),
    [
      changeNovelReaderCorrection,
      clearSelection,
      closeReader,
      openEpisodeSelection,
      readerState.putReaderState,
      readerState.resetReaderStateCache,
      readerState.setBookmarks,
      readerState.setReaderState,
      readerState.setReaderSyncConflict,
      readerState.setReaderSyncConflictResolutionState,
      returnToLibrary,
      selectNovel,
      updateSelectedPosition
    ]
  );

  return {
    ...publicReaderState,
    activeReaderSettings,
    commands,
    isReaderCorrectionSaving,
    isReaderCorrectionUnavailable
  };
}
