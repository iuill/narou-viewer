import { useCallback, useEffect, useRef, useState, type Dispatch, type MutableRefObject, type SetStateAction } from "react";
import { fetchReaderState, putReaderState as putReaderStateApi, ReaderStateConflictError } from "../features/reader/api";
import type { NovelSummary } from "../features/library/types";
import type { EpisodeIndex, ReaderState } from "../features/reader/types";
import {
  classifyReaderStateAdoption,
  createAbortError,
  isNewerReaderSyncConflict,
  isSelfWriterConflictResolution,
  shouldKeepConflictBlockedForAcceptedState,
  shouldRegisterReaderSyncConflict as shouldRegisterReaderSyncConflictForPosition,
  type PutReaderStateInput,
  type ReaderStateAdoptionResult,
  type ReaderSyncConflict,
  type ReaderSyncConflictResolutionState,
  type ScreenMode
} from "./readerStateSyncCore";

type UseReaderStateSyncOptions = {
  pendingReadingStateKeyRef: MutableRefObject<string | null>;
  readerClientId: string;
  screenMode: ScreenMode;
  screenModeRef: MutableRefObject<ScreenMode>;
  selectedEpisodeIndexRef: MutableRefObject<EpisodeIndex | null>;
  selectedNovelId: string | null;
  selectedNovelIdRef: MutableRefObject<string | null>;
  selectedPositionRef: MutableRefObject<number | null>;
  setNovels: Dispatch<SetStateAction<NovelSummary[]>>;
};

type UseReaderStateSyncResult = {
  getReaderStateGeneration: (novelId: string) => number;
  putReaderState: (input: PutReaderStateInput) => Promise<ReaderState>;
  readerState: ReaderState | null;
  readerSyncConflict: ReaderSyncConflict | null;
  readerSyncConflictResolutionState: ReaderSyncConflictResolutionState;
  reconcileIncomingReaderState: (nextReaderState: ReaderState) => IncomingReaderStateReconciliation;
  resetReaderStateCache: (novelId: string) => void;
  setReaderState: Dispatch<SetStateAction<ReaderState | null>>;
  setReaderSyncConflict: Dispatch<SetStateAction<ReaderSyncConflict | null>>;
  setReaderSyncConflictResolutionState: Dispatch<SetStateAction<ReaderSyncConflictResolutionState>>;
};

export type IncomingReaderStateReconciliation = {
  activeReaderState: ReaderState;
  disposition: "accepted" | "conflict" | "stale";
};

function timestampValue(value: string | null | undefined): string {
  return typeof value === "string" ? value.trim() : "";
}

function isTimestampAfter(left: string | null | undefined, right: string | null | undefined): boolean {
  const leftValue = timestampValue(left);
  const rightValue = timestampValue(right);
  if (leftValue === "") {
    return false;
  }
  if (rightValue === "") {
    return true;
  }

  const leftTime = Date.parse(leftValue);
  const rightTime = Date.parse(rightValue);
  if (!Number.isNaN(leftTime) && !Number.isNaN(rightTime)) {
    return leftTime > rightTime;
  }
  return leftValue > rightValue;
}

function latestTimestamp(left: string | null | undefined, right: string | null | undefined): string | null {
  const leftValue = timestampValue(left);
  const rightValue = timestampValue(right);
  if (isTimestampAfter(rightValue, leftValue)) {
    return rightValue;
  }
  return leftValue.length > 0 ? leftValue : null;
}

function sortNovelsByActivity(novels: NovelSummary[]): NovelSummary[] {
  return [...novels].sort((left, right) => {
    const leftActivityAt = left.lastActivityAt ?? left.updatedAt;
    const rightActivityAt = right.lastActivityAt ?? right.updatedAt;
    if (isTimestampAfter(leftActivityAt, rightActivityAt)) {
      return -1;
    }
    if (isTimestampAfter(rightActivityAt, leftActivityAt)) {
      return 1;
    }
    return left.novelId < right.novelId ? -1 : left.novelId > right.novelId ? 1 : 0;
  });
}

