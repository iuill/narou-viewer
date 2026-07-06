import { useEffect, useRef, useState } from "react";
import { fetchFetcherStatus } from "../features/fetcher/api";
import type { FetcherQueueResponse } from "../features/fetcher/types";
import { toFetcherTaskSummary, type FetcherTaskSummary } from "../features/fetcher/model";

const FETCHER_STATUS_POLL_INTERVAL_MS = 2000;

type UseFetcherStatusOptions = {
  isPaused: boolean;
  refreshKey: unknown;
  onQueueSettled: () => void;
};

type UseFetcherStatusResult = {
  queue: FetcherQueueResponse | null;
  tasks: FetcherTaskSummary | null;
  checkedAt: string | null;
  isLoading: boolean;
  error: string | null;
};

const EMPTY_FETCHER_TASK_SUMMARY: FetcherTaskSummary = {
  current: null,
  queued: [],
  recentCompleted: [],
  recentFailed: [],
  completedCount: 0,
  failedCount: 0,
  convertCurrent: null,
  convertQueued: []
};

function hasActiveFetcherWork(queue: FetcherQueueResponse | null, tasks: FetcherTaskSummary): boolean {
  return (
    (queue?.running ?? false) ||
    (queue?.total ?? 0) > 0 ||
    tasks.current !== null ||
    tasks.queued.length > 0 ||
    tasks.convertCurrent !== null ||
    tasks.convertQueued.length > 0
  );
}

export function useFetcherStatus({ isPaused, refreshKey, onQueueSettled }: UseFetcherStatusOptions): UseFetcherStatusResult {
  const [queue, setQueue] = useState<FetcherQueueResponse | null>(null);
  const [tasks, setTasks] = useState<FetcherTaskSummary | null>(null);
  const [checkedAt, setCheckedAt] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const onQueueSettledRef = useRef(onQueueSettled);
  const previousQueueActiveRef = useRef(false);
  const previousTaskCountersRef = useRef<{ completedCount: number; failedCount: number } | null>(null);
  const statusRequestInFlightRef = useRef(false);

  useEffect(() => {
    onQueueSettledRef.current = onQueueSettled;
  }, [onQueueSettled]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: refreshKey intentionally forces an immediate status reload.
  useEffect(() => {
    if (isPaused) {
      return;
    }

    let cancelled = false;

    async function loadStatus() {
      if (statusRequestInFlightRef.current) {
        return;
      }

      statusRequestInFlightRef.current = true;
      try {
        const nextStatus = await fetchFetcherStatus();

        if (!cancelled) {
          if (nextStatus.queue !== null) {
            setQueue(nextStatus.queue);
          }
          if (nextStatus.tasks !== null) {
            setTasks(toFetcherTaskSummary(nextStatus.tasks));
          }
          if (nextStatus.didUpdate) {
            setCheckedAt(new Date().toISOString());
          }
          setError(nextStatus.error);
        }
      } catch (loadError) {
        if (!cancelled) {
          setError(loadError instanceof Error ? loadError.message : "Unknown error");
        }
      } finally {
        statusRequestInFlightRef.current = false;
        if (!cancelled) {
          setIsLoading(false);
        }
      }
    }

    void loadStatus();
    const intervalId = window.setInterval(() => {
      if (!document.hidden) {
        void loadStatus();
      }
    }, FETCHER_STATUS_POLL_INTERVAL_MS);

    return () => {
      cancelled = true;
      window.clearInterval(intervalId);
    };
  }, [isPaused, refreshKey]);

  useEffect(() => {
    if (!queue && !tasks) {
      return;
    }

    const taskSummary = tasks ?? EMPTY_FETCHER_TASK_SUMMARY;
    const isQueueActive = hasActiveFetcherWork(queue, taskSummary);
    const previousWasActive = previousQueueActiveRef.current;
    const previousCounters = previousTaskCountersRef.current;
    const countersChangedAfterActive =
      previousWasActive &&
      previousCounters !== null &&
      (previousCounters.completedCount !== taskSummary.completedCount ||
        previousCounters.failedCount !== taskSummary.failedCount);

    previousQueueActiveRef.current = isQueueActive;
    previousTaskCountersRef.current = {
      completedCount: taskSummary.completedCount,
      failedCount: taskSummary.failedCount
    };

    if ((previousWasActive && !isQueueActive) || (countersChangedAfterActive && !isQueueActive)) {
      onQueueSettledRef.current();
    }
  }, [queue, tasks]);

  return {
    queue,
    tasks,
    checkedAt,
    isLoading,
    error
  };
}
