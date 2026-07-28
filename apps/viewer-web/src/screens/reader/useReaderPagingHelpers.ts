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
  readingPosition?: number | null;
  visibilityTargets?: readonly HTMLElement[];
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
  const verticalPageRuntimeCacheRef = useRef<{
    activePageIndex: number | null;
    pages: VerticalPage[] | null;
  }>({ activePageIndex: null, pages: null });

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
      verticalPageRuntimeCacheRef.current = { activePageIndex: null, pages: null };

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
      if (mode === "vertical") {
        const pages = verticalPagingCacheRef.current.pages;
        const clampedIndex = Math.min(Math.max(pageIndex, 0), Math.max(pages.length - 1, 0));
        viewport.scrollLeft = pages[clampedIndex]?.offset ?? 0;
        return;
      }

      const pageSize = viewport.clientHeight;
      if (pageSize <= 0) {
        return;
      }
      const maxOffset = Math.max(0, viewport.scrollHeight - pageSize);
      viewport.scrollTop = Math.min(Math.max(Math.round(pageIndex * pageSize), 0), maxOffset);
    },
    []
  );

  const prepareVerticalPageSnapshot = useCallback((viewport: HTMLDivElement) => {
    const pages = verticalPagingCacheRef.current.pages;
    if (verticalPageRuntimeCacheRef.current.pages === pages) {
      return pages;
    }

    const article = viewport.querySelector(".reader-prose-paged");
    if (!(article instanceof HTMLElement)) {
      return pages;
    }

    const viewportRect = viewport.getBoundingClientRect();
    const targetSets = pages.map(() => new Set<HTMLElement>());
    const readingPositions = pages.map<number | null>(() => null);
    const findPageIndex = (rect: Pick<DOMRect, "left" | "right" | "width" | "height">): number | null => {
      if (rect.width <= 0 || rect.height <= 0) {
        return null;
      }
      const midpoint = toViewportContentOffset(
        (Math.min(rect.left, rect.right) + Math.max(rect.left, rect.right)) / 2,
        viewportRect.left,
        viewport.scrollLeft,
        viewport.clientLeft
      );

      let low = 0;
      let high = pages.length - 1;
      while (low <= high) {
        const index = Math.floor((low + high) / 2);
        const page = pages[index];
        if (!page) {
          return null;
        }
        const adjustedMidpoint = midpoint - page.shiftX;
        if (adjustedMidpoint > page.end + 0.5) {
          high = index - 1;
        } else if (adjustedMidpoint < page.start - 0.5) {
          low = index + 1;
        } else {
          return index;
        }
      }
      return null;
    };

    const fragmentPositions = new Map<HTMLElement, number>();
    const positionTargets = Array.from(
      article.querySelectorAll<HTMLElement>("[data-reader-position-start][data-reader-position-end]")
    );
    for (const target of positionTargets) {
      const start = Number.parseInt(target.dataset.readerPositionStart ?? "", 10);
      const end = Number.parseInt(target.dataset.readerPositionEnd ?? "", 10);
      if (!Number.isInteger(start) || !Number.isInteger(end) || start < 0 || end <= start) {
        continue;
      }
      const fragments = Array.from(target.querySelectorAll<HTMLElement>("[data-reader-visibility-fragment]"));
      for (let index = 0; index < Math.min(fragments.length, end - start); index += 1) {
        const fragment = fragments[index];
        if (fragment) {
          fragmentPositions.set(fragment, start + index);
        }
      }
    }

    const visibilityTargets = Array.from(
      article.querySelectorAll<HTMLElement>(
        '.reader-dash-run, [data-reader-visibility-fragment], [data-reader-pagination-fragment="image"], [data-reader-pagination-fragment="html"]'
      )
    );
    for (const target of visibilityTargets) {
      for (const rect of Array.from(target.getClientRects())) {
        const pageIndex = findPageIndex(rect);
        if (pageIndex !== null) {
          targetSets[pageIndex]?.add(target);
          const position = fragmentPositions.get(target);
          if (position !== undefined) {
            const current = readingPositions[pageIndex];
            readingPositions[pageIndex] = current === null ? position : Math.min(current, position);
          }
        }
      }
    }

    for (const target of positionTargets) {
      const start = Number.parseInt(target.dataset.readerPositionStart ?? "", 10);
      const end = Number.parseInt(target.dataset.readerPositionEnd ?? "", 10);
      if (!Number.isInteger(start) || !Number.isInteger(end) || start < 0 || end <= start) {
        continue;
      }

      if (target.querySelector("[data-reader-visibility-fragment]")) {
        continue;
      }
      const rects = Array.from(target.getClientRects()).filter((rect) => rect.width > 0 && rect.height > 0);
      for (let rectIndex = 0; rectIndex < rects.length; rectIndex += 1) {
        const rect = rects[rectIndex];
        if (!rect) {
          continue;
        }
        const pageIndex = findPageIndex(rect);
        if (pageIndex === null) {
          continue;
        }
        const position = start + Math.floor(((end - start) * rectIndex) / Math.max(rects.length, 1));
        const current = readingPositions[pageIndex];
        readingPositions[pageIndex] = current === null ? position : Math.min(current, position);
      }
    }

    let previousPosition: number | null = null;
    for (let index = 0; index < readingPositions.length; index += 1) {
      if (readingPositions[index] === null) {
        readingPositions[index] = previousPosition;
      } else {
        previousPosition = readingPositions[index];
      }
    }
    let nextPosition: number | null = null;
    for (let index = readingPositions.length - 1; index >= 0; index -= 1) {
      if (readingPositions[index] === null) {
        readingPositions[index] = nextPosition;
      } else {
        nextPosition = readingPositions[index];
      }
    }

    const snapshotPages = pages.map((page, index) => ({
      ...page,
      readingPosition: readingPositions[index],
      visibilityTargets: Array.from(targetSets[index] ?? [])
    }));
    verticalPagingCacheRef.current.pages = snapshotPages;
    verticalPageRuntimeCacheRef.current = { activePageIndex: null, pages: snapshotPages };
    return snapshotPages;
  }, []);

  const syncVerticalPageVisibility = useCallback((pageIndex: number, debug: boolean) => {
    const runtime = verticalPageRuntimeCacheRef.current;
    const pages = runtime.pages;
    if (!pages) {
      return;
    }

    const previousTargets =
      runtime.activePageIndex === null
        ? pages.flatMap((page) => page.visibilityTargets ?? [])
        : pages[runtime.activePageIndex]?.visibilityTargets ?? [];
    const currentTargets = pages[pageIndex]?.visibilityTargets ?? [];
    const targets = new Set([...previousTargets, ...currentTargets]);
    const currentTargetSet = new Set(currentTargets);
    for (const target of targets) {
      const hidden = !currentTargetSet.has(target);
      target.classList.toggle("reader-page-overflow-hidden", hidden && !debug);
      target.classList.toggle("reader-page-overflow-debug", hidden && debug);
    }
    runtime.activePageIndex = pageIndex;
  }, []);

  const clearVerticalPageVisibility = useCallback(() => {
    const runtime = verticalPageRuntimeCacheRef.current;
    for (const page of runtime.pages ?? []) {
      for (const target of page.visibilityTargets ?? []) {
        target.classList.remove("reader-page-overflow-hidden", "reader-page-overflow-debug");
      }
    }
    runtime.activePageIndex = null;
  }, []);

  const getCurrentReaderViewportPosition = useCallback((): number | null => {
    const viewport = readerViewportRef.current;
    if (!viewport) {
      return null;
    }

    if (readingMode === "vertical") {
      const currentVerticalPage = verticalPagingCacheRef.current.pages[currentPageIndex] ?? null;
      if (currentVerticalPage?.readingPosition !== null && currentVerticalPage?.readingPosition !== undefined) {
        return currentVerticalPage.readingPosition;
      }

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
  }, [currentPageIndex, readerViewportRef, readingMode]);

  return {
    clearVerticalPageVisibility,
    getCurrentPageIndexFromViewport,
    getCurrentReaderViewportPosition,
    getPagingMetrics,
    measureVerticalPages,
    prepareVerticalPageSnapshot,
    scrollToPage,
    syncVerticalPageVisibility,
    verticalPagingCacheRef
  };
}