function updateNovelReadingActivity(novels: NovelSummary[], nextReaderState: ReaderState): NovelSummary[] {
  let didUpdate = false;
  const nextNovels = novels.map((novel) => {
    if (novel.novelId !== nextReaderState.novelId) {
      return novel;
    }
    didUpdate = true;
    return {
      ...novel,
      lastReadEpisodeIndex: nextReaderState.lastReadEpisodeIndex,
      lastActivityAt: latestTimestamp(novel.lastActivityAt ?? novel.updatedAt, nextReaderState.updatedAt)
    };
  });
  return didUpdate ? sortNovelsByActivity(nextNovels) : novels;
}

export function useReaderStateSync({
  pendingReadingStateKeyRef,
  readerClientId,
  screenMode,
  screenModeRef,
  selectedEpisodeIndexRef,
  selectedNovelId,
  selectedNovelIdRef,
  selectedPositionRef,
  setNovels
}: UseReaderStateSyncOptions): UseReaderStateSyncResult {
  const [readerState, setReaderStateDirect] = useState<ReaderState | null>(null);
  const readerStateRef = useRef<ReaderState | null>(null);
  const [readerSyncConflict, setReaderSyncConflictDirect] = useState<ReaderSyncConflict | null>(null);
  const readerSyncConflictRef = useRef<ReaderSyncConflict | null>(null);
  const [readerSyncConflictResolutionState, setReaderSyncConflictResolutionState] =
    useState<ReaderSyncConflictResolutionState>("idle");
  const acknowledgedReaderStateByNovelIdRef = useRef<Map<string, ReaderState>>(new Map());
  const putReaderStateQueueByNovelIdRef = useRef<Map<string, Promise<void>>>(new Map());
  const blockedByConflictNovelIdsRef = useRef<Set<string>>(new Set());
  const readerStateGenerationByNovelIdRef = useRef<Map<string, number>>(new Map());

  const getReaderStateGeneration = useCallback((novelId: string): number => {
    return readerStateGenerationByNovelIdRef.current.get(novelId) ?? 0;
  }, []);

  const classifyAcknowledgedReaderState = useCallback((nextReaderState: ReaderState): ReaderStateAdoptionResult => {
    return classifyReaderStateAdoption(acknowledgedReaderStateByNovelIdRef.current.get(nextReaderState.novelId), nextReaderState);
  }, []);

  const adoptAcknowledgedReaderState = useCallback(
    (nextReaderState: ReaderState): ReaderStateAdoptionResult => {
      const adoptionResult = classifyAcknowledgedReaderState(nextReaderState);
      if (adoptionResult === "stale") {
        return adoptionResult;
      }

      acknowledgedReaderStateByNovelIdRef.current.set(nextReaderState.novelId, nextReaderState);
      return adoptionResult;
    },
    [classifyAcknowledgedReaderState]
  );

  const clearReaderSyncConflict = useCallback((novelId: string | null = null) => {
    if (novelId === null) {
      blockedByConflictNovelIdsRef.current.clear();
      readerSyncConflictRef.current = null;
      setReaderSyncConflictDirect(null);
      return;
    }

    blockedByConflictNovelIdsRef.current.delete(novelId);
    if (readerSyncConflictRef.current?.serverState.novelId === novelId) {
      readerSyncConflictRef.current = null;
    }
    setReaderSyncConflictDirect((current) => (current?.serverState.novelId === novelId ? null : current));
  }, []);

  const clearReaderSyncConflictForAcceptedState = useCallback((acceptedState: ReaderState) => {
    const currentConflict = readerSyncConflictRef.current;
    if (shouldKeepConflictBlockedForAcceptedState(currentConflict, acceptedState)) {
      blockedByConflictNovelIdsRef.current.add(acceptedState.novelId);
      return;
    }

    blockedByConflictNovelIdsRef.current.delete(acceptedState.novelId);
    if (currentConflict?.serverState.novelId === acceptedState.novelId) {
      readerSyncConflictRef.current = null;
      setReaderSyncConflictDirect(null);
    }
  }, []);

  const updateLibraryReadingActivity = useCallback(
    (nextReaderState: ReaderState) => {
      setNovels((current) => updateNovelReadingActivity(current, nextReaderState));
    },
    [setNovels]
  );

  const acceptAcknowledgedReaderState = useCallback(
    (nextReaderState: ReaderState): ReaderStateAdoptionResult => {
      const adoptionResult = adoptAcknowledgedReaderState(nextReaderState);
      if (adoptionResult !== "stale") {
        clearReaderSyncConflictForAcceptedState(nextReaderState);
        updateLibraryReadingActivity(nextReaderState);
      }
      return adoptionResult;
    },
    [adoptAcknowledgedReaderState, clearReaderSyncConflictForAcceptedState, updateLibraryReadingActivity]
  );

  const registerReaderSyncConflict = useCallback((serverState: ReaderState) => {
    blockedByConflictNovelIdsRef.current.add(serverState.novelId);
    if (selectedNovelIdRef.current !== serverState.novelId) {
      return;
    }

    const current = readerSyncConflictRef.current;
    if (!isNewerReaderSyncConflict(current, serverState)) {
      return;
    }

    const nextConflict = { serverState };
    readerSyncConflictRef.current = nextConflict;
    setReaderSyncConflictDirect(nextConflict);
  }, [selectedNovelIdRef]);

  const shouldRegisterReaderSyncConflict = useCallback(
    (nextReaderState: ReaderState): boolean =>
      shouldRegisterReaderSyncConflictForPosition({
        nextReaderState,
        readerClientId,
        screenMode: screenModeRef.current,
        selectedNovelId: selectedNovelIdRef.current,
        visiblePosition: {
          episodeIndex: selectedEpisodeIndexRef.current,
          position: selectedPositionRef.current
        }
      }),
    [readerClientId, screenModeRef, selectedEpisodeIndexRef, selectedNovelIdRef, selectedPositionRef]
  );

  const reconcileAcknowledgedReaderState = useCallback(
    (nextReaderState: ReaderState): ReaderStateAdoptionResult | "conflict" => {
      const adoptionResult = classifyAcknowledgedReaderState(nextReaderState);
      if (adoptionResult !== "advanced") {
        return adoptionResult;
      }
      if (shouldRegisterReaderSyncConflict(nextReaderState)) {
        registerReaderSyncConflict(nextReaderState);
        return "conflict";
      }

      const acceptedResult = acceptAcknowledgedReaderState(nextReaderState);
      if (acceptedResult !== "stale" && selectedNovelIdRef.current === nextReaderState.novelId) {
        readerStateRef.current = nextReaderState;
        setReaderStateDirect(nextReaderState);
      }
      return acceptedResult;
    },
    [acceptAcknowledgedReaderState, classifyAcknowledgedReaderState, registerReaderSyncConflict, selectedNovelIdRef, shouldRegisterReaderSyncConflict]
  );

  const reconcileIncomingReaderState = useCallback(
    (nextReaderState: ReaderState): IncomingReaderStateReconciliation => {
      const currentReaderState = readerStateRef.current;
      const readerStateAdoption = classifyAcknowledgedReaderState(nextReaderState);
      const shouldShowReaderSyncConflict =
        currentReaderState?.novelId === nextReaderState.novelId &&
        readerStateAdoption === "advanced" &&
        shouldRegisterReaderSyncConflict(nextReaderState);

      if (shouldShowReaderSyncConflict) {
        registerReaderSyncConflict(nextReaderState);
        return {
          activeReaderState: currentReaderState,
          disposition: "conflict"
        };
      }

      if (readerStateAdoption === "stale") {
        return {
          activeReaderState: acknowledgedReaderStateByNovelIdRef.current.get(nextReaderState.novelId) ?? nextReaderState,
          disposition: "stale"
        };
      }

      acceptAcknowledgedReaderState(nextReaderState);
      const activeReaderState = acknowledgedReaderStateByNovelIdRef.current.get(nextReaderState.novelId) ?? nextReaderState;
      if (selectedNovelIdRef.current === nextReaderState.novelId) {
        readerStateRef.current = activeReaderState;
        setReaderStateDirect(activeReaderState);
      }
      return {
        activeReaderState,
        disposition: "accepted"
      };
    },
    [
      acceptAcknowledgedReaderState,
      classifyAcknowledgedReaderState,
      registerReaderSyncConflict,
      selectedNovelIdRef,
      shouldRegisterReaderSyncConflict
    ]
  );

  const setReaderSyncConflict = useCallback<Dispatch<SetStateAction<ReaderSyncConflict | null>>>(
    (nextConflictAction) => {
      setReaderSyncConflictDirect((currentConflict) => {
        const nextConflict = typeof nextConflictAction === "function" ? nextConflictAction(currentConflict) : nextConflictAction;
        if (nextConflict === null) {
          const conflictNovelId = currentConflict?.serverState.novelId ?? selectedNovelIdRef.current;
          if (conflictNovelId) {
            blockedByConflictNovelIdsRef.current.delete(conflictNovelId);
          } else {
            blockedByConflictNovelIdsRef.current.clear();
          }
          readerSyncConflictRef.current = null;
          return null;
        }

        blockedByConflictNovelIdsRef.current.add(nextConflict.serverState.novelId);
        readerSyncConflictRef.current = nextConflict;
        return nextConflict;
      });
    },
    [selectedNovelIdRef]
  );

  const resetReaderStateCache = useCallback(
    (novelId: string) => {
      const currentGeneration = getReaderStateGeneration(novelId);
      readerStateGenerationByNovelIdRef.current.set(novelId, currentGeneration + 1);
      acknowledgedReaderStateByNovelIdRef.current.delete(novelId);
      putReaderStateQueueByNovelIdRef.current.delete(novelId);
      clearReaderSyncConflict(novelId);
      if (pendingReadingStateKeyRef.current?.startsWith(`${novelId}:`)) {
        pendingReadingStateKeyRef.current = null;
      }
      setReaderSyncConflictResolutionState("idle");
      if (readerStateRef.current?.novelId === novelId) {
        readerStateRef.current = null;
        setReaderStateDirect(null);
      }
    },
    [clearReaderSyncConflict, getReaderStateGeneration, pendingReadingStateKeyRef]
  );

  const setReaderState = useCallback<Dispatch<SetStateAction<ReaderState | null>>>(
    (nextReaderStateAction) => {
      setReaderStateDirect((currentReaderState) => {
        const nextReaderState =
          typeof nextReaderStateAction === "function"
            ? nextReaderStateAction(currentReaderState)
            : nextReaderStateAction;
        if (nextReaderState) {
          const adoptionResult = acceptAcknowledgedReaderState(nextReaderState);
          if (adoptionResult === "stale") {
            return currentReaderState;
          }
          readerStateRef.current = nextReaderState;
        } else {
          readerStateRef.current = null;
        }
        return nextReaderState;
      });
    },
    [acceptAcknowledgedReaderState]
  );

  readerSyncConflictRef.current = readerSyncConflict;
  readerStateRef.current = readerState;
  if (readerState) {
    adoptAcknowledgedReaderState(readerState);
  }

  const putReaderState = useCallback(
    async (input: PutReaderStateInput): Promise<ReaderState> => {
      const queueByNovelId = putReaderStateQueueByNovelIdRef.current;
      const requestGeneration = getReaderStateGeneration(input.novelId);
      const previousRequest = queueByNovelId.get(input.novelId) ?? Promise.resolve();
      let releaseQueue: () => void = () => {};
      const currentRequest = new Promise<void>((resolve) => {
        releaseQueue = resolve;
      });
      queueByNovelId.set(input.novelId, currentRequest);

      try {
        await previousRequest;

        if (input.signal?.aborted) {
          throw createAbortError();
        }
        if (requestGeneration !== getReaderStateGeneration(input.novelId)) {
          throw createAbortError();
        }
        if (!input.allowDuringConflict && blockedByConflictNovelIdsRef.current.has(input.novelId)) {
          throw createAbortError();
        }

        const acknowledgedReaderState = acknowledgedReaderStateByNovelIdRef.current.get(input.novelId);
        const expectedStateVersion = input.expectedStateVersion ?? acknowledgedReaderState?.stateVersion ?? 0;
        if (input.readingStateKey) {
          pendingReadingStateKeyRef.current = input.readingStateKey;
        }
        const nextReaderState = await putReaderStateApi({
          novelId: input.novelId,
          lastReadEpisodeIndex: input.episodeIndex,
          position: input.position,
          scroll: null,
          clientId: readerClientId,
          expectedStateVersion
        });
        if (requestGeneration !== getReaderStateGeneration(input.novelId)) {
          throw createAbortError();
        }
        const readerStateAdoption = acceptAcknowledgedReaderState(nextReaderState);
        const latestReaderState = acknowledgedReaderStateByNovelIdRef.current.get(nextReaderState.novelId) ?? nextReaderState;
        if (readerStateAdoption !== "stale" && selectedNovelIdRef.current === nextReaderState.novelId) {
          readerStateRef.current = nextReaderState;
          setReaderStateDirect(nextReaderState);
        }
        return latestReaderState;
      } catch (saveError) {
        if (saveError instanceof ReaderStateConflictError) {
          if (requestGeneration !== getReaderStateGeneration(input.novelId)) {
            throw createAbortError();
          }
          const serverStateAdoption = classifyAcknowledgedReaderState(saveError.serverState);
          if (isSelfWriterConflictResolution(input, saveError.serverState, readerClientId)) {
            const selfWriterAdoption = acceptAcknowledgedReaderState(saveError.serverState);
            if (selfWriterAdoption !== "stale" && selectedNovelIdRef.current === saveError.serverState.novelId) {
              readerStateRef.current = saveError.serverState;
              setReaderStateDirect(saveError.serverState);
            }
            return acknowledgedReaderStateByNovelIdRef.current.get(saveError.serverState.novelId) ?? saveError.serverState;
          }
          if (serverStateAdoption === "advanced" && selectedNovelIdRef.current === saveError.serverState.novelId) {
            registerReaderSyncConflict(saveError.serverState);
          }
        }
        throw saveError;
      } finally {
        if (input.readingStateKey && pendingReadingStateKeyRef.current === input.readingStateKey) {
          pendingReadingStateKeyRef.current = null;
        }
        releaseQueue();
        if (queueByNovelId.get(input.novelId) === currentRequest) {
          queueByNovelId.delete(input.novelId);
        }
      }
    },
    [
      acceptAcknowledgedReaderState,
      classifyAcknowledgedReaderState,
      getReaderStateGeneration,
      pendingReadingStateKeyRef,
      readerClientId,
      registerReaderSyncConflict,
      selectedNovelIdRef
    ]
  );

  // biome-ignore lint/correctness/useExhaustiveDependencies: selectedNovelId intentionally resets per-novel sync state.
  useEffect(() => {
    pendingReadingStateKeyRef.current = null;
    clearReaderSyncConflict();
    setReaderSyncConflictResolutionState("idle");
  }, [clearReaderSyncConflict, pendingReadingStateKeyRef, selectedNovelId]);

  useEffect(() => {
    if (screenMode !== "reader") {
      clearReaderSyncConflict();
      setReaderSyncConflictResolutionState("idle");
    }
  }, [clearReaderSyncConflict, screenMode]);

  useEffect(() => {
    if (screenMode !== "reader" || !selectedNovelId || !readerState || readerState.novelId !== selectedNovelId) {
      return;
    }

    const activeNovelId = selectedNovelId;
    const currentReaderState = readerState;
    let cancelled = false;
    let inFlight = false;

    async function syncReaderStateFromServer() {
      if (cancelled || inFlight || document.hidden) {
        return;
      }

      inFlight = true;

      try {
        const nextReaderState = await fetchReaderState(activeNovelId);
        if (nextReaderState === null) {
          return;
        }
        const currentAcknowledgedReaderState = acknowledgedReaderStateByNovelIdRef.current.get(activeNovelId) ?? currentReaderState;
        if (cancelled || nextReaderState.stateVersion <= currentAcknowledgedReaderState.stateVersion) {
          return;
        }

        reconcileAcknowledgedReaderState(nextReaderState);
      } catch {
        return;
      } finally {
        inFlight = false;
      }
    }

    const handleVisibilityChange = () => {
      if (!document.hidden) {
        void syncReaderStateFromServer();
      }
    };
    const handleFocus = () => {
      void syncReaderStateFromServer();
    };

    document.addEventListener("visibilitychange", handleVisibilityChange);
    window.addEventListener("focus", handleFocus);

    return () => {
      cancelled = true;
      document.removeEventListener("visibilitychange", handleVisibilityChange);
      window.removeEventListener("focus", handleFocus);
    };
  }, [reconcileAcknowledgedReaderState, readerState, screenMode, selectedNovelId]);

  return {
    getReaderStateGeneration,
    putReaderState,
    readerState,
    readerSyncConflict,
    readerSyncConflictResolutionState,
    reconcileIncomingReaderState,
    resetReaderStateCache,
    setReaderState,
    setReaderSyncConflict,
    setReaderSyncConflictResolutionState
  };
}

export type {
  PutReaderStateInput,
  ReaderStateAdoptionResult,
  ReaderSyncConflict,
  ReaderSyncConflictResolutionState,
  ScreenMode
} from "./readerStateSyncCore";
