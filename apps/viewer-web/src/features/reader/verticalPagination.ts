export function detectWebKitEngine(userAgent: string = navigator.userAgent): boolean {
  return /AppleWebKit/i.test(userAgent) && !/(Chrome\/|Chromium|Edg\/|OPR\/|SamsungBrowser)/i.test(userAgent);
}

const PAGE_BOUNDARY_EPSILON = 0.5;
const COLUMN_BOUNDARY_EPSILON = 0.5;

export type HorizontalInterval = {
  left: number;
  right: number;
};

export type VerticalPagingPage = {
  start: number;
  end: number;
  offset: number;
  blankLeft: number;
  blankRight: number;
  shiftX: number;
};

type VerticalColumnBox = {
  left: number;
  right: number;
};

type ScrollMetrics = {
  scrollWidth: number;
  scrollHeight: number;
};

export function resolveVerticalPagingContentMetrics(
  viewport: ScrollMetrics,
  article: ScrollMetrics | null
): { contentWidth: number; contentHeight: number } {
  const articleContentWidth = Number.isFinite(article?.scrollWidth) ? Math.max(article?.scrollWidth ?? 0, 0) : 0;
  const articleContentHeight = Number.isFinite(article?.scrollHeight) ? Math.max(article?.scrollHeight ?? 0, 0) : 0;
  const viewportContentWidth = Number.isFinite(viewport.scrollWidth) ? Math.max(viewport.scrollWidth, 0) : 0;
  const viewportContentHeight = Number.isFinite(viewport.scrollHeight) ? Math.max(viewport.scrollHeight, 0) : 0;

  return {
    contentWidth: articleContentWidth > 0 ? articleContentWidth : viewportContentWidth,
    contentHeight: articleContentHeight > 0 ? articleContentHeight : viewportContentHeight
  };
}

export function buildVerticalColumnBoundaries(intervals: HorizontalInterval[], contentWidth: number): number[] {
  if (!Number.isFinite(contentWidth) || contentWidth <= 0) {
    return [0];
  }

  const normalizedIntervals = intervals
    .map((interval) => {
      if (!Number.isFinite(interval.left) || !Number.isFinite(interval.right)) {
        return null;
      }

      const left = Math.min(Math.max(interval.left, 0), contentWidth);
      const right = Math.min(Math.max(interval.right, 0), contentWidth);
      return left <= right ? { left, right } : { left: right, right: left };
    })
    .filter((interval): interval is HorizontalInterval => interval !== null && interval.right - interval.left > COLUMN_BOUNDARY_EPSILON)
    .sort((left, right) => left.left - right.left || left.right - right.right);

  if (normalizedIntervals.length === 0) {
    return [0, contentWidth];
  }

  const columns: HorizontalInterval[] = [];
  for (const interval of normalizedIntervals) {
    const current = columns[columns.length - 1];
    if (!current || interval.left > current.right + COLUMN_BOUNDARY_EPSILON) {
      columns.push(interval);
      continue;
    }

    current.left = Math.min(current.left, interval.left);
    current.right = Math.max(current.right, interval.right);
  }

  const boundaries = Array.from(
    new Set(
      [0, ...columns.map((column) => Math.round(column.left * 100) / 100), contentWidth].map(
        (value) => Math.round(value * 100) / 100
      )
    )
  ).sort((left, right) => left - right);

  return boundaries;
}

