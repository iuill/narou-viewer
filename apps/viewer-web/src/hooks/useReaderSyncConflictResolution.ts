import { useCallback, useEffect, useRef, useState, type Dispatch, type MutableRefObject, type SetStateAction } from "react";
import { fetchEpisode, ReaderStateConflictError } from "../features/reader/api";
import type { EpisodeIndex, EpisodeResponse, ReaderState, TocEpisode } from "../features/reader/types";
import { createReadingStateKey } from "../features/reader/readerStateKey";
import type { AppliedReaderStateAutoSaveGuard } from "../readerStateAutoSaveGuard";
import type { PutReaderStateInput, ReaderSyncConflict, ReaderSyncConflictResolutionState } from "./useReaderStateSync";

type UseReaderSyncConflictResolutionOptions = {
  appliedReaderStateAutoSaveGuardRef: MutableRefObject<AppliedReaderStateAutoSaveGuard | null>;
  applyDisabledReason: string | null;
  closeActiveReaderPanel: () => void;
  closeImageViewer: () => void;
  episode: EpisodeResponse | null;
  getCurrentReaderViewportPosition: () => number | null;
  handleOpenEpisode: (episodeIndex: EpisodeIndex, position?: number | null) => boolean;
  hasStableReaderEpisode: boolean;
  isEpisodeLoading: boolean;
  pendingReadingStateKeyRef: MutableRefObject<string | null>;
  savePosition: (input: PutReaderStateInput) => Promise<ReaderState>;
  readerSyncConflict: ReaderSyncConflict | null;
  readerSyncConflictResolutionState: ReaderSyncConflictResolutionState;
  selectedEpisodeIndex: EpisodeIndex | null;
  selectedNovelId: string | null;
  selectedPosition: number | null;
  setError: (message: string | null) => void;
  setIsReaderOverflowOpen: Dispatch<SetStateAction<boolean>>;
  setPendingNextEpisodeConfirmation: Dispatch<SetStateAction<TocEpisode | null>>;
  setReaderNotice: (message: string | null) => void;
  applyServerState: (readerState: ReaderState) => void;
  clearReaderSyncConflict: () => void;
  markReaderSyncConflictResolution: (state: ReaderSyncConflictResolutionState) => void;
  stopReaderSpeech: () => Promise<void>;
};

type UseReaderSyncConflictResolutionResult = {
  canApplyReaderSyncConflict: boolean;
  handleApplyReaderSyncConflict: () => Promise<void>;
  handleOverwriteReaderSyncConflict: () => Promise<void>;
  readerSyncConflictResolutionError: string | null;
};

function toErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Unknown error";
}

