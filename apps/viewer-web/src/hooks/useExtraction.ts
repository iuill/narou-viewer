import {
  useEffect,
  useEffectEvent,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { fetchCharacterSummary } from "../features/characters/api";
import type { CharacterSummaryResponse } from "../features/characters/types";
import {
  clearExtraction,
  fetchExtractionJobs,
  submitExtraction,
} from "../features/extraction/api";
import type {
  ExtractionGenerationStrategy,
  ExtractionJobsResponse,
} from "../features/extraction/types";
import { fetchTerms } from "../features/terms/api";
import type { TermsResponse } from "../features/terms/types";
import type { EpisodeIndex, TocResponse } from "../features/reader/types";
import { compareEpisodeIndex } from "../features/reader/episodeIndex";
import {
  isCharacterSummaryActiveJob,
  isCharacterSummaryCompletedJob,
  isCharacterSummaryRequestAllowed,
  resolveCharacterSummaryRefreshTarget,
} from "../characterSummaryUtils";

type UseExtractionOptions = {
  currentTocEpisodeIndex: number;
  formatEpisodeOrderLabel: (episodeIndex: string) => string;
  isOpen: boolean;
  onClosePanel: () => void;
  onOpenPanel: () => void;
  onOpenTermsPanel: () => void;
  selectedNovelId: string | null;
  setReaderNotice: Dispatch<SetStateAction<string | null>>;
  screenMode: "library" | "reader";
  toc: TocResponse | null;
};

type UseExtractionResult = {
  activeJobs: NonNullable<ExtractionJobsResponse["jobs"]>;
  canGenerate: boolean;
  canClear: boolean;
  completedJobs: NonNullable<ExtractionJobsResponse["jobs"]>;
  data: CharacterSummaryResponse | null;
  termsData: TermsResponse | null;
  defaultUpToEpisodeIndex: string | null;
  error: string | null;
  handleClear: () => Promise<void>;
  handleGenerate: () => Promise<void>;
  handleOpen: () => Promise<void>;
  handleOpenTerms: () => Promise<void>;
  isClearing: boolean;
  isLoading: boolean;
  isSubmitting: boolean;
  includeCurrentEpisode: boolean;
  notice: string | null;
  requestedGenerationStrategy: ExtractionGenerationStrategy;
  requestedUpToEpisodeIndex: string;
  setRequestedGenerationStrategy: Dispatch<
    SetStateAction<ExtractionGenerationStrategy>
  >;
  setIncludeCurrentEpisode: (include: boolean) => void;
  setRequestedUpToEpisodeIndex: Dispatch<SetStateAction<string>>;
};

export function useExtraction({
  currentTocEpisodeIndex,
  formatEpisodeOrderLabel,
  isOpen,
  onClosePanel,
  onOpenPanel,
  onOpenTermsPanel,
  selectedNovelId,
  setReaderNotice,
  screenMode,
  toc,
}: UseExtractionOptions): UseExtractionResult {
  const [requestedUpToEpisodeIndex, setRequestedUpToEpisodeIndex] =
    useState("");
  const [requestedGenerationStrategy, setRequestedGenerationStrategy] =
    useState<ExtractionGenerationStrategy>("parallel_identity");
  const [includeCurrentEpisode, setIncludeCurrentEpisodeState] = useState(false);
  const [data, setData] = useState<CharacterSummaryResponse | null>(null);
  const [termsData, setTermsData] = useState<TermsResponse | null>(null);
  const [jobs, setJobs] = useState<ExtractionJobsResponse | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [isClearing, setIsClearing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestSeqRef = useRef(0);

  const defaultUpToEpisodeIndex = useMemo(() => {
    const episodeOrder = currentTocEpisodeIndex + (includeCurrentEpisode ? 1 : 0);
    if (episodeOrder <= 0) {
      return null;
    }

    return String(episodeOrder);
  }, [currentTocEpisodeIndex, includeCurrentEpisode]);
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

    return (
      toc.episodes[Number.parseInt(defaultUpToEpisodeIndex, 10) - 1]
        ?.episodeIndex ?? null
    );
  }, [defaultUpToEpisodeIndex, toc]);
  const canGenerate = isCharacterSummaryRequestAllowed({
    defaultUpToEpisodeIndex,
    requestedUpToEpisodeIndex: requestedUpToEpisodeOrder,
  });
  const activeJobs = useMemo(
    () =>
      jobs?.jobs.filter((job) => isCharacterSummaryActiveJob(job.status)) ?? [],
    [jobs],
  );
  const completedJobs = useMemo(
    () =>
      jobs?.jobs.filter((job) => isCharacterSummaryCompletedJob(job.status)) ??
      [],
    [jobs],
  );
  const canClear =
    selectedNovelId !== null &&
    activeJobs.length === 0 &&
    (data?.status === "ready" ||
      data?.status === "partial" ||
      termsData?.status === "ready" ||
      termsData?.status === "partial" ||
      completedJobs.length > 0);

  function setIncludeCurrentEpisode(include: boolean) {
    setIncludeCurrentEpisodeState(include);
    const nextEpisodeOrder = currentTocEpisodeIndex + (include ? 1 : 0);
    setRequestedUpToEpisodeIndex(nextEpisodeOrder > 0 ? String(nextEpisodeOrder) : "");

    const targetEpisodeIndex = nextEpisodeOrder > 0 ? toc?.episodes[nextEpisodeOrder - 1]?.episodeIndex ?? null : null;
    if (isOpen && targetEpisodeIndex !== null) {
      void load(targetEpisodeIndex);
    } else if (isOpen) {
      setData(null);
      setTermsData(null);
      setJobs(null);
      setNotice(null);
      setError(null);
    }
  }

  async function load(
    targetUpToEpisodeIndex: EpisodeIndex,
    options?: { background?: boolean },
  ) {
    if (!selectedNovelId) {
      return;
    }

    const requestSeq = requestSeqRef.current + 1;
    requestSeqRef.current = requestSeq;
    const isBackgroundRefresh = options?.background === true;
    const hasVisibleSummary =
      data !== null || termsData !== null || jobs !== null;

    if (!isBackgroundRefresh) {
      setIsLoading(true);
      setNotice(null);
    }

    setError(null);

    try {
      const novelId = selectedNovelId;
      const [initialSummary, initialTerms, nextJobs] = await Promise.all([
        fetchCharacterSummary(novelId, targetUpToEpisodeIndex),
        fetchTerms(novelId, targetUpToEpisodeIndex),
        fetchExtractionJobs(novelId),
      ]);

      const notices: string[] = [];

      if (initialSummary.status === "partial" && initialSummary.processedUpToEpisodeIndex !== null) {
        notices.push(
          `第${formatEpisodeOrderLabel(initialSummary.processedUpToEpisodeIndex)}話時点までの生成済み人物一覧を表示しています。` +
            `第${formatEpisodeOrderLabel(targetUpToEpisodeIndex)}話時点まではまだ生成されていません。`,
        );
      }

      if (initialTerms.status === "partial" && initialTerms.processedUpToEpisodeIndex !== null) {
        notices.push(
          `第${formatEpisodeOrderLabel(initialTerms.processedUpToEpisodeIndex)}話時点までの生成済み用語一覧を表示しています。` +
            `第${formatEpisodeOrderLabel(targetUpToEpisodeIndex)}話時点まではまだ生成されていません。`,
        );
      }

      if (
        initialSummary.status === "ready" &&
        initialTerms.status === "not_generated" &&
        initialTerms.processedUpToEpisodeIndex === null
      ) {
        notices.push(
          "旧生成データには用語が含まれないため、抽出データをクリアして再生成してください。",
        );
      }

      if (requestSeq !== requestSeqRef.current) {
        return;
      }

      setData(initialSummary);
      setTermsData(initialTerms);
      setJobs(nextJobs);
      setNotice(notices.length > 0 ? notices.join(" ") : null);
    } catch (loadError) {
      if (requestSeq !== requestSeqRef.current) {
        return;
      }

      setError(
        loadError instanceof Error ? loadError.message : "Unknown error",
      );

      if (!isBackgroundRefresh || !hasVisibleSummary) {
        setData(null);
        setTermsData(null);
        setJobs(null);
        setNotice(null);
      }
    } finally {
      if (requestSeq === requestSeqRef.current) {
        setIsLoading(false);
      }
    }
  }

  const refreshInBackground = useEffectEvent(
    (targetUpToEpisodeIndex: EpisodeIndex) => {
      void load(targetUpToEpisodeIndex, { background: true });
    },
  );

  // biome-ignore lint/correctness/useExhaustiveDependencies: selectedNovelId intentionally resets panel state without reacting to handler identity.
  useEffect(() => {
    requestSeqRef.current += 1;
    onClosePanel();
    setData(null);
    setTermsData(null);
    setJobs(null);
    setError(null);
    setRequestedUpToEpisodeIndex("");
    setRequestedGenerationStrategy("parallel_identity");
    setIncludeCurrentEpisodeState(false);
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

      return compareEpisodeIndex(current, defaultUpToEpisodeIndex) <= 0
        ? current
        : defaultUpToEpisodeIndex;
    });
  }, [defaultUpToEpisodeIndex, isOpen]);

  useEffect(() => {
    const refreshTarget = resolveCharacterSummaryRefreshTarget({
      defaultUpToEpisodeIndex,
      requestedUpToEpisodeIndex: requestedUpToEpisodeOrder,
    });
    const refreshTargetEpisodeIndex =
      refreshTarget !== null && toc
        ? (toc.episodes[Number.parseInt(refreshTarget, 10) - 1]?.episodeIndex ??
          null)
        : null;

    if (
      !isOpen ||
      selectedNovelId === null ||
      refreshTargetEpisodeIndex === null
    ) {
      return;
    }

    const intervalId = window.setInterval(() => {
      refreshInBackground(refreshTargetEpisodeIndex);
    }, 4000);

    return () => {
      window.clearInterval(intervalId);
    };
  }, [
    defaultUpToEpisodeIndex,
    isOpen,
    requestedUpToEpisodeOrder,
    selectedNovelId,
    toc,
  ]);

  async function handleOpen() {
    setRequestedUpToEpisodeIndex(defaultUpToEpisodeIndex ?? "");
    onOpenPanel();

    if (defaultUpToEpisodeActualIndex) {
      await load(defaultUpToEpisodeActualIndex);
    } else {
      setData(null);
      setTermsData(null);
      setJobs(null);
      setNotice(null);
      setError(null);
    }
  }

  async function handleOpenTerms() {
    setRequestedUpToEpisodeIndex(defaultUpToEpisodeIndex ?? "");
    onOpenTermsPanel();
    if (defaultUpToEpisodeActualIndex) {
      await load(defaultUpToEpisodeActualIndex);
    } else {
      setData(null);
      setTermsData(null);
      setJobs(null);
      setNotice(null);
      setError(null);
    }
  }

  async function handleClear() {
    if (!selectedNovelId || activeJobs.length > 0) {
      return;
    }

    const refreshTarget =
      requestedUpToEpisodeActualIndex ?? defaultUpToEpisodeActualIndex;
    setIsClearing(true);
    setError(null);

    try {
      const result = await clearExtraction(selectedNovelId);
      setReaderNotice(result.message);
      setNotice(null);
      setJobs({ jobs: [] });
      if (refreshTarget) {
        await load(refreshTarget);
      } else {
        setData(null);
        setTermsData(null);
      }
    } catch (clearError) {
      setError(
        clearError instanceof Error ? clearError.message : "Unknown error",
      );
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
        requestedUpToEpisodeIndex: requestedUpToEpisodeOrder,
      })
    ) {
      return;
    }

    setIsSubmitting(true);
    setError(null);

    try {
      const result = await submitExtraction(selectedNovelId, {
        upToEpisodeIndex: requestedUpToEpisodeActualIndex,
        generationStrategy: requestedGenerationStrategy,
      });
      setReaderNotice(result.message);
      await load(requestedUpToEpisodeActualIndex);
    } catch (submitError) {
      setError(
        submitError instanceof Error ? submitError.message : "Unknown error",
      );
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
    termsData,
    defaultUpToEpisodeIndex,
    error,
    handleClear,
    handleGenerate,
    handleOpen,
    handleOpenTerms,
    isClearing,
    isLoading,
    isSubmitting,
    includeCurrentEpisode,
    notice,
    requestedGenerationStrategy,
    requestedUpToEpisodeIndex,
    setRequestedGenerationStrategy,
    setIncludeCurrentEpisode,
    setRequestedUpToEpisodeIndex,
  };
}
