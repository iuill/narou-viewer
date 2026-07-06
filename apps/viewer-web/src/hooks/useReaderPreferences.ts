import { useCallback, useEffect, useMemo, useRef, useState, type Dispatch, type SetStateAction } from "react";
import { fetchReaderPreferences, putReaderPreferences } from "../features/reader/api";
import {
  DEFAULT_READER_EXPERIMENTAL_FONT_ID,
  getReaderArticleFontFamilyCss,
  getReaderArticleFontWeight,
  getReaderExperimentalFontOption
} from "../readerExperimentalFonts";
import {
  DEFAULT_READER_LOCAL_PREFERENCES,
  DEFAULT_READER_SERVER_PREFERENCES,
  getReaderServerPreferencesKey,
  loadReaderLocalPreferences,
  saveReaderLocalPreferences,
  type ReaderExperimentalFontId,
  type ReaderExperimentalFontWeight,
  type ReaderFontFamily,
  type ReaderServerPreferences,
  type ReaderTheme,
  type ReadingMode
} from "../readerPreferences";

type ReaderExperimentalFontLoadStatus = "idle" | "loading" | "ready" | "error";

type UseReaderPreferencesOptions = {
  initialLocalPreferences: ReturnType<typeof loadReaderLocalPreferences>;
  setError: Dispatch<SetStateAction<string | null>>;
  setReaderNotice: Dispatch<SetStateAction<string | null>>;
};

type UseReaderPreferencesResult = {
  debugPageOverflow: boolean;
  handleResetReaderPreferences: () => void;
  handleRetryReaderExperimentalFontLoad: () => void;
  readerArticleFontFamilyCss: string;
  readerArticleFontWeight: ReaderExperimentalFontWeight | null;
  readerExperimentalFontId: ReaderExperimentalFontId;
  readerExperimentalFontLayoutVersion: number;
  readerExperimentalFontLoadStatus: ReaderExperimentalFontLoadStatus;
  readerExperimentalFontOption: ReturnType<typeof getReaderExperimentalFontOption>;
  readerExperimentalFontWeight: ReaderExperimentalFontWeight;
  readerFontFamily: ReaderFontFamily;
  readerFontSizePx: number;
  readerLetterSpacingEm: number;
  readerTheme: ReaderTheme;
  readingMode: ReadingMode;
  reverseTapPageNavigation: boolean;
  setDebugPageOverflow: Dispatch<SetStateAction<boolean>>;
  setReaderExperimentalFontId: Dispatch<SetStateAction<ReaderExperimentalFontId>>;
  setReaderExperimentalFontWeight: Dispatch<SetStateAction<ReaderExperimentalFontWeight>>;
  setReaderFontFamily: Dispatch<SetStateAction<ReaderFontFamily>>;
  setReaderFontSizePx: Dispatch<SetStateAction<number>>;
  setReaderLetterSpacingEm: Dispatch<SetStateAction<number>>;
  setReaderTheme: Dispatch<SetStateAction<ReaderTheme>>;
  setReadingMode: Dispatch<SetStateAction<ReadingMode>>;
  setReverseTapPageNavigation: Dispatch<SetStateAction<boolean>>;
};

