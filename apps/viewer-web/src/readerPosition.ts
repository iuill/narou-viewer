import { isRectWithinVerticalPage } from "./features/reader/verticalPagination";

const graphemeSegmenter =
  typeof Intl !== "undefined" && "Segmenter" in Intl ? new Intl.Segmenter("ja", { granularity: "grapheme" }) : null;

type ReadingMode = "vertical" | "horizontal";

type PositionRange = {
  start: number;
  end: number;
};

type CurrentVerticalPage = {
  start: number;
  end: number;
  shiftX: number;
};

type TextSlice = {
  node: Text;
  startOffset: number;
  text: string;
  startGrapheme: number;
  endGrapheme: number;
};

type GraphemeBoundary = {
  node: Text;
  offset: number;
};

export function countGraphemes(value: string): number {
  if (value.length === 0) {
    return 0;
  }

  if (graphemeSegmenter) {
    let count = 0;
    for (const _segment of graphemeSegmenter.segment(value)) {
      count += 1;
    }
    return count;
  }

  return Array.from(value).length;
}

function getCodeUnitOffsetForGrapheme(value: string, graphemeOffset: number): number {
  if (graphemeOffset <= 0 || value.length === 0) {
    return 0;
  }

  if (graphemeSegmenter) {
    let currentOffset = 0;
    for (const segment of graphemeSegmenter.segment(value)) {
      if (currentOffset === graphemeOffset) {
        return segment.index;
      }

      currentOffset += 1;
    }
  }

  return Array.from(value)
    .slice(0, graphemeOffset)
    .join("").length;
}

function getPositionRange(element: HTMLElement): PositionRange | null {
  const start = Number.parseInt(element.dataset.readerPositionStart ?? "", 10);
  const end = Number.parseInt(element.dataset.readerPositionEnd ?? "", 10);

  if (!Number.isInteger(start) || !Number.isInteger(end) || start < 0 || end <= start) {
    return null;
  }

  return { start, end };
}

function collectTextSlices(element: HTMLElement): TextSlice[] {
  const doc = element.ownerDocument;
  const view = doc?.defaultView;
  if (!view) {
    return [];
  }

  const slices: TextSlice[] = [];
  const walker = doc.createTreeWalker(element, view.NodeFilter.SHOW_TEXT, {
    acceptNode(node) {
      const parent = node.parentElement;
      if (!parent || parent.closest("rt")) {
        return view.NodeFilter.FILTER_REJECT;
      }

      return (node.textContent ?? "").length > 0 ? view.NodeFilter.FILTER_ACCEPT : view.NodeFilter.FILTER_REJECT;
    }
  });

  let current: Node | null = walker.nextNode();
  let graphemeStart = 0;
  while (current) {
    if (current instanceof Text) {
      const length = countGraphemes(current.data);
      if (length > 0) {
        slices.push({
          node: current,
          startOffset: 0,
          text: current.data,
          startGrapheme: graphemeStart,
          endGrapheme: graphemeStart + length
        });
        graphemeStart += length;
      }
    }

    current = walker.nextNode();
  }

  return slices;
}

function getGraphemeBoundary(slices: TextSlice[], graphemeOffset: number): GraphemeBoundary | null {
  const lastSlice = slices.at(-1);
  if (!lastSlice) {
    return null;
  }

  const totalLength = lastSlice.endGrapheme;
  const clampedOffset = Math.min(Math.max(graphemeOffset, 0), totalLength);

  for (const slice of slices) {
    if (clampedOffset > slice.endGrapheme) {
      continue;
    }

    const localOffset = clampedOffset - slice.startGrapheme;
    return {
      node: slice.node,
      offset: slice.startOffset + getCodeUnitOffsetForGrapheme(slice.text, localOffset)
    };
  }

  return {
    node: lastSlice.node,
    offset: lastSlice.startOffset + lastSlice.text.length
  };
}

function createGraphemeRange(
  element: HTMLElement,
  slices: TextSlice[],
  graphemeOffset: number,
  range: PositionRange | null = getPositionRange(element)
): Range | null {
  if (!range || range.end - range.start <= 0 || slices.length === 0) {
    return null;
  }

  const startBoundary = getGraphemeBoundary(slices, graphemeOffset);
  const endBoundary = getGraphemeBoundary(slices, graphemeOffset + 1);
  if (!startBoundary || !endBoundary) {
    return null;
  }

  const doc = element.ownerDocument;
  const domRange = doc.createRange();
  domRange.setStart(startBoundary.node, startBoundary.offset);
  domRange.setEnd(endBoundary.node, endBoundary.offset);
  return domRange;
}

