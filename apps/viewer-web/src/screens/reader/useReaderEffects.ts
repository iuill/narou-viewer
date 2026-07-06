import { useEffect, type Dispatch, type MutableRefObject, type RefObject, type SetStateAction } from "react";
import { ReaderStateConflictError } from "../../features/reader/api";
import { isReaderEdgeClick } from "../../features/reader/gestureNavigation";
import { extractImageViewerState, type ImageViewerState } from "../../features/reader/imageViewer";
import { createReadingStateKey } from "../../features/reader/readerStateKey";
import type { EpisodeIndex, EpisodeResponse } from "../../features/reader/types";
import type { ReaderSessionCommands } from "../../features/reader/useReaderSession";
import {
  hasMeaningfulVerticalReserveChange,
  isRectWithinVerticalPage,
  normalizeVerticalReservePx,
} from "../../features/reader/verticalPagination";
import type { ReaderSyncConflict, ReaderSyncConflictResolutionState } from "../../hooks/useReaderState";
import { getReaderPositionFromViewport, scrollReaderPositionIntoView } from "../../readerPosition";
import type { ReaderExperimentalFontWeight } from "../../readerExperimentalFonts";
import type { ReadingMode } from "../../readerPreferences";
import {
  type AppliedReaderStateAutoSaveGuard,
  consumeAppliedReaderStateAutoSaveGuard
} from "../../readerStateAutoSaveGuard";
import { isReaderStateSaveDisabled } from "../../testing/e2eControl";
import type { useReaderPagingHelpers } from "./useReaderPagingHelpers";

const READER_PAGE_OVERFLOW_HIDDEN_CLASS = "reader-page-overflow-hidden";
const READER_PAGE_OVERFLOW_DEBUG_CLASS = "reader-page-overflow-debug";
const READER_SPEECH_PROGRESS_SAVE_DEBOUNCE_MS = 2500;

type ScreenMode = "library" | "reader";

type UseReaderEffectsOptions = ReturnType<typeof useReaderPagingHelpers> & {
  appliedReaderStateAutoSaveGuardRef: MutableRefObject<AppliedReaderStateAutoSaveGuard | null>;
  currentPageIndex: number;
  debugPageOverflow: boolean;
  episode: EpisodeResponse | null;
  hasStableReaderEpisode: boolean;
  isEpisodeLayoutReady: boolean;
  isEpisodeLoading: boolean;
  isReaderFullscreen: boolean;
  isReaderSpeechProgressAutoScrollSuppressed: (position: number) => boolean;
  layoutAnchorPositionRef: MutableRefObject<number | null>;
  logReaderSpeechDebugEvent: (eventName: string, payload: Record<string, unknown>) => void;
  openImageViewer: (imageViewer: ImageViewerState) => void;
  pendingReadingStateKeyRef: MutableRefObject<string | null>;
  readerArticleFontFamilyCss: string;
  readerArticleFontWeight: ReaderExperimentalFontWeight | null;
  readerExperimentalFontLayoutVersion: number;
  readerFontSizePx: number;
  readerLetterSpacingEm: number;
  readerSessionCommands: ReaderSessionCommands;
  readerSpeechState: string;
  readerStateStateVersion: number | null | undefined;
  readerSyncConflict: ReaderSyncConflict | null;
  readerSyncConflictResolutionState: ReaderSyncConflictResolutionState;
  readerViewportRef: RefObject<HTMLDivElement | null>;
  readingMode: ReadingMode;
  resetPageIndex: () => void;
  savedReadingStateKey: string | null;
  screenMode: ScreenMode;
  selectedEpisodeIndex: EpisodeIndex | null;
  selectedEpisodeIndexRef: MutableRefObject<EpisodeIndex | null>;
  selectedNovelId: string | null;
  selectedPosition: number | null;
  setCurrentPageIndex: Dispatch<SetStateAction<number>>;
  setError: Dispatch<SetStateAction<string | null>>;
  setIsEpisodeLayoutReady: Dispatch<SetStateAction<boolean>>;
  setTotalPages: Dispatch<SetStateAction<number>>;
  setVerticalLastPageReservePx: Dispatch<SetStateAction<number>>;
  shouldCapturePageAnchorRef: MutableRefObject<boolean>;
  totalPages: number;
  verticalLastPageReservePx: number;
};

