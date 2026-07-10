import { ReaderFloatingPanel } from "./ReaderFloatingPanel";
import type { TermCategory, TermsResponse } from "./features/terms/types";

type Props = {
  data: TermsResponse | null;
  error: string | null;
  formatEpisodeOrderLabel: (episodeIndex: string) => string;
  isLoading: boolean;
  notice: string | null;
  onClose: () => void;
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
  data,
  error,
  formatEpisodeOrderLabel,
  isLoading,
  notice,
  onClose,
}: Props) {
  const displayedBoundary =
    data?.processedUpToEpisodeIndex ?? data?.upToEpisodeIndex ?? null;
  return (
    <ReaderFloatingPanel
      ariaLabel="用語一覧"
      bodyClassName="reader-term-panel-body"
      className="reader-term-panel reader-overlay-panel--character"
      description={
        displayedBoundary
          ? `第${formatEpisodeOrderLabel(displayedBoundary)}話時点の用語です。`
          : "抽出済みの用語を表示します。"
      }
      onClose={onClose}
      title="用語一覧"
    >
      {error ? <p className="message error">{error}</p> : null}
      {notice ? <p className="message">{notice}</p> : null}
      <p
        aria-live="polite"
        className={`reader-character-status${isLoading ? " is-visible" : ""}`}
      >
        <span>{isLoading ? "用語情報を読み込み中..." : "読み込み完了"}</span>
      </p>
      {data?.status === "ready" ? (
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
          用語一覧はまだ生成されていません。
        </p>
      )}
    </ReaderFloatingPanel>
  );
}
