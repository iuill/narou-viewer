import type { ReaderFontFamily, ReaderTheme, ReadingMode } from "./readerPreferences";
import { ReaderFloatingPanel } from "./ReaderFloatingPanel";

type Props = {
  readingMode: ReadingMode;
  readerFontSizePx: number;
  readerLetterSpacingEm: number;
  reverseTapPageNavigation: boolean;
  debugPageOverflow: boolean;
  quoteNormalizationEnabled: boolean;
  hyphenDashNormalizationEnabled: boolean;
  parenthesisNormalizationEnabled: boolean;
  halfwidthAlnumPunctuationNormalizationEnabled: boolean;
  isReaderCorrectionSaving: boolean;
  readerFontFamily: ReaderFontFamily;
  readerTheme: ReaderTheme;
  onClose: () => void;
  onReadingModeChange: (mode: ReadingMode) => void;
  onReaderFontSizeChange: (fontSizePx: number) => void;
  onReaderLetterSpacingChange: (letterSpacingEm: number) => void;
  onReverseTapPageNavigationChange: (reverseTapPageNavigation: boolean) => void;
  onDebugPageOverflowChange: (debugPageOverflow: boolean) => void;
  onQuoteNormalizationChange: (enabled: boolean) => void;
  onHyphenDashNormalizationChange: (enabled: boolean) => void;
  onParenthesisNormalizationChange: (enabled: boolean) => void;
  onHalfwidthAlnumPunctuationNormalizationChange: (enabled: boolean) => void;
  onReaderFontFamilyChange: (fontFamily: ReaderFontFamily) => void;
  onReaderThemeChange: (theme: ReaderTheme) => void;
  onReset: () => void;
};

const READER_THEME_OPTIONS: Array<{ value: ReaderTheme; label: string }> = [
  { value: "classic", label: "クラシック" },
  { value: "paper", label: "和紙" },
  { value: "forest", label: "森林" },
  { value: "ocean", label: "深海" },
  { value: "midnight", label: "ミッドナイト" }
];
const READER_FONT_SIZE_MIN = 14;
const READER_FONT_SIZE_MAX = 36;
const READER_FONT_SIZE_STEP = 1;
const READER_LETTER_SPACING_MIN = 0;
const READER_LETTER_SPACING_MAX = 0.24;
const READER_LETTER_SPACING_STEP = 0.01;

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

function roundToStep(value: number, digits: number) {
  return Number(value.toFixed(digits));
}

