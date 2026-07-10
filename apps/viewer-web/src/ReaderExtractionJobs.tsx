import type { ExtractionJobSummary } from "./features/extraction/types";
import { formatDate } from "./shared/date";
import { EXTRACTION_GENERATION_STRATEGY_LABELS } from "./ReaderExtractionControls";

type Props = {
  activeJobs: ExtractionJobSummary[];
  completedJobs: ExtractionJobSummary[];
  formatEpisodeOrderLabel: (episodeIndex: string) => string;
};

const EXTRACTION_JOB_STAGE_LABELS: Record<string, string> = {
  queued: "待機中",
  running: "実行中",
  preparing: "準備中",
  batch: "batch 生成中",
  batchComplete: "batch 完了",
  completed: "完了",
  failed: "失敗",
  recovered: "再開待ち"
};

export function ReaderExtractionJobs({ activeJobs, completedJobs, formatEpisodeOrderLabel }: Props) {
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
              <ExtractionJobCard formatEpisodeOrderLabel={formatEpisodeOrderLabel} job={job} key={job.jobId} />
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
              <ExtractionJobCard formatEpisodeOrderLabel={formatEpisodeOrderLabel} job={job} key={job.jobId} />
            ))}
          </div>
        </details>
      ) : null}
    </>
  );
}

function ExtractionJobCard({
  formatEpisodeOrderLabel,
  job
}: {
  formatEpisodeOrderLabel: (episodeIndex: string) => string;
  job: ExtractionJobSummary;
}) {
  return (
    <article className={`reader-panel-card reader-character-job job-${job.status}`}>
      <div className="reader-character-job-header">
        <strong>
          第{formatEpisodeOrderLabel(job.requestedUpToEpisodeIndex)}話まで / {formatExtractionGenerationStrategy(job.generationStrategy)}
        </strong>
        <span>{formatExtractionJobStage(job)}</span>
      </div>
      <ExtractionJobProgress job={job} />
      <p>{formatDate(job.createdAt)}</p>
      {job.errorMessage ? <p className="message error">{job.errorMessage}</p> : null}
    </article>
  );
}

function ExtractionJobProgress({ job }: { job: ExtractionJobSummary }) {
  const progress = clampProgress(job.progress);
  const batchLabel =
    job.currentBatchIndex && job.batchCount ? `batch ${job.currentBatchIndex}/${job.batchCount}` : null;
  const generatedLabel =
    typeof job.generatedCharacterCount === "number" ? `${job.generatedCharacterCount} 人まで反映` : null;
  const generatedTermLabel = typeof job.generatedTermCount === "number" ? `${job.generatedTermCount} 用語まで反映` : null;

  if (progress === null && batchLabel === null && generatedLabel === null && generatedTermLabel === null) {
    return null;
  }

  return (
    <div className="reader-character-job-progress">
      {progress !== null ? (
        <div
          aria-label={`生成進捗 ${progress}%`}
          aria-valuemax={100}
          aria-valuemin={0}
          aria-valuenow={progress}
          className="queue-inline-progress-bar"
          role="progressbar"
        >
          <span style={{ width: `${progress}%` }} />
        </div>
      ) : null}
      <p>
        {progress !== null ? `${progress}%` : null}
        {batchLabel ? `${progress !== null ? " / " : ""}${batchLabel}` : null}
        {generatedLabel ? `${progress !== null || batchLabel ? " / " : ""}${generatedLabel}` : null}
        {generatedTermLabel
          ? `${progress !== null || batchLabel || generatedLabel ? " / " : ""}${generatedTermLabel}`
          : null}
      </p>
    </div>
  );
}

function formatExtractionJobStage(job: ExtractionJobSummary): string {
  return EXTRACTION_JOB_STAGE_LABELS[job.progressStage ?? job.status] ?? job.progressStage ?? job.status;
}

function formatExtractionGenerationStrategy(strategy: ExtractionJobSummary["generationStrategy"]): string {
  if (strategy === "serial" || strategy === "parallel_identity" || strategy === "discovery_parallel_correction") {
    return EXTRACTION_GENERATION_STRATEGY_LABELS[strategy];
  }
  return EXTRACTION_GENERATION_STRATEGY_LABELS.serial;
}

function clampProgress(value: number | undefined): number | null {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return null;
  }
  return Math.max(0, Math.min(100, Math.round(value)));
}
