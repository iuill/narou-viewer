import { ReaderFloatingPanel } from "./ReaderFloatingPanel";
import {
  DEFAULT_READER_EXPERIMENTAL_FONT_ID,
  READER_EXPERIMENTAL_FONT_OPTIONS,
  READER_EXPERIMENTAL_FONT_WEIGHT_OPTIONS,
  getReaderExperimentalFontOption,
  isReaderExperimentalFontWeight,
  isReaderExperimentalFontId,
  resolveReaderExperimentalFontWeight,
  type ReaderExperimentalFontId
} from "./readerExperimentalFonts";
import type { ReaderExperimentalFontWeight } from "./readerExperimentalFonts";

type Props = {
  loadStatus: "idle" | "loading" | "ready" | "error";
  onClose: () => void;
  onReaderExperimentalFontChange: (fontId: ReaderExperimentalFontId) => void;
  onReaderExperimentalFontWeightChange: (fontWeight: ReaderExperimentalFontWeight) => void;
  onRetryRemoteFontLoad: () => void;
  previewFontFamilyCss: string;
  previewFontWeight: ReaderExperimentalFontWeight | null;
  readerExperimentalFontId: ReaderExperimentalFontId;
  readerExperimentalFontWeight: ReaderExperimentalFontWeight;
};

const PREVIEW_TEXT =
  "春はあけぼの。やうやう白くなりゆく山ぎは、少しあかりて、紫だちたる雲の細くたなびきたる。";

function getLoadStatusLabel(loadStatus: Props["loadStatus"], usesRemoteStylesheet: boolean): string {
  if (!usesRemoteStylesheet) {
    return "端末ローカルフォントを使用";
  }

  if (loadStatus === "loading") {
    return "Google Fonts を読み込み中";
  }

  if (loadStatus === "ready") {
    return "Google Fonts 読み込み済み";
  }

  if (loadStatus === "error") {
    return "Google Fonts の読み込み失敗。既定フォントへフォールバック中";
  }

  return "Google Fonts を必要時のみ読み込み";
}

function formatOptionLabel(label: string, familyKind: string): string {
  return familyKind === "既定" ? label : `${familyKind} | ${label}`;
}

function getFamilyKindClassName(familyKind: string): string {
  if (familyKind === "明朝系") {
    return "kind-mincho";
  }

  if (familyKind === "ゴシック系") {
    return "kind-gothic";
  }

  return "kind-default";
}

function formatWeightLabel(weight: ReaderExperimentalFontWeight): string {
  const option = READER_EXPERIMENTAL_FONT_WEIGHT_OPTIONS.find((candidate) => candidate.value === weight);
  return option ? `${option.label} (${option.value})` : String(weight);
}

function formatAppliedWeightLabel(weight: ReaderExperimentalFontWeight | null): string {
  return weight === null ? "標準設定" : formatWeightLabel(weight);
}

