import {
  useCallback,
  useEffect,
  useEffectEvent,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type MutableRefObject,
  type SetStateAction
} from "react";
import type { EpisodeIndex, EpisodeResponse } from "../features/reader/types";
import {
  getReaderVisiblePositionRange,
  isReaderPositionVisible,
  scrollReaderPositionIntoView
} from "../readerPosition";
import { loadReaderLocalPreferences, saveReaderLocalPreferences, type ReadingMode } from "../readerPreferences";
import {
  buildReaderSpeechChunks,
  createBrowserReaderSpeechEngine,
  findReaderSpeechChunkIndex,
  getBrowserReaderSpeechVoices,
  isBrowserReaderSpeechSupported,
  type ReaderSpeechChunk,
  type ReaderSpeechEngine,
  type ReaderSpeechEngineEvent,
  type ReaderSpeechEngineState,
  type ReaderSpeechVoiceOption
} from "../readerSpeech";

const READER_SPEECH_CHUNK_DEBUG_CLASS = "reader-speech-chunk-debug";

type ReaderSpeechUiState = Extract<ReaderSpeechEngineState, "idle" | "playing" | "paused" | "stopped">;

type ReaderVisiblePositionRange = {
  start: number;
  end: number;
};

type ReaderSpeechPendingPageAdvance = {
  rangeStart: number;
  rangeEnd: number;
};

type ReaderSpeechProgressAutoScrollSuppression = {
  episodeIndex: EpisodeIndex | null;
  position: number;
  chunkStartPosition: number;
  chunkEndPosition: number;
};

type ReaderSpeechVerticalPage = {
  start: number;
  end: number;
  shiftX: number;
};

type ReaderSpeechPagingMetrics = {
  verticalPages: ReaderSpeechVerticalPage[] | null;
};

type UseReaderSpeechOptions = {
  currentPageIndex: number;
  episode: EpisodeResponse | null;
  getCurrentPageIndexFromViewport: (viewport: HTMLDivElement, mode: ReadingMode) => number;
  getCurrentReaderViewportPosition: () => number | null;
  getPagingMetrics: (viewport: HTMLDivElement, mode: ReadingMode) => ReaderSpeechPagingMetrics;
  initialDebugHighlight: boolean;
  initialEnabled: boolean;
  initialPreferRubyText: boolean;
  initialRate: number;
  initialVoiceUri: string | null;
  readerViewportRef: MutableRefObject<HTMLDivElement | null>;
  readingMode: ReadingMode;
  renderedEpisodeHtml: string;
  screenMode: "library" | "reader";
  scrollToPage: (viewport: HTMLDivElement, pageIndex: number, mode: ReadingMode) => void;
  selectedEpisodeIndex: EpisodeIndex | null;
  selectedEpisodeIndexRef: MutableRefObject<EpisodeIndex | null>;
  selectedPositionRef: MutableRefObject<number | null>;
  setCurrentPageIndex: Dispatch<SetStateAction<number>>;
  setError: Dispatch<SetStateAction<string | null>>;
  setIsReaderOverflowOpen: Dispatch<SetStateAction<boolean>>;
  setReaderNotice: Dispatch<SetStateAction<string | null>>;
  setSelectedPosition: Dispatch<SetStateAction<number | null>>;
};

type StopReaderSpeechOptions = {
  notice?: string | null;
};

type UseReaderSpeechResult = {
  handleReaderSpeechPause: () => Promise<void>;
  handleReaderSpeechPlay: () => Promise<void>;
  handleReaderSpeechResume: () => Promise<void>;
  isReaderSpeechPaused: boolean;
  isReaderSpeechPlaying: boolean;
  isReaderSpeechProgressAutoScrollSuppressed: (position: number) => boolean;
  isReaderSpeechSupported: boolean;
  logReaderSpeechDebugEvent: (eventName: string, payload: Record<string, unknown>) => void;
  readerSpeechActiveChunkIndex: number | null;
  readerSpeechChunks: ReaderSpeechChunk[];
  readerSpeechDebugHighlight: boolean;
  readerSpeechEnabled: boolean;
  readerSpeechPreferRubyText: boolean;
  readerSpeechRate: number;
  readerSpeechState: ReaderSpeechUiState;
  readerSpeechVoiceUri: string | null;
  readerSpeechVoices: ReaderSpeechVoiceOption[];
  setReaderSpeechDebugHighlight: Dispatch<SetStateAction<boolean>>;
  setReaderSpeechEnabled: Dispatch<SetStateAction<boolean>>;
  setReaderSpeechPreferRubyText: Dispatch<SetStateAction<boolean>>;
  setReaderSpeechRate: Dispatch<SetStateAction<number>>;
  setReaderSpeechVoiceUri: Dispatch<SetStateAction<string | null>>;
  shouldShowReaderSpeechControls: boolean;
  stopReaderSpeech: (options?: StopReaderSpeechOptions) => Promise<void>;
};