export function useReaderSyncConflictResolution({
  appliedReaderStateAutoSaveGuardRef,
  applyDisabledReason,
  closeActiveReaderPanel,
  closeImageViewer,
  episode,
  getCurrentReaderViewportPosition,
  handleOpenEpisode,
  hasStableReaderEpisode,
  isEpisodeLoading,
  pendingReadingStateKeyRef,
  savePosition,
  readerSyncConflict,
  readerSyncConflictResolutionState,
  selectedEpisodeIndex,
  selectedNovelId,
  selectedPosition,
  setError,
  setIsReaderOverflowOpen,
  setPendingNextEpisodeConfirmation,
  setReaderNotice,
  applyServerState,
  clearReaderSyncConflict,
  markReaderSyncConflictResolution,
  stopReaderSpeech
}: UseReaderSyncConflictResolutionOptions): UseReaderSyncConflictResolutionResult {
  const [readerSyncConflictResolutionError, setReaderSyncConflictResolutionError] = useState<string | null>(null);
  const previousReaderSyncConflictRef = useRef(readerSyncConflict);
  const latestReaderSyncConflictRef = useRef(readerSyncConflict);
  latestReaderSyncConflictRef.current = readerSyncConflict;

  useEffect(() => {
    const previousReaderSyncConflict = previousReaderSyncConflictRef.current;
    previousReaderSyncConflictRef.current = readerSyncConflict;

    if (!readerSyncConflict) {
      markReaderSyncConflictResolution("idle");
      setReaderSyncConflictResolutionError(null);
      return;
    }

    if (!previousReaderSyncConflict) {
      setReaderSyncConflictResolutionError(null);
    }
    setPendingNextEpisodeConfirmation(null);
    closeImageViewer();
    closeActiveReaderPanel();
    setIsReaderOverflowOpen(false);
    void stopReaderSpeech();
  }, [
    closeActiveReaderPanel,
    closeImageViewer,
    markReaderSyncConflictResolution,
    readerSyncConflict,
    setIsReaderOverflowOpen,
    setPendingNextEpisodeConfirmation,
    stopReaderSpeech
  ]);

  const handleApplyReaderSyncConflict = useCallback(async () => {
    if (!readerSyncConflict || readerSyncConflictResolutionState !== "idle") {
      return;
    }

    const nextReaderState = readerSyncConflict.serverState;
    if (applyDisabledReason !== null) {
      setReaderSyncConflictResolutionError(applyDisabledReason);
      return;
    }
    if (nextReaderState.lastReadEpisodeIndex === null) {
      setReaderSyncConflictResolutionError("反映できる既読位置がありません。");
      return;
    }

    markReaderSyncConflictResolution("applying");
    setReaderSyncConflictResolutionError(null);
    const isTargetEpisodeAlreadyLoaded =
      episode?.novelId === nextReaderState.novelId &&
      episode.episodeIndex === nextReaderState.lastReadEpisodeIndex &&
      !isEpisodeLoading;
    if (!isTargetEpisodeAlreadyLoaded) {
      try {
        await fetchEpisode(nextReaderState.novelId, nextReaderState.lastReadEpisodeIndex);
      } catch {
        markReaderSyncConflictResolution("idle");
        setReaderSyncConflictResolutionError("別端末の既読位置へ移動できませんでした。");
        return;
      }
    }
    const visibleConflict = latestReaderSyncConflictRef.current;
    const isResolvingCurrentConflict =
      visibleConflict?.serverState.novelId === nextReaderState.novelId &&
      visibleConflict.serverState.stateVersion === nextReaderState.stateVersion;
    if (!isResolvingCurrentConflict) {
      markReaderSyncConflictResolution("idle");
      setReaderSyncConflictResolutionError("さらに新しい既読位置が保存されています。もう一度確認してください。");
      return;
    }
    const didOpenEpisode = handleOpenEpisode(nextReaderState.lastReadEpisodeIndex, nextReaderState.position);
    if (!didOpenEpisode) {
      markReaderSyncConflictResolution("idle");
      setReaderSyncConflictResolutionError("別端末の既読位置へ移動できませんでした。");
      return;
    }

    const appliedReadingStateKey = createReadingStateKey(
      nextReaderState.novelId,
      nextReaderState.lastReadEpisodeIndex,
      nextReaderState.position
    );
    if (appliedReadingStateKey !== null) {
      pendingReadingStateKeyRef.current = appliedReadingStateKey;
      appliedReaderStateAutoSaveGuardRef.current = {
        novelId: nextReaderState.novelId,
        episodeIndex: nextReaderState.lastReadEpisodeIndex,
        readingStateKey: appliedReadingStateKey
      };
    }
    clearReaderSyncConflict();
    applyServerState(nextReaderState);
    setError(null);
    setReaderNotice("別端末の既読位置を反映しました。");
    markReaderSyncConflictResolution("idle");
  }, [
    appliedReaderStateAutoSaveGuardRef,
    applyDisabledReason,
    applyServerState,
    clearReaderSyncConflict,
    episode,
    handleOpenEpisode,
    isEpisodeLoading,
    markReaderSyncConflictResolution,
    pendingReadingStateKeyRef,
    readerSyncConflict,
    readerSyncConflictResolutionState,
    setError,
    setReaderNotice
  ]);

  const handleOverwriteReaderSyncConflict = useCallback(async () => {
    if (
      !readerSyncConflict ||
      readerSyncConflictResolutionState !== "idle" ||
      !selectedNovelId ||
      selectedEpisodeIndex === null ||
      !hasStableReaderEpisode
    ) {
      if (readerSyncConflict && !hasStableReaderEpisode) {
        setReaderSyncConflictResolutionError("本文の読み込み完了後に上書きできます。");
      }
      return;
    }

    const position = getCurrentReaderViewportPosition() ?? selectedPosition ?? 0;
    const nextReadingStateKey = createReadingStateKey(selectedNovelId, selectedEpisodeIndex, position);
    markReaderSyncConflictResolution("overwriting");
    setReaderSyncConflictResolutionError(null);

    try {
      const conflictAtOverwriteStart = readerSyncConflict.serverState;
      const savedReaderState = await savePosition({
        novelId: selectedNovelId,
        allowDuringConflict: true,
        episodeIndex: selectedEpisodeIndex,
        expectedStateVersion: conflictAtOverwriteStart.stateVersion,
        position,
        readingStateKey: nextReadingStateKey
      });
      const visibleConflict = latestReaderSyncConflictRef.current;
      const hasNewerVisibleConflict =
        visibleConflict?.serverState.novelId === conflictAtOverwriteStart.novelId &&
        visibleConflict.serverState.stateVersion > savedReaderState.stateVersion;
      if (!hasNewerVisibleConflict) {
        setReaderNotice("この端末の位置を最終既読にしました。");
      }
    } catch (saveError) {
      setReaderSyncConflictResolutionError(
        saveError instanceof ReaderStateConflictError
          ? "さらに新しい既読位置が保存されています。もう一度確認してください。"
          : toErrorMessage(saveError)
      );
    } finally {
      markReaderSyncConflictResolution("idle");
    }
  }, [
    markReaderSyncConflictResolution,
    getCurrentReaderViewportPosition,
    hasStableReaderEpisode,
    savePosition,
    readerSyncConflict,
    readerSyncConflictResolutionState,
    selectedEpisodeIndex,
    selectedNovelId,
    selectedPosition,
    setReaderNotice
  ]);

  return {
    canApplyReaderSyncConflict: readerSyncConflict !== null && applyDisabledReason === null,
    handleApplyReaderSyncConflict,
    handleOverwriteReaderSyncConflict,
    readerSyncConflictResolutionError
  };
}
