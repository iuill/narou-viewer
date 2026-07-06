import { forwardRef } from "react";
import { ReaderFloatingPanel } from "./ReaderFloatingPanel";
import { formatBookmarkLocation } from "./features/library/bookmarkSummary";
import {
  buildEpisodeLabel,
  formatEpisodeIndexLabel,
  type EpisodeDisplayReference
} from "./features/reader/episodeLabels";
import type { Bookmark, EpisodeIndex } from "./features/reader/types";

type Props = {
  bookmarks: Bookmark[];
  currentEpisodeLabel: string;
  currentEpisodeTitle: string;
  episodeDisplayLookup: Map<EpisodeIndex, EpisodeDisplayReference>;
  isBookmarkSaving: boolean;
  isEpisodeLoaded: boolean;
  isShowingAllBookmarks: boolean;
  onClose: () => void;
  onCreateBookmark: () => void;
  onDeleteBookmark: (bookmarkId: string) => void;
  onOpenBookmark: (bookmark: Bookmark) => void;
  onToggleAllBookmarks: () => void;
  pendingBookmarkId: string | null;
  preferFriendlyEpisodeLabels: boolean;
  visibleBookmarks: Bookmark[];
};

function formatBookmarkPanelTimestamp(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return new Intl.DateTimeFormat("ja-JP", {
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit"
  }).format(date);
}

export const ReaderBookmarkPanel = forwardRef<HTMLElement, Props>(function ReaderBookmarkPanel(
  {
    bookmarks,
    currentEpisodeLabel,
    currentEpisodeTitle,
    episodeDisplayLookup,
    isBookmarkSaving,
    isEpisodeLoaded,
    isShowingAllBookmarks,
    onClose,
    onCreateBookmark,
    onDeleteBookmark,
    onOpenBookmark,
    onToggleAllBookmarks,
    pendingBookmarkId,
    preferFriendlyEpisodeLabels,
    visibleBookmarks
  },
  ref
) {
  return (
    <ReaderFloatingPanel
      ariaLabel="本文画面の栞"
      className="reader-bookmark-panel reader-overlay-panel--bookmarks"
      description={`${bookmarks.length} 件の栞を確認できます。`}
      onClose={onClose}
      ref={ref}
      title="栞"
    >
      <section className="reader-panel-card reader-panel-card--compact">
        <p className="reader-panel-section-label">現在位置</p>
        <p className="reader-panel-section-description">
          {currentEpisodeLabel} / {currentEpisodeTitle}
        </p>
        <div className="reader-actions">
          <button disabled={!isEpisodeLoaded || isBookmarkSaving} onClick={onCreateBookmark} type="button">
            {isBookmarkSaving ? "保存中..." : "この位置に栞を追加"}
          </button>
        </div>
      </section>
      <section className="reader-panel-card reader-panel-card--compact">
        <div className="panel-header compact">
          <h3>栞一覧</h3>
          <p>{bookmarks.length} 件</p>
        </div>
        {bookmarks.length === 0 ? (
          <p className="message">まだ栞はありません。</p>
        ) : (
          <>
            <div className="bookmark-list reader-bookmarks reader-bookmark-panel-list">
              {visibleBookmarks.map((bookmark) => {
                const bookmarkEpisode = episodeDisplayLookup.get(bookmark.episodeIndex);

                return (
                  <div
                    className="bookmark-item reader-bookmark-panel-item"
                    data-episode-index={bookmark.episodeIndex}
                    key={bookmark.id}
                  >
                    <div className="reader-bookmark-panel-item-content">
                      <div
                        className="reader-bookmark-panel-item-heading"
                        title={`${formatBookmarkLocation(bookmark, episodeDisplayLookup, preferFriendlyEpisodeLabels)}${
                          bookmark.label ? ` - ${bookmark.label}` : ""
                        }`}
                      >
                        <strong className="reader-bookmark-panel-item-index">
                          {formatEpisodeIndexLabel(
                            bookmark.episodeIndex,
                            episodeDisplayLookup,
                            preferFriendlyEpisodeLabels
                          )}
                        </strong>
                        <span className="reader-bookmark-panel-item-title">
                          {bookmarkEpisode
                            ? buildEpisodeLabel(bookmarkEpisode)
                            : formatBookmarkLocation(bookmark, episodeDisplayLookup, preferFriendlyEpisodeLabels)}
                          {bookmark.label ? ` - ${bookmark.label}` : ""}
                        </span>
                      </div>
                      <span className="reader-bookmark-panel-item-meta">
                        {formatBookmarkPanelTimestamp(bookmark.createdAt)} / 位置 {bookmark.position}
                      </span>
                    </div>
                    <div className="reader-bookmark-panel-item-actions">
                      <button onClick={() => onOpenBookmark(bookmark)} type="button">
                        開く
                      </button>
                      <button
                        className="danger"
                        disabled={pendingBookmarkId === bookmark.id}
                        onClick={() => onDeleteBookmark(bookmark.id)}
                        type="button"
                      >
                        削除
                      </button>
                    </div>
                  </div>
                );
              })}
            </div>
            {bookmarks.length > visibleBookmarks.length || isShowingAllBookmarks ? (
              <button
                aria-expanded={isShowingAllBookmarks}
                className="summary-link-button bookmark-list-toggle"
                onClick={onToggleAllBookmarks}
                type="button"
              >
                {isShowingAllBookmarks ? "折りたたみ" : "すべて表示"}
              </button>
            ) : null}
          </>
        )}
      </section>
    </ReaderFloatingPanel>
  );
});
