import { useEffect, useState } from "react";
import { formatDate } from "./shared/date";
import { ListPaginationControls } from "./ListPaginationControls";
import { PublicationPanel } from "./PublicationPanel";
import { formatBookmarkLocation } from "./features/library/bookmarkSummary";
import type { NovelSummary } from "./features/library/types";
import { buildEpisodeLabel, formatEpisodeIndexLabel, formatEpisodeReferenceLabel } from "./features/reader/episodeLabels";
import type { EpisodeDisplayReference } from "./features/reader/episodeLabels";
import type { Bookmark, ReaderState, TocEpisode, TocResponse } from "./features/reader/types";
import type { PublicationEntry, PublicationKind } from "./features/publications/types";

type Pagination = {
  currentPage: number;
  endItemNumber: number;
  startItemNumber: number;
  totalItems: number;
  totalPages: number;
};

type CurrentNovel = Pick<
  NovelSummary,
  "novelId" | "title" | "author" | "fetcherWorkId" | "fetchStatus" | "savedEpisodes" | "totalEpisodes" | "resumeEpisodeId"
>;

type TocSection = "episodes" | "publications" | "bookmarks";

type Props = {
  currentNovel: CurrentNovel | null;
  initialSection?: TocSection;
  isMobileLibraryViewport: boolean;
  isNovelLoading: boolean;
  publicationProps: {
    displayCoverEntryId: string;
    entries: PublicationEntry[];
    isLoading: boolean;
    savingEntryId: string | null;
    onCreateISBN: (kind: PublicationKind, isbn13: string) => void | Promise<void>;
    onSaveISBN: (entryId: string, isbn13: string) => void | Promise<void>;
    onClear: (entry: PublicationEntry) => void | Promise<void>;
    onDisable: (entry: PublicationEntry) => void | Promise<void>;
    onRedisplay: (entry: PublicationEntry) => void | Promise<void>;
    onSetDisplayCover: (entryId: string) => void | Promise<void>;
  };
  toc: TocResponse | null;
  novelsCount: number;
  tocStoryText: string;
  isTocStoryTruncated: boolean;
  isTocStoryExpanded: boolean;
  lastReadEpisodeIndex: string | null;
  readerState: ReaderState | null;
  latestBookmark: Bookmark | null;
  isNovelActionSubmitting: boolean;
  isResumeSubmitting: boolean;
  bookmarks: Bookmark[];
  visibleBookmarks: Bookmark[];
  pendingBookmarkId: string | null;
  isShowingAllBookmarks: boolean;
  visibleTocEpisodes: TocEpisode[];
  selectedEpisodeIndex: string | null;
  tocPagination: Pagination;
  episodeDisplayLookup: Map<string, EpisodeDisplayReference>;
  preferFriendlyEpisodeLabels: boolean;
  onBackToLibrary: () => void;
  onToggleStoryExpanded: () => void;
  onOpenEpisode: (episodeIndex: string, position?: number | null) => void;
  onOpenBookmark: (bookmark: Bookmark) => void;
  onResumeNovel: (novelId: string) => void | Promise<void>;
  onUpdateNovel: () => void | Promise<void>;
  onRemoveNovel: () => void | Promise<void>;
  onDeleteBookmark: (bookmarkId: string) => void | Promise<void>;
  onToggleShowingAllBookmarks: () => void;
  onTocPageChange: (page: number) => void;
};

