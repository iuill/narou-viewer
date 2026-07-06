import { useMemo, useState } from "react";
import { formatDate } from "./shared/date";
import { ReaderFloatingPanel } from "./ReaderFloatingPanel";
import type {
  CharacterGenerationStrategy,
  CharacterJobSummary,
  CharacterSummaryEntry,
  CharacterSummaryResponse
} from "./features/characters/types";

type CharacterImportanceCategory = NonNullable<CharacterSummaryEntry["importance"]>["category"];

type CharacterSummaryResponseLike = Pick<
  CharacterSummaryResponse,
  "status" | "upToEpisodeIndex" | "processedUpToEpisodeIndex" | "characters"
>;

type Props = {
  defaultUpToEpisodeIndex: string | null;
  error: string | null;
  notice: string | null;
  formatEpisodeOrderLabel: (episodeIndex: string) => string;
  isLoading: boolean;
  requestedGenerationStrategy: CharacterGenerationStrategy;
  requestedUpToEpisodeIndex: string;
  canGenerate: boolean;
  canClear: boolean;
  isSubmitting: boolean;
  isClearing: boolean;
  data: CharacterSummaryResponseLike | null;
  activeJobs: CharacterJobSummary[];
  completedJobs: CharacterJobSummary[];
  onClose: () => void;
  onClear: () => void | Promise<void>;
  onRequestedGenerationStrategyChange: (strategy: CharacterGenerationStrategy) => void;
  onRequestedUpToEpisodeIndexChange: (episodeIndex: string) => void;
  onSubmit: () => void | Promise<void>;
};

const CHARACTER_CATEGORY_LABELS: Record<CharacterImportanceCategory, string> = {
  main: "メインキャラ",
  regular: "レギュラーキャラ",
  "semi-regular": "准レギュラーキャラ"
};
const CHARACTER_CATEGORY_ORDER: CharacterImportanceCategory[] = ["main", "regular", "semi-regular"];
const CHARACTER_JOB_STAGE_LABELS: Record<string, string> = {
  queued: "待機中",
  running: "実行中",
  preparing: "準備中",
  batch: "batch 生成中",
  batchComplete: "batch 完了",
  completed: "完了",
  failed: "失敗",
  recovered: "再開待ち"
};
const CHARACTER_GENERATION_STRATEGY_LABELS: Record<CharacterGenerationStrategy, string> = {
  discovery_parallel_correction: "名前発見 + 並列抽出 + 補正",
  parallel_identity: "並列抽出 + 同一人物解決",
  serial: "現行 serial"
};

