import { useCallback, useRef, useState, type Dispatch, type MutableRefObject, type SetStateAction } from "react";

type UseReaderPagingStateOptions = {
  initialPosition: number | null;
  onSelectedPositionChange: Dispatch<SetStateAction<number | null>>;
};

type UseReaderPagingStateResult = {
  currentPageIndex: number;
  isEpisodeLayoutReady: boolean;
  layoutAnchorPositionRef: MutableRefObject<number | null>;
  movePage: (direction: -1 | 1) => void;
  resetPageIndex: () => void;
  setCurrentPageIndex: Dispatch<SetStateAction<number>>;
  setIsEpisodeLayoutReady: Dispatch<SetStateAction<boolean>>;
  setTotalPages: Dispatch<SetStateAction<number>>;
  setVerticalLastPageReservePx: Dispatch<SetStateAction<number>>;
  shouldCapturePageAnchorRef: MutableRefObject<boolean>;
  totalPages: number;
  verticalLastPageReservePx: number;
};

export function useReaderPagingState({
  initialPosition,
  onSelectedPositionChange
}: UseReaderPagingStateOptions): UseReaderPagingStateResult {
  const [currentPageIndex, setCurrentPageIndex] = useState(0);
  const [totalPages, setTotalPages] = useState(1);
  const [verticalLastPageReservePx, setVerticalLastPageReservePx] = useState(0);
  const [isEpisodeLayoutReady, setIsEpisodeLayoutReady] = useState(false);
  const layoutAnchorPositionRef = useRef<number | null>(initialPosition);
  const shouldCapturePageAnchorRef = useRef(false);

  const resetPageIndex = useCallback(() => {
    setCurrentPageIndex(0);
  }, []);

  const movePage = useCallback(
    (direction: -1 | 1) => {
      layoutAnchorPositionRef.current = null;
      shouldCapturePageAnchorRef.current = true;
      onSelectedPositionChange(null);
      setCurrentPageIndex((current) => Math.min(Math.max(current + direction, 0), totalPages - 1));
    },
    [onSelectedPositionChange, totalPages]
  );

  return {
    currentPageIndex,
    isEpisodeLayoutReady,
    layoutAnchorPositionRef,
    movePage,
    resetPageIndex,
    setCurrentPageIndex,
    setIsEpisodeLayoutReady,
    setTotalPages,
    setVerticalLastPageReservePx,
    shouldCapturePageAnchorRef,
    totalPages,
    verticalLastPageReservePx
  };
}