export function useReaderPreferences({
  initialLocalPreferences,
  setError,
  setReaderNotice
}: UseReaderPreferencesOptions): UseReaderPreferencesResult {
  const [readingMode, setReadingMode] = useState<ReadingMode>(DEFAULT_READER_SERVER_PREFERENCES.readingMode);
  const [readerFontSizePx, setReaderFontSizePx] = useState(initialLocalPreferences.fontSizePx);
  const [readerLetterSpacingEm, setReaderLetterSpacingEm] = useState(initialLocalPreferences.letterSpacingEm);
  const [reverseTapPageNavigation, setReverseTapPageNavigation] = useState(
    initialLocalPreferences.reverseTapPageNavigation
  );
  const [debugPageOverflow, setDebugPageOverflow] = useState(initialLocalPreferences.debugPageOverflow);
  const [readerExperimentalFontId, setReaderExperimentalFontId] = useState<ReaderExperimentalFontId>(
    initialLocalPreferences.experimentalFontId
  );
  const [readerExperimentalFontWeight, setReaderExperimentalFontWeight] = useState<ReaderExperimentalFontWeight>(
    initialLocalPreferences.experimentalFontWeight
  );
  const [readerExperimentalFontLoadStatus, setReaderExperimentalFontLoadStatus] =
    useState<ReaderExperimentalFontLoadStatus>("idle");
  const [readerExperimentalFontLayoutVersion, setReaderExperimentalFontLayoutVersion] = useState(0);
  const [readerExperimentalFontRetryNonce, setReaderExperimentalFontRetryNonce] = useState(0);
  const [readerFontFamily, setReaderFontFamily] = useState<ReaderFontFamily>(DEFAULT_READER_SERVER_PREFERENCES.fontFamily);
  const [readerTheme, setReaderTheme] = useState<ReaderTheme>(DEFAULT_READER_SERVER_PREFERENCES.theme);
  const [isReaderPreferencesLoaded, setIsReaderPreferencesLoaded] = useState(false);
  const readerExperimentalFontLinkRef = useRef<HTMLLinkElement | null>(null);
  const savedReaderPreferencesKeyRef = useRef<string | null>(getReaderServerPreferencesKey(DEFAULT_READER_SERVER_PREFERENCES));
  const readerPreferencesRequestSeqRef = useRef(0);
  const readerExperimentalFontOption = useMemo(
    () => getReaderExperimentalFontOption(readerExperimentalFontId),
    [readerExperimentalFontId]
  );
  const readerArticleFontFamilyCss = useMemo(
    () => getReaderArticleFontFamilyCss(readerFontFamily, readerExperimentalFontId),
    [readerExperimentalFontId, readerFontFamily]
  );
  const readerArticleFontWeight = useMemo(
    () => getReaderArticleFontWeight(readerExperimentalFontId, readerExperimentalFontWeight),
    [readerExperimentalFontId, readerExperimentalFontWeight]
  );

  useEffect(() => {
    let cancelled = false;

    async function loadReaderPreferences() {
      try {
        const nextPreferences = await fetchReaderPreferences();
        if (cancelled) {
          return;
        }

        setReadingMode(nextPreferences.readingMode);
        setReaderFontFamily(nextPreferences.fontFamily);
        setReaderTheme(nextPreferences.theme);
        savedReaderPreferencesKeyRef.current = getReaderServerPreferencesKey(nextPreferences);
      } catch (loadError) {
        console.error("Failed to load reader preferences", loadError);
        if (!cancelled) {
          savedReaderPreferencesKeyRef.current = getReaderServerPreferencesKey(DEFAULT_READER_SERVER_PREFERENCES);
        }
      } finally {
        if (!cancelled) {
          setIsReaderPreferencesLoaded(true);
        }
      }
    }

    void loadReaderPreferences();

    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const currentPreferences = loadReaderLocalPreferences();
    saveReaderLocalPreferences({
      ...currentPreferences,
      fontSizePx: readerFontSizePx,
      letterSpacingEm: readerLetterSpacingEm,
      reverseTapPageNavigation,
      debugPageOverflow,
      experimentalFontId: readerExperimentalFontId,
      experimentalFontWeight: readerExperimentalFontWeight
    });
  }, [
    debugPageOverflow,
    readerExperimentalFontId,
    readerExperimentalFontWeight,
    readerFontSizePx,
    readerLetterSpacingEm,
    reverseTapPageNavigation
  ]);

  useEffect(() => {
    const stylesheetHref = readerExperimentalFontOption.stylesheetHref;
    let existingLink =
      readerExperimentalFontLinkRef.current ??
      document.head.querySelector<HTMLLinkElement>('link[data-reader-experimental-font="true"]');

    if (!stylesheetHref) {
      if (existingLink) {
        existingLink.remove();
        readerExperimentalFontLinkRef.current = null;
      }
      setReaderExperimentalFontLoadStatus("idle");
      return;
    }

    const existingLinkHref = existingLink?.dataset.readerExperimentalFontHref ?? "";
    const existingLinkLoadState = existingLink?.dataset.loadState;
    const existingLinkRetryNonce = existingLink?.dataset.retryNonce;
    const shouldRetryCurrentHref =
      existingLink !== null &&
      existingLinkHref === stylesheetHref &&
      existingLinkLoadState === "error" &&
      existingLinkRetryNonce !== String(readerExperimentalFontRetryNonce);

    if (existingLink && shouldRetryCurrentHref) {
      existingLink.remove();
      readerExperimentalFontLinkRef.current = null;
      existingLink = null;
    }

    const link =
      existingLink ??
      (() => {
        const nextLink = document.createElement("link");
        nextLink.rel = "stylesheet";
        nextLink.dataset.readerExperimentalFont = "true";
        document.head.append(nextLink);
        return nextLink;
      })();
    readerExperimentalFontLinkRef.current = link;

    const currentHref = link.dataset.readerExperimentalFontHref ?? "";
    const shouldUpdateHref = currentHref !== stylesheetHref;
    if (shouldUpdateHref) {
      link.dataset.readerExperimentalFontHref = stylesheetHref;
      link.dataset.loadState = "loading";
    }
    link.dataset.retryNonce = String(readerExperimentalFontRetryNonce);

    if (link.dataset.loadState === "ready") {
      setReaderExperimentalFontLoadStatus("ready");
      return;
    }

    if (link.dataset.loadState === "error") {
      setReaderExperimentalFontLoadStatus("error");
      return;
    }

    let cancelled = false;
    setReaderExperimentalFontLoadStatus("loading");

    const handleLoad = () => {
      link.dataset.loadState = "ready";
      if (!cancelled) {
        setReaderExperimentalFontLoadStatus("ready");
        setReaderExperimentalFontLayoutVersion((current) => current + 1);
      }
    };
    const handleError = () => {
      link.dataset.loadState = "error";
      if (!cancelled) {
        setReaderExperimentalFontLoadStatus("error");
        setReaderExperimentalFontLayoutVersion((current) => current + 1);
      }
    };

    link.addEventListener("load", handleLoad);
    link.addEventListener("error", handleError);

    if (shouldUpdateHref || shouldRetryCurrentHref) {
      link.href = stylesheetHref;
    }

    return () => {
      cancelled = true;
      link.removeEventListener("load", handleLoad);
      link.removeEventListener("error", handleError);
    };
  }, [readerExperimentalFontOption.stylesheetHref, readerExperimentalFontRetryNonce]);

  useEffect(() => {
    if (!isReaderPreferencesLoaded) {
      return;
    }

    const nextPreferences: ReaderServerPreferences = {
      readingMode,
      fontFamily: readerFontFamily,
      theme: readerTheme
    };
    const nextPreferenceKey = getReaderServerPreferencesKey(nextPreferences);

    if (nextPreferenceKey === savedReaderPreferencesKeyRef.current) {
      return;
    }

    const requestSeq = readerPreferencesRequestSeqRef.current + 1;
    readerPreferencesRequestSeqRef.current = requestSeq;
    let cancelled = false;

    void (async () => {
      try {
        const savedPreferences = await putReaderPreferences(nextPreferences);
        if (cancelled || requestSeq !== readerPreferencesRequestSeqRef.current) {
          return;
        }

        savedReaderPreferencesKeyRef.current = getReaderServerPreferencesKey(savedPreferences);
      } catch (saveError) {
        if (!cancelled) {
          setError(saveError instanceof Error ? saveError.message : "Unknown error");
        }
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [isReaderPreferencesLoaded, readingMode, readerFontFamily, readerTheme, setError]);

  const handleResetReaderPreferences = useCallback(() => {
    setReadingMode(DEFAULT_READER_SERVER_PREFERENCES.readingMode);
    setReaderFontFamily(DEFAULT_READER_SERVER_PREFERENCES.fontFamily);
    setReaderTheme(DEFAULT_READER_SERVER_PREFERENCES.theme);
    setReaderFontSizePx(DEFAULT_READER_LOCAL_PREFERENCES.fontSizePx);
    setReaderLetterSpacingEm(DEFAULT_READER_LOCAL_PREFERENCES.letterSpacingEm);
    setReverseTapPageNavigation(DEFAULT_READER_LOCAL_PREFERENCES.reverseTapPageNavigation);
    setDebugPageOverflow(DEFAULT_READER_LOCAL_PREFERENCES.debugPageOverflow);
    setReaderExperimentalFontId(DEFAULT_READER_LOCAL_PREFERENCES.experimentalFontId);
    setReaderExperimentalFontWeight(DEFAULT_READER_LOCAL_PREFERENCES.experimentalFontWeight);
    setReaderNotice("読書設定を初期化しました。");
  }, [setReaderNotice]);

  const handleRetryReaderExperimentalFontLoad = useCallback(() => {
    if (readerExperimentalFontId === DEFAULT_READER_EXPERIMENTAL_FONT_ID) {
      return;
    }

    setReaderExperimentalFontRetryNonce((current) => current + 1);
  }, [readerExperimentalFontId]);

  return {
    debugPageOverflow,
    handleResetReaderPreferences,
    handleRetryReaderExperimentalFontLoad,
    readerArticleFontFamilyCss,
    readerArticleFontWeight,
    readerExperimentalFontId,
    readerExperimentalFontLayoutVersion,
    readerExperimentalFontLoadStatus,
    readerExperimentalFontOption,
    readerExperimentalFontWeight,
    readerFontFamily,
    readerFontSizePx,
    readerLetterSpacingEm,
    readerTheme,
    readingMode,
    reverseTapPageNavigation,
    setDebugPageOverflow,
    setReaderExperimentalFontId,
    setReaderExperimentalFontWeight,
    setReaderFontFamily,
    setReaderFontSizePx,
    setReaderLetterSpacingEm,
    setReaderTheme,
    setReadingMode,
    setReverseTapPageNavigation
  };
}
