import type { ExtractionJobSummary } from "./features/extraction/types";
import { formatDate } from "./shared/date";
import { EXTRACTION_GENERATION_STRATEGY_LABELS } from "./ReaderExtractionControls";

type Props = {
  activeJobs: ExtractionJobSummary[];
  completedJobs: ExtractionJobSummary[];
  controllingJobId: string | null;
  formatEpisodeOrderLabel: (episodeIndex: string) => string;
  onControlJob: (jobId: string, action: "pause" | "resume" | "cancel") => void | Promise<void>;
};

const EXTRACTION_JOB_STAGE_LABELS: Record<string, string> = {
  queued: "待機中",
  running: "実行中",
  preparing: "準備中",
  batch: "人物・用語一覧を生成中",
  batchComplete: "人物・用語一覧を反映中",
  discovery: "人物候補を発見中",
  completed: "完了",
  failed: "失敗",
  incompatible: "互換性なし・要復旧",
  recovered: "再開待ち",
  pausing: "停止処理中",
  paused: "一時停止",
  interrupted: "中断・再開可能",
  canceled: "取消済み"
};

export function ReaderExtractionJobs({ activeJobs, completedJobs, controllingJobId, formatEpisodeOrderLabel, onControlJob }: Props) {
  return (
    <>
      {activeJobs.length ? (
        <section className="reader-character-job-list">
          <div className="panel-header compact">
            <h3>進行中の生成</h3>
            <p>{activeJobs.length} 件</p>
          </div>
          <div className="reader-character-job-items">
            {activeJobs.map((job) => (
              <ExtractionJobCard controllingJobId={controllingJobId} formatEpisodeOrderLabel={formatEpisodeOrderLabel} job={job} key={job.jobId} onControlJob={onControlJob} />
            ))}
          </div>
        </section>
      ) : null}

      {completedJobs.length ? (
        <details className="reader-panel-card reader-character-job-history">
          <summary>
            <span>過去の生成履歴</span>
            <span>{completedJobs.length} 件</span>
          </summary>
          <div className="reader-character-job-items">
            {completedJobs.map((job) => (
              <ExtractionJobCard controllingJobId={controllingJobId} formatEpisodeOrderLabel={formatEpisodeOrderLabel} job={job} key={job.jobId} onControlJob={onControlJob} />
            ))}
          </div>
        </details>
      ) : null}
    </>
  );
}

function ExtractionJobCard({
  controllingJobId,
  formatEpisodeOrderLabel,
  job,
  onControlJob
}: {
  controllingJobId: string | null;
  formatEpisodeOrderLabel: (episodeIndex: string) => string;
  job: ExtractionJobSummary;
  onControlJob: Props["onControlJob"];
}) {
  return (
    <article className={`reader-panel-card reader-character-job job-${job.status}`}>
      <div className="reader-character-job-header">
        <strong>
          {formatExtractionJobTarget(job.requestedUpToEpisodeIndex, formatEpisodeOrderLabel)} /{" "}
          {formatExtractionGenerationStrategy(job)}
        </strong>
        <span>{formatExtractionJobStage(job)}</span>
      </div>
      <ExtractionJobProgress formatEpisodeOrderLabel={formatEpisodeOrderLabel} job={job} />
      <p>{formatDate(job.createdAt)}</p>
      {job.errorMessage ? <p className="message error">{job.errorMessage}</p> : null}
      {canControlJob(job) ? (
        <div className="reader-character-actions">
          {job.status === "queued" || job.status === "running" ? (
            <button disabled={controllingJobId === job.jobId} onClick={() => void onControlJob(job.jobId, "pause")} type="button">
              一時停止
            </button>
          ) : null}
          {job.status === "paused" || job.status === "interrupted" ? (
            <button disabled={controllingJobId === job.jobId} onClick={() => void onControlJob(job.jobId, "resume")} type="button">
              再開
            </button>
          ) : null}
          <button className="reader-character-clear-button" disabled={controllingJobId === job.jobId} onClick={() => void onControlJob(job.jobId, "cancel")} type="button">
            取消
          </button>
        </div>
      ) : null}
    </article>
  );
}

function canControlJob(job: ExtractionJobSummary): boolean {
  return ["queued", "running", "pausing", "paused", "interrupted"].includes(job.status);
}

