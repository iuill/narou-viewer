import type { EpisodeIndex, ReaderState } from "../features/reader/types";
import { createReadingStateKey } from "../features/reader/readerStateKey";

// Pure reader-state synchronization helpers. The hook lives in useReaderStateSync.ts.
export type ScreenMode = "library" | "reader";

export type ReaderStateAdoptionResult = "stale" | "equal" | "advanced";

export type ReaderSyncConflict = {
  serverState: ReaderState;
};

export type ReaderSyncConflictResolutionState = "idle" | "applying" | "overwriting";

export type PutReaderStateInput = {
  novelId: string;
  episodeIndex: EpisodeIndex | null;
  allowDuringConflict?: boolean;
  expectedStateVersion?: number | null;
  position: number;
  readingStateKey?: string | null;
  signal?: AbortSignal;
};

export type VisibleReaderPosition = {
  episodeIndex: EpisodeIndex | null;
  position: number | null;
};

export function createAbortError(): Error {
  const abortError = new Error("The operation was aborted.");
  abortError.name = "AbortError";
  return abortError;
}

export function classifyReaderStateAdoption(
  currentReaderState: ReaderState | null | undefined,
  nextReaderState: ReaderState
): ReaderStateAdoptionResult {
  if (currentReaderState && nextReaderState.stateVersion < currentReaderState.stateVersion) {
    return "stale";
  }

  if (currentReaderState && nextReaderState.stateVersion === currentReaderState.stateVersion) {
    return "equal";
  }

  return "advanced";
}

export function isSameReadingState(input: PutReaderStateInput, state: ReaderState): boolean {
  return (
    state.novelId === input.novelId &&
    state.lastReadEpisodeIndex === input.episodeIndex &&
    state.position === input.position
  );
}

export function isSelfWriterConflictResolution(input: PutReaderStateInput, serverState: ReaderState, readerClientId: string): boolean {
  return serverState.updatedByClientId === readerClientId && isSameReadingState(input, serverState);
}

export function shouldKeepConflictBlockedForAcceptedState(
  currentConflict: ReaderSyncConflict | null,
  acceptedState: ReaderState
): boolean {
  return (
    currentConflict?.serverState.novelId === acceptedState.novelId &&
    acceptedState.stateVersion < currentConflict.serverState.stateVersion
  );
}

export function isNewerReaderSyncConflict(currentConflict: ReaderSyncConflict | null, serverState: ReaderState): boolean {
  return !currentConflict || currentConflict.serverState.stateVersion < serverState.stateVersion;
}

export function shouldRegisterReaderSyncConflict(input: {
  nextReaderState: ReaderState;
  readerClientId: string;
  screenMode: ScreenMode;
  selectedNovelId: string | null;
  visiblePosition: VisibleReaderPosition;
}): boolean {
  const { nextReaderState, readerClientId, screenMode, selectedNovelId, visiblePosition } = input;
  if (
    screenMode !== "reader" ||
    selectedNovelId !== nextReaderState.novelId ||
    nextReaderState.updatedByClientId === readerClientId ||
    visiblePosition.episodeIndex === null
  ) {
    return false;
  }

  const currentReadingStateKey = createReadingStateKey(
    nextReaderState.novelId,
    visiblePosition.episodeIndex,
    visiblePosition.position
  );
  const nextReadingStateKey = createReadingStateKey(
    nextReaderState.novelId,
    nextReaderState.lastReadEpisodeIndex,
    nextReaderState.position
  );
  return currentReadingStateKey === null || nextReadingStateKey !== currentReadingStateKey;
}