function getReaderSpeechPageAdvancePosition(
  range: ReaderVisiblePositionRange,
  activeChunk: ReaderSpeechChunk | null
): number | null {
  if (activeChunk !== null && activeChunk.endPosition <= range.end) {
    return null;
  }

  return range.end;
}

function logReaderSpeechDebug(eventName: string, payload: Record<string, unknown>): void {
  try {
    console.debug(`[reader-tts] ${eventName} ${JSON.stringify(payload)}`);
  } catch {
    console.debug(`[reader-tts] ${eventName}`, payload);
  }
}

export function useReaderSpeech({
  currentPageIndex,
  episode,
  getCurrentPageIndexFromViewport,
  getCurrentReaderViewportPosition,
  getPagingMetrics,
  initialDebugHighlight,
  initialEnabled,
  initialPreferRubyText,
  initialRate,
  initialVoiceUri,
  readerViewportRef,
  readingMode,
  renderedEpisodeHtml,
  screenMode,
  scrollToPage,
  selectedEpisodeIndex,
  selectedEpisodeIndexRef,
  selectedPositionRef,
  setCurrentPageIndex,
  setError,
  setIsReaderOverflowOpen,
  setReaderNotice,
  setSelectedPosition
}: UseReaderSpeechOptions): UseReaderSpeechResult {
  const [readerSpeechEnabled, setReaderSpeechEnabled] = useState(initialEnabled);
  const [readerSpeechRate, setReaderSpeechRate] = useState(initialRate);
  const [readerSpeechVoiceUri, setReaderSpeechVoiceUri] = useState<string | null>(initialVoiceUri);
  const [readerSpeechPreferRubyText, setReaderSpeechPreferRubyText] = useState(initialPreferRubyText);
  const [readerSpeechDebugHighlight, setReaderSpeechDebugHighlight] = useState(initialDebugHighlight);
  const [readerSpeechVoices, setReaderSpeechVoices] = useState<ReaderSpeechVoiceOption[]>([]);
  const [readerSpeechState, setReaderSpeechState] = useState<ReaderSpeechUiState>("idle");
  const [readerSpeechActiveChunkIndex, setReaderSpeechActiveChunkIndex] = useState<number | null>(null);
  const readerSpeechEngineRef = useRef<ReaderSpeechEngine | null>(null);
  const readerSpeechChunksRef = useRef<ReaderSpeechChunk[]>([]);
  const readerSpeechActiveChunkIndexRef = useRef<number | null>(null);
  const readerSpeechStateRef = useRef<ReaderSpeechUiState>("idle");
  const readerSpeechHighlightedTargetsRef = useRef<Set<HTMLElement>>(new Set());
  const readerSpeechPendingPageAdvanceRef = useRef<ReaderSpeechPendingPageAdvance | null>(null);
  const readerSpeechProgressAutoScrollSuppressionRef = useRef<ReaderSpeechProgressAutoScrollSuppression | null>(null);
  const readerSpeechDebugLogSequenceRef = useRef(0);

  const isReaderSpeechSupported = useMemo(() => isBrowserReaderSpeechSupported(), []);
  const readerSpeechChunks = useMemo(
    () =>
      episode
        ? buildReaderSpeechChunks(episode.readerDocument, {
            preferRubyText: readerSpeechPreferRubyText
          })
        : [],
    [episode, readerSpeechPreferRubyText]
  );
  const isReaderSpeechPlaying = readerSpeechState === "playing";
  const isReaderSpeechPaused = readerSpeechState === "paused";
  const shouldShowReaderSpeechControls = isReaderSpeechSupported;

  readerSpeechChunksRef.current = readerSpeechChunks;
  readerSpeechActiveChunkIndexRef.current = readerSpeechActiveChunkIndex;
  readerSpeechStateRef.current = readerSpeechState;

  const stopReaderSpeech = useCallback(
    async (options?: StopReaderSpeechOptions) => {
      try {
        await readerSpeechEngineRef.current?.stop();
      } catch (speechError) {
        console.error("Failed to stop reader speech", speechError);
      } finally {
        readerSpeechProgressAutoScrollSuppressionRef.current = null;
        readerSpeechActiveChunkIndexRef.current = null;
        readerSpeechStateRef.current = "stopped";
        setReaderSpeechActiveChunkIndex(null);
        setReaderSpeechState("stopped");
        if (options?.notice) {
          setReaderNotice(options.notice);
        }
      }
    },
    [setReaderNotice]
  );

  const isReaderSpeechProgressAutoScrollSuppressed = useCallback((position: number): boolean => {
    const suppressedProgress = readerSpeechProgressAutoScrollSuppressionRef.current;
    if (suppressedProgress?.episodeIndex !== selectedEpisodeIndexRef.current) {
      return false;
    }

    if (suppressedProgress.position === position) {
      return true;
    }

    return (
      readerSpeechStateRef.current === "playing" &&
      position >= suppressedProgress.chunkStartPosition &&
      position <= suppressedProgress.chunkEndPosition
    );
  }, [selectedEpisodeIndexRef]);

  const logReaderSpeechDebugEvent = useEffectEvent((eventName: string, payload: Record<string, unknown>) => {
    if (!readerSpeechDebugHighlight) {
      return;
    }

    readerSpeechDebugLogSequenceRef.current += 1;
    logReaderSpeechDebug(eventName, {
      sequence: readerSpeechDebugLogSequenceRef.current,
      timestampMs: Math.round(window.performance.now()),
      ...payload
    });
  });

  const handleReaderSpeechEngineEvent = useEffectEvent((event: ReaderSpeechEngineEvent) => {
    if (event.type === "state") {
      if (event.state === "playing" || event.state === "paused" || event.state === "stopped") {
        readerSpeechStateRef.current = event.state;
        setReaderSpeechState(event.state);
      }
      return;
    }

    if (event.type === "error") {
      readerSpeechProgressAutoScrollSuppressionRef.current = null;
      readerSpeechActiveChunkIndexRef.current = null;
      readerSpeechStateRef.current = "stopped";
      setReaderSpeechActiveChunkIndex(null);
      setReaderSpeechState("stopped");
      setError(event.message);
      return;
    }

    if (event.type === "progress") {
      if (readerSpeechStateRef.current !== "playing") {
        return;
      }

      const viewport = readerViewportRef.current;
      const chunk = readerSpeechChunksRef.current[event.chunkIndex] ?? null;
      readerSpeechProgressAutoScrollSuppressionRef.current = {
        episodeIndex: selectedEpisodeIndexRef.current,
        position: event.position,
        chunkStartPosition: chunk?.startPosition ?? event.position,
        chunkEndPosition: chunk?.endPosition ?? event.position
      };
      const scrollBefore = viewport
        ? {
            left: viewport.scrollLeft,
            top: viewport.scrollTop
          }
        : null;
      const progressVerticalPage =
        viewport && readingMode === "vertical"
          ? (() => {
              const { verticalPages } = getPagingMetrics(viewport, readingMode);
              const pageIndex = getCurrentPageIndexFromViewport(viewport, readingMode);
              return verticalPages?.[pageIndex] ?? null;
            })()
          : null;
      const progressCurrentVerticalPage = progressVerticalPage
        ? {
            start: progressVerticalPage.start,
            end: progressVerticalPage.end,
            shiftX: progressVerticalPage.shiftX
          }
        : null;
      const visiblePositionRange = viewport
        ? getReaderVisiblePositionRange(viewport, {
            mode: readingMode,
            currentVerticalPage: progressCurrentVerticalPage
          })
        : null;
      const pageAdvancePosition =
        visiblePositionRange !== null ? getReaderSpeechPageAdvancePosition(visiblePositionRange, chunk) : null;
      const currentPositionVisible =
        viewport && pageAdvancePosition !== null
          ? isReaderPositionVisible(viewport, event.position, {
              mode: readingMode,
              currentVerticalPage: progressCurrentVerticalPage
            })
          : false;
      const isPageAdvanceCandidate =
        pageAdvancePosition !== null && event.position >= pageAdvancePosition && !currentPositionVisible;
      const pendingPageAdvanceBefore = readerSpeechPendingPageAdvanceRef.current;
      const shouldAdvancePage = isPageAdvanceCandidate
        ? readerSpeechPendingPageAdvanceRef.current?.rangeStart === visiblePositionRange?.start &&
          readerSpeechPendingPageAdvanceRef.current?.rangeEnd === visiblePositionRange?.end
        : false;
      if (visiblePositionRange && isPageAdvanceCandidate) {
        readerSpeechPendingPageAdvanceRef.current = {
          rangeStart: visiblePositionRange.start,
          rangeEnd: visiblePositionRange.end
        };
      } else {
        readerSpeechPendingPageAdvanceRef.current = null;
      }
      let didAdvancePage = false;
      let pageIndexAfter = currentPageIndex;
      if (viewport && shouldAdvancePage && scrollReaderPositionIntoView(viewport, event.position, readingMode)) {
        pageIndexAfter = getCurrentPageIndexFromViewport(viewport, readingMode);
        scrollToPage(viewport, pageIndexAfter, readingMode);
        setCurrentPageIndex(pageIndexAfter);
        readerSpeechPendingPageAdvanceRef.current = null;
        didAdvancePage = true;
      }
      const scrollAfter = viewport
        ? {
            left: viewport.scrollLeft,
            top: viewport.scrollTop
          }
        : null;
      logReaderSpeechDebugEvent("progress", {
        source: event.source,
        chunkIndex: event.chunkIndex,
        charIndex: event.charIndex,
        elapsedTimeMs: event.elapsedTimeMs,
        position: event.position,
        selectedPosition: selectedPositionRef.current,
        chunk:
          chunk === null
            ? null
            : {
                startPosition: chunk.startPosition,
                endPosition: chunk.endPosition,
                textLength: chunk.text.length,
                anchorCount: chunk.positionAnchors?.length ?? 0
              },
        visiblePositionRange,
        visibleVerticalPage:
          progressVerticalPage === null
            ? null
            : {
                start: progressVerticalPage.start,
                end: progressVerticalPage.end,
                shiftX: progressVerticalPage.shiftX
              },
        pageAdvancePosition,
        pendingPageAdvanceBefore,
        pendingPageAdvanceAfter: readerSpeechPendingPageAdvanceRef.current,
        currentPositionVisible,
        isPageAdvanceCandidate,
        shouldAdvancePage,
        didAdvancePage,
        selectedPositionAutoScrollSuppressed: true,
        readingMode,
        pageIndexBefore: currentPageIndex,
        pageIndexAfter,
        scrollBefore,
        scrollAfter
      });
      if (selectedPositionRef.current !== event.position) {
        setSelectedPosition(event.position);
      }
      return;
    }

    if (event.type !== "chunkEnd") {
      return;
    }

    const nextChunkIndex = event.chunkIndex + 1;
    if (nextChunkIndex >= readerSpeechChunksRef.current.length) {
      readerSpeechProgressAutoScrollSuppressionRef.current = null;
      readerSpeechActiveChunkIndexRef.current = null;
      readerSpeechStateRef.current = "stopped";
      setReaderSpeechActiveChunkIndex(null);
      setReaderSpeechState("stopped");
      setReaderNotice("読み上げが終わりました。");
      return;
    }

    readerSpeechActiveChunkIndexRef.current = nextChunkIndex;
    setReaderSpeechActiveChunkIndex(nextChunkIndex);
    void readerSpeechEngineRef.current?.play(nextChunkIndex);
  });

  const clearReaderSpeechChunkDebugHighlight = useEffectEvent(() => {
    for (const target of readerSpeechHighlightedTargetsRef.current) {
      target.classList.remove(READER_SPEECH_CHUNK_DEBUG_CLASS);
    }
    readerSpeechHighlightedTargetsRef.current.clear();
  });

  const syncReaderSpeechChunkDebugHighlight = useEffectEvent(() => {
    clearReaderSpeechChunkDebugHighlight();

    const viewport = readerViewportRef.current;
    const activeChunk =
      readerSpeechDebugHighlight && readerSpeechActiveChunkIndexRef.current !== null
        ? readerSpeechChunksRef.current[readerSpeechActiveChunkIndexRef.current] ?? null
        : null;
    if (!viewport || !activeChunk) {
      return;
    }

    const targets = viewport.querySelectorAll<HTMLElement>("[data-reader-position-start][data-reader-position-end]");
    for (const target of targets) {
      const start = Number.parseInt(target.dataset.readerPositionStart ?? "", 10);
      const end = Number.parseInt(target.dataset.readerPositionEnd ?? "", 10);
      if (!Number.isInteger(start) || !Number.isInteger(end) || end <= start) {
        continue;
      }

      if (start < activeChunk.endPosition && end > activeChunk.startPosition) {
        target.classList.add(READER_SPEECH_CHUNK_DEBUG_CLASS);
        readerSpeechHighlightedTargetsRef.current.add(target);
      }
    }
  });

  useEffect(() => {
    const currentPreferences = loadReaderLocalPreferences();
    saveReaderLocalPreferences({
      ...currentPreferences,
      speechEnabled: readerSpeechEnabled,
      speechRate: readerSpeechRate,
      speechVoiceUri: readerSpeechVoiceUri,
      speechPreferRubyText: readerSpeechPreferRubyText,
      speechDebugHighlight: readerSpeechDebugHighlight
    });
  }, [
    readerSpeechDebugHighlight,
    readerSpeechEnabled,
    readerSpeechPreferRubyText,
    readerSpeechRate,
    readerSpeechVoiceUri
  ]);

  useEffect(() => {
    if (!isReaderSpeechSupported) {
      setReaderSpeechVoices([]);
      return;
    }

    const updateVoices = () => {
      setReaderSpeechVoices(getBrowserReaderSpeechVoices(window.speechSynthesis));
    };

    updateVoices();

    const synth = window.speechSynthesis;
    if (typeof synth.addEventListener === "function") {
      synth.addEventListener("voiceschanged", updateVoices);
      return () => {
        synth.removeEventListener("voiceschanged", updateVoices);
      };
    }

    const previousHandler = synth.onvoiceschanged;
    synth.onvoiceschanged = updateVoices;
    return () => {
      synth.onvoiceschanged = previousHandler;
    };
  }, [isReaderSpeechSupported]);

  useEffect(() => {
    if (readerSpeechVoiceUri === null) {
      return;
    }

    if (readerSpeechVoices.length === 0) {
      return;
    }

    if (readerSpeechVoices.some((voice) => voice.voiceURI === readerSpeechVoiceUri)) {
      return;
    }

    setReaderSpeechVoiceUri(null);
  }, [readerSpeechVoiceUri, readerSpeechVoices]);

  useEffect(() => {
    if (!isReaderSpeechSupported) {
      return;
    }

    const engine = createBrowserReaderSpeechEngine();
    readerSpeechEngineRef.current = engine;
    const unsubscribe = engine.subscribe(handleReaderSpeechEngineEvent);

    return () => {
      unsubscribe();
      void engine.dispose();
      if (readerSpeechEngineRef.current === engine) {
        readerSpeechEngineRef.current = null;
      }
    };
  }, [isReaderSpeechSupported]);

  useEffect(() => {
    if (screenMode === "reader" && readerSpeechEnabled) {
      return;
    }

    void stopReaderSpeech();
  }, [readerSpeechEnabled, screenMode, stopReaderSpeech]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: episode changes should stop existing speech without stopping immediately when playback starts.
  useEffect(() => {
    if (readerSpeechState !== "playing" && readerSpeechState !== "paused") {
      return;
    }

    void stopReaderSpeech();
  }, [episode?.contentEtag, selectedEpisodeIndex, stopReaderSpeech]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: speech setting changes should stop active playback without stopping immediately when playback starts.
  useEffect(() => {
    if (readerSpeechState !== "playing" && readerSpeechState !== "paused") {
      return;
    }

    void stopReaderSpeech({
      notice: "読み上げ設定が変わったため停止しました。"
    });
  }, [readerSpeechPreferRubyText, readerSpeechRate, readerSpeechVoiceUri, stopReaderSpeech]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: debug highlight sync is intentionally triggered by rendered content and speech state changes.
  useEffect(() => {
    syncReaderSpeechChunkDebugHighlight();
  }, [
    episode?.contentEtag,
    readerSpeechActiveChunkIndex,
    readerSpeechChunks,
    readerSpeechDebugHighlight,
    renderedEpisodeHtml,
    screenMode,
    syncReaderSpeechChunkDebugHighlight
  ]);

  useEffect(() => () => clearReaderSpeechChunkDebugHighlight(), []);

  const handleReaderSpeechPlay = useCallback(async () => {
    if (!episode) {
      return;
    }

    if (!readerSpeechEnabled) {
      setReaderNotice("読み上げパネルで読み上げを有効にしてください。");
      return;
    }

    if (!isReaderSpeechSupported) {
      setReaderNotice("このブラウザでは読み上げを利用できません。");
      return;
    }

    const engine = readerSpeechEngineRef.current;
    if (!engine) {
      setReaderNotice("読み上げエンジンを初期化できませんでした。");
      return;
    }

    if (readerSpeechChunks.length === 0) {
      setReaderNotice("読み上げできる本文が見つかりませんでした。");
      return;
    }

    const startPosition = getCurrentReaderViewportPosition();
    if (startPosition === null) {
      setReaderNotice("現在の表示位置を特定できませんでした。");
      return;
    }

    const startChunkIndex = findReaderSpeechChunkIndex(readerSpeechChunks, startPosition);
    if (startChunkIndex === null) {
      setReaderNotice("読み上げ開始位置を特定できませんでした。");
      return;
    }

    setError(null);
    setIsReaderOverflowOpen(false);

    try {
      await engine.stop();
      await engine.prepare(readerSpeechChunks, {
        rate: readerSpeechRate,
        voiceURI: readerSpeechVoiceUri,
        preferRubyText: readerSpeechPreferRubyText
      });
      readerSpeechActiveChunkIndexRef.current = startChunkIndex;
      setReaderSpeechActiveChunkIndex(startChunkIndex);
      await engine.play(startChunkIndex, { startPosition });
    } catch (speechError) {
      readerSpeechActiveChunkIndexRef.current = null;
      readerSpeechStateRef.current = "stopped";
      setReaderSpeechActiveChunkIndex(null);
      setReaderSpeechState("stopped");
      setError(speechError instanceof Error ? speechError.message : "音声読み上げの開始に失敗しました。");
    }
  }, [
    episode,
    getCurrentReaderViewportPosition,
    isReaderSpeechSupported,
    readerSpeechChunks,
    readerSpeechEnabled,
    readerSpeechPreferRubyText,
    readerSpeechRate,
    readerSpeechVoiceUri,
    setError,
    setIsReaderOverflowOpen,
    setReaderNotice
  ]);

  const handleReaderSpeechPause = useCallback(async () => {
    try {
      await readerSpeechEngineRef.current?.pause();
    } catch (speechError) {
      setError(speechError instanceof Error ? speechError.message : "読み上げの一時停止に失敗しました。");
    }
  }, [setError]);

  const handleReaderSpeechResume = useCallback(async () => {
    try {
      await readerSpeechEngineRef.current?.resume();
    } catch (speechError) {
      setError(speechError instanceof Error ? speechError.message : "読み上げの再開に失敗しました。");
    }
  }, [setError]);

  return {
    handleReaderSpeechPause,
    handleReaderSpeechPlay,
    handleReaderSpeechResume,
    isReaderSpeechPaused,
    isReaderSpeechPlaying,
    isReaderSpeechProgressAutoScrollSuppressed,
    isReaderSpeechSupported,
    logReaderSpeechDebugEvent,
    readerSpeechActiveChunkIndex,
    readerSpeechChunks,
    readerSpeechDebugHighlight,
    readerSpeechEnabled,
    readerSpeechPreferRubyText,
    readerSpeechRate,
    readerSpeechState,
    readerSpeechVoiceUri,
    readerSpeechVoices,
    setReaderSpeechDebugHighlight,
    setReaderSpeechEnabled,
    setReaderSpeechPreferRubyText,
    setReaderSpeechRate,
    setReaderSpeechVoiceUri,
    shouldShowReaderSpeechControls,
    stopReaderSpeech
  };
}
