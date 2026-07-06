import { useEffect, useEffectEvent, useMemo, useRef, useState, type Dispatch, type SetStateAction } from "react";
import { clearCharacterSummary, fetchCharacterJobs, fetchCharacterSummary, submitCharacterJob } from "../features/characters/api";
import type { CharacterGenerationStrategy, CharacterJobsResponse, CharacterSummaryResponse } from "../features/characters/types";
import type { EpisodeIndex, TocResponse } from "../features/reader/types";
import { compareEpisodeIndex } from "../features/reader/episodeIndex";
import {
  isCharacterSummaryActiveJob,
  isCharacterSummaryCompletedJob,
  isCharacterSummaryRequestAllowed,
  resolveCharacterSummaryRefreshTarget
} from "../characterSummaryUtils";

type UseCharacterSummaryOptions = {
  currentTocEpisodeIndex: number;
  formatEpisodeOrderLabel: (episodeIndex: string) => string;
  isOpen: boolean;
  onClosePanel: () => void;
  onOpenPanel: () => void;
  selectedNovelId: string | null;
  setReaderNotice: Dispatch<SetStateAction<string | null>>;
  screenMode: "library" | "reader";
  toc: TocResponse | null;
};

type UseCharacterSummaryResult = {
  activeJobs: NonNullable<CharacterJobsResponse["jobs"]>;
  canGenerate: boolean;
  canClear: boolean;
  completedJobs: NonNullable<CharacterJobsResponse["jobs"]>;
  data: CharacterSummaryResponse | null;
  defaultUpToEpisodeIndex: string | null;
  error: string | null;
  handleClear: () => Promise<void>;
  handleGenerate: () => Promise<void>;
  handleOpen: () => Promise<void>;
  isClearing: boolean;
  isLoading: boolean;
  isSubmitting: boolean;
  notice: string | null;
  requestedGenerationStrategy: CharacterGenerationStrategy;
  requestedUpToEpisodeIndex: string;
  setRequestedGenerationStrategy: Dispatch<SetStateAction<CharacterGenerationStrategy>>;
  setRequestedUpToEpisodeIndex: Dispatch<SetStateAction<string>>;
};