export function ReaderSettingsPanel({
  readingMode,
  readerFontSizePx,
  readerLetterSpacingEm,
  reverseTapPageNavigation,
  debugPageOverflow,
  quoteNormalizationEnabled,
  hyphenDashNormalizationEnabled,
  parenthesisNormalizationEnabled,
  halfwidthAlnumPunctuationNormalizationEnabled,
  isReaderCorrectionSaving,
  readerFontFamily,
  readerTheme,
  onClose,
  onReadingModeChange,
  onReaderFontSizeChange,
  onReaderLetterSpacingChange,
  onReverseTapPageNavigationChange,
  onDebugPageOverflowChange,
  onQuoteNormalizationChange,
  onHyphenDashNormalizationChange,
  onParenthesisNormalizationChange,
  onHalfwidthAlnumPunctuationNormalizationChange,
  onReaderFontFamilyChange,
  onReaderThemeChange,
  onReset
}: Props) {
  const canDecreaseFontSize = readerFontSizePx > READER_FONT_SIZE_MIN;
  const canIncreaseFontSize = readerFontSizePx < READER_FONT_SIZE_MAX;
  const canDecreaseLetterSpacing = readerLetterSpacingEm > READER_LETTER_SPACING_MIN;
  const canIncreaseLetterSpacing = readerLetterSpacingEm < READER_LETTER_SPACING_MAX;

  function handleAdjustFontSize(delta: number) {
    onReaderFontSizeChange(clamp(readerFontSizePx + delta, READER_FONT_SIZE_MIN, READER_FONT_SIZE_MAX));
  }

  function handleAdjustLetterSpacing(delta: number) {
    onReaderLetterSpacingChange(
      roundToStep(
        clamp(readerLetterSpacingEm + delta, READER_LETTER_SPACING_MIN, READER_LETTER_SPACING_MAX),
        2
      )
    );
  }

  return (
    <ReaderFloatingPanel
      className="reader-settings-panel reader-overlay-panel--settings"
      description="本文の見え方と操作方法を調整します。"
      onClose={onClose}
      title="読書設定"
    >
      <div className="reader-settings-sections">
        <section className="reader-panel-card reader-panel-card--compact reader-settings-section">
          <p className="reader-panel-section-label">表示</p>
          <p className="reader-panel-section-description">
            組み方向・フォント・テーマは作品共通、文字サイズと文字間隔は端末ごとに保存します。
          </p>
          <div className="reader-settings-field">
            <span>組み方向</span>
            <div className="mode-toggle">
              <button className={readingMode === "vertical" ? "active" : ""} onClick={() => onReadingModeChange("vertical")} type="button">
                縦書き
              </button>
              <button
                className={readingMode === "horizontal" ? "active" : ""}
                onClick={() => onReadingModeChange("horizontal")}
                type="button"
              >
                横書き
              </button>
            </div>
          </div>
          <label className="reader-settings-field">
            <span>文字サイズ: {readerFontSizePx}px</span>
            <div className="reader-settings-range-control">
              <button
                aria-label="文字サイズを小さくする"
                className="reader-settings-step-button"
                disabled={!canDecreaseFontSize}
                onClick={() => handleAdjustFontSize(-READER_FONT_SIZE_STEP)}
                type="button"
              >
                -
              </button>
              <input
                aria-label={`文字サイズ: ${readerFontSizePx}px`}
                max={READER_FONT_SIZE_MAX}
                min={READER_FONT_SIZE_MIN}
                onChange={(event) => onReaderFontSizeChange(Number.parseInt(event.target.value, 10))}
                step={READER_FONT_SIZE_STEP}
                type="range"
                value={readerFontSizePx}
              />
              <button
                aria-label="文字サイズを大きくする"
                className="reader-settings-step-button"
                disabled={!canIncreaseFontSize}
                onClick={() => handleAdjustFontSize(READER_FONT_SIZE_STEP)}
                type="button"
              >
                +
              </button>
            </div>
          </label>
          <label className="reader-settings-field">
            <span>文字間隔: {readerLetterSpacingEm.toFixed(2)}em</span>
            <div className="reader-settings-range-control">
              <button
                aria-label="文字間隔を狭くする"
                className="reader-settings-step-button"
                disabled={!canDecreaseLetterSpacing}
                onClick={() => handleAdjustLetterSpacing(-READER_LETTER_SPACING_STEP)}
                type="button"
              >
                -
              </button>
              <input
                aria-label={`文字間隔: ${readerLetterSpacingEm.toFixed(2)}em`}
                max={READER_LETTER_SPACING_MAX}
                min={READER_LETTER_SPACING_MIN}
                onChange={(event) => onReaderLetterSpacingChange(Number.parseFloat(event.target.value))}
                step={READER_LETTER_SPACING_STEP}
                type="range"
                value={readerLetterSpacingEm}
              />
              <button
                aria-label="文字間隔を広くする"
                className="reader-settings-step-button"
                disabled={!canIncreaseLetterSpacing}
                onClick={() => handleAdjustLetterSpacing(READER_LETTER_SPACING_STEP)}
                type="button"
              >
                +
              </button>
            </div>
          </label>
          <label className="reader-settings-field">
            <span>フォント</span>
            <select onChange={(event) => onReaderFontFamilyChange(event.target.value as ReaderFontFamily)} value={readerFontFamily}>
              <option value="mincho">明朝</option>
              <option value="gothic">ゴシック</option>
            </select>
          </label>
          <label className="reader-settings-field">
            <span>テーマ</span>
            <select onChange={(event) => onReaderThemeChange(event.target.value as ReaderTheme)} value={readerTheme}>
              {READER_THEME_OPTIONS.map((theme) => (
                <option key={theme.value} value={theme.value}>
                  {theme.label}
                </option>
              ))}
            </select>
          </label>
        </section>
        <section className="reader-panel-card reader-panel-card--compact reader-settings-section">
          <p className="reader-panel-section-label">操作</p>
          <p className="reader-panel-section-description">左右端タップだけを切り替えます。左右スワイプのページ移動方向は変わりません。</p>
          <label className="reader-settings-field">
            <span>左右端タップ</span>
            <select
              onChange={(event) => onReverseTapPageNavigationChange(event.target.value === "reversed")}
              value={reverseTapPageNavigation ? "reversed" : "default"}
            >
              <option value="default">標準</option>
              <option value="reversed">ページ移動を反転</option>
            </select>
          </label>
        </section>
        <section className="reader-panel-card reader-panel-card--compact reader-settings-section">
          <p className="reader-panel-section-label">本文校正</p>
          <p className="reader-panel-section-description">この作品だけに適用します。縦書き・横書き共通で、既読位置に影響しにくい文字数不変の校正から扱います。</p>
          <label className="reader-settings-field">
            <span>引用符を〝〟へ置換</span>
            <select
              disabled={isReaderCorrectionSaving}
              onChange={(event) => onQuoteNormalizationChange(event.target.value === "enabled")}
              value={quoteNormalizationEnabled ? "enabled" : "disabled"}
            >
              <option value="disabled">オフ</option>
              <option value="enabled">オン</option>
            </select>
          </label>
          <label className="reader-settings-field">
            <span>連続ハイフンをダッシュへ置換</span>
            <select
              disabled={isReaderCorrectionSaving}
              onChange={(event) => onHyphenDashNormalizationChange(event.target.value === "enabled")}
              value={hyphenDashNormalizationEnabled ? "enabled" : "disabled"}
            >
              <option value="disabled">オフ</option>
              <option value="enabled">オン</option>
            </select>
          </label>
          <label className="reader-settings-field">
            <span>半角括弧を全角へ置換</span>
            <select
              disabled={isReaderCorrectionSaving}
              onChange={(event) => onParenthesisNormalizationChange(event.target.value === "enabled")}
              value={parenthesisNormalizationEnabled ? "enabled" : "disabled"}
            >
              <option value="disabled">オフ</option>
              <option value="enabled">オン</option>
            </select>
          </label>
          <label className="reader-settings-field">
            <span>半角英数字・!?を全角へ置換</span>
            <select
              disabled={isReaderCorrectionSaving}
              onChange={(event) => onHalfwidthAlnumPunctuationNormalizationChange(event.target.value === "enabled")}
              value={halfwidthAlnumPunctuationNormalizationEnabled ? "enabled" : "disabled"}
            >
              <option value="disabled">オフ</option>
              <option value="enabled">オン</option>
            </select>
          </label>
        </section>
        <section className="reader-panel-card reader-panel-card--compact reader-settings-section">
          <p className="reader-panel-section-label">デバッグ</p>
          <p className="reader-panel-section-description">ページからあふれる列を通常は隠し、確認したい時だけ色付きで残します。</p>
          <label className="reader-settings-field">
            <span>列はみ出し表示</span>
            <select
              onChange={(event) => onDebugPageOverflowChange(event.target.value === "debug")}
              value={debugPageOverflow ? "debug" : "hidden"}
            >
              <option value="hidden">非表示</option>
              <option value="debug">緑で可視化</option>
            </select>
          </label>
        </section>
      </div>
      <div className="reader-settings-actions">
        <button className="reader-settings-reset" disabled={isReaderCorrectionSaving} onClick={onReset} type="button">
          読書設定を初期化
        </button>
      </div>
    </ReaderFloatingPanel>
  );
}