function isRectVisible(rect: DOMRect, viewportRect: DOMRect): boolean {
  return rect.bottom > viewportRect.top && rect.top < viewportRect.bottom && rect.right > viewportRect.left && rect.left < viewportRect.right;
}

function isRectVisibleInReaderPage(
  rect: Pick<DOMRect, "top" | "bottom" | "left" | "right" | "width" | "height">,
  viewport: HTMLDivElement,
  viewportRect: DOMRect,
  mode: ReadingMode,
  currentVerticalPage: CurrentVerticalPage | null
): boolean {
  if (mode === "vertical" && currentVerticalPage !== null) {
    return isRectWithinVerticalPage(
      rect,
      {
        viewportRectLeft: viewportRect.left,
        scrollLeft: viewport.scrollLeft,
        clientLeft: viewport.clientLeft,
        shiftX: currentVerticalPage.shiftX
      },
      currentVerticalPage
    );
  }

  return isRectVisible(rect as DOMRect, viewportRect);
}

function isLayoutBoxVisibleInReaderPage(
  box: Pick<HTMLElement | Range, "getBoundingClientRect" | "getClientRects">,
  viewport: HTMLDivElement,
  viewportRect: DOMRect,
  mode: ReadingMode,
  currentVerticalPage: CurrentVerticalPage | null
): boolean {
  const rects = Array.from(box.getClientRects()).filter((rect) => rect.width > 0 && rect.height > 0);
  return rects.length === 0
    ? isRectVisibleInReaderPage(box.getBoundingClientRect(), viewport, viewportRect, mode, currentVerticalPage)
    : rects.some((rect) => isRectVisibleInReaderPage(rect, viewport, viewportRect, mode, currentVerticalPage));
}

function getPositionTargets(viewport: HTMLDivElement): HTMLElement[] {
  const article = viewport.querySelector(".reader-prose-paged");
  if (!(article instanceof HTMLElement)) {
    return [];
  }

  return Array.from(
    article.querySelectorAll<HTMLElement>("[data-reader-position-start][data-reader-position-end]")
  ).filter((target) => getPositionRange(target) !== null);
}

function sortTargetsByReadingOrder(targets: HTMLElement[], viewportRect: DOMRect, mode: ReadingMode): HTMLElement[] {
  return [...targets].sort((left, right) => {
    const leftRect = left.getBoundingClientRect();
    const rightRect = right.getBoundingClientRect();

    if (mode === "vertical") {
      const columnDiff = Math.abs(viewportRect.right - leftRect.right) - Math.abs(viewportRect.right - rightRect.right);
      if (columnDiff !== 0) {
        return columnDiff;
      }

      return Math.abs(leftRect.top - viewportRect.top) - Math.abs(rightRect.top - viewportRect.top);
    }

    const rowDiff = Math.abs(leftRect.top - viewportRect.top) - Math.abs(rightRect.top - viewportRect.top);
    if (rowDiff !== 0) {
      return rowDiff;
    }

    return Math.abs(leftRect.left - viewportRect.left) - Math.abs(rightRect.left - viewportRect.left);
  });
}

function findVisibleTarget(
  viewport: HTMLDivElement,
  mode: ReadingMode,
  currentVerticalPage: CurrentVerticalPage | null = null
): HTMLElement | null {
  const viewportRect = viewport.getBoundingClientRect();
  const targets = getPositionTargets(viewport);
  if (targets.length === 0) {
    return null;
  }

  const readableTargets = targets.filter((target) => !target.classList.contains("reader-page-overflow-hidden"));
  const candidatePool = readableTargets.length > 0 ? readableTargets : targets;
  const pageAwareCandidatePool =
    mode === "vertical" && currentVerticalPage
      ? candidatePool.filter((target) => {
          const rects = Array.from(target.getClientRects()).filter((rect) => rect.width > 0 && rect.height > 0);
          return rects.some((rect) =>
            isRectWithinVerticalPage(
              rect,
              {
                viewportRectLeft: viewportRect.left,
                scrollLeft: viewport.scrollLeft,
                clientLeft: viewport.clientLeft,
                shiftX: currentVerticalPage.shiftX
              },
              currentVerticalPage
            )
          );
        })
      : [];
  const effectiveCandidatePool = pageAwareCandidatePool.length > 0 ? pageAwareCandidatePool : candidatePool;
  const visibleTargets = effectiveCandidatePool.filter((target) => isRectVisible(target.getBoundingClientRect(), viewportRect));
  const candidates = visibleTargets.length > 0 ? visibleTargets : effectiveCandidatePool;

  return sortTargetsByReadingOrder(candidates, viewportRect, mode)[0] ?? null;
}

