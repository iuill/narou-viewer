import type { ExtractionGenerationStrategy } from "./features/extraction/types";

type Props = {
  canClear: boolean;
  canGenerate: boolean;
  defaultUpToEpisodeIndex: string | null;
  isClearing: boolean;
  isSubmitting: boolean;
  includeCurrentEpisode: boolean;
  onClear: () => void | Promise<void>;
  onIncludeCurrentEpisodeChange: (include: boolean) => void;
  onRequestedGenerationStrategyChange: (strategy: ExtractionGenerationStrategy) => void;
  onRequestedUpToEpisodeIndexChange: (episodeIndex: string) => void;
  onSubmit: () => void | Promise<void>;
  requestedGenerationStrategy: ExtractionGenerationStrategy;
  requestedUpToEpisodeIndex: string;
};

export const EXTRACTION_GENERATION_STRATEGY_LABELS: Record<ExtractionGenerationStrategy, string> = {
  discovery_parallel_correction: "名前発見 + 並列抽出 + 補正",
  parallel_identity: "並列抽出 + 同一人物解決",
  serial: "現行 serial"
};

export function ReaderExtractionControls({
  canClear,
  canGenerate,
  defaultUpToEpisodeIndex,
  isClearing,
  isSubmitting,
  includeCurrentEpisode,
  onClear,
  onIncludeCurrentEpisodeChange,
  onRequestedGenerationStrategyChange,
  onRequestedUpToEpisodeIndexChange,
  onSubmit,
  requestedGenerationStrategy,
  requestedUpToEpisodeIndex
}: Props) {
  return (
    <form
      className="reader-panel-card reader-panel-card--compact reader-character-form"
      onSubmit={(event) => {
        event.preventDefault();
        void onSubmit();
      }}
    >
      <label className="reader-extraction-boundary-toggle">
        <input
          checked={includeCurrentEpisode}
          disabled={isSubmitting}
          onChange={(event) => onIncludeCurrentEpisodeChange(event.target.checked)}
          type="checkbox"
        />
        <span>現在話を含む</span>
      </label>
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
          onChange={(event) => onRequestedGenerationStrategyChange(event.target.value as ExtractionGenerationStrategy)}
          value={requestedGenerationStrategy}
        >
          <option value="parallel_identity">{EXTRACTION_GENERATION_STRATEGY_LABELS.parallel_identity}</option>
          <option value="discovery_parallel_correction">
            {EXTRACTION_GENERATION_STRATEGY_LABELS.discovery_parallel_correction}
          </option>
          <option value="serial">{EXTRACTION_GENERATION_STRATEGY_LABELS.serial}</option>
        </select>
      </label>
      <div className="reader-character-actions">
        <button
          className="reader-character-clear-button"
          disabled={!canClear || isClearing || isSubmitting}
          onClick={() => {
            if (window.confirm("保存済みの人物・用語の抽出データと履歴をクリアします。よろしいですか？")) {
              void onClear();
            }
          }}
          type="button"
        >
          {isClearing ? "クリア中..." : "生成データをクリア"}
        </button>
        <button disabled={!canGenerate || isSubmitting} type="submit">
          {isSubmitting ? "登録中..." : "人物と用語を抽出"}
        </button>
      </div>
    </form>
  );
}
