import { useEffect, useState, type RefObject } from "react";
import type { EpisodeResponse } from "../features/reader/types";
import type { ScreenMode } from "./useReaderState";

type UseReaderControlsLayoutParams = {
  currentPageIndex: number;
  episode: EpisodeResponse | null;
  readerPageIndicatorRef: RefObject<HTMLParagraphElement | null>;
  readerShellRef: RefObject<HTMLElement | null>;
  readerViewportRef: RefObject<HTMLDivElement | null>;
  screenMode: ScreenMode;
  totalPages: number;
};

function getViewportWidth(): number {
  return typeof window === "undefined" ? 0 : window.innerWidth;
}

export function useReaderControlsLayout({
  currentPageIndex,
  episode,
  readerPageIndicatorRef,
  readerShellRef,
  readerViewportRef,
  screenMode,
  totalPages
}: UseReaderControlsLayoutParams) {
  const [readerControlViewportWidth, setReaderControlViewportWidth] = useState(() => getViewportWidth());
  const [readerPageIndicatorWidth, setReaderPageIndicatorWidth] = useState(0);

  useEffect(() => {
    if (screenMode !== "reader") {
      setReaderControlViewportWidth(getViewportWidth());
      return;
    }

    const target = readerViewportRef.current ?? readerShellRef.current;
    if (!target) {
      setReaderControlViewportWidth(getViewportWidth());
      return;
    }

    const updateViewportWidth = () => {
      setReaderControlViewportWidth(target.clientWidth > 0 ? target.clientWidth : getViewportWidth());
    };

    updateViewportWidth();
    const resizeObserver = new ResizeObserver(updateViewportWidth);
    resizeObserver.observe(target);
    window.addEventListener("resize", updateViewportWidth);

    return () => {
      resizeObserver.disconnect();
      window.removeEventListener("resize", updateViewportWidth);
    };
  }, [readerShellRef, readerViewportRef, screenMode]);

  const hasEpisode = episode !== null;
  const episodeContentEtag = episode?.contentEtag ?? null;

  // biome-ignore lint/correctness/useExhaustiveDependencies: page count/index changes can change the measured indicator text width.
  useEffect(() => {
    if (screenMode !== "reader" || !hasEpisode) {
      setReaderPageIndicatorWidth(0);
      return;
    }

    const pageIndicator = readerPageIndicatorRef.current;
    if (!pageIndicator) {
      setReaderPageIndicatorWidth(0);
      return;
    }

    const updatePageIndicatorWidth = () => {
      setReaderPageIndicatorWidth(pageIndicator.offsetWidth);
    };

    updatePageIndicatorWidth();
    const resizeObserver = new ResizeObserver(updatePageIndicatorWidth);
    resizeObserver.observe(pageIndicator);
    window.addEventListener("resize", updatePageIndicatorWidth);

    return () => {
      resizeObserver.disconnect();
      window.removeEventListener("resize", updatePageIndicatorWidth);
    };
  }, [currentPageIndex, episodeContentEtag, hasEpisode, readerPageIndicatorRef, screenMode, totalPages]);

  return {
    readerControlViewportWidth,
    readerPageIndicatorWidth
  };
}
