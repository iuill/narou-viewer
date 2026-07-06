import { useCallback, useRef, type RefObject } from "react";
import type { EpisodeResponse } from "../../features/reader/types";
import {
  buildVerticalColumnBoundaries,
  buildVerticalPages,
  resolveVerticalPagingContentMetrics,
  toViewportContentOffset
} from "../../features/reader/verticalPagination";
import { getReaderPositionFromViewport } from "../../readerPosition";
import type { ReaderExperimentalFontWeight } from "../../readerExperimentalFonts";
import type { ReadingMode } from "../../readerPreferences";

export type VerticalPage = {
  start: number;
  end: number;
  offset: number;
  blankLeft: number;
  blankRight: number;
  shiftX: number;
};

export type PagingMetrics = {
  pageSize: number;
  maxOffset: number;
  totalPages: number;
  pageOffsets: number[] | null;
  verticalPages: VerticalPage[] | null;
};

type UseReaderPagingHelpersOptions = {
  currentPageIndex: number;
  episode: EpisodeResponse | null;
  readerArticleFontFamilyCss: string;
  readerArticleFontWeight: ReaderExperimentalFontWeight | null;
  readerExperimentalFontLayoutVersion: number;
  readerFontSizePx: number;
  readerLetterSpacingEm: number;
  readerViewportRef: RefObject<HTMLDivElement | null>;
  readingMode: ReadingMode;
  verticalLastPageReservePx: number;
};