export function ReaderExperimentalFontPanel({
  loadStatus,
  onClose,
  onReaderExperimentalFontChange,
  onReaderExperimentalFontWeightChange,
  onRetryRemoteFontLoad,
  previewFontFamilyCss,
  previewFontWeight,
  readerExperimentalFontId,
  readerExperimentalFontWeight
}: Props) {
  const selectedOption = getReaderExperimentalFontOption(readerExperimentalFontId);
  const resolvedWeight = resolveReaderExperimentalFontWeight(readerExperimentalFontId, readerExperimentalFontWeight);
  const loadStatusLabel = getLoadStatusLabel(loadStatus, selectedOption.stylesheetHref !== null);
  const usesDefaultReaderFont = readerExperimentalFontId === DEFAULT_READER_EXPERIMENTAL_FONT_ID;
  const supportedWeightLabel = selectedOption.supportedWeights
    ? selectedOption.supportedWeights.map((weight) => formatWeightLabel(weight)).join(" / ")
    : usesDefaultReaderFont
      ? "読書設定に従う"
      : "端末依存";
  const isWeightFallbackActive =
    !usesDefaultReaderFont && selectedOption.supportedWeights !== null && resolvedWeight !== readerExperimentalFontWeight;

  function handleFontChange(value: string) {
    onReaderExperimentalFontChange(isReaderExperimentalFontId(value) ? value : "none");
  }

  function handleFontWeightChange(value: string) {
    const parsed = Number(value);
    onReaderExperimentalFontWeightChange(isReaderExperimentalFontWeight(parsed) ? parsed : 400);
  }

  return (
    <ReaderFloatingPanel
      className="reader-experimental-font-panel reader-overlay-panel--settings"
      description="端末ローカル保存の比較用フォントです。ここで選んだフォントは読書設定の明朝 / ゴシックより優先します。"
      onClose={onClose}
      title="実験フォント"
    >
      <div className="reader-settings-sections">
        <section className="reader-panel-card reader-panel-card--hero reader-experimental-font-preview">
          <p className="reader-panel-section-label">見本</p>
          <p
            className="reader-experimental-font-preview-text"
            style={{ fontFamily: previewFontFamilyCss, fontWeight: previewFontWeight ?? undefined }}
          >
            {PREVIEW_TEXT}
          </p>
        </section>
        <section className="reader-panel-card reader-panel-card--compact reader-settings-section">
          <p className="reader-panel-section-label">候補</p>
          <p className="reader-panel-section-description">
            iPhone / iPad のローカルフォントと Google Fonts の日本語書体を切り替えて、本文の見え心地を比較できます。
          </p>
          <label className="reader-settings-field">
            <span>実験フォント</span>
            <select onChange={(event) => handleFontChange(event.target.value)} value={readerExperimentalFontId}>
              {READER_EXPERIMENTAL_FONT_OPTIONS.map((option) => (
                <option key={option.id} value={option.id}>
                  {formatOptionLabel(option.label, option.familyKind)}
                </option>
              ))}
            </select>
          </label>
          <label className="reader-settings-field">
            <span>文字の太さ</span>
            <select
              disabled={usesDefaultReaderFont}
              onChange={(event) => handleFontWeightChange(event.target.value)}
              value={String(readerExperimentalFontWeight)}
            >
              {READER_EXPERIMENTAL_FONT_WEIGHT_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label} ({option.value})
                </option>
              ))}
            </select>
          </label>
          <div className="reader-panel-chip-row">
            <span
              className={`reader-panel-chip reader-experimental-font-kind ${getFamilyKindClassName(selectedOption.familyKind)}`}
            >
              {selectedOption.familyKind}
            </span>
            <span className="reader-panel-chip">{selectedOption.sourceLabel}</span>
            <span className={`reader-panel-chip reader-experimental-font-status status-${loadStatus}`}>
              {loadStatusLabel}
            </span>
          </div>
          <div className="reader-panel-chip-row">
            <span className="reader-panel-chip">指定 {formatWeightLabel(readerExperimentalFontWeight)}</span>
            <span className="reader-panel-chip">対応 {supportedWeightLabel}</span>
            <span className="reader-panel-chip">適用 {formatAppliedWeightLabel(previewFontWeight)}</span>
          </div>
          <p className="reader-experimental-font-note">{selectedOption.description}</p>
          {usesDefaultReaderFont ? (
            <p className="reader-experimental-font-note">標準設定を使う間は、ここで選んだ太さは本文へ適用しません。</p>
          ) : null}
          {isWeightFallbackActive ? (
            <p className="reader-experimental-font-note">
              この書体は指定した太さに未対応のため、最も近い {formatWeightLabel(resolvedWeight)} で表示します。
            </p>
          ) : null}
          {loadStatus === "error" && selectedOption.stylesheetHref ? (
            <button className="reader-panel-link reader-panel-link-button" onClick={onRetryRemoteFontLoad} type="button">
              Google Fonts を再読み込み
            </button>
          ) : null}
        </section>
      </div>
    </ReaderFloatingPanel>
  );
}
