import type { ReadingMode } from "../../readerPreferences";

export type ReaderTouchGesture = {
  startClientX: number;
  startClientY: number;
  startTimeMs: number;
};

export type ReaderPageMoveDirections = { previous: -1 | 1; next: -1 | 1 };

export const READER_EDGE_TAP_MAX_DURATION_MS = 500;

export function getReaderEdgeZoneWidth(viewportWidth: number): number {
  return Math.min(viewportWidth * 0.16, 120);
}

export function isReaderEdgeClick(
  target: Pick<HTMLDivElement, "getBoundingClientRect">,
  clientX: number
): boolean {
  const rect = target.getBoundingClientRect();
  const zoneWidth = getReaderEdgeZoneWidth(rect.width);
  const offsetX = clientX - rect.left;

  return offsetX <= zoneWidth || offsetX >= rect.width - zoneWidth;
}

const READER_SWIPE_MIN_DISTANCE_PX = 48;
const READER_SWIPE_HORIZONTAL_DOMINANCE_RATIO = 1.2;

export function getReaderSwipeDirection(
  startX: number,
  startY: number,
  endX: number,
  endY: number
): "left" | "right" | null {
  const deltaX = endX - startX;
  const deltaY = endY - startY;
  const absX = Math.abs(deltaX);
  const absY = Math.abs(deltaY);

  if (
    absX < READER_SWIPE_MIN_DISTANCE_PX ||
    absX <= absY * READER_SWIPE_HORIZONTAL_DOMINANCE_RATIO
  ) {
    return null;
  }

  return deltaX > 0 ? "right" : "left";
}

export function getReaderPageMoveDirections(mode: ReadingMode): { previous: -1 | 1; next: -1 | 1 } {
  return mode === "vertical"
    ? { previous: 1, next: -1 }
    : { previous: -1, next: 1 };
}

export function getReaderEdgeTapPageMoveDirections(
  pageMoveDirections: ReaderPageMoveDirections,
  reverseTapPageNavigation: boolean
): ReaderPageMoveDirections {
  if (!reverseTapPageNavigation) {
    return pageMoveDirections;
  }

  return {
    previous: pageMoveDirections.next,
    next: pageMoveDirections.previous
  };
}

export function canMoveReaderPage(currentPageIndex: number, totalPages: number, direction: -1 | 1): boolean {
  return Math.min(Math.max(currentPageIndex + direction, 0), totalPages - 1) !== currentPageIndex;
}

export function getReaderTouchGestureStart(input: {
  firstTouch: { clientX: number; clientY: number } | null | undefined;
  nowMs?: number;
  touchCount: number;
}): ReaderTouchGesture | null {
  if (!input.firstTouch || input.touchCount !== 1) {
    return null;
  }

  return {
    startClientX: input.firstTouch.clientX,
    startClientY: input.firstTouch.clientY,
    startTimeMs: input.nowMs ?? Date.now()
  };
}

export function resolveReaderEdgeNavigationDirection(input: {
  viewportLeft: number;
  viewportWidth: number;
  clientX: number;
  pageMoveDirections: ReaderPageMoveDirections;
}): -1 | 1 | null {
  const zoneWidth = getReaderEdgeZoneWidth(input.viewportWidth);
  const offsetX = input.clientX - input.viewportLeft;

  if (offsetX <= zoneWidth) {
    return input.pageMoveDirections.previous;
  }

  if (offsetX >= input.viewportWidth - zoneWidth) {
    return input.pageMoveDirections.next;
  }

  return null;
}

export function resolveReaderTouchNavigationDirection(input: {
  touchGesture: ReaderTouchGesture | null;
  endTouch: { clientX: number; clientY: number } | null | undefined;
  viewportLeft: number;
  viewportWidth: number;
  nowMs?: number;
  isTextSelectionGesture?: boolean;
  swipePageMoveDirections: ReaderPageMoveDirections;
  tapPageMoveDirections: ReaderPageMoveDirections;
}): -1 | 1 | null {
  if (!input.touchGesture || !input.endTouch) {
    return null;
  }

  if (input.isTextSelectionGesture) {
    return null;
  }

  const swipeDirection = getReaderSwipeDirection(
    input.touchGesture.startClientX,
    input.touchGesture.startClientY,
    input.endTouch.clientX,
    input.endTouch.clientY
  );

  if (swipeDirection) {
    return swipeDirection === "right" ? input.swipePageMoveDirections.previous : input.swipePageMoveDirections.next;
  }

  const elapsedMs = (input.nowMs ?? Date.now()) - input.touchGesture.startTimeMs;
  if (elapsedMs >= READER_EDGE_TAP_MAX_DURATION_MS) {
    return null;
  }

  return resolveReaderEdgeNavigationDirection({
    viewportLeft: input.viewportLeft,
    viewportWidth: input.viewportWidth,
    clientX: input.endTouch.clientX,
    pageMoveDirections: input.tapPageMoveDirections
  });
}

export function hasActiveTextSelection(selection: Selection | null, container: Node | null = null): boolean {
  if (!selection || selection.isCollapsed || selection.rangeCount === 0) {
    return false;
  }

  if (!container) {
    return true;
  }

  for (let index = 0; index < selection.rangeCount; index += 1) {
    const range = selection.getRangeAt(index);
    try {
      if (range.intersectsNode(container)) {
        return true;
      }
    } catch {
      if (container.contains(range.commonAncestorContainer)) {
        return true;
      }
    }
  }

  return false;
}