export function useReaderEffects({
  appliedReaderStateAutoSaveGuardRef,
  currentPageIndex,
  debugPageOverflow,
  episode,
  getCurrentPageIndexFromViewport,
  getCurrentReaderViewportPosition,
  getPagingMetrics,
  hasStableReaderEpisode,
  isEpisodeLayoutReady,
  isEpisodeLoading,
  isReaderFullscreen,
  isReaderSpeechProgressAutoScrollSuppressed,
  layoutAnchorPositionRef,
  logReaderSpeechDebugEvent,
  measureVerticalPages,
  openImageViewer,
  pendingReadingStateKeyRef,
  readerArticleFontFamilyCss,
  readerArticleFontWeight,
  readerExperimentalFontLayoutVersion,
  readerFontSizePx,
  readerLetterSpacingEm,
  readerSessionCommands,
  readerSpeechState,
  readerStateStateVersion,
  readerSyncConflict,
  readerSyncConflictResolutionState,
  readerViewportRef,
  readingMode,
  resetPageIndex,
  savedReadingStateKey,
  screenMode,
  scrollToPage,
  selectedEpisodeIndex,
  selectedEpisodeIndexRef,
  selectedNovelId,
  selectedPosition,
  setCurrentPageIndex,
  setError,
  setIsEpisodeLayoutReady,
  setTotalPages,
  setVerticalLastPageReservePx,
  shouldCapturePageAnchorRef,
  totalPages,
  verticalLastPageReservePx,
  verticalPagingCacheRef
}: UseReaderEffectsOptions) {
  // biome-ignore lint/correctness/useExhaustiveDependencies: pending reading keys are mutable guards, not render dependencies.
  useEffect(() => {
    if (pendingReadingStateKeyRef.current === savedReadingStateKey) {
      pendingReadingStateKeyRef.current = null;
    }
  }, [savedReadingStateKey]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: layout anchor refs are updated in response to selected position changes.
  useEffect(() => {
    if (selectedPosition !== null && isReaderSpeechProgressAutoScrollSuppressed(selectedPosition)) {
      logReaderSpeechDebugEvent("layout-anchor-update-suppressed", {
        position: selectedPosition,
        episodeIndex: selectedEpisodeIndexRef.current
      });
      return;
    }

    layoutAnchorPositionRef.current = selectedPosition;
  }, [isReaderSpeechProgressAutoScrollSuppressed, selectedPosition]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: reader layout-affecting inputs must reset the page index.
  useEffect(() => {
    resetPageIndex();
  }, [
    selectedNovelId,
    selectedEpisodeIndex,
    episode?.contentEtag,
    readingMode,
    readerArticleFontFamilyCss,
    readerArticleFontWeight,
    readerExperimentalFontLayoutVersion,
    readerFontSizePx,
    readerLetterSpacingEm,
    resetPageIndex
  ]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: reader pagination is measured from DOM layout and style-trigger inputs.
  useEffect(() => {
    if (screenMode !== "reader" || !episode) {
      setTotalPages(1);
      setVerticalLastPageReservePx(0);
      setIsEpisodeLayoutReady(false);
      return;
    }

    const viewport = readerViewportRef.current;
    if (!viewport) {
      return;
    }

    let pendingImageCount = 0;

    const setLayoutReady = (ready: boolean) => {
      setIsEpisodeLayoutReady((current) => (current === ready ? current : ready));
    };

    const syncPages = () => {
      if (readingMode === "vertical") {
        const measuredVerticalPages = measureVerticalPages(viewport, verticalLastPageReservePx);
        const unreservedVerticalPages = measuredVerticalPages.pages;
        const lastUnreservedPage =
          unreservedVerticalPages.length > 0 ? unreservedVerticalPages[unreservedVerticalPages.length - 1] : null;
        const nextReservePx = normalizeVerticalReservePx(lastUnreservedPage?.shiftX ?? 0);

        if (hasMeaningfulVerticalReserveChange(verticalLastPageReservePx, nextReservePx)) {
          verticalPagingCacheRef.current = {
            key: "",
            pages: [{ start: 0, end: 0, offset: 0, blankLeft: 0, blankRight: 0, shiftX: 0 }]
          };
          setVerticalLastPageReservePx(nextReservePx);
          setLayoutReady(false);
          return;
        }
      } else if (verticalLastPageReservePx !== 0) {
        verticalPagingCacheRef.current = {
          key: "",
          pages: [{ start: 0, end: 0, offset: 0, blankLeft: 0, blankRight: 0, shiftX: 0 }]
        };
        setVerticalLastPageReservePx(0);
        setLayoutReady(false);
        return;
      }

      const { pageSize, totalPages: calculatedPages } = getPagingMetrics(viewport, readingMode);
      if (pageSize <= 0) {
        setTotalPages(1);
        setLayoutReady(false);
        setCurrentPageIndex(0);
        return;
      }

      setTotalPages(pendingImageCount > 0 ? 1 : calculatedPages);
      const isSpeechProgressPosition =
        selectedPosition !== null && isReaderSpeechProgressAutoScrollSuppressed(selectedPosition);
      const anchoredPosition = isSpeechProgressPosition ? null : selectedPosition ?? layoutAnchorPositionRef.current;
      if (anchoredPosition !== null) {
        const scrollBefore = {
          left: viewport.scrollLeft,
          top: viewport.scrollTop
        };
        if (scrollReaderPositionIntoView(viewport, anchoredPosition, readingMode)) {
          const pageIndexAfter = getCurrentPageIndexFromViewport(viewport, readingMode);
          scrollToPage(viewport, pageIndexAfter, readingMode);
          setLayoutReady(pendingImageCount === 0);
          setCurrentPageIndex(pageIndexAfter);
          logReaderSpeechDebugEvent("layout-anchor-scroll", {
            anchoredPosition,
            readingMode,
            pageIndexAfter,
            scrollBefore,
            scrollAfter: {
              left: viewport.scrollLeft,
              top: viewport.scrollTop
            }
          });
          return;
        }
      }

      setLayoutReady(pendingImageCount === 0);
      setCurrentPageIndex((current) => {
        const clamped = Math.min(Math.max(current, 0), calculatedPages - 1);
        scrollToPage(viewport, clamped, readingMode);
        return clamped;
      });
    };

    const article = viewport.querySelector(".reader-prose-paged");
    const imageCleanup: Array<() => void> = [];
    if (article instanceof HTMLElement) {
      const images = Array.from(article.querySelectorAll("img"));
      for (const image of images) {
        if (image.complete) {
          continue;
        }

        pendingImageCount += 1;

        const onImageReady = () => {
          pendingImageCount = Math.max(0, pendingImageCount - 1);
          syncPages();
        };

        image.addEventListener("load", onImageReady);
        image.addEventListener("error", onImageReady);
        imageCleanup.push(() => {
          image.removeEventListener("load", onImageReady);
          image.removeEventListener("error", onImageReady);
        });
      }
    }

    syncPages();
    const observer = new ResizeObserver(syncPages);
    observer.observe(viewport);

    return () => {
      observer.disconnect();
      for (const dispose of imageCleanup) {
        dispose();
      }
    };
  }, [
    screenMode,
    episode,
    readingMode,
    readerArticleFontFamilyCss,
    readerArticleFontWeight,
    readerExperimentalFontLayoutVersion,
    readerFontSizePx,
    readerLetterSpacingEm,
    selectedPosition,
    isReaderSpeechProgressAutoScrollSuppressed,
    verticalLastPageReservePx
  ]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: selected-position scrolling follows reader content and layout triggers.
  useEffect(() => {
    if (screenMode !== "reader" || !episode || selectedPosition === null) {
      return;
    }

    const viewport = readerViewportRef.current;
    if (!viewport) {
      return;
    }

    if (isReaderSpeechProgressAutoScrollSuppressed(selectedPosition)) {
      logReaderSpeechDebugEvent("selected-position-scroll-suppressed", {
        selectedPosition,
        readingMode
      });
      return;
    }

    const frameId = window.requestAnimationFrame(() => {
      const scrollBefore = {
        left: viewport.scrollLeft,
        top: viewport.scrollTop
      };
      if (!scrollReaderPositionIntoView(viewport, selectedPosition, readingMode)) {
        return;
      }

      const pageIndexAfter = getCurrentPageIndexFromViewport(viewport, readingMode);
      scrollToPage(viewport, pageIndexAfter, readingMode);
      setCurrentPageIndex(pageIndexAfter);
      logReaderSpeechDebugEvent("selected-position-scroll", {
        selectedPosition,
        readingMode,
        pageIndexAfter,
        scrollBefore,
        scrollAfter: {
          left: viewport.scrollLeft,
          top: viewport.scrollTop
        }
      });
    });

    return () => {
      window.cancelAnimationFrame(frameId);
    };
  }, [
    screenMode,
    episode?.contentEtag,
    selectedPosition,
    readingMode,
    readerArticleFontFamilyCss,
    readerArticleFontWeight,
    readerExperimentalFontLayoutVersion,
    readerFontSizePx,
    readerLetterSpacingEm,
    isReaderSpeechProgressAutoScrollSuppressed
  ]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: page-index scrolling intentionally tracks the current rendered page state.
  useEffect(() => {
    if (screenMode !== "reader") {
      return;
    }

    const viewport = readerViewportRef.current;
    if (!viewport) {
      return;
    }

    const { pageSize } = getPagingMetrics(viewport, readingMode);
    if (pageSize > 0) {
      const scrollBefore = {
        left: viewport.scrollLeft,
        top: viewport.scrollTop
      };
      scrollToPage(viewport, currentPageIndex, readingMode);
      logReaderSpeechDebugEvent("page-index-scroll", {
        currentPageIndex,
        readingMode,
        scrollBefore,
        scrollAfter: {
          left: viewport.scrollLeft,
          top: viewport.scrollTop
        }
      });
    }
  }, [currentPageIndex, screenMode, readingMode]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: overflow visibility is synchronized to DOM layout and reader style changes.
  useEffect(() => {
    const viewport = readerViewportRef.current;
    if (!viewport) {
      return;
    }

    const article = viewport.querySelector(".reader-prose-paged");
    if (!(article instanceof HTMLElement)) {
      return;
    }

    const getVisibilityTargets = () =>
      Array.from(
        article.querySelectorAll<HTMLElement>(
          '.reader-dash-run, [data-reader-visibility-fragment], [data-reader-pagination-fragment="image"], [data-reader-pagination-fragment="html"]'
        )
      );

    const clearOverflowState = () => {
      for (const target of getVisibilityTargets()) {
        target.classList.remove(READER_PAGE_OVERFLOW_HIDDEN_CLASS, READER_PAGE_OVERFLOW_DEBUG_CLASS);
      }
    };

    let overflowRafId = 0;

    const runOverflowState = () => {
      const { verticalPages } = getPagingMetrics(viewport, readingMode);
      const currentPage = verticalPages?.[currentPageIndex];
      if (!currentPage) {
        clearOverflowState();
        return;
      }

      const viewportRect = viewport.getBoundingClientRect();
      const targets = getVisibilityTargets();
      for (const target of targets) {
        const rects = Array.from(target.getClientRects()).filter((rect) => rect.width > 0 && rect.height > 0);
        const isOnCurrentPage =
          rects.length === 0 ||
          rects.some((rect) =>
            isRectWithinVerticalPage(
              rect,
              {
                viewportRectLeft: viewportRect.left,
                scrollLeft: viewport.scrollLeft,
                clientLeft: viewport.clientLeft,
                shiftX: currentPage.shiftX
              },
              currentPage
            )
          );

        target.classList.toggle(READER_PAGE_OVERFLOW_HIDDEN_CLASS, !isOnCurrentPage && !debugPageOverflow);
        target.classList.toggle(READER_PAGE_OVERFLOW_DEBUG_CLASS, !isOnCurrentPage && debugPageOverflow);
      }
    };

    const scheduleOverflowState = () => {
      if (overflowRafId !== 0) {
        window.cancelAnimationFrame(overflowRafId);
      }

      overflowRafId = window.requestAnimationFrame(() => {
        overflowRafId = 0;
        runOverflowState();
      });
    };

    if (screenMode !== "reader" || !episode || readingMode !== "vertical") {
      clearOverflowState();
      return;
    }

    scheduleOverflowState();
    const MutationObserverConstructor =
      article.ownerDocument.defaultView?.MutationObserver ??
      (typeof MutationObserver !== "undefined" ? MutationObserver : null);
    if (!MutationObserverConstructor) {
      return () => {
        if (overflowRafId !== 0) {
          window.cancelAnimationFrame(overflowRafId);
        }
        clearOverflowState();
      };
    }

    const mutationObserver = new MutationObserverConstructor(() => {
      scheduleOverflowState();
    });
    mutationObserver.observe(article, { childList: true, subtree: true });

    return () => {
      mutationObserver.disconnect();
      if (overflowRafId !== 0) {
        window.cancelAnimationFrame(overflowRafId);
      }
      clearOverflowState();
    };
  }, [
    currentPageIndex,
    debugPageOverflow,
    episode?.contentEtag,
    isReaderFullscreen,
    isEpisodeLayoutReady,
    readerArticleFontFamilyCss,
    readerArticleFontWeight,
    readerExperimentalFontLayoutVersion,
    readerFontSizePx,
    readerLetterSpacingEm,
    readingMode,
    screenMode,
    totalPages
  ]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: autosave is intentionally scheduled by page, layout, and reader state signals.
  useEffect(() => {
    if (!hasStableReaderEpisode || !selectedNovelId || selectedEpisodeIndex === null) {
      return;
    }

    if (isReaderStateSaveDisabled()) {
      return;
    }

    if (readerSyncConflict || readerSyncConflictResolutionState !== "idle") {
      return;
    }

    let cancelled = false;
    const saveAbortController = new AbortController();
    let saveTimeoutId = 0;
    let firstFrameId = 0;
    let secondFrameId = 0;

    const scheduleSave = () => {
      firstFrameId = window.requestAnimationFrame(() => {
        secondFrameId = window.requestAnimationFrame(() => {
          const viewport = readerViewportRef.current;
          if (!viewport) {
            return;
          }

          if (
            !hasStableReaderEpisode ||
            !selectedNovelId ||
            selectedEpisodeIndex === null ||
            readerSyncConflict ||
            readerSyncConflictResolutionState !== "idle"
          ) {
            return;
          }

          const position = getCurrentReaderViewportPosition();
          const nextReadingStateKey = createReadingStateKey(selectedNovelId, selectedEpisodeIndex, position);

          if (position === null || nextReadingStateKey === null) {
            return;
          }
          const appliedReaderStateGuard = appliedReaderStateAutoSaveGuardRef.current;
          const appliedReaderStateGuardResult = consumeAppliedReaderStateAutoSaveGuard(
            appliedReaderStateGuard,
            selectedNovelId,
            selectedEpisodeIndex,
            nextReadingStateKey
          );
          appliedReaderStateAutoSaveGuardRef.current = appliedReaderStateGuardResult.nextGuard;
          if (appliedReaderStateGuardResult.shouldSkipCurrentSave) {
            return;
          }

          if (nextReadingStateKey === savedReadingStateKey || nextReadingStateKey === pendingReadingStateKeyRef.current) {
            return;
          }

          void (async () => {
            try {
              await readerSessionCommands.savePosition({
                novelId: selectedNovelId,
                episodeIndex: selectedEpisodeIndex,
                position,
                readingStateKey: nextReadingStateKey,
                signal: saveAbortController.signal
              });
            } catch (saveError) {
              if (saveError instanceof Error && saveError.name === "AbortError") {
                return;
              }
              if (saveError instanceof ReaderStateConflictError) {
                return;
              }
              if (!cancelled) {
                setError(saveError instanceof Error ? saveError.message : "Unknown error");
              }
            }
          })();
        });
      });
    };

    if (readerSpeechState === "playing") {
      saveTimeoutId = window.setTimeout(scheduleSave, READER_SPEECH_PROGRESS_SAVE_DEBOUNCE_MS);
    } else {
      scheduleSave();
    }

    return () => {
      cancelled = true;
      saveAbortController.abort();
      window.clearTimeout(saveTimeoutId);
      window.cancelAnimationFrame(firstFrameId);
      window.cancelAnimationFrame(secondFrameId);
    };
  }, [
    currentPageIndex,
    episode,
    readerArticleFontFamilyCss,
    readerArticleFontWeight,
    readerExperimentalFontLayoutVersion,
    readerFontSizePx,
    readerLetterSpacingEm,
    readingMode,
    readerSessionCommands.savePosition,
    readerStateStateVersion,
    savedReadingStateKey,
    screenMode,
    selectedEpisodeIndex,
    hasStableReaderEpisode,
    isEpisodeLayoutReady,
    isEpisodeLoading,
    readerSpeechState,
    readerSyncConflict,
    readerSyncConflictResolutionState,
    selectedPosition,
    selectedNovelId
  ]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: the ref object is stable; rebinding is driven by episode/screen changes.
  useEffect(() => {
    if (screenMode !== "reader" || !episode) {
      return;
    }

    const viewport = readerViewportRef.current;
    if (!viewport) {
      return;
    }

    const article = viewport.querySelector(".reader-prose-paged");
    if (!(article instanceof HTMLElement)) {
      return;
    }

    const imageElements = Array.from(article.querySelectorAll("img"));
    for (const image of imageElements) {
      image.classList.add("reader-inline-image");
    }

    const onArticleClick = (event: MouseEvent) => {
      const target = event.target;
      if (!(target instanceof Element)) {
        return;
      }

      if (isReaderEdgeClick(viewport, event.clientX)) {
        return;
      }

      const image = target.closest("img");
      if (!(image instanceof HTMLImageElement) || !article.contains(image)) {
        return;
      }

      event.preventDefault();
      event.stopPropagation();

      const nextImageViewer = extractImageViewerState(image);
      if (!nextImageViewer) {
        return;
      }

      openImageViewer(nextImageViewer);
    };

    article.addEventListener("click", onArticleClick, true);

    return () => {
      article.removeEventListener("click", onArticleClick, true);
      for (const image of imageElements) {
        image.classList.remove("reader-inline-image");
      }
    };
  }, [episode, openImageViewer, screenMode]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: page anchor capture is gated by mutable refs and layout timing signals.
  useEffect(() => {
    if (!shouldCapturePageAnchorRef.current || screenMode !== "reader" || selectedPosition !== null) {
      return;
    }

    const viewport = readerViewportRef.current;
    if (!viewport) {
      return;
    }

    let firstFrameId = 0;
    let secondFrameId = 0;

    firstFrameId = window.requestAnimationFrame(() => {
      secondFrameId = window.requestAnimationFrame(() => {
        const position = getReaderPositionFromViewport(viewport, readingMode);
        if (position !== null) {
          layoutAnchorPositionRef.current = position;
        }
        shouldCapturePageAnchorRef.current = false;
      });
    });

    return () => {
      window.cancelAnimationFrame(firstFrameId);
      window.cancelAnimationFrame(secondFrameId);
    };
  }, [currentPageIndex, episode?.contentEtag, readingMode, screenMode, selectedPosition]);
}
