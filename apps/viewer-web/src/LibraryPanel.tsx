import { useState, type DragEvent, type FormEvent } from "react";
import {
  formatFetcherTaskProgress,
  formatFetcherTaskStepProgress,
  formatFetcherTaskFailure,
  getFetcherTaskProgressValue,
  getFetcherTaskStatusLabel,
  getFetcherTaskTargetLabel,
  getFetcherTaskTypeLabel,
  hasFetcherTaskDeterminateProgress,
  type FetcherTask
} from "./features/fetcher/model";
import { formatNovelLastReadLabel } from "./features/library/bookmarkSummary";
import { normalizeStoryText, createStoryPreview } from "./features/library/text";
import { formatDate } from "./shared/date";
import { ListPaginationControls } from "./ListPaginationControls";
import { StorageUsagePopover } from "./StorageUsagePopover";
import type { NovelSummary } from "./features/library/types";

const LIBRARY_STORY_PREVIEW_LENGTH = 120;

type Pagination = {
  currentPage: number;
  endItemNumber: number;
  startItemNumber: number;
  totalItems: number;
  totalPages: number;
};

type ActiveTaskEntry = {
  key: string;
  task: FetcherTask;
};

function formatFetchStatusLabel(novel: NovelSummary): string {
  switch (novel.fetchStatus) {
    case "partial":
      return "取得中";
    case "failed":
      return "失敗";
    case "canceled":
      return "中止";
    default:
      return novel.fetchStatus ?? "取得状態";
  }
}

function formatFetchStatusDetail(novel: NovelSummary): string {
  const savedEpisodes = novel.savedEpisodes ?? 0;
  const base = `${savedEpisodes} / ${novel.totalEpisodes} 話を保存済み`;
  if (novel.resumeEpisodeId) {
    return `${base}、${novel.resumeEpisodeId} 話から再開できます。`;
  }
  if (novel.failedEpisodeId) {
    return `${base}、${novel.failedEpisodeId} 話で停止しました。`;
  }
  return base;
}

function canResumeNovel(novel: NovelSummary): boolean {
  const hasIncompleteFetchStatus = Boolean(novel.fetchStatus && novel.fetchStatus !== "complete");
  const hasUnsavedEpisodes = typeof novel.savedEpisodes === "number" && novel.savedEpisodes < novel.totalEpisodes;

  return Boolean(novel.fetcherWorkId && (novel.resumeEpisodeId || hasIncompleteFetchStatus || hasUnsavedEpisodes));
}

function isGoogleBooksCover(novel: NovelSummary): novel is NovelSummary & { publicationCoverSourceUrl: string } {
  return novel.publicationCoverSource === "Google Books" && Boolean(novel.publicationCoverSourceUrl);
}

function formatPublicationKindLabel(kind: NovelSummary["publicationCoverKind"]): string {
  switch (kind) {
    case "novel":
      return "小説版";
    case "comic":
      return "コミック版";
    default:
      return "書籍";
  }
}

type Props = {
  novelsCount: number;
  filteredNovelsCount: number;
  libraryFilterQuery: string;
  mobileHomeTab?: "library" | "download";
  isDownloadComposerOpen: boolean;
  isDownloadDropActive: boolean;
  downloadTarget: string;
  downloadForce: boolean;
  isDownloadSubmitting: boolean;
  isLibraryExporting?: boolean;
  libraryNotice: string | null;
  hasActiveFetcherTasks: boolean;
  activeFetcherTasksCount: number;
  activeFetcherTaskEntries: ActiveTaskEntry[];
  fetcherStatusError: string | null;
  fetcherQueueRunning: boolean;
  queueStatusLabel: string;
  fetcherStatusCheckedAt: string | null;
  cancelingFetcherTaskIds: Set<string>;
  resumingNovelIds: Set<string>;
  updatingNovelIds?: Set<string>;
  libraryPagination: Pagination;
  resumableNovels?: NovelSummary[];
  updatableNovels?: NovelSummary[];
  visibleLibraryNovels: NovelSummary[];
  selectedNovelId: string | null;
  showStoryAction: boolean;
  onToggleDownloadComposer: () => void;
  onDownloadTargetChange: (value: string) => void;
  onDownloadForceChange: (checked: boolean) => void;
  onCloseDownloadComposer: () => void;
  onDownloadSubmit: (event: FormEvent<HTMLFormElement>) => void | Promise<void>;
  onExportLibrary?: () => void | Promise<void>;
  onDownloadDragEnter: (event: DragEvent<HTMLDivElement>) => void;
  onDownloadDragLeave: (event: DragEvent<HTMLDivElement>) => void;
  onDownloadDragOver: (event: DragEvent<HTMLDivElement>) => void;
  onDownloadDrop: (event: DragEvent<HTMLDivElement>) => void;
  onCancelFetcherTask: (taskId: string) => void | Promise<void>;
  onLibraryFilterQueryChange: (value: string) => void;
  onClearLibraryFilter: () => void;
  onLibraryPageChange: (page: number) => void;
  onOpenNovelPublications?: (novelId: string) => void;
  onSelectNovel: (novelId: string) => void;
  onResumeNovel: (novelId: string) => void | Promise<void>;
  onUpdateNovel?: (novelId: string) => void | Promise<void>;
};

