import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type PointerEvent,
  type SetStateAction
} from "react";
import type { ImageViewerState } from "../ReaderImageViewer";

type UseReaderImageViewerResult = {
  imageViewer: ImageViewerState | null;
  imageViewerZoomPercent: number;
  isImageViewerDragging: boolean;
  isImageViewerInfoOpen: boolean;
  closeImageViewer: () => void;
  openImageViewer: (imageViewer: ImageViewerState) => void;
  setImageViewerZoomPercent: Dispatch<SetStateAction<number>>;
  setIsImageViewerInfoOpen: Dispatch<SetStateAction<boolean>>;
  handleImageViewerPointerDown: (event: PointerEvent<HTMLDivElement>) => void;
  handleImageViewerPointerMove: (event: PointerEvent<HTMLDivElement>) => void;
  handleImageViewerPointerUp: (event: PointerEvent<HTMLDivElement>) => void;
};

export function useReaderImageViewer(): UseReaderImageViewerResult {
  const [imageViewer, setImageViewer] = useState<ImageViewerState | null>(null);
  const [imageViewerZoomPercent, setImageViewerZoomPercent] = useState(100);
  const [isImageViewerInfoOpen, setIsImageViewerInfoOpen] = useState(false);
  const [isImageViewerDragging, setIsImageViewerDragging] = useState(false);
  const imageViewerDragStateRef = useRef<{
    pointerId: number;
    startClientX: number;
    startClientY: number;
    startScrollLeft: number;
    startScrollTop: number;
  } | null>(null);

  const closeImageViewer = useCallback(() => {
    setImageViewer(null);
    setIsImageViewerInfoOpen(false);
  }, []);

  const openImageViewer = useCallback((nextImageViewer: ImageViewerState) => {
    setImageViewer(nextImageViewer);
    setImageViewerZoomPercent(100);
    setIsImageViewerInfoOpen(false);
  }, []);

  const clearImageViewerDrag = useCallback((target?: HTMLDivElement | null, pointerId?: number) => {
    const dragState = imageViewerDragStateRef.current;
    if (target && pointerId !== undefined && dragState?.pointerId === pointerId && target.hasPointerCapture(pointerId)) {
      target.releasePointerCapture(pointerId);
    }

    imageViewerDragStateRef.current = null;
    setIsImageViewerDragging(false);
  }, []);

  const handleImageViewerPointerDown = useCallback((event: PointerEvent<HTMLDivElement>) => {
    if (event.pointerType !== "mouse" || event.button !== 0) {
      return;
    }

    imageViewerDragStateRef.current = {
      pointerId: event.pointerId,
      startClientX: event.clientX,
      startClientY: event.clientY,
      startScrollLeft: event.currentTarget.scrollLeft,
      startScrollTop: event.currentTarget.scrollTop
    };
    event.currentTarget.setPointerCapture(event.pointerId);
    setIsImageViewerDragging(true);
    event.preventDefault();
  }, []);

  const handleImageViewerPointerMove = useCallback((event: PointerEvent<HTMLDivElement>) => {
    const dragState = imageViewerDragStateRef.current;
    if (!dragState || dragState.pointerId !== event.pointerId) {
      return;
    }

    const deltaX = event.clientX - dragState.startClientX;
    const deltaY = event.clientY - dragState.startClientY;
    event.currentTarget.scrollLeft = dragState.startScrollLeft - deltaX;
    event.currentTarget.scrollTop = dragState.startScrollTop - deltaY;
    event.preventDefault();
  }, []);

  const handleImageViewerPointerUp = useCallback(
    (event: PointerEvent<HTMLDivElement>) => {
      clearImageViewerDrag(event.currentTarget, event.pointerId);
    },
    [clearImageViewerDrag]
  );

  useEffect(() => {
    if (!imageViewer) {
      imageViewerDragStateRef.current = null;
      setIsImageViewerDragging(false);
    }
  }, [imageViewer]);

  useEffect(() => {
    if (!imageViewer) {
      return;
    }

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        closeImageViewer();
      }
    };

    window.addEventListener("keydown", onKeyDown);
    return () => {
      window.removeEventListener("keydown", onKeyDown);
    };
  }, [closeImageViewer, imageViewer]);

  return {
    imageViewer,
    imageViewerZoomPercent,
    isImageViewerDragging,
    isImageViewerInfoOpen,
    closeImageViewer,
    openImageViewer,
    setImageViewerZoomPercent,
    setIsImageViewerInfoOpen,
    handleImageViewerPointerDown,
    handleImageViewerPointerMove,
    handleImageViewerPointerUp
  };
}
