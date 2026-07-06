import type { Dispatch, KeyboardEvent, MouseEvent, PointerEvent, RefObject, SetStateAction } from "react";

const IMAGE_VIEWER_MIN_ZOOM_PERCENT = 25;
const IMAGE_VIEWER_MAX_ZOOM_PERCENT = 300;
const IMAGE_VIEWER_ZOOM_STEP_PERCENT = 10;

export type ImageViewerState = {
  src: string;
  originalUrl: string | null;
  title: string | null;
  alt: string | null;
  naturalWidth: number | null;
  naturalHeight: number | null;
};

type ReaderImageViewerProps = {
  imageViewer: ImageViewerState;
  imageViewerWidth: number | null;
  isDragging: boolean;
  isInfoOpen: boolean;
  stageRef: RefObject<HTMLDivElement | null>;
  zoomPercent: number;
  onClose: () => void;
  onInfoOpenChange: Dispatch<SetStateAction<boolean>>;
  onPointerDown: (event: PointerEvent<HTMLDivElement>) => void;
  onPointerMove: (event: PointerEvent<HTMLDivElement>) => void;
  onPointerUp: (event: PointerEvent<HTMLDivElement>) => void;
  onZoomPercentChange: Dispatch<SetStateAction<number>>;
};

export function ReaderImageViewer({
  imageViewer,
  imageViewerWidth,
  isDragging,
  isInfoOpen,
  stageRef,
  zoomPercent,
  onClose,
  onInfoOpenChange,
  onPointerDown,
  onPointerMove,
  onPointerUp,
  onZoomPercentChange
}: ReaderImageViewerProps) {
  function handleBackdropClick(event: MouseEvent<HTMLDivElement>) {
    if (event.target === event.currentTarget) {
      onClose();
    }
  }

  function handleBackdropKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === "Escape") {
      event.stopPropagation();
      onClose();
    }
  }

  return (
    <div
      aria-label="画像拡大表示"
      className="reader-image-viewer"
      onClick={handleBackdropClick}
      onKeyDown={handleBackdropKeyDown}
      role="dialog"
    >
      <button
        aria-label="画像拡大表示を閉じる"
        className="reader-image-viewer-close"
        onClick={(event) => {
          event.stopPropagation();
          onClose();
        }}
        type="button"
      >
        ×
      </button>
      <div
        className={`reader-image-viewer-stage ${isDragging ? "dragging" : ""}`}
        onPointerCancel={onPointerUp}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        ref={stageRef}
        role="presentation"
      >
        <div className="reader-image-viewer-canvas">
          <img
            alt={imageViewer.alt ?? imageViewer.title ?? ""}
            className="reader-image-viewer-image"
            src={imageViewer.src}
            style={imageViewerWidth ? { width: `${imageViewerWidth}px` } : undefined}
          />
        </div>
      </div>
      <div className="reader-image-viewer-toolbar reader-image-viewer-toolbar-left">
        <button
          aria-expanded={isInfoOpen}
          aria-label="画像情報"
          className="reader-image-viewer-info-button"
          onClick={() => onInfoOpenChange((current) => !current)}
          type="button"
        >
          i
        </button>
        {isInfoOpen ? (
          <section className="reader-image-viewer-info-panel">
            {imageViewer.title ? <p className="reader-image-viewer-info-title">{imageViewer.title}</p> : null}
            {imageViewer.originalUrl ? (
              <p className="reader-image-viewer-info-link">
                <a href={imageViewer.originalUrl} rel="noreferrer" target="_blank">
                  オリジナル画像を開く
                </a>
              </p>
            ) : (
              <p className="reader-image-viewer-info-link">元URLなし</p>
            )}
          </section>
        ) : null}
      </div>
      <div className="reader-image-viewer-toolbar reader-image-viewer-toolbar-right">
        <button
          aria-label="縮小"
          className="reader-image-viewer-zoom-button"
          onClick={() =>
            onZoomPercentChange((current) =>
              Math.max(IMAGE_VIEWER_MIN_ZOOM_PERCENT, current - IMAGE_VIEWER_ZOOM_STEP_PERCENT)
            )
          }
          type="button"
        >
          -
        </button>
        <label className="reader-image-viewer-zoom-control">
          <span>{zoomPercent}%</span>
          <input
            max={IMAGE_VIEWER_MAX_ZOOM_PERCENT}
            min={IMAGE_VIEWER_MIN_ZOOM_PERCENT}
            onChange={(event) => onZoomPercentChange(Number.parseInt(event.target.value, 10))}
            step={IMAGE_VIEWER_ZOOM_STEP_PERCENT}
            type="range"
            value={zoomPercent}
          />
        </label>
        <button
          aria-label="拡大"
          className="reader-image-viewer-zoom-button"
          onClick={() =>
            onZoomPercentChange((current) =>
              Math.min(IMAGE_VIEWER_MAX_ZOOM_PERCENT, current + IMAGE_VIEWER_ZOOM_STEP_PERCENT)
            )
          }
          type="button"
        >
          +
        </button>
      </div>
    </div>
  );
}