export function LibraryPanel({
  novelsCount,
  filteredNovelsCount,
  libraryFilterQuery,
  mobileHomeTab,
  isDownloadComposerOpen,
  isDownloadDropActive,
  downloadTarget,
  downloadForce,
  isDownloadSubmitting,
  isLibraryExporting = false,
  libraryNotice,
  hasActiveFetcherTasks,
  activeFetcherTasksCount,
  activeFetcherTaskEntries,
  fetcherStatusError,
  fetcherQueueRunning,
  queueStatusLabel,
  fetcherStatusCheckedAt,
  cancelingFetcherTaskIds = new Set<string>(),
  resumingNovelIds = new Set<string>(),
  updatingNovelIds = new Set<string>(),
  libraryPagination,
  resumableNovels = [],
  updatableNovels = [],
  visibleLibraryNovels,
  selectedNovelId,
  showStoryAction,
  onToggleDownloadComposer,
  onDownloadTargetChange,
  onDownloadForceChange,
  onCloseDownloadComposer,
  onDownloadSubmit,
  onExportLibrary = () => {},
  onDownloadDragEnter,
  onDownloadDragLeave,
  onDownloadDragOver,
  onDownloadDrop,
  onCancelFetcherTask = () => {},
  onLibraryFilterQueryChange,
  onClearLibraryFilter,
  onLibraryPageChange,
  onOpenNovelPublications,
  onSelectNovel,
  onResumeNovel,
  onUpdateNovel = () => {}
}: Props) {
  const [openStoryNovelId, setOpenStoryNovelId] = useState<string | null>(null);
  const [expandedStoryNovelId, setExpandedStoryNovelId] = useState<string | null>(null);
  const isMobileDownloadTab = mobileHomeTab === "download";
  const shouldShowDownloadComposer = isDownloadComposerOpen || isMobileDownloadTab;
  const shouldShowQueueSection = hasActiveFetcherTasks || fetcherStatusError || isMobileDownloadTab;
  const shouldShowLibraryList = !isMobileDownloadTab;
  const hasResumableNovels = resumableNovels.length > 0;
  const hasUpdatableNovels = updatableNovels.length > 0;
  const googleBooksCoverNovels = visibleLibraryNovels.filter(isGoogleBooksCover);
  const googleBooksCoverCredit =
    googleBooksCoverNovels.length > 0 ? (
      <details className="library-provider-credit">
        <summary>
          <span aria-hidden="true" className="library-provider-credit-icon">
            i
          </span>
          <span className="library-provider-credit-label">カバー出典</span>
          <span className="library-provider-credit-powered">Powered by Google</span>
        </summary>
        <ul>
          {googleBooksCoverNovels.map((novel) => (
            <li key={`${novel.novelId}-${novel.publicationCoverKind ?? "book"}`}>
              <span>
                {novel.title}（{formatPublicationKindLabel(novel.publicationCoverKind)}）
              </span>
              <a href={novel.publicationCoverSourceUrl} rel="noreferrer" target="_blank">
                Google Books で見る
              </a>
            </li>
          ))}
        </ul>
      </details>
    ) : null;

  function handleToggleStory(novelId: string) {
    const isClosingCurrentStory = openStoryNovelId === novelId;
    setOpenStoryNovelId(isClosingCurrentStory ? null : novelId);
    if (isClosingCurrentStory || expandedStoryNovelId !== novelId) {
      setExpandedStoryNovelId(null);
    }
  }

  function handleToggleExpandedStory(novelId: string) {
    setExpandedStoryNovelId((current) => (current === novelId ? null : novelId));
  }

  return (
    <section className={`panel library-panel ${isMobileDownloadTab ? "library-panel-download" : ""}`}>
      <div className="panel-header">
        <h2>{isMobileDownloadTab ? "取得" : "Library"}</h2>
        <div className="panel-header-actions">
          <p>
            {isMobileDownloadTab
              ? "URLから追加し、停止した取得を再開します"
              : libraryFilterQuery.trim().length > 0
                ? `${filteredNovelsCount} / ${novelsCount} 作品`
                : `${novelsCount} 作品`}
          </p>
          {isMobileDownloadTab ? null : (
            <div className="library-header-buttons">
              <StorageUsagePopover selectedNovelId={selectedNovelId} />
              <button
                className={`library-export-button ${isLibraryExporting ? "is-exporting" : ""}`}
                disabled={isLibraryExporting || novelsCount === 0}
                onClick={() => void onExportLibrary()}
                type="button"
              >
                {isLibraryExporting ? "出力中..." : "エクスポート"}
              </button>
              <button
                aria-expanded={isDownloadComposerOpen}
                aria-label="小説を追加"
                className="library-add-button"
                onClick={onToggleDownloadComposer}
                title="小説を追加"
                type="button"
              >
                +
              </button>
            </div>
          )}
        </div>
      </div>
      {shouldShowDownloadComposer ? (
        <div
          className={`library-download-composer ${isDownloadDropActive ? "drag-active" : ""}`}
          onDragEnterCapture={onDownloadDragEnter}
          onDragLeaveCapture={onDownloadDragLeave}
          onDragOverCapture={onDownloadDragOver}
          onDropCapture={onDownloadDrop}
        >
          <form className="download-form inline" onSubmit={(event) => void onDownloadSubmit(event)}>
            <label className="download-form-field">
              <span>対象</span>
              <input
                onChange={(event) => onDownloadTargetChange(event.target.value)}
                placeholder="Nコード、作品URL、またはここへ URL をドロップ"
                type="text"
                value={downloadTarget}
              />
            </label>
            <div className="download-form-options">
              <label className="download-form-checkbox">
                <input checked={downloadForce} onChange={(event) => onDownloadForceChange(event.target.checked)} type="checkbox" />
                <span>既存分も再取得</span>
              </label>
            </div>
            <div className="download-form-actions">
              {isMobileDownloadTab ? null : (
                <button className="download-cancel-button" onClick={onCloseDownloadComposer} type="button">
                  閉じる
                </button>
              )}
              <button disabled={isDownloadSubmitting || downloadTarget.trim().length === 0} type="submit">
                {isDownloadSubmitting ? "開始中..." : "ダウンロード"}
              </button>
            </div>
          </form>
          <p className="download-drop-hint">URL のドラッグ＆ドロップにも対応します。</p>
        </div>
      ) : null}
      {libraryNotice ? <p className="message">{libraryNotice}</p> : null}
      {shouldShowQueueSection ? (
        <section className="library-queue-section" id="library-queue-progress">
          <div className="panel-header compact library-queue-header">
            <div>
              <h3>ダウンロード進捗</h3>
              <p>
                {fetcherStatusError
                  ? fetcherStatusError
                  : hasActiveFetcherTasks
                    ? `${activeFetcherTasksCount} 件のタスクを監視中`
                    : "待機中のタスクはありません。"}
              </p>
            </div>
            <div className="library-queue-header-meta">
              <span className={`queue-chip ${fetcherQueueRunning ? "active" : ""}`}>{queueStatusLabel}</span>
              <span>{fetcherStatusCheckedAt ? formatDate(fetcherStatusCheckedAt) : "確認中"}</span>
            </div>
          </div>
          {hasActiveFetcherTasks ? (
            <div className="library-queue-list">
              {activeFetcherTaskEntries.map(({ key, task }) => (
                <article className="library-queue-card" key={key}>
                  <div className="library-queue-card-heading">
                    <div className="library-queue-card-copy">
                      <strong>{getFetcherTaskTargetLabel(task) ?? "対象未取得"}</strong>
                      <p>{task.novelAuthor ?? task.message ?? "メッセージなし"}</p>
                    </div>
                    <div className="library-queue-card-badges">
                      <span className={`queue-task-badge type-${task.type}`}>{getFetcherTaskTypeLabel(task.type)}</span>
                      <span className={`queue-task-badge status-${task.status}`}>{getFetcherTaskStatusLabel(task.status)}</span>
                      {task.status === "running" || task.status === "queued" ? (
                        <button
                          className="queue-task-cancel-button"
                          disabled={cancelingFetcherTaskIds.has(task.id)}
                          onClick={() => void onCancelFetcherTask(task.id)}
                          type="button"
                        >
                          {cancelingFetcherTaskIds.has(task.id) ? "中止中..." : "中止"}
                        </button>
                      ) : null}
                    </div>
                  </div>
                  <div className="library-queue-progress">
                    <div className="library-queue-progress-copy">
                      <strong>{formatFetcherTaskProgress(task)}</strong>
                      <span>{formatFetcherTaskStepProgress(task) ?? "話数進捗は未取得"}</span>
                    </div>
                    <div
                      aria-hidden="true"
                      className={`library-queue-progress-bar ${hasFetcherTaskDeterminateProgress(task) ? "" : "indeterminate"}`}
                    >
                      <span style={{ width: `${getFetcherTaskProgressValue(task)}%` }} />
                    </div>
                  </div>
                  <div className="library-queue-card-meta">
                    {task.novelId ? <span>ID: {task.novelId}</span> : null}
                    {task.target && task.target !== task.novelId ? <span>対象: {task.target}</span> : null}
                    {task.message ? <span>{task.message}</span> : null}
                    {task.errorMessage ? <span>理由: {formatFetcherTaskFailure(task)}</span> : null}
                    <span>受付: {formatDate(task.createdAt)}</span>
                    {task.startedAt ? <span>開始: {formatDate(task.startedAt)}</span> : null}
                  </div>
                </article>
              ))}
            </div>
          ) : (
            <p className="message">現在表示できる進行中タスクはありません。</p>
          )}
        </section>
      ) : null}
      {isMobileDownloadTab ? (
        <>
          <section className="library-maintenance-section">
            <div className="panel-header compact">
              <div>
                <h3>保存済み作品の更新</h3>
                <p>{hasUpdatableNovels ? `${updatableNovels.length} 作品` : "更新できる作品はありません"}</p>
              </div>
            </div>
            {hasUpdatableNovels ? (
              <div className="library-maintenance-list">
                {updatableNovels.map((novel) => (
                  <article className="library-maintenance-row" key={novel.novelId}>
                    <div>
                      <strong>{novel.title}</strong>
                      <span>追加話や目次の変更を確認します。既存話は変更がない限りスキップします。</span>
                    </div>
                    <button
                      disabled={updatingNovelIds.has(novel.novelId) || !novel.fetcherWorkId}
                      onClick={() => void onUpdateNovel(novel.novelId)}
                      type="button"
                    >
                      {updatingNovelIds.has(novel.novelId) ? "更新中..." : "更新"}
                    </button>
                  </article>
                ))}
              </div>
            ) : (
              <p className="message">保存済みの作品があると、追加話の確認をここから開始できます。</p>
            )}
          </section>
          <section className="library-maintenance-section">
            <div className="panel-header compact">
              <div>
                <h3>再開待ち</h3>
                <p>{hasResumableNovels ? `${resumableNovels.length} 作品` : "中断中の作品はありません"}</p>
              </div>
            </div>
            {hasResumableNovels ? (
              <div className="library-maintenance-list">
                {resumableNovels.map((novel) => (
                  <article className="library-maintenance-row" key={novel.novelId}>
                    <div>
                      <strong>{novel.title}</strong>
                      <span>{formatFetchStatusDetail(novel)}</span>
                    </div>
                    <button
                      disabled={resumingNovelIds.has(novel.novelId) || !canResumeNovel(novel)}
                      onClick={() => void onResumeNovel(novel.novelId)}
                      type="button"
                    >
                      {resumingNovelIds.has(novel.novelId) ? "再開中..." : "再開"}
                    </button>
                  </article>
                ))}
              </div>
            ) : (
              <p className="message">失敗や中断がある作品は、ここからすぐ再開できます。</p>
            )}
          </section>
        </>
      ) : null}
      {shouldShowLibraryList ? (
        <>
          <div className="library-filter-bar">
            <label className="library-filter-field">
              <span>絞り込み</span>
              <input
                aria-label="ライブラリ絞り込み"
                onChange={(event) => onLibraryFilterQueryChange(event.target.value)}
                placeholder="作品名・作者名・サイト名で絞り込み"
                type="search"
                value={libraryFilterQuery}
              />
            </label>
            {libraryFilterQuery.trim().length > 0 ? (
              <button className="filter-clear-button" onClick={onClearLibraryFilter} type="button">
                クリア
              </button>
            ) : null}
          </div>
          <ListPaginationControls
            currentPage={libraryPagination.currentPage}
            endItemNumber={libraryPagination.endItemNumber}
            label="ライブラリ一覧"
            onPageChange={onLibraryPageChange}
            startItemNumber={libraryPagination.startItemNumber}
            totalItems={libraryPagination.totalItems}
            totalPages={libraryPagination.totalPages}
          />
          <div className="library-list">
            {filteredNovelsCount === 0 ? (
              <div className="library-empty-card">
                <strong>{novelsCount === 0 ? "まだ作品がありません" : "条件に一致する作品がありません"}</strong>
                <p>
                  {novelsCount === 0
                    ? "`＋` から Nコードや作品 URL を追加できます。保存済みデータが入ると、ここに一覧が表示されます。"
                    : "絞り込み条件を変えると、別の作品も表示できます。"}
                </p>
              </div>
            ) : (
              visibleLibraryNovels.map((novel) => {
              const normalizedStory = normalizeStoryText(novel.story ?? "");
              const storyPreview = createStoryPreview(normalizedStory, LIBRARY_STORY_PREVIEW_LENGTH);
              const isStoryOpen = showStoryAction && openStoryNovelId === novel.novelId;
              const isStoryExpanded = expandedStoryNovelId === novel.novelId;
              const hasStory = normalizedStory.length > 0;
              const hasCover = Boolean(novel.publicationCoverImageUrl);
              const hasGoogleBooksCover = isGoogleBooksCover(novel);
              const storyText = hasStory
                ? isStoryExpanded
                  ? normalizedStory
                  : storyPreview.text
                : "あらすじはありません。";

              const cardTextContents = (
                <>
                  <span className="library-card-title-row">
                    <strong>{novel.title}</strong>
                    <span className="library-site">{novel.siteName}</span>
                  </span>
                  <span className="library-card-author">{novel.author || "著者未設定"}</span>
                  <span className="library-card-chip reading">既読: {formatNovelLastReadLabel(novel)}</span>
                  <span className="library-card-meta">
                    <span className="library-card-chip compact episodes">話数: {novel.totalEpisodes}</span>
                    {novel.fetchStatus && novel.fetchStatus !== "complete" ? (
                      <span className={`library-card-chip compact fetch-${novel.fetchStatus}`}>
                        {formatFetchStatusLabel(novel)}
                      </span>
                    ) : null}
                    <span className="library-card-chip compact bookmarks">栞: {novel.bookmarkCount}</span>
                  </span>
                  <span className="library-card-updated">更新: {formatDate(novel.updatedAt)}</span>
                  {novel.fetchStatus && novel.fetchStatus !== "complete" ? (
                    <span className="library-fetch-status">
                      {formatFetchStatusDetail(novel)}
                    </span>
                  ) : null}
                </>
              );
              const cardContents = (
                <>
                  {novel.publicationCoverImageUrl ? (
                    <span className="library-card-cover" aria-hidden="true">
                      <img alt="" src={novel.publicationCoverImageUrl} />
                    </span>
                  ) : null}
                  <span className="library-card-main">{cardTextContents}</span>
                </>
              );
              const coverSourceLink = hasGoogleBooksCover ? (
                <a
                  aria-label={`${novel.title} ${formatPublicationKindLabel(novel.publicationCoverKind)}を Google Books で見る`}
                  className="library-card-cover-source-link"
                  href={novel.publicationCoverSourceUrl}
                  onClick={(event) => event.stopPropagation()}
                  rel="noreferrer"
                  target="_blank"
                  title="Google Books で見る"
                >
                  ⧉
                </a>
              ) : null;

              if (!showStoryAction) {
                if (coverSourceLink) {
                  return (
                    <article
                      key={novel.novelId}
                      className={`library-card ${hasCover ? "has-cover" : ""} ${novel.novelId === selectedNovelId ? "selected" : ""}`}
                      data-novel-id={novel.novelId}
                    >
                      <button
                        className={`library-card-primary-button ${hasCover ? "has-cover" : ""}`}
                        onClick={() => onSelectNovel(novel.novelId)}
                        type="button"
                      >
                        {cardContents}
                      </button>
                      {coverSourceLink}
                    </article>
                  );
                }
                return (
                  <button
                    key={novel.novelId}
                    className={`library-card ${hasCover ? "has-cover" : ""} ${novel.novelId === selectedNovelId ? "selected" : ""}`}
                    data-novel-id={novel.novelId}
                    onClick={() => onSelectNovel(novel.novelId)}
                    type="button"
                  >
                    {cardContents}
                  </button>
                );
              }

              return (
                <article
                  key={novel.novelId}
                  className={`library-card ${hasCover ? "has-cover" : ""} ${novel.novelId === selectedNovelId ? "selected" : ""}`}
                  data-novel-id={novel.novelId}
                >
                  <button
                    className={`library-card-primary-button ${hasCover ? "has-cover" : ""}`}
                    onClick={() => onSelectNovel(novel.novelId)}
                    type="button"
                  >
                    {cardContents}
                  </button>
                  {coverSourceLink}
                  <div className="library-card-actions">
                    {onOpenNovelPublications ? (
                      <button
                        aria-label={`${novel.title} の書籍情報を開く`}
                        className="summary-link-button library-card-publication-button"
                        onClick={() => onOpenNovelPublications(novel.novelId)}
                        type="button"
                      >
                        書籍情報
                      </button>
                    ) : null}
                    {novel.resumeEpisodeId ? (
                      <button
                        aria-label={`${novel.title} の未取得話を再開`}
                        className="summary-link-button"
                        disabled={resumingNovelIds.has(novel.novelId)}
                        onClick={() => void onResumeNovel(novel.novelId)}
                        title="保存済みの話は再取得せず、未取得または失敗した話から取得します。既存話も取り直す場合は更新を使います。"
                        type="button"
                      >
                        {resumingNovelIds.has(novel.novelId) ? "再開中..." : "再開"}
                      </button>
                    ) : null}
                    <button
                      aria-expanded={isStoryOpen}
                      className="summary-link-button library-card-story-button"
                      onClick={() => handleToggleStory(novel.novelId)}
                      type="button"
                    >
                      {isStoryOpen ? "あらすじを閉じる" : "あらすじ"}
                    </button>
                  </div>
                  {isStoryOpen ? (
                    <div className="library-card-summary novel-summary-story">
                      <p>{storyText}</p>
                      {hasStory && storyPreview.isTruncated ? (
                        <button
                          aria-expanded={isStoryExpanded}
                          className="summary-link-button"
                          onClick={() => handleToggleExpandedStory(novel.novelId)}
                          type="button"
                        >
                          {isStoryExpanded ? "折りたたみ" : "続きを読む"}
                        </button>
                      ) : null}
                    </div>
                  ) : null}
                </article>
              );
              })
            )}
          </div>
          {googleBooksCoverCredit}
        </>
      ) : null}
    </section>
  );
}
