import {
  formatFetcherTask,
  formatFetcherTaskProgress,
  formatFetcherTaskStepProgress,
  getFetcherTaskProgressValue,
  hasFetcherTaskDeterminateProgress
} from "../../features/fetcher/model";
import { AiGenerationNavList, type AiGenerationMenuProps } from "./AiGenerationMenu";
import type { QueuePanelProps } from "./QueuePanel";
import type { LibraryStatusPanelProps } from "./StatusPanel";

export type MobileStatusPanelProps = {
  aiGeneration: AiGenerationMenuProps;
  queue: QueuePanelProps;
  status: LibraryStatusPanelProps;
};

export function MobileStatusPanel({ aiGeneration, queue, status }: MobileStatusPanelProps) {
  const { formatDate, runtimeStatus, runtimeStatusLabel } = status;
  const {
    currentFetcherTask,
    fetcherQueue,
    fetcherStatusCheckedAt,
    fetcherStatusError,
    fetcherUpdateNotice,
    onScrollToQueueProgress
  } = queue;
  const {
    activeJobsCount,
    failedJobsCount,
    jobsError,
    onOpenView,
    runtimeErrorDetail,
    settingsError,
    summaryLabel
  } = aiGeneration;

  return (
    <section className="panel mobile-status-panel">
      <div className="panel-header">
        <div>
          <h2>状況</h2>
          <p>{runtimeStatusLabel}</p>
        </div>
      </div>
      {fetcherUpdateNotice ? (
        <p className="hero-update-notice mobile-status-update" role="status">
          {fetcherUpdateNotice}
        </p>
      ) : null}
      <div className="mobile-status-section">
        <div className="mobile-status-section-header">
          <strong>動作状況</strong>
          <span>{runtimeStatus ? formatDate(runtimeStatus.checkedAt) : "確認中"}</span>
        </div>
        <div className="status-popover-list mobile-status-list">
          {(runtimeStatus?.services ?? []).map((service) => (
            <article className="status-item" key={service.id}>
              <div className="status-item-heading">
                <strong>{service.label}</strong>
                <span className={`status-badge ${service.status}`}>{service.summary}</span>
              </div>
              <p>{service.detail}</p>
            </article>
          ))}
          {!runtimeStatus ? <p className="message">動作状況を確認中...</p> : null}
        </div>
      </div>
      <div className="mobile-status-section">
        <div className="mobile-status-section-header">
          <strong>取得状況</strong>
          <span>{fetcherStatusCheckedAt ? formatDate(fetcherStatusCheckedAt) : "確認中"}</span>
        </div>
        <div className="queue-card-metrics">
          <span className={`queue-chip ${fetcherQueue?.running ? "active" : ""}`}>
            実行中: {fetcherQueue?.running ? "yes" : "no"}
          </span>
          <span className="queue-chip">待機: {fetcherQueue?.total ?? 0}</span>
          <span className="queue-chip">worker: {fetcherQueue?.worker ?? 0}</span>
          <span className="queue-chip">web: {fetcherQueue?.webWorker ?? 0}</span>
        </div>
        {fetcherStatusError ? <p className="queue-card-message">{fetcherStatusError}</p> : null}
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
        <button className="queue-detail-button" onClick={onScrollToQueueProgress} type="button">
          取得タブで見る
        </button>
      </div>
      <div className="mobile-status-section">
        <div className="mobile-status-section-header">
          <strong>AI機能</strong>
          <span>{summaryLabel}</span>
        </div>
        <div className="queue-card-metrics">
          <span className={`queue-chip ${activeJobsCount > 0 ? "active" : ""}`}>進行中: {activeJobsCount} 件</span>
          <span className="queue-chip">失敗: {failedJobsCount} 件</span>
        </div>
        {settingsError ? <p className="queue-card-message">{settingsError}</p> : null}
        {runtimeErrorDetail ? <p className="queue-card-message">{runtimeErrorDetail}</p> : null}
        {jobsError ? <p className="queue-card-message">{jobsError}</p> : null}
        <AiGenerationNavList onOpenView={onOpenView} />
      </div>
    </section>
  );
}
