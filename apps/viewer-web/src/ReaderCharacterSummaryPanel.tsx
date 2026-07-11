import { useMemo, useState } from "react";
import { ReaderFloatingPanel } from "./ReaderFloatingPanel";
import { ReaderExtractionTabs } from "./ReaderExtractionTabs";
import { ReaderExtractionControls } from "./ReaderExtractionControls";
import { ReaderExtractionJobs } from "./ReaderExtractionJobs";
import type { CharacterSummaryEntry, CharacterSummaryResponse } from "./features/characters/types";
import type { ExtractionGenerationStrategy, ExtractionJobSummary } from "./features/extraction/types";

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
  requestedGenerationStrategy: ExtractionGenerationStrategy;
  requestedUpToEpisodeIndex: string;
  canGenerate: boolean;
  canClear: boolean;
  isSubmitting: boolean;
  isClearing: boolean;
  includeCurrentEpisode: boolean;
  data: CharacterSummaryResponseLike | null;
  activeJobs: ExtractionJobSummary[];
  completedJobs: ExtractionJobSummary[];
  onClose: () => void;
  onShowTerms: () => void | Promise<void>;
  onClear: () => void | Promise<void>;
  onIncludeCurrentEpisodeChange: (include: boolean) => void;
  onRequestedGenerationStrategyChange: (strategy: ExtractionGenerationStrategy) => void;
  onRequestedUpToEpisodeIndexChange: (episodeIndex: string) => void;
  onSubmit: () => void | Promise<void>;
};

const CHARACTER_CATEGORY_LABELS: Record<CharacterImportanceCategory, string> = {
  main: "メインキャラ",
  regular: "レギュラーキャラ",
  "semi-regular": "准レギュラーキャラ"
};
const CHARACTER_CATEGORY_ORDER: CharacterImportanceCategory[] = ["main", "regular", "semi-regular"];
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
  includeCurrentEpisode,
  data,
  activeJobs,
  completedJobs,
  onClose,
  onShowTerms,
  onClear,
  onIncludeCurrentEpisodeChange,
  onRequestedGenerationStrategyChange,
  onRequestedUpToEpisodeIndexChange,
  onSubmit
}: Props) {
  const [selectedCategory, setSelectedCategory] = useState<"all" | CharacterImportanceCategory>("all");
  const displayedBoundary =
    data?.status === "partial"
      ? (data.processedUpToEpisodeIndex ?? data.upToEpisodeIndex)
      : (data?.upToEpisodeIndex ?? null);
  const visibleCharacters = useMemo(() => {
    if (data?.status !== "ready" && data?.status !== "partial") {
      return [];
    }

    return data.characters.filter((character) =>
      selectedCategory === "all" ? true : character.importance?.category === selectedCategory
    );
  }, [data, selectedCategory]);

  return (
    <ReaderFloatingPanel
      ariaLabel="人物・用語一覧"
      bodyClassName="reader-character-panel-body"
      className="reader-character-panel reader-overlay-panel--character"
      description={
        defaultUpToEpisodeIndex
          ? `第${defaultUpToEpisodeIndex}話時点までの情報を確認します。`
          : includeCurrentEpisode
            ? "生成対象の話を確認できません。"
            : "第1話より前には生成対象がありません。"
      }
      onClose={onClose}
      title="人物・用語一覧"
    >
      {error ? <p className="message error">{error}</p> : null}
      {notice ? <p className="message">{notice}</p> : null}
      <p aria-live="polite" className={`reader-character-status${isLoading ? " is-visible" : ""}`}>
        <span>{isLoading ? "人物と用語の抽出情報を読み込み中..." : "読み込み完了"}</span>
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
        activeView="characters"
        onChange={(view) => {
          if (view === "terms") {
            void onShowTerms();
          }
        }}
      />

      {data?.status === "ready" || data?.status === "partial" ? (
        data.characters.length > 0 ? (
          <section className="reader-character-list">
            <div className="panel-header compact reader-extraction-list-header">
              <div>
                <h3>一覧</h3>
                <p className="reader-extraction-list-summary">
                  <span>第{formatEpisodeOrderLabel(displayedBoundary ?? data.upToEpisodeIndex)}話時点</span>
                  <span>人物 {visibleCharacters.length} of {data.characters.length}</span>
                </p>
              </div>
              <label className="reader-extraction-filter">
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
        <p className="reader-panel-card reader-panel-card--compact">
          {defaultUpToEpisodeIndex === null
            ? includeCurrentEpisode
              ? "生成対象の話を確認できません。"
              : "「現在話を含む」を有効にすると第1話を生成できます。"
            : "人物一覧はまだ生成されていません。"}
        </p>
      )}

    </ReaderFloatingPanel>
  );
}
