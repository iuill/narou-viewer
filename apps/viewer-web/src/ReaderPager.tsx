import type { CSSProperties, MouseEvent, RefObject, TouchEvent } from "react";
import type { EpisodeResponse } from "./features/reader/types";
import type { ReadingMode } from "./readerPreferences";

const READER_VERTICAL_INLINE_PADDING_PX = 24;

type ReaderPagerProps = {
  articleFontFamilyCss: string;
  articleFontWeight: number | null;
  displayedPageNumber: number;
  episode: EpisodeResponse | null;
  isEpisodeLoading: boolean;
  isFullscreen: boolean;
  isLoadingOverlayVisible: boolean;
  isTouchDevice: boolean;
  letterSpacingEm: number;
  loadingEpisodeTitle: string;
  loadingNovelTitle: string;
  pageIndicatorRef: RefObject<HTMLParagraphElement | null>;
  readerFontSizePx: number;
  readingMode: ReadingMode;
  renderedEpisodeHtml: string;
  totalPages: number;
  verticalLastPageReservePx: number;
  viewportRef: RefObject<HTMLDivElement | null>;
  onViewportClick: (event: MouseEvent<HTMLDivElement>) => void;
  onViewportTouchCancel: (event: TouchEvent<HTMLDivElement>) => void;
  onViewportTouchEnd: (event: TouchEvent<HTMLDivElement>) => void;
  onViewportTouchStart: (event: TouchEvent<HTMLDivElement>) => void;
};

export function ReaderPager({
  articleFontFamilyCss,
  articleFontWeight,
  displayedPageNumber,
  episode,
  isEpisodeLoading,
  isFullscreen,
  isLoadingOverlayVisible,
  isTouchDevice,
  letterSpacingEm,
  loadingEpisodeTitle,
  loadingNovelTitle,
  pageIndicatorRef,
  readerFontSizePx,
  readingMode,
  renderedEpisodeHtml,
  totalPages,
  verticalLastPageReservePx,
  viewportRef,
  onViewportClick,
  onViewportTouchCancel,
  onViewportTouchEnd,
  onViewportTouchStart
}: ReaderPagerProps) {
  if (!episode && !isEpisodeLoading && !isLoadingOverlayVisible) {
    return <p className="message">本文を読み込めませんでした。</p>;
  }

  const articleStyle: CSSProperties = {
    fontFamily: articleFontFamilyCss,
    fontWeight: articleFontWeight ?? undefined,
    fontSize: `${readerFontSizePx}px`,
    letterSpacing: `${letterSpacingEm}em`,
    paddingLeft:
      readingMode === "vertical"
        ? `${READER_VERTICAL_INLINE_PADDING_PX + verticalLastPageReservePx}px`
        : undefined
  };

  return (
    <div className="reader-pager">
      {episode ? (
        <>
          {/* biome-ignore lint/a11y/useKeyWithClickEvents: keyboard page movement is handled by the reader-level ArrowLeft/ArrowRight shortcuts; Enter/Space must remain available to links inside the article. */}
          <div
            aria-label="本文ページ操作領域"
            className={`reader-page-viewport ${readingMode}`}
            onClick={isTouchDevice ? undefined : onViewportClick}
            onTouchStart={isTouchDevice ? onViewportTouchStart : undefined}
            onTouchEnd={isTouchDevice ? onViewportTouchEnd : undefined}
            onTouchCancel={isTouchDevice ? onViewportTouchCancel : undefined}
            ref={viewportRef}
            role="document"
            tabIndex={isTouchDevice ? undefined : 0}
          >
            <article
              className={`reader-prose reader-prose-paged ${readingMode} ${isFullscreen ? "fullscreen" : ""}`}
              // biome-ignore lint/security/noDangerouslySetInnerHtml: renderedEpisodeHtml is structured reader HTML produced by readerDocument.
              dangerouslySetInnerHTML={{ __html: renderedEpisodeHtml }}
              style={articleStyle}
            />
          </div>
          <p className="reader-page-indicator" ref={pageIndicatorRef}>
            {displayedPageNumber} / {totalPages}
          </p>
        </>
      ) : null}
      {isEpisodeLoading || isLoadingOverlayVisible ? (
        <div aria-live="polite" className="reader-loading-overlay" role="status">
          <div className="reader-loading-overlay-card">
            <span aria-hidden="true" className="reader-loading-logo">
              <span className="reader-loading-book">
                <span className="reader-loading-book-page reader-loading-book-page-left" />
                <span className="reader-loading-book-page reader-loading-book-page-right" />
                <span className="reader-loading-book-bookmark" />
              </span>
              <span className="reader-loading-logo-line" />
            </span>
            <strong>本文を読み込み中</strong>
            <span className="reader-loading-title">
              <span>{loadingNovelTitle}</span>
              <span>{loadingEpisodeTitle}</span>
            </span>
          </div>
        </div>
      ) : null}
    </div>
  );
}