function ExtractionJobProgress({
  formatEpisodeOrderLabel,
  job
}: {
  formatEpisodeOrderLabel: (episodeIndex: string) => string;
  job: ExtractionJobSummary;
}) {
  const progress = clampProgress(job.progress);
  const batchStat =
    typeof job.completedBatchCount === "number" && job.batchCount
      ? { value: `${job.completedBatchCount} of ${job.batchCount}`, detail: "完了" }
      : job.currentBatchIndex && job.batchCount
        ? { value: `${job.currentBatchIndex} of ${job.batchCount}`, detail: "実行中" }
        : null;

  const activeWorkers = job.activeWorkers ?? [];
  const progressStats = [
    progress !== null ? { label: "全体", value: `${progress}%`, detail: "" } : null,
    batchStat ? { label: "batch", ...batchStat } : null,
    typeof job.generatedCharacterCount === "number"
      ? { label: "人物", value: String(job.generatedCharacterCount), detail: "反映済" }
      : null,
    typeof job.generatedTermCount === "number"
      ? { label: "用語", value: String(job.generatedTermCount), detail: "反映済" }
      : null
  ].filter((value): value is { label: string; value: string; detail: string } => value !== null);

  if (progressStats.length === 0 && activeWorkers.length === 0) {
    return null;
  }

  return (
    <div className="reader-character-job-progress">
      {progress !== null ? (
        <div
          aria-label={`生成全体進捗 ${progress}%`}
          aria-valuemax={100}
          aria-valuemin={0}
          aria-valuenow={progress}
          className="queue-inline-progress-bar"
          role="progressbar"
        >
          <span style={{ width: `${progress}%` }} />
        </div>
      ) : null}
      {progressStats.length > 0 ? (
        <dl className="reader-extraction-progress-stats">
          {progressStats.map((stat) => (
            <div key={stat.label}>
              <dt>{stat.label}</dt>
              <dd>
                <strong>{stat.value}</strong>
                {stat.detail ? <span>{stat.detail}</span> : null}
              </dd>
            </div>
          ))}
        </dl>
      ) : null}
      {activeWorkers.length > 0 ? (
        <ul aria-label="並列処理中のworker" className="reader-extraction-workers">
          {activeWorkers.map((worker) => {
            const start = formatEpisodeOrderLabel(worker.startEpisodeIndex);
            const end = formatEpisodeOrderLabel(worker.endEpisodeIndex);
            const episodeRange = start === end ? `第${start}話` : `第${start}〜${end}話`;
            const activity = worker.phase === "discovery" ? "人物候補を探索中…" : "人物・用語を抽出中…";
            return (
              <li key={`${worker.workerIndex}-${worker.batchIndex}-${worker.phase}`}>
                <span aria-hidden="true" className="reader-extraction-worker-pulse" />
                <span className="reader-extraction-worker-name">worker {worker.workerIndex}</span>
                <span className="reader-extraction-worker-batch">batch {worker.batchIndex}</span>
                <span className="reader-extraction-worker-activity">
                  {episodeRange} {activity}
                </span>
              </li>
            );
          })}
        </ul>
      ) : null}
    </div>
  );
}

function formatExtractionJobStage(job: ExtractionJobSummary): string {
  return EXTRACTION_JOB_STAGE_LABELS[job.progressStage ?? job.status] ?? job.progressStage ?? job.status;
}

function formatExtractionJobTarget(
  episodeIndex: string,
  formatEpisodeOrderLabel: (episodeIndex: string) => string
): string {
  if (!/^\d+$/.test(episodeIndex)) {
    return "対象範囲不明";
  }
  return `第${formatEpisodeOrderLabel(episodeIndex)}話まで`;
}

function formatExtractionGenerationStrategy(job: ExtractionJobSummary): string {
  const strategy = job.generationStrategy;
  if (strategy === "serial" || strategy === "parallel_identity" || strategy === "discovery_parallel_correction") {
    return EXTRACTION_GENERATION_STRATEGY_LABELS[strategy];
  }
  if (job.status === "incompatible") {
    return "生成方式不明";
  }
  return EXTRACTION_GENERATION_STRATEGY_LABELS.serial;
}

function clampProgress(value: number | undefined): number | null {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return null;
  }
  return Math.max(0, Math.min(100, Math.round(value)));
}