function getFirstVisibleGraphemeOffset(target: HTMLElement, viewportRect: DOMRect): number {
  const range = getPositionRange(target);
  if (!range) {
    return 0;
  }

  const length = range.end - range.start;
  if (length <= 1) {
    return 0;
  }

  const slices = collectTextSlices(target);
  if (slices.length === 0) {
    return 0;
  }

  for (let index = 0; index < length; index += 1) {
    const graphemeRange = createGraphemeRange(target, slices, index, range);
    if (!graphemeRange) {
      break;
    }

    if (isRectVisible(graphemeRange.getBoundingClientRect(), viewportRect)) {
      return index;
    }
  }

  return 0;
}

export function findReaderPositionTarget(viewport: HTMLDivElement, position: number): HTMLElement | null {
  const targets = getPositionTargets(viewport);
  if (targets.length === 0) {
    return null;
  }

  const normalizedPosition = Math.max(0, position);
  let fallback = targets[0] ?? null;

  for (const target of targets) {
    const range = getPositionRange(target);
    if (!range) {
      continue;
    }

    fallback = target;
    if (normalizedPosition < range.start) {
      return target;
    }

    if (normalizedPosition >= range.start && normalizedPosition < range.end) {
      return target;
    }
  }

  return fallback;
}

function findReaderPositionTargetContaining(viewport: HTMLDivElement, position: number): HTMLElement | null {
  const targets = getPositionTargets(viewport);
  if (targets.length === 0) {
    return null;
  }

  const normalizedPosition = Math.max(0, Math.round(position));
  for (const target of targets) {
    const range = getPositionRange(target);
    if (!range) {
      continue;
    }

    if (normalizedPosition >= range.start && normalizedPosition < range.end) {
      return target;
    }
  }

  return null;
}

export function getReaderPositionFromViewport(
  viewport: HTMLDivElement,
  mode: ReadingMode,
  options?: {
    currentVerticalPage?: CurrentVerticalPage | null;
  }
): number | null {
  const target = findVisibleTarget(viewport, mode, options?.currentVerticalPage ?? null);
  const range = target ? getPositionRange(target) : null;
  if (!target || !range) {
    return null;
  }

  return range.start + getFirstVisibleGraphemeOffset(target, viewport.getBoundingClientRect());
}

export function isReaderPositionVisible(
  viewport: HTMLDivElement,
  position: number,
  options: {
    mode?: ReadingMode;
    currentVerticalPage?: CurrentVerticalPage | null;
  } = {}
): boolean {
  const target = findReaderPositionTargetContaining(viewport, position);
  const range = target ? getPositionRange(target) : null;
  if (!target || !range) {
    return false;
  }

  if (target.classList.contains("reader-page-overflow-hidden")) {
    return false;
  }

  const viewportRect = viewport.getBoundingClientRect();
  const mode = options.mode ?? "horizontal";
  const currentVerticalPage = options.currentVerticalPage ?? null;
  const length = range.end - range.start;
  const offset = Math.min(Math.max(Math.round(position) - range.start, 0), length - 1);

  const visibilityFragments = Array.from(target.querySelectorAll<HTMLElement>("[data-reader-visibility-fragment]"));
  const visibilityFragment = visibilityFragments[offset] ?? null;
  if (visibilityFragment) {
    return (
      !visibilityFragment.classList.contains("reader-page-overflow-hidden") &&
      isLayoutBoxVisibleInReaderPage(visibilityFragment, viewport, viewportRect, mode, currentVerticalPage)
    );
  }

  if (length <= 1) {
    return isLayoutBoxVisibleInReaderPage(target, viewport, viewportRect, mode, currentVerticalPage);
  }

  const slices = collectTextSlices(target);
  const graphemeRange = createGraphemeRange(target, slices, Math.min(Math.max(position - range.start, 0), length - 1), range);
  return graphemeRange
    ? isLayoutBoxVisibleInReaderPage(graphemeRange, viewport, viewportRect, mode, currentVerticalPage)
    : isLayoutBoxVisibleInReaderPage(target, viewport, viewportRect, mode, currentVerticalPage);
}

