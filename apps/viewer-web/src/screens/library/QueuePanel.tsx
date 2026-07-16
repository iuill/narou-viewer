import type { Dispatch, RefObject, SetStateAction } from "react";
import {
  type FetcherTask,
  formatFetcherTask,
  formatFetcherTaskFailure,
  formatFetcherTaskProgress,
  formatFetcherTaskStepProgress,
  getFetcherTaskProgressValue,
  getFetcherTaskTargetLabel,
  hasFetcherTaskDeterminateProgress
} from "../../features/fetcher/model";
import type { FetcherQueueResponse } from "../../features/fetcher/types";

export type FetcherTaskListEntry = {
  key: string;
  task: FetcherTask;
};

export type QueuePanelProps = {
  currentFetcherTask: FetcherTask | null;
  fetcherQueue: FetcherQueueResponse | null;
  fetcherStatusCheckedAt: string | null;
  fetcherStatusError: string | null;
  fetcherTasksFailedCount: number;
  fetcherTasksPausedCount?: number;
  fetcherTasksInterruptedCount?: number;
  fetcherUpdateNotice: string | null;
  hasActiveFetcherTasks: boolean;
  hasFetcherStatus: boolean;
  isOpen: boolean;
  onScrollToQueueProgress: () => void;
  panelRef: RefObject<HTMLDivElement | null>;
  queuedTaskPreviewEntries: FetcherTaskListEntry[];
  pausedFetcherTaskPreviewEntries?: FetcherTaskListEntry[];
  interruptedFetcherTaskPreviewEntries?: FetcherTaskListEntry[];
  queueStatusLabel: string;
  recentFailedFetcherTaskPreviewEntries: FetcherTaskListEntry[];
  setIsOpen: Dispatch<SetStateAction<boolean>>;
};

export type QueuePanelComponentProps = {
  formatDate: (value: string | null) => string;
  onToggle: () => void;
  queue: QueuePanelProps;
};

export function QueuePanel({ formatDate, onToggle, queue }: QueuePanelComponentProps) {
  const {
    currentFetcherTask,
    fetcherQueue,
    fetcherStatusCheckedAt,
    fetcherStatusError,
    fetcherTasksFailedCount,
    fetcherTasksPausedCount = 0,
    fetcherTasksInterruptedCount = 0,
    hasActiveFetcherTasks,
    hasFetcherStatus,
    isOpen,
    onScrollToQueueProgress,
    panelRef,
    queuedTaskPreviewEntries,
    pausedFetcherTaskPreviewEntries = [],
    interruptedFetcherTaskPreviewEntries = [],
    queueStatusLabel,
    recentFailedFetcherTaskPreviewEntries
  } = queue;

  return (
    <div className="queue-panel" ref={panelRef}>
      <button
        aria-expanded={isOpen}
        aria-haspopup="dialog"
        aria-label="取得状況"
        className={`status-trigger queue-trigger ${fetcherQueue?.running ? "active" : ""}`}
        onClick={onToggle}
        type="button"
      >
        <span aria-hidden="true" className="status-trigger-icon queue-trigger-icon" />
        <span className="status-trigger-copy queue-trigger-copy">
          <strong>取得状況</strong>
          <small>{fetcherQueue?.total ?? 0}</small>
        </span>
      </button>
      {isOpen ? (
        <section aria-label="取得状況詳細" className="queue-popover">
          <header className="queue-card-header">
            <div>
              <strong>取得状況</strong>
              <small>{queueStatusLabel}</small>
            </div>
            <span>{fetcherStatusCheckedAt ? formatDate(fetcherStatusCheckedAt) : "確認中"}</span>
          </header>
          <div className="queue-card-metrics">
            <span className={`queue-chip ${fetcherQueue?.running ? "active" : ""}`}>
              実行中: {fetcherQueue?.running ? "yes" : "no"}
            </span>
            <span className="queue-chip">待機: {fetcherQueue?.total ?? 0}</span>
            <span className="queue-chip">worker: {fetcherQueue?.worker ?? 0}</span>
            <span className="queue-chip">web: {fetcherQueue?.webWorker ?? 0}</span>
          </div>
          {fetcherStatusError ? <p className="queue-card-message">{fetcherStatusError}</p> : null}
          {hasFetcherStatus ? (
            <div className="queue-card-body">
              <div className="queue-card-block">
                <strong>現在</strong>
                <p>{formatFetcherTask(currentFetcherTask)}</p>
                {currentFetcherTask ? (
                  <div className="queue-inline-progress">
                    <div
                      aria-hidden="true"
                      className={`queue-inline-progress-bar ${hasFetcherTaskDeterminateProgress(currentFetcherTask) ? "" : "indeterminate"}`}
                    >
                      <span style={{ width: `${getFetcherTaskProgressValue(currentFetcherTask)}%` }} />
                    </div>
                    <div className="queue-inline-progress-copy">
                      <strong>{formatFetcherTaskProgress(currentFetcherTask)}</strong>
                      <span>{formatFetcherTaskStepProgress(currentFetcherTask) ?? (currentFetcherTask.message ?? "進捗情報なし")}</span>
                    </div>
                  </div>
                ) : null}
              </div>
              <div className="queue-card-block">
                <strong>待機列</strong>
                {queuedTaskPreviewEntries.length > 0 ? (
                  <ul className="queue-task-list">
                    {queuedTaskPreviewEntries.map((entry) => (
                      <li key={entry.key}>
                        {formatFetcherTask(entry.task)}
                        {formatFetcherTaskStepProgress(entry.task) ? ` (${formatFetcherTaskStepProgress(entry.task)})` : ""}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p>待機中のタスクはありません。</p>
                )}
              </div>
              <div className="queue-card-block">
                <strong>一時停止・中断</strong>
                <p>
                  一時停止: {fetcherTasksPausedCount} 件 / 中断: {fetcherTasksInterruptedCount} 件
                </p>
                {pausedFetcherTaskPreviewEntries.length > 0 || interruptedFetcherTaskPreviewEntries.length > 0 ? (
                  <ul className="queue-task-list">
                    {pausedFetcherTaskPreviewEntries.map((entry) => (
                      <li key={entry.key}>{formatFetcherTask(entry.task)}</li>
                    ))}
                    {interruptedFetcherTaskPreviewEntries.map((entry) => (
                      <li key={entry.key}>{formatFetcherTask(entry.task)}</li>
                    ))}
                  </ul>
                ) : (
                  <p>再開できるタスクはありません。</p>
                )}
              </div>
              <div className="queue-card-block compact">
                <strong>直近失敗・中止</strong>
                <p>{fetcherTasksFailedCount} 件</p>
                {recentFailedFetcherTaskPreviewEntries.length > 0 ? (
                  <ul className="queue-task-list">
                    {recentFailedFetcherTaskPreviewEntries.map((entry) => (
                      <li key={entry.key}>
                        {getFetcherTaskTargetLabel(entry.task) ?? "対象未取得"}: {formatFetcherTaskFailure(entry.task)}
                        {formatFetcherTaskStepProgress(entry.task) ? ` (${formatFetcherTaskStepProgress(entry.task)})` : ""}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p>失敗履歴はありません。</p>
                )}
              </div>
            </div>
          ) : null}
          {hasActiveFetcherTasks ? (
            <button className="queue-detail-button" onClick={onScrollToQueueProgress} type="button">
              ライブラリで進捗を見る
            </button>
          ) : null}
        </section>
      ) : null}
    </div>
  );
}
