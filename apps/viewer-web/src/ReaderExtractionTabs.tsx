type ExtractionListView = "characters" | "terms";

type Props = {
  activeView: ExtractionListView;
  onChange: (view: ExtractionListView) => void;
};

export function ReaderExtractionTabs({ activeView, onChange }: Props) {
  return (
    <fieldset className="reader-extraction-tabs">
      <legend>表示する一覧</legend>
      <button
        aria-pressed={activeView === "characters"}
        className={activeView === "characters" ? "is-active" : undefined}
        onClick={() => onChange("characters")}
        type="button"
      >
        人物
      </button>
      <button
        aria-pressed={activeView === "terms"}
        className={activeView === "terms" ? "is-active" : undefined}
        onClick={() => onChange("terms")}
        type="button"
      >
        用語
      </button>
    </fieldset>
  );
}