export function TocPanel({
  currentNovel,
  initialSection = "episodes",
  isMobileLibraryViewport,
  isNovelLoading,
  publicationProps,
  toc,
  novelsCount,
  tocStoryText,
  isTocStoryTruncated,
  isTocStoryExpanded,
  lastReadEpisodeIndex,
  readerState,
  latestBookmark,
  isNovelActionSubmitting,
  isResumeSubmitting,
  bookmarks,
  visibleBookmarks,
  pendingBookmarkId,
  isShowingAllBookmarks,
  visibleTocEpisodes,
  selectedEpisodeIndex,
  tocPagination,
  episodeDisplayLookup,
  preferFriendlyEpisodeLabels,
  onBackToLibrary,
  onToggleStoryExpanded,
  onOpenEpisode,
  onOpenBookmark,
  onResumeNovel,
  onUpdateNovel,
  onRemoveNovel,
  onDeleteBookmark,
  onToggleShowingAllBookmarks,
  onTocPageChange
}: Props) {
  const hasVisibleUnfetchedEpisode = visibleTocEpisodes.some(
    (tocEpisode) => tocEpisode.bodyStatus && tocEpisode.bodyStatus !== "complete"
  );
  const hasIncompleteFetchStatus = Boolean(currentNovel?.fetchStatus && currentNovel.fetchStatus !== "complete");
  const hasUnsavedEpisodes =
    typeof currentNovel?.savedEpisodes === "number" && currentNovel.savedEpisodes < currentNovel.totalEpisodes;
  const canResumeCurrentNovel = Boolean(
    currentNovel?.fetcherWorkId &&
      (currentNovel.resumeEpisodeId || hasVisibleUnfetchedEpisode || hasIncompleteFetchStatus || hasUnsavedEpisodes)
  );
  const [activeSection, setActiveSection] = useState<TocSection>(initialSection);
  const currentNovelId = currentNovel?.novelId ?? null;
  const publicationCount = publicationProps.entries.filter(
    (entry) => entry.status !== "unknown" && entry.status !== "disabled"
  ).length;

  useEffect(() => {
    if (currentNovelId === null) {
      return;
    }
    setActiveSection(initialSection);
  }, [currentNovelId, initialSection]);

  return (
    <section className="panel toc-panel">
      <div className="panel-header">
        <div>
          <h2>{currentNovel?.title ?? "目次"}</h2>
          <p>{currentNovel?.author || "著者未設定"}</p>
        </div>
        <div className="panel-header-actions toc-panel-header-actions">
          {isMobileLibraryViewport ? (
            <button className="toc-back-button" onClick={onBackToLibrary} type="button">
              ライブラリへ
            </button>
          ) : null}
          {isNovelLoading ? <p>loading...</p> : null}
        </div>
      </div>

      {toc ? (
        <div className="toc-panel-content">
          <div className="novel-summary">
            <div className="novel-summary-story">
              <p>{tocStoryText}</p>
              {isTocStoryTruncated ? (
                <button
                  aria-expanded={isTocStoryExpanded}
                  className="summary-link-button"
                  onClick={onToggleStoryExpanded}
                  type="button"
                >
                  {isTocStoryExpanded ? "折りたたみ" : "すべて表示"}
                </button>
              ) : null}
            </div>
            <dl>
              <div>
                <dt>最終既読</dt>
                <dd>
                  {lastReadEpisodeIndex === null ? (
                    "未読"
                  ) : (
                    <button
                      className="summary-link-button"
                      data-episode-index={lastReadEpisodeIndex}
                      onClick={() =>
                        onOpenEpisode(
                          lastReadEpisodeIndex,
                          lastReadEpisodeIndex === readerState?.lastReadEpisodeIndex ? readerState.position : null
                        )
                      }
                      type="button"
                    >
                      {formatEpisodeReferenceLabel(lastReadEpisodeIndex, episodeDisplayLookup, preferFriendlyEpisodeLabels)}
                    </button>
                  )}
                </dd>
              </div>
              <div>
                <dt>最新栞</dt>
                <dd>
                  {latestBookmark === null ? (
                    "なし"
                  ) : (
                    <button
                      className="summary-link-button"
                      data-episode-index={latestBookmark.episodeIndex}
                      onClick={() => onOpenBookmark(latestBookmark)}
                      type="button"
                    >
                      {formatBookmarkLocation(latestBookmark, episodeDisplayLookup, preferFriendlyEpisodeLabels)}
                    </button>
                  )}
                </dd>
              </div>
              <div>
                <dt>最終更新</dt>
                <dd>{formatDate(toc.updatedAt)}</dd>
              </div>
            </dl>
            <div className="novel-summary-actions">
              <button
                className="summary-action-button"
                disabled={isResumeSubmitting || !canResumeCurrentNovel}
                onClick={() => {
                  if (!currentNovel || !canResumeCurrentNovel) {
                    return;
                  }
                  void onResumeNovel(currentNovel.novelId);
                }}
                title={
                  canResumeCurrentNovel
                    ? "保存済みの話は再取得せず、未取得または失敗した話から取得します。既存話も取り直す場合は更新を使います。"
                    : "未取得または失敗した話がある場合に、保存済みの話をスキップして取得を再開します。"
                }
                type="button"
              >
                {isResumeSubmitting ? "再開中..." : "再開"}
              </button>
              <button
                className="summary-action-button"
                disabled={isNovelActionSubmitting || !currentNovel?.fetcherWorkId}
                onClick={() => {
                  void onUpdateNovel();
                }}
                type="button"
              >
                {isNovelActionSubmitting ? "処理中..." : "更新"}
              </button>
              <button
                className="summary-action-button danger"
                disabled={isNovelActionSubmitting || !currentNovel?.fetcherWorkId}
                onClick={() => {
                  void onRemoveNovel();
                }}
                type="button"
              >
                削除
              </button>
            </div>
          </div>

          <div aria-label="作品詳細セクション" className="toc-section-tabs" role="tablist">
            <button
              aria-controls="toc-section-episodes"
              aria-selected={activeSection === "episodes"}
              id="toc-section-tab-episodes"
              onClick={() => setActiveSection("episodes")}
              role="tab"
              type="button"
            >
              話
              <span>{tocPagination.totalItems} 件</span>
            </button>
            <button
              aria-controls="toc-section-publications"
              aria-selected={activeSection === "publications"}
              id="toc-section-tab-publications"
              onClick={() => setActiveSection("publications")}
              role="tab"
              type="button"
            >
              書籍情報
              <span>{publicationCount} 件</span>
            </button>
            <button
              aria-controls="toc-section-bookmarks"
              aria-selected={activeSection === "bookmarks"}
              id="toc-section-tab-bookmarks"
              onClick={() => setActiveSection("bookmarks")}
              role="tab"
              type="button"
            >
              栞
              <span>{bookmarks.length} 件</span>
            </button>
          </div>

          {activeSection === "episodes" ? (
          <section
            aria-labelledby="toc-section-tab-episodes"
            className="episode-block"
            id="toc-section-episodes"
            role="tabpanel"
          >
            <div className="panel-header compact">
              <h3>話</h3>
              <p>
                {tocPagination.totalItems > 0
                  ? `${tocPagination.startItemNumber}-${tocPagination.endItemNumber} / ${tocPagination.totalItems} 件`
                  : "0 件"}
              </p>
            </div>
            <ListPaginationControls
              currentPage={tocPagination.currentPage}
              endItemNumber={tocPagination.endItemNumber}
              label="話一覧"
              onPageChange={onTocPageChange}
              startItemNumber={tocPagination.startItemNumber}
              totalItems={tocPagination.totalItems}
              totalPages={tocPagination.totalPages}
            />
            {tocPagination.totalItems === 0 ? (
              <p className="message">話データがありません。</p>
            ) : (
              <div className="toc-list">
                {visibleTocEpisodes.map((tocEpisode) => {
                  const isFetched = !tocEpisode.bodyStatus || tocEpisode.bodyStatus === "complete";
                  const statusLabel = isFetched ? (tocEpisode.updatedAt ? formatDate(tocEpisode.updatedAt) : "更新日未取得") : "未取得";

                  return (
                    <button
                      data-episode-index={tocEpisode.episodeIndex}
                      key={`${tocEpisode.episodeIndex}-${tocEpisode.contentEtag}`}
                      className={`toc-item ${selectedEpisodeIndex === tocEpisode.episodeIndex ? "selected" : ""} ${isFetched ? "" : "unfetched"}`}
                      disabled={!isFetched}
                      onClick={() => onOpenEpisode(tocEpisode.episodeIndex)}
                      type="button"
                    >
                      <span className="toc-index">
                        {formatEpisodeIndexLabel(tocEpisode.episodeIndex, episodeDisplayLookup, preferFriendlyEpisodeLabels)}
                      </span>
                      <strong className="toc-title">{buildEpisodeLabel(tocEpisode)}</strong>
                      <span className="toc-status">{statusLabel}</span>
                    </button>
                  );
                })}
              </div>
            )}
            {tocPagination.totalPages > 1 ? (
              <ListPaginationControls
                ariaLabel="話一覧ページ切り替え（下部）"
                currentPage={tocPagination.currentPage}
                endItemNumber={tocPagination.endItemNumber}
                label="話一覧"
                onPageChange={onTocPageChange}
                startItemNumber={tocPagination.startItemNumber}
                totalItems={tocPagination.totalItems}
                totalPages={tocPagination.totalPages}
              />
            ) : null}
          </section>
          ) : null}

          {activeSection === "publications" ? (
            <div aria-labelledby="toc-section-tab-publications" id="toc-section-publications" role="tabpanel">
              <PublicationPanel {...publicationProps} />
            </div>
          ) : null}

          {activeSection === "bookmarks" ? (
          <section
            aria-labelledby="toc-section-tab-bookmarks"
            className="bookmark-block"
            id="toc-section-bookmarks"
            role="tabpanel"
          >
            <div className="panel-header compact">
              <h3>栞</h3>
              <p>{bookmarks.length} 件</p>
            </div>
            {bookmarks.length === 0 ? (
              <p className="message">まだ栞はありません。</p>
            ) : (
              <>
                <div className="bookmark-list reader-bookmarks">
                  {visibleBookmarks.map((bookmark) => (
                    <div className="bookmark-item" data-episode-index={bookmark.episodeIndex} key={bookmark.id}>
                      <span>
                        {formatBookmarkLocation(bookmark, episodeDisplayLookup, preferFriendlyEpisodeLabels)}
                        {bookmark.label ? ` - ${bookmark.label}` : ""}
                        {` (${formatDate(bookmark.createdAt)})`}
                      </span>
                      <button onClick={() => onOpenBookmark(bookmark)} type="button">
                        開く
                      </button>
                      <button
                        className="danger"
                        disabled={pendingBookmarkId === bookmark.id}
                        onClick={() => {
                          void onDeleteBookmark(bookmark.id);
                        }}
                        type="button"
                      >
                        削除
                      </button>
                    </div>
                  ))}
                </div>
                {bookmarks.length > 3 ? (
                  <button
                    aria-expanded={isShowingAllBookmarks}
                    className="summary-link-button bookmark-list-toggle"
                    onClick={onToggleShowingAllBookmarks}
                    type="button"
                  >
                    {isShowingAllBookmarks ? "折りたたみ" : "すべて表示"}
                  </button>
                ) : null}
              </>
            )}
          </section>
          ) : null}
        </div>
      ) : (
        <p className="message">
          {novelsCount === 0 ? "ライブラリに作品を追加すると、ここに目次が表示されます。" : "作品を選択すると目次が表示されます。"}
        </p>
      )}
    </section>
  );
}