export function getReaderVisiblePositionRange(
  viewport: HTMLDivElement,
  options: {
    mode?: ReadingMode;
    currentVerticalPage?: CurrentVerticalPage | null;
  } = {}
): PositionRange | null {
  const viewportRect = viewport.getBoundingClientRect();
  const mode = options.mode ?? "horizontal";
  const currentVerticalPage = options.currentVerticalPage ?? null;
  const visibleRanges = getPositionTargets(viewport)
    .filter((target) => !target.classList.contains("reader-page-overflow-hidden"))
    .flatMap((target) => {
      const range = getPositionRange(target);
      if (!range) {
        return [];
      }

      const targetRects = Array.from(target.getClientRects()).filter((rect) => rect.width > 0 && rect.height > 0);
      const isTargetVisible =
        targetRects.length === 0
          ? isRectVisibleInReaderPage(target.getBoundingClientRect(), viewport, viewportRect, mode, currentVerticalPage)
          : targetRects.some((rect) => isRectVisibleInReaderPage(rect, viewport, viewportRect, mode, currentVerticalPage));
      if (!isTargetVisible) {
        return [];
      }

      const length = range.end - range.start;
      if (length <= 1) {
        return [range];
      }

      const visibilityFragments = Array.from(target.querySelectorAll<HTMLElement>("[data-reader-visibility-fragment]"));
      if (visibilityFragments.length > 0) {
        const visibleGraphemeRanges = visibilityFragments
          .slice(0, length)
          .flatMap((fragment, offset): PositionRange[] => {
            if (fragment.classList.contains("reader-page-overflow-hidden")) {
              return [];
            }

            const rects = Array.from(fragment.getClientRects()).filter((rect) => rect.width > 0 && rect.height > 0);
            const isFragmentVisible =
              rects.length === 0
                ? isRectVisibleInReaderPage(fragment.getBoundingClientRect(), viewport, viewportRect, mode, currentVerticalPage)
                : rects.some((rect) => isRectVisibleInReaderPage(rect, viewport, viewportRect, mode, currentVerticalPage));
            return isFragmentVisible
              ? [
                  {
                    start: range.start + offset,
                    end: range.start + offset + 1
                  }
                ]
              : [];
          });

        return visibleGraphemeRanges.length > 0 ? visibleGraphemeRanges : [range];
      }

      const slices = collectTextSlices(target);
      if (slices.length === 0) {
        return [range];
      }

      const visibleGraphemeRanges: PositionRange[] = [];
      for (let offset = 0; offset < length; offset += 1) {
        const graphemeRange = createGraphemeRange(target, slices, offset, range);
        if (!graphemeRange) {
          continue;
        }

        const rects = Array.from(graphemeRange.getClientRects()).filter((rect) => rect.width > 0 && rect.height > 0);
        const isGraphemeVisible =
          rects.length === 0
            ? isRectVisibleInReaderPage(graphemeRange.getBoundingClientRect(), viewport, viewportRect, mode, currentVerticalPage)
            : rects.some((rect) => isRectVisibleInReaderPage(rect, viewport, viewportRect, mode, currentVerticalPage));
        if (!isGraphemeVisible) {
          continue;
        }

        visibleGraphemeRanges.push({
          start: range.start + offset,
          end: range.start + offset + 1
        });
      }

      return visibleGraphemeRanges.length > 0 ? visibleGraphemeRanges : [range];
    });

  if (visibleRanges.length === 0) {
    return null;
  }

  return {
    start: Math.min(...visibleRanges.map((range) => range.start)),
    end: Math.max(...visibleRanges.map((range) => range.end))
  };
}

function scrollReaderTargetIntoView(
  viewport: HTMLDivElement,
  target: Element,
  mode: ReadingMode
): void {
  const preservedScrollTop = mode === "vertical" ? viewport.scrollTop : null;

  target.scrollIntoView({
    block: "start",
    inline: "start"
  });

  if (preservedScrollTop !== null) {
    viewport.scrollTop = preservedScrollTop;
  }
}

export function scrollReaderPositionIntoView(viewport: HTMLDivElement, position: number, mode: ReadingMode): boolean {
  const target = findReaderPositionTarget(viewport, position);
  const range = target ? getPositionRange(target) : null;
  if (!target || !range) {
    return false;
  }

  const length = range.end - range.start;
  if (length <= 1) {
    scrollReaderTargetIntoView(viewport, target, mode);
    return true;
  }

  const slices = collectTextSlices(target);
  const boundary = getGraphemeBoundary(slices, Math.min(Math.max(position - range.start, 0), length - 1));
  if (!boundary) {
    scrollReaderTargetIntoView(viewport, target, mode);
    return true;
  }

  const marker = target.ownerDocument.createElement("span");
  marker.setAttribute("aria-hidden", "true");
  marker.dataset.readerPositionMarker = "true";
  marker.style.cssText = "display:inline-block;width:0;height:0;overflow:hidden;padding:0;margin:0;";

  const markerRange = target.ownerDocument.createRange();
  markerRange.setStart(boundary.node, boundary.offset);
  markerRange.collapse(true);
  markerRange.insertNode(marker);

  scrollReaderTargetIntoView(viewport, marker, mode);

  marker.remove();
  target.normalize();
  return true;
}