export function buildVerticalPages(boundaries: number[], contentWidth: number, pageWidth: number): VerticalPagingPage[] {
  if (!Number.isFinite(contentWidth) || contentWidth <= 0 || !Number.isFinite(pageWidth) || pageWidth <= 0) {
    return [
      {
        start: 0,
        end: 0,
        offset: 0,
        blankLeft: 0,
        blankRight: 0,
        shiftX: 0
      }
    ];
  }

  const maxOffset = Math.max(0, contentWidth - pageWidth);
  const normalizedBoundaries = Array.from(
    new Set(
      boundaries
        .filter((value) => Number.isFinite(value))
        .map((value) => Math.min(Math.max(value, 0), contentWidth))
        .map((value) => Math.round(value * 100) / 100)
    )
  ).sort((left, right) => left - right);

  if (normalizedBoundaries[0] !== 0) {
    normalizedBoundaries.unshift(0);
  }

  if (normalizedBoundaries[normalizedBoundaries.length - 1] !== contentWidth) {
    normalizedBoundaries.push(contentWidth);
  }

  const boundaryColumns: VerticalColumnBox[] = [];
  for (let index = 0; index < normalizedBoundaries.length - 1; index += 1) {
    const left = normalizedBoundaries[index];
    const right = normalizedBoundaries[index + 1];
    if (left === undefined || right === undefined) {
      continue;
    }
    boundaryColumns.push({ left, right });
  }

  const columnBoxes: VerticalColumnBox[] =
    normalizedBoundaries.length <= 2
      ? [{ left: 0, right: contentWidth }]
      : boundaryColumns.filter((column) => column.right - column.left > PAGE_BOUNDARY_EPSILON).reverse();

  const queue: VerticalColumnBox[] = [];
  for (const column of columnBoxes) {
    let sliceRight = column.right;

    while (sliceRight - column.left > pageWidth + PAGE_BOUNDARY_EPSILON) {
      queue.push({
        left: sliceRight - pageWidth,
        right: sliceRight
      });
      sliceRight -= pageWidth;
    }

    if (sliceRight - column.left > PAGE_BOUNDARY_EPSILON) {
      queue.push({
        left: column.left,
        right: sliceRight
      });
    }
  }

  const pageBoxes: VerticalColumnBox[] = [];
  let currentPage: VerticalColumnBox | null = null;

  for (const column of queue) {
    if (!currentPage) {
      currentPage = { ...column };
      continue;
    }

    if (currentPage.right - column.left <= pageWidth + PAGE_BOUNDARY_EPSILON) {
      currentPage.left = column.left;
      continue;
    }

    pageBoxes.push(currentPage);
    currentPage = { ...column };
  }

  if (currentPage) {
    pageBoxes.push(currentPage);
  }

  const pages = pageBoxes.map((page) => {
    const offset = Math.min(Math.max(page.right - pageWidth, 0), maxOffset);
    const rawBlankLeft = Math.max(0, page.left - offset);
    const rawBlankRight = Math.max(0, offset + pageWidth - page.right);
    const shiftX = rawBlankRight > PAGE_BOUNDARY_EPSILON ? rawBlankRight : 0;

    return {
      start: page.left,
      end: page.right,
      offset,
      blankLeft: rawBlankLeft + shiftX,
      blankRight: Math.max(0, rawBlankRight - shiftX),
      shiftX
    };
  });

  return pages.length > 0
    ? pages
    : [
        {
          start: 0,
          end: contentWidth,
          offset: 0,
          blankLeft: Math.max(0, pageWidth - contentWidth),
          blankRight: 0,
          shiftX: Math.max(0, pageWidth - contentWidth)
        }
      ];
}

export function buildVerticalPageOffsets(boundaries: number[], contentWidth: number, pageWidth: number): number[] {
  return buildVerticalPages(boundaries, contentWidth, pageWidth).map((page) => page.offset);
}

export function normalizeVerticalReservePx(value: number): number {
  if (!Number.isFinite(value)) {
    return 0;
  }

  const normalized = Math.max(value, 0);
  if (normalized <= PAGE_BOUNDARY_EPSILON) {
    return 0;
  }

  return Math.round(normalized * 100) / 100;
}

export function hasMeaningfulVerticalReserveChange(current: number, next: number): boolean {
  return Math.abs(normalizeVerticalReservePx(next) - normalizeVerticalReservePx(current)) > PAGE_BOUNDARY_EPSILON;
}

export function toViewportContentOffset(
  rectEdge: number,
  viewportRectLeft: number,
  scrollLeft: number,
  clientLeft: number
): number {
  return scrollLeft + rectEdge - viewportRectLeft - clientLeft;
}

export function isRectWithinVerticalPage(
  rect: Pick<DOMRect, "left" | "right" | "width" | "height">,
  metrics: {
    viewportRectLeft: number;
    scrollLeft: number;
    clientLeft: number;
    shiftX: number;
  },
  page: Pick<VerticalPagingPage, "start" | "end">
): boolean {
  if (!Number.isFinite(rect.width) || !Number.isFinite(rect.height) || rect.width <= 0 || rect.height <= 0) {
    return false;
  }

  const contentLeft =
    toViewportContentOffset(rect.left, metrics.viewportRectLeft, metrics.scrollLeft, metrics.clientLeft) - metrics.shiftX;
  const contentRight =
    toViewportContentOffset(rect.right, metrics.viewportRectLeft, metrics.scrollLeft, metrics.clientLeft) - metrics.shiftX;
  const midpoint = (Math.min(contentLeft, contentRight) + Math.max(contentLeft, contentRight)) / 2;

  return midpoint >= page.start - PAGE_BOUNDARY_EPSILON && midpoint <= page.end + PAGE_BOUNDARY_EPSILON;
}
