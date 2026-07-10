import { ReaderFloatingPanel } from "./ReaderFloatingPanel";
import { ReaderExtractionTabs } from "./ReaderExtractionTabs";
import { ReaderExtractionControls } from "./ReaderExtractionControls";
import { ReaderExtractionJobs } from "./ReaderExtractionJobs";
import type { ExtractionGenerationStrategy, ExtractionJobSummary } from "./features/extraction/types";
import type { TermCategory, TermsResponse } from "./features/terms/types";

type Props = {
  activeJobs: ExtractionJobSummary[];
  canClear: boolean;
  canGenerate: boolean;
  completedJobs: ExtractionJobSummary[];
  data: TermsResponse | null;
  defaultUpToEpisodeIndex: string | null;
  error: string | null;
  formatEpisodeOrderLabel: (episodeIndex: string) => string;
  isClearing: boolean;
  includeCurrentEpisode: boolean;
  isLoading: boolean;
  isSubmitting: boolean;
  notice: string | null;
  onClear: () => void | Promise<void>;
  onIncludeCurrentEpisodeChange: (include: boolean) => void;
  onClose: () => void;
  onRequestedGenerationStrategyChange: (strategy: ExtractionGenerationStrategy) => void;
  onRequestedUpToEpisodeIndexChange: (episodeIndex: string) => void;
  onShowCharacters: () => void | Promise<void>;
  onSubmit: () => void | Promise<void>;
  requestedGenerationStrategy: ExtractionGenerationStrategy;
  requestedUpToEpisodeIndex: string;
};

const CATEGORY_LABELS: Record<TermCategory, string> = {
  organization: "組織",
  place: "場所",
  item: "物品",
  skill: "技能",
  race: "種族",
  event: "出来事",
  other: "その他",
};

export function ReaderTermListPanel({
  activeJobs,
  canClear,
  canGenerate,
  completedJobs,
  data,
  defaultUpToEpisodeIndex,
  error,
  formatEpisodeOrderLabel,
  isClearing,
  includeCurrentEpisode,
  isLoading,
  isSubmitting,
  notice,
  onClear,
  onIncludeCurrentEpisodeChange,
  onClose,
  onRequestedGenerationStrategyChange,
  onRequestedUpToEpisodeIndexChange,
  onShowCharacters,
  onSubmit,
  requestedGenerationStrategy,
  requestedUpToEpisodeIndex,
}: Props) {
  const displayedBoundary =
    data?.processedUpToEpisodeIndex ?? data?.upToEpisodeIndex ?? null;
  return (
    <ReaderFloatingPanel
      ariaLabel="人物・用語一覧"
      bodyClassName="reader-term-panel-body"
      className="reader-term-panel reader-overlay-panel--character"
      description={
        displayedBoundary
          ? `第${formatEpisodeOrderLabel(displayedBoundary)}話時点の用語です。`
          : defaultUpToEpisodeIndex === null && !includeCurrentEpisode
            ? "第1話より前には生成対象がありません。"
          : "抽出済みの用語を表示します。"
      }
      onClose={onClose}
      title="人物・用語一覧"
    >
      {error ? <p className="message error">{error}</p> : null}
      {notice ? <p className="message">{notice}</p> : null}
      <p
        aria-live="polite"
        className={`reader-character-status${isLoading ? " is-visible" : ""}`}
      >
        <span>{isLoading ? "用語情報を読み込み中..." : "読み込み完了"}</span>
      </p>
      <ReaderExtractionControls
        canClear={canClear}
        canGenerate={canGenerate}
        defaultUpToEpisodeIndex={defaultUpToEpisodeIndex}
        isClearing={isClearing}
        isSubmitting={isSubmitting}
        includeCurrentEpisode={includeCurrentEpisode}
        onClear={onClear}
        onIncludeCurrentEpisodeChange={onIncludeCurrentEpisodeChange}
        onRequestedGenerationStrategyChange={onRequestedGenerationStrategyChange}
        onRequestedUpToEpisodeIndexChange={onRequestedUpToEpisodeIndexChange}
        onSubmit={onSubmit}
        requestedGenerationStrategy={requestedGenerationStrategy}
        requestedUpToEpisodeIndex={requestedUpToEpisodeIndex}
      />
      <ReaderExtractionJobs
        activeJobs={activeJobs}
        completedJobs={completedJobs}
        formatEpisodeOrderLabel={formatEpisodeOrderLabel}
      />
      <ReaderExtractionTabs
        activeView="terms"
        onChange={(view) => {
          if (view === "characters") {
            void onShowCharacters();
          }
        }}
      />
      {data?.status === "ready" || data?.status === "partial" ? (
        data.terms.length > 0 ? (
          <div className="reader-term-cards">
            {data.terms.map((term) => (
              <article
                className="reader-panel-card reader-term-card"
                key={term.term}
              >
                <header>
                  <div className="reader-term-title">
                    <strong>{term.term}</strong>
                    {term.reading ? <span>{term.reading}</span> : null}
                  </div>
                  <span
                    className={`reader-panel-chip reader-term-category is-${term.category}`}
                  >
                    {CATEGORY_LABELS[term.category]}
                  </span>
                </header>
                <p>{term.description}</p>
              </article>
            ))}
          </div>
        ) : (
          <p className="reader-panel-card reader-panel-card--compact">
            この話数までに表示できる固有用語はありません。
          </p>
        )
      ) : (
        <p className="reader-panel-card reader-panel-card--compact">
          {defaultUpToEpisodeIndex === null && !includeCurrentEpisode
            ? "「現在話を含む」を有効にすると第1話を生成できます。"
            : "用語一覧はまだ生成されていません。"}
        </p>
      )}
    </ReaderFloatingPanel>
  );
}