export function useCharacterSummary({
  currentTocEpisodeIndex,
  formatEpisodeOrderLabel,
  isOpen,
  onClosePanel,
  onOpenPanel,
  selectedNovelId,
  setReaderNotice,
  screenMode,
  toc
}: UseCharacterSummaryOptions): UseCharacterSummaryResult {
  const [requestedUpToEpisodeIndex, setRequestedUpToEpisodeIndex] = useState("");
  const [requestedGenerationStrategy, setRequestedGenerationStrategy] =
    useState<CharacterGenerationStrategy>("parallel_identity");
  const [data, setData] = useState<CharacterSummaryResponse | null>(null);
  const [jobs, setJobs] = useState<CharacterJobsResponse | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [isClearing, setIsClearing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestSeqRef = useRef(0);

  const defaultUpToEpisodeIndex = useMemo(() => {
    if (currentTocEpisodeIndex <= 0) {
      return null;
    }

    return String(currentTocEpisodeIndex);
  }, [currentTocEpisodeIndex]);
  const requestedUpToEpisodeOrder = useMemo(() => {
    const requested = requestedUpToEpisodeIndex.trim();
    return requested.length > 0 && /^\d+$/.test(requested) ? requested : null;
  }, [requestedUpToEpisodeIndex]);
  const requestedUpToEpisodeActualIndex = useMemo(() => {
    if (!toc || requestedUpToEpisodeOrder === null) {
      return null;
    }

    const requestedOrder = Number.parseInt(requestedUpToEpisodeOrder, 10);
    return toc.episodes[requestedOrder - 1]?.episodeIndex ?? null;
  }, [requestedUpToEpisodeOrder, toc]);
  const defaultUpToEpisodeActualIndex = useMemo(() => {
    if (!toc || defaultUpToEpisodeIndex === null) {
      return null;
    }

    return toc.episodes[Number.parseInt(defaultUpToEpisodeIndex, 10) - 1]?.episodeIndex ?? null;
  }, [defaultUpToEpisodeIndex, toc]);
  const canGenerate = isCharacterSummaryRequestAllowed({
    defaultUpToEpisodeIndex,
    requestedUpToEpisodeIndex: requestedUpToEpisodeOrder
  });
  const activeJobs = useMemo(
    () => jobs?.jobs.filter((job) => isCharacterSummaryActiveJob(job.status)) ?? [],
    [jobs]
  );
  const completedJobs = useMemo(
    () => jobs?.jobs.filter((job) => isCharacterSummaryCompletedJob(job.status)) ?? [],
    [jobs]
  );
  const canClear =
    selectedNovelId !== null &&
    activeJobs.length === 0 &&
    (data?.status === "ready" || completedJobs.length > 0);

  async function load(targetUpToEpisodeIndex: EpisodeIndex, options?: { background?: boolean }) {
    if (!selectedNovelId) {
      return;
    }

    const requestSeq = requestSeqRef.current + 1;
    requestSeqRef.current = requestSeq;
    const isBackgroundRefresh = options?.background === true;
    const hasVisibleSummary = data !== null || jobs !== null;

    if (!isBackgroundRefresh) {
      setIsLoading(true);
      setNotice(null);
    }

    setError(null);

    try {
      const novelId = selectedNovelId;
      const [initialSummary, nextJobs] = await Promise.all([
        fetchCharacterSummary(novelId, targetUpToEpisodeIndex),
        fetchCharacterJobs(novelId)
      ]);

      let summary = initialSummary;
      let nextNotice: string | null = null;

      if (
        initialSummary.status === "not_generated" &&
        initialSummary.processedUpToEpisodeIndex !== null &&
        compareEpisodeIndex(initialSummary.processedUpToEpisodeIndex, targetUpToEpisodeIndex) < 0
      ) {
        if (requestSeq !== requestSeqRef.current) {
          return;
        }

        const fallbackSummary = await fetchCharacterSummary(novelId, initialSummary.processedUpToEpisodeIndex);

        if (fallbackSummary.status === "ready") {
          summary = fallbackSummary;
          nextNotice =
            `第${formatEpisodeOrderLabel(fallbackSummary.upToEpisodeIndex)}話時点までの生成済み一覧を表示しています。` +
            `第${formatEpisodeOrderLabel(targetUpToEpisodeIndex)}話時点の一覧はまだ生成されていません。`;
        }
      }

      if (requestSeq !== requestSeqRef.current) {
        return;
      }

      setData(summary);
      setJobs(nextJobs);
      setNotice(nextNotice);
    } catch (loadError) {
      if (requestSeq !== requestSeqRef.current) {
        return;
      }

      setError(loadError instanceof Error ? loadError.message : "Unknown error");

      if (!isBackgroundRefresh || !hasVisibleSummary) {
        setData(null);
        setJobs(null);
        setNotice(null);
      }
    } finally {
      if (requestSeq === requestSeqRef.current) {
        setIsLoading(false);
      }
    }
  }

  const refreshInBackground = useEffectEvent((targetUpToEpisodeIndex: EpisodeIndex) => {
    void load(targetUpToEpisodeIndex, { background: true });
  });

  // biome-ignore lint/correctness/useExhaustiveDependencies: selectedNovelId intentionally resets panel state without reacting to handler identity.
  useEffect(() => {
    requestSeqRef.current += 1;
    onClosePanel();
    setData(null);
    setJobs(null);
    setError(null);
    setRequestedUpToEpisodeIndex("");
    setRequestedGenerationStrategy("parallel_identity");
    setIsLoading(false);
    setIsSubmitting(false);
    setIsClearing(false);
  }, [selectedNovelId]);

  useEffect(() => {
    if (screenMode !== "reader") {
      requestSeqRef.current += 1;
      onClosePanel();
    }
  }, [onClosePanel, screenMode]);

  useEffect(() => {
    if (!isOpen) {
      return;
    }

    if (defaultUpToEpisodeIndex === null) {
      setRequestedUpToEpisodeIndex("");
      return;
    }

    setRequestedUpToEpisodeIndex((current) => {
      if (!/^\d+$/.test(current)) {
        return defaultUpToEpisodeIndex;
      }

      return compareEpisodeIndex(current, defaultUpToEpisodeIndex) <= 0 ? current : defaultUpToEpisodeIndex;
    });
  }, [defaultUpToEpisodeIndex, isOpen]);

  useEffect(() => {
    const refreshTarget = resolveCharacterSummaryRefreshTarget({
      defaultUpToEpisodeIndex,
      requestedUpToEpisodeIndex: requestedUpToEpisodeOrder
    });
    const refreshTargetEpisodeIndex =
      refreshTarget !== null && toc ? toc.episodes[Number.parseInt(refreshTarget, 10) - 1]?.episodeIndex ?? null : null;

    if (!isOpen || selectedNovelId === null || refreshTargetEpisodeIndex === null) {
      return;
    }

    const intervalId = window.setInterval(() => {
      refreshInBackground(refreshTargetEpisodeIndex);
    }, 4000);

    return () => {
      window.clearInterval(intervalId);
    };
  }, [defaultUpToEpisodeIndex, isOpen, requestedUpToEpisodeOrder, selectedNovelId, toc]);

  async function handleOpen() {
    setRequestedUpToEpisodeIndex(defaultUpToEpisodeIndex ?? "");
    onOpenPanel();

    if (defaultUpToEpisodeActualIndex) {
      await load(defaultUpToEpisodeActualIndex);
    } else {
      setData(null);
      setJobs(null);
      setNotice(null);
      setError(null);
    }
  }

  async function handleClear() {
    if (!selectedNovelId || activeJobs.length > 0) {
      return;
    }

    const refreshTarget = requestedUpToEpisodeActualIndex ?? defaultUpToEpisodeActualIndex;
    setIsClearing(true);
    setError(null);

    try {
      const result = await clearCharacterSummary(selectedNovelId);
      setReaderNotice(result.message);
      setNotice(null);
      setJobs({ jobs: [] });
      if (refreshTarget) {
        await load(refreshTarget);
      } else {
        setData(null);
      }
    } catch (clearError) {
      setError(clearError instanceof Error ? clearError.message : "Unknown error");
    } finally {
      setIsClearing(false);
    }
  }

  async function handleGenerate() {
    if (!selectedNovelId) {
      return;
    }

    if (
      requestedUpToEpisodeActualIndex === null ||
      !isCharacterSummaryRequestAllowed({
        defaultUpToEpisodeIndex,
        requestedUpToEpisodeIndex: requestedUpToEpisodeOrder
      })
    ) {
      return;
    }

    setIsSubmitting(true);
    setError(null);

    try {
      const result = await submitCharacterJob(selectedNovelId, {
        upToEpisodeIndex: requestedUpToEpisodeActualIndex,
        generationStrategy: requestedGenerationStrategy
      });
      setReaderNotice(result.message);
      await load(requestedUpToEpisodeActualIndex);
    } catch (submitError) {
      setError(submitError instanceof Error ? submitError.message : "Unknown error");
    } finally {
      setIsSubmitting(false);
    }
  }

  return {
    activeJobs,
    canClear,
    canGenerate,
    completedJobs,
    data,
    defaultUpToEpisodeIndex,
    error,
    handleClear,
    handleGenerate,
    handleOpen,
    isClearing,
    isLoading,
    isSubmitting,
    notice,
    requestedGenerationStrategy,
    requestedUpToEpisodeIndex,
    setRequestedGenerationStrategy,
    setRequestedUpToEpisodeIndex
  };
}
