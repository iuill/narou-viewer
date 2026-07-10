import { formatDate } from "../../shared/date";
import { getAiGenerationModeLabel, getCharacterGenerationStrategyLabel, type AiGenerationJobFilter } from "./model";
import type { AiGenerationJobSummary } from "./types";

export type AiJobsViewProps = {
  aiGenerationJobsError: string | null;
  isAiGenerationJobsLoading: boolean;
  hasAiGenerationJobs: boolean;
  aiGenerationJobFilter: AiGenerationJobFilter;
  onSetAiGenerationJobFilter: (filter: AiGenerationJobFilter) => void;
  aiGenerationActiveJobsCount: number;
  aiGenerationFailedJobsCount: number;
  aiGenerationCompletedJobsCount: number;
  visibleAiGenerationJobs: AiGenerationJobSummary[];
  onOpenNovelFromJob: (novelId: string) => void;
};

export function AiJobsView({
  aiGenerationJobsError,
  isAiGenerationJobsLoading,
  hasAiGenerationJobs,
  aiGenerationJobFilter,
  onSetAiGenerationJobFilter,
  aiGenerationActiveJobsCount,
  aiGenerationFailedJobsCount,
  aiGenerationCompletedJobsCount,
  visibleAiGenerationJobs,
  onOpenNovelFromJob
}: AiJobsViewProps) {
  return <div className="ai-workspace-body">
          {aiGenerationJobsError ? <p className="message error">{aiGenerationJobsError}</p> : null}
          {isAiGenerationJobsLoading && !hasAiGenerationJobs ? <p className="message">キャラ生成履歴を読み込み中...</p> : null}
          <div className="panel-header compact">
            <div>
              <h3>キャラ生成履歴</h3>
              <p>人物と用語の抽出状況と失敗履歴を確認できます。</p>
            </div>
            <div className="mode-toggle ai-job-filter-tabs">
              <button className={aiGenerationJobFilter === "active" ? "active" : ""} onClick={() => onSetAiGenerationJobFilter("active")} type="button">
                進行中 {aiGenerationActiveJobsCount}
              </button>
              <button className={aiGenerationJobFilter === "failed" ? "active" : ""} onClick={() => onSetAiGenerationJobFilter("failed")} type="button">
                失敗 {aiGenerationFailedJobsCount}
              </button>
              <button
                className={aiGenerationJobFilter === "completed" ? "active" : ""}
                onClick={() => onSetAiGenerationJobFilter("completed")}
                type="button"
              >
                完了 {aiGenerationCompletedJobsCount}
              </button>
            </div>
          </div>
          {visibleAiGenerationJobs.length > 0 ? (
            <div className="ai-job-list ai-job-list-limited">
              {visibleAiGenerationJobs.map((job) => (
                <article className="library-queue-card ai-job-card" key={job.jobId}>
                  <div className="library-queue-card-heading">
                    <div className="library-queue-card-copy">
                      <strong>{job.novelTitle ?? job.novelId}</strong>
                      <p>
                        {job.profileLabel ? `${job.profileLabel} / ` : ""}
                        第{job.requestedUpToEpisodeIndex}話まで / {getAiGenerationModeLabel(job.generationMode)} /{" "}
                        {getCharacterGenerationStrategyLabel(job.generationStrategy)}
                        {job.modelId ? ` / ${job.modelId}` : ""}
                      </p>
                    </div>
                    <div className="library-queue-card-badges">
                      <span className={`queue-task-badge status-${job.status}`}>{job.status}</span>
                    </div>
                  </div>
                  <div className="library-queue-card-meta">
                    <span>受付: {formatDate(job.createdAt)}</span>
                    {job.startedAt ? <span>開始: {formatDate(job.startedAt)}</span> : null}
                    {job.finishedAt ? <span>終了: {formatDate(job.finishedAt)}</span> : null}
                  </div>
                  {job.errorMessage ? <p className="message error">{job.errorMessage}</p> : null}
                  <div className="panel-header-actions">
                    <button onClick={() => onOpenNovelFromJob(job.novelId)} type="button">
                      作品を開く
                    </button>
                  </div>
                </article>
              ))}
            </div>
          ) : (
            <p className="message">
              {aiGenerationJobFilter === "active"
                ? "進行中のキャラ生成はありません。"
                : aiGenerationJobFilter === "failed"
                  ? "失敗したキャラ生成はありません。"
                  : "完了済みのキャラ生成はまだありません。"}
            </p>
          )}

  </div>;
}