export function ReaderCharacterSummaryPanel({
  defaultUpToEpisodeIndex,
  error,
  notice,
  formatEpisodeOrderLabel,
  isLoading,
  requestedGenerationStrategy,
  requestedUpToEpisodeIndex,
  canGenerate,
  canClear,
  isSubmitting,
  isClearing,
  data,
  activeJobs,
  completedJobs,
  onClose,
  onClear,
  onRequestedGenerationStrategyChange,
  onRequestedUpToEpisodeIndexChange,
  onSubmit
}: Props) {
  const [selectedCategory, setSelectedCategory] = useState<"all" | CharacterImportanceCategory>("all");
  const visibleCharacters = useMemo(() => {
    if (data?.status !== "ready") {
      return [];
    }

    return data.characters.filter((character) =>
      selectedCategory === "all" ? true : character.importance?.category === selectedCategory
    );
  }, [data, selectedCategory]);

  return (
    <ReaderFloatingPanel
      ariaLabel="キャラクター一覧"
      bodyClassName="reader-character-panel-body"
      className="reader-character-panel reader-overlay-panel--character"
      description={
        defaultUpToEpisodeIndex
          ? `第${defaultUpToEpisodeIndex}話時点までの情報を確認します。`
          : "第1話閲覧中のため、キャラクター一覧は生成できません。"
      }
      onClose={onClose}
      title="キャラクター一覧"
    >
      {error ? <p className="message error">{error}</p> : null}
      {notice ? <p className="message">{notice}</p> : null}
      <p aria-live="polite" className={`reader-character-status${isLoading ? " is-visible" : ""}`}>
        <span>{isLoading ? "キャラクター情報を読み込み中..." : "読み込み完了"}</span>
      </p>
      <form
        className="reader-panel-card reader-panel-card--compact reader-character-form"
        onSubmit={(event) => {
          event.preventDefault();
          void onSubmit();
        }}
      >
        <label className="reader-character-field">
          <span>生成対象話数</span>
          <input
            disabled={defaultUpToEpisodeIndex === null}
            inputMode="numeric"
            max={defaultUpToEpisodeIndex ?? undefined}
            min={1}
            onChange={(event) => onRequestedUpToEpisodeIndexChange(event.target.value)}
            type="number"
            value={requestedUpToEpisodeIndex}
          />
        </label>
        <label className="reader-character-field">
          <span>生成方式</span>
          <select
            disabled={defaultUpToEpisodeIndex === null || isSubmitting}
            onChange={(event) => onRequestedGenerationStrategyChange(event.target.value as CharacterGenerationStrategy)}
            value={requestedGenerationStrategy}
          >
            <option value="parallel_identity">{CHARACTER_GENERATION_STRATEGY_LABELS.parallel_identity}</option>
            <option value="discovery_parallel_correction">
              {CHARACTER_GENERATION_STRATEGY_LABELS.discovery_parallel_correction}
            </option>
            <option value="serial">{CHARACTER_GENERATION_STRATEGY_LABELS.serial}</option>
          </select>
        </label>
        <div className="reader-character-actions">
          <button
            className="reader-character-clear-button"
            disabled={!canClear || isClearing || isSubmitting}
            onClick={() => {
              if (window.confirm("保存済みのキャラクター一覧生成データと履歴をクリアします。よろしいですか？")) {
                void onClear();
              }
            }}
            type="button"
          >
            {isClearing ? "クリア中..." : "生成データをクリア"}
          </button>
          <button disabled={!canGenerate || isSubmitting} type="submit">
            {isSubmitting ? "登録中..." : "生成を依頼"}
          </button>
        </div>
      </form>

      {data?.status === "ready" ? (
        data.characters.length > 0 ? (
          <section className="reader-character-list">
            <div className="panel-header compact reader-character-list-header">
              <div>
                <h3>一覧</h3>
                <p>
                  第{formatEpisodeOrderLabel(data.processedUpToEpisodeIndex ?? data.upToEpisodeIndex)}話時点 / {visibleCharacters.length} / {data.characters.length} 人
                </p>
              </div>
              <label className="reader-character-filter">
                <span>カテゴリ</span>
                <select
                  onChange={(event) => setSelectedCategory(event.target.value as "all" | CharacterImportanceCategory)}
                  value={selectedCategory}
                >
                  <option value="all">すべて</option>
                  {CHARACTER_CATEGORY_ORDER.map((category) => (
                    <option key={category} value={category}>
                      {CHARACTER_CATEGORY_LABELS[category]}
                    </option>
                  ))}
                </select>
              </label>
            </div>
            {visibleCharacters.length > 0 ? (
              <div className="reader-character-cards">
                {visibleCharacters.map((character) => (
                  <article className="reader-panel-card reader-character-card" key={character.characterId}>
                    <header>
                      <div className="reader-character-title">
                        <strong>{character.canonicalName}</strong>
                        <span>
                          {character.fullName ?? "フルネーム未確定"}
                          {character.gender ? ` / ${character.gender}` : ""}
                        </span>
                      </div>
                      <span
                        className={`reader-panel-chip reader-character-category${
                          character.importance ? ` is-${character.importance.category}` : ""
                        }`}
                      >
                        {character.importance ? CHARACTER_CATEGORY_LABELS[character.importance.category] : "未分類"}
                      </span>
                    </header>
                    <dl>
                      <div>
                        <dt>初登場</dt>
                        <dd>第{formatEpisodeOrderLabel(character.firstAppearanceEpisodeIndex)}話</dd>
                      </div>
                      <div>
                        <dt>別名</dt>
                        <dd>{character.aliases.length > 0 ? character.aliases.join(" / ") : "なし"}</dd>
                      </div>
                      <div>
                        <dt>重要度</dt>
                        <dd>
                          {character.importance
                            ? `${CHARACTER_CATEGORY_LABELS[character.importance.category]} (${character.importance.score.toFixed(3)})`
                            : "未分類"}
                        </dd>
                      </div>
                      <div>
                        <dt>概要</dt>
                        <dd>{character.summary ?? "未取得"}</dd>
                      </div>
                      <div>
                        <dt>容姿</dt>
                        <dd>{character.appearance ?? "未取得"}</dd>
                      </div>
                      <div>
                        <dt>性格</dt>
                        <dd>{character.personality ?? "未取得"}</dd>
                      </div>
                    </dl>
                  </article>
                ))}
              </div>
            ) : (
              <p className="message">このカテゴリに一致するキャラクターはいません。</p>
            )}
          </section>
        ) : (
          <p className="message">キャラクターは抽出されませんでした。必要なら対象話数を変えて再生成できます。</p>
        )
      ) : (
        <p className="message">
          {defaultUpToEpisodeIndex === null
            ? "第1話ではキャラクター一覧を生成できません。"
            : "まだキャラクター一覧は生成されていません。生成ボタンから依頼できます。"}
        </p>
      )}

      {activeJobs.length ? (
        <section className="reader-character-job-list">
          <div className="panel-header compact">
            <h3>進行中の生成</h3>
            <p>{activeJobs.length} 件</p>
          </div>
          <div className="reader-character-job-items">
            {activeJobs.map((job) => (
              <article className={`reader-panel-card reader-character-job job-${job.status}`} key={job.jobId}>
                <div className="reader-character-job-header">
                  <strong>
                    第{formatEpisodeOrderLabel(job.requestedUpToEpisodeIndex)}話まで / {formatCharacterGenerationStrategy(job.generationStrategy)}
                  </strong>
                  <span>{formatCharacterJobStage(job)}</span>
                </div>
                <CharacterJobProgress job={job} />
                <p>{formatDate(job.createdAt)}</p>
                {job.errorMessage ? <p className="message error">{job.errorMessage}</p> : null}
              </article>
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
              <article className={`reader-panel-card reader-character-job job-${job.status}`} key={job.jobId}>
                <div className="reader-character-job-header">
                  <strong>
                    第{formatEpisodeOrderLabel(job.requestedUpToEpisodeIndex)}話まで / {formatCharacterGenerationStrategy(job.generationStrategy)}
                  </strong>
                  <span>{formatCharacterJobStage(job)}</span>
                </div>
                <CharacterJobProgress job={job} />
                <p>{formatDate(job.createdAt)}</p>
                {job.errorMessage ? <p className="message error">{job.errorMessage}</p> : null}
              </article>
            ))}
          </div>
        </details>
      ) : null}
    </ReaderFloatingPanel>
  );
}

function CharacterJobProgress({ job }: { job: CharacterJobSummary }) {
  const progress = clampProgress(job.progress);
  const batchLabel =
    job.currentBatchIndex && job.batchCount ? `batch ${job.currentBatchIndex}/${job.batchCount}` : null;
  const generatedLabel =
    typeof job.generatedCharacterCount === "number" ? `${job.generatedCharacterCount} 人まで反映` : null;

  if (progress === null && batchLabel === null && generatedLabel === null) {
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
      </p>
    </div>
  );
}

function formatCharacterJobStage(job: CharacterJobSummary): string {
  return CHARACTER_JOB_STAGE_LABELS[job.progressStage ?? job.status] ?? job.progressStage ?? job.status;
}

function formatCharacterGenerationStrategy(strategy: CharacterJobSummary["generationStrategy"]): string {
  if (strategy === "serial" || strategy === "parallel_identity" || strategy === "discovery_parallel_correction") {
    return CHARACTER_GENERATION_STRATEGY_LABELS[strategy];
  }
  return CHARACTER_GENERATION_STRATEGY_LABELS.serial;
}

function clampProgress(value: number | undefined): number | null {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return null;
  }
  return Math.max(0, Math.min(100, Math.round(value)));
}