export function useReaderPagingHelpers({
  currentPageIndex,
  episode,
  readerArticleFontFamilyCss,
  readerArticleFontWeight,
  readerExperimentalFontLayoutVersion,
  readerFontSizePx,
  readerLetterSpacingEm,
  readerViewportRef,
  readingMode,
  verticalLastPageReservePx
}: UseReaderPagingHelpersOptions) {
  const verticalPagingCacheRef = useRef<{
    key: string;
    pages: VerticalPage[];
  }>({ key: "", pages: [{ start: 0, end: 0, offset: 0, blankLeft: 0, blankRight: 0, shiftX: 0 }] });

  const measureVerticalPages = useCallback(
    (
      viewport: HTMLDivElement,
      reserveCompensationPx: number = 0
    ): {
      contentWidth: number;
      pages: VerticalPage[];
    } => {
      const pageWidth = viewport.clientWidth;
      const article = viewport.querySelector(".reader-prose-paged");
      const { contentWidth } = resolveVerticalPagingContentMetrics(
        viewport,
        article instanceof HTMLElement ? article : null
      );
      const normalizedReserveCompensationPx = Math.max(0, reserveCompensationPx);
      const adjustedContentWidth = Math.max(0, contentWidth - normalizedReserveCompensationPx);
      if (pageWidth <= 0 || adjustedContentWidth <= 0) {
        return {
          contentWidth: adjustedContentWidth,
          pages: [{ start: 0, end: 0, offset: 0, blankLeft: 0, blankRight: 0, shiftX: 0 }]
        };
      }

      const viewportRect = viewport.getBoundingClientRect();
      const intervals: Array<{ left: number; right: number }> = [];

      if (article instanceof HTMLElement) {
        const fragmentTargets = Array.from(article.querySelectorAll<HTMLElement>("[data-reader-pagination-fragment]"));
        const fallbackTargets = Array.from(article.querySelectorAll<HTMLElement>(".reader-title, .reader-meta, .reader-section p, img"));
        const targets = fragmentTargets.length > 0 ? fragmentTargets : fallbackTargets.length > 0 ? fallbackTargets : [article];

        for (const target of targets) {
          const rects = Array.from(target.getClientRects());
          for (const rect of rects) {
            if (rect.width <= 0 || rect.height <= 0) {
              continue;
            }

            intervals.push({
              left: Math.max(
                0,
                toViewportContentOffset(rect.left, viewportRect.left, viewport.scrollLeft, viewport.clientLeft) -
                  normalizedReserveCompensationPx
              ),
              right: Math.max(
                0,
                toViewportContentOffset(rect.right, viewportRect.left, viewport.scrollLeft, viewport.clientLeft) -
                  normalizedReserveCompensationPx
              )
            });
          }
        }
      }

      return {
        contentWidth: adjustedContentWidth,
        pages: buildVerticalPages(buildVerticalColumnBoundaries(intervals, adjustedContentWidth), adjustedContentWidth, pageWidth)
      };
    },
    []
  );

  const getVerticalPages = useCallback(
    (viewport: HTMLDivElement) => {
      const pageWidth = viewport.clientWidth;
      const article = viewport.querySelector(".reader-prose-paged");
      const { contentWidth, contentHeight } = resolveVerticalPagingContentMetrics(
        viewport,
        article instanceof HTMLElement ? article : null
      );
      if (pageWidth <= 0 || contentWidth <= 0) {
        return {
          contentWidth,
          pages: [{ start: 0, end: 0, offset: 0, blankLeft: 0, blankRight: 0, shiftX: 0 }]
        };
      }

      const cacheKey = [
        episode?.contentEtag ?? "",
        readerArticleFontFamilyCss,
        readerArticleFontWeight,
        readerExperimentalFontLayoutVersion,
        readerFontSizePx,
        readerLetterSpacingEm,
        pageWidth,
        viewport.clientHeight,
        contentWidth,
        contentHeight,
        verticalLastPageReservePx
      ].join("|");
      const cached = verticalPagingCacheRef.current;
      if (cached.key === cacheKey) {
        return {
          contentWidth,
          pages: cached.pages
        };
      }

      const measured = measureVerticalPages(viewport);
      verticalPagingCacheRef.current = {
        key: cacheKey,
        pages: measured.pages
      };

      return measured;
    },
    [
      episode?.contentEtag,
      measureVerticalPages,
      readerArticleFontFamilyCss,
      readerArticleFontWeight,
      readerExperimentalFontLayoutVersion,
      readerFontSizePx,
      readerLetterSpacingEm,
      verticalLastPageReservePx
    ]
  );

  const getPagingMetrics = useCallback(
    (viewport: HTMLDivElement, mode: ReadingMode): PagingMetrics => {
      const pageSize = mode === "vertical" ? viewport.clientWidth : viewport.clientHeight;

      if (mode === "vertical") {
        const { contentWidth, pages: verticalPages } = getVerticalPages(viewport);

        return {
          pageSize,
          maxOffset: Math.max(0, contentWidth - viewport.clientWidth),
          totalPages: Math.max(verticalPages.length, 1),
          pageOffsets: verticalPages.map((page) => page.offset),
          verticalPages
        };
      }

      const maxOffset = Math.max(0, viewport.scrollHeight - viewport.clientHeight);
      const totalPages = pageSize <= 0 ? 1 : Math.max(1, Math.floor((maxOffset + pageSize - 0.5) / pageSize) + 1);

      return {
        pageSize,
        maxOffset,
        totalPages,
        pageOffsets: null,
        verticalPages: null
      };
    },
    [getVerticalPages]
  );

  const getCurrentPageIndexFromViewport = useCallback(
    (viewport: HTMLDivElement, mode: ReadingMode) => {
      const { pageSize, maxOffset, totalPages: calculatedPages, pageOffsets } = getPagingMetrics(viewport, mode);
      if (pageSize <= 0) {
        return 0;
      }

      if (mode === "vertical") {
        const currentOffset = Math.min(Math.max(viewport.scrollLeft, 0), maxOffset);
        let nearestIndex = 0;
        let nearestDistance = Number.POSITIVE_INFINITY;

        for (let index = 0; index < (pageOffsets?.length ?? 0); index += 1) {
          const distance = Math.abs((pageOffsets?.[index] ?? 0) - currentOffset);
          if (distance < nearestDistance) {
            nearestDistance = distance;
            nearestIndex = index;
          }
        }

        return Math.min(Math.max(nearestIndex, 0), calculatedPages - 1);
      }

      return Math.min(Math.max(Math.round(viewport.scrollTop / pageSize), 0), calculatedPages - 1);
    },
    [getPagingMetrics]
  );

  const scrollToPage = useCallback(
    (viewport: HTMLDivElement, pageIndex: number, mode: ReadingMode) => {
      const { pageSize, maxOffset, pageOffsets, verticalPages } = getPagingMetrics(viewport, mode);
      if (pageSize <= 0) {
        return;
      }

      if (mode === "vertical") {
        const clampedIndex = Math.min(Math.max(pageIndex, 0), Math.max((verticalPages?.length ?? 1) - 1, 0));
        const targetPage = verticalPages?.[clampedIndex] ?? null;
        viewport.scrollLeft = targetPage?.offset ?? pageOffsets?.[clampedIndex] ?? maxOffset;
        return;
      } else {
        const rawTarget = pageIndex * pageSize;
        const target = Math.min(Math.max(Math.round(rawTarget), 0), maxOffset);
        viewport.scrollTop = target;
      }
    },
    [getPagingMetrics]
  );

  const getCurrentReaderViewportPosition = useCallback((): number | null => {
    const viewport = readerViewportRef.current;
    if (!viewport) {
      return null;
    }

    if (readingMode === "vertical") {
      const { verticalPages } = getPagingMetrics(viewport, readingMode);
      const viewportPageIndex = getCurrentPageIndexFromViewport(viewport, readingMode);
      const currentVerticalPage = verticalPages?.[viewportPageIndex] ?? verticalPages?.[currentPageIndex] ?? null;

      return getReaderPositionFromViewport(viewport, readingMode, {
        currentVerticalPage: currentVerticalPage
          ? {
              start: currentVerticalPage.start,
              end: currentVerticalPage.end,
              shiftX: currentVerticalPage.shiftX
            }
          : null
      });
    }

    return getReaderPositionFromViewport(viewport, readingMode);
  }, [currentPageIndex, getCurrentPageIndexFromViewport, getPagingMetrics, readerViewportRef, readingMode]);

  return {
    getCurrentPageIndexFromViewport,
    getCurrentReaderViewportPosition,
    getPagingMetrics,
    measureVerticalPages,
    scrollToPage,
    verticalPagingCacheRef
  };
}
