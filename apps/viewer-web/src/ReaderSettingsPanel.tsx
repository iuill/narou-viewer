import { useEffect, useState } from "react";
import type { NovelReaderReplacementRule } from "./features/reader/types";
import type { ReaderFontFamily, ReaderTheme, ReadingMode } from "./readerPreferences";
import { ReaderFloatingPanel } from "./ReaderFloatingPanel";
import type { ReaderAIProofreadState } from "./hooks/useReaderAIProofread";

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
  tildeNormalizationEnabled: boolean;
  consecutivePeriodNormalizationEnabled: boolean;
  customReplacements: NovelReaderReplacementRule[];
  isReaderCorrectionSaving: boolean;
  readerAIProofreadState: ReaderAIProofreadState;
  isShowingAIProofread: boolean;
  hasAIProofread: boolean;
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
  onTildeNormalizationChange: (enabled: boolean) => void;
  onConsecutivePeriodNormalizationChange: (enabled: boolean) => void;
  onCustomReplacementsChange: (rules: NovelReaderReplacementRule[]) => void;
  onGenerateAIProofread: () => void;
  onDeleteAIProofread: () => void;
  onShowingAIProofreadChange: (enabled: boolean) => void;
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
const CUSTOM_REPLACEMENT_MAX_RULES = 50;
const CUSTOM_REPLACEMENT_MAX_LENGTH = 100;

type CustomReplacementDraft = NovelReaderReplacementRule & { id: number };
let nextCustomReplacementDraftId = 0;

function createCustomReplacementDraft(rules: NovelReaderReplacementRule[]): CustomReplacementDraft[] {
  return rules.map((rule) => ({ ...rule, id: ++nextCustomReplacementDraftId }));
}

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

function roundToStep(value: number, digits: number) {
  return Number(value.toFixed(digits));
}

type ReaderSettingsSwitchProps = {
  checked: boolean;
  checkedLabel?: string;
  disabled?: boolean;
  label: string;
  onChange: (checked: boolean) => void;
  uncheckedLabel?: string;
};

function ReaderSettingsSwitch({
  checked,
  checkedLabel = "オン",
  disabled = false,
  label,
  onChange,
  uncheckedLabel = "オフ"
}: ReaderSettingsSwitchProps) {
  return (
    <label className="reader-settings-switch-field">
      <span className="reader-settings-switch-label">{label}</span>
      <span className="reader-settings-switch-control">
        <span aria-hidden="true" className="reader-settings-switch-state">
          {checked ? checkedLabel : uncheckedLabel}
        </span>
        <input
          aria-checked={checked}
          checked={checked}
          disabled={disabled}
          onChange={(event) => onChange(event.target.checked)}
          role="switch"
          type="checkbox"
        />
        <span aria-hidden="true" className="reader-settings-switch-track">
          <span className="reader-settings-switch-thumb" />
        </span>
      </span>
    </label>
  );
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
  tildeNormalizationEnabled,
  consecutivePeriodNormalizationEnabled,
  customReplacements,
  isReaderCorrectionSaving,
  readerAIProofreadState,
  isShowingAIProofread,
  hasAIProofread,
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
  onTildeNormalizationChange,
  onConsecutivePeriodNormalizationChange,
  onCustomReplacementsChange,
  onGenerateAIProofread,
  onDeleteAIProofread,
  onShowingAIProofreadChange,
  onReaderFontFamilyChange,
  onReaderThemeChange,
  onReset
}: Props) {
  const canDecreaseFontSize = readerFontSizePx > READER_FONT_SIZE_MIN;
  const canIncreaseFontSize = readerFontSizePx < READER_FONT_SIZE_MAX;
  const canDecreaseLetterSpacing = readerLetterSpacingEm > READER_LETTER_SPACING_MIN;
  const canIncreaseLetterSpacing = readerLetterSpacingEm < READER_LETTER_SPACING_MAX;
  const [customReplacementDraft, setCustomReplacementDraft] = useState<CustomReplacementDraft[]>(() =>
    createCustomReplacementDraft(customReplacements)
  );

  useEffect(() => {
    setCustomReplacementDraft(createCustomReplacementDraft(customReplacements));
  }, [customReplacements]);

  const duplicateFromValues = new Set<string>();
  const seenFromValues = new Set<string>();
  for (const rule of customReplacementDraft) {
    if (seenFromValues.has(rule.from)) {
      duplicateFromValues.add(rule.from);
    }
    seenFromValues.add(rule.from);
  }
  const hasIncompleteCustomReplacement = customReplacementDraft.some((rule) => rule.from.trim() === "");
  const hasInvalidCustomReplacement = customReplacementDraft.some(
    (rule) =>
      rule.from.trim() === "" ||
      Array.from(rule.from).length > CUSTOM_REPLACEMENT_MAX_LENGTH ||
      Array.from(rule.to).length > CUSTOM_REPLACEMENT_MAX_LENGTH ||
      duplicateFromValues.has(rule.from)
  );
  const draftReplacementRules = customReplacementDraft.map(({ from, to }) => ({ from, to }));
  const isCustomReplacementDirty = JSON.stringify(draftReplacementRules) !== JSON.stringify(customReplacements);

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

  function updateCustomReplacement(id: number, field: keyof NovelReaderReplacementRule, value: string) {
    setCustomReplacementDraft((current) =>
      current.map((rule) => (rule.id === id ? { ...rule, [field]: value } : rule))
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
          <ReaderSettingsSwitch
            checked={reverseTapPageNavigation}
            checkedLabel="反転"
            label="左右端タップのページ移動を反転"
            onChange={onReverseTapPageNavigationChange}
            uncheckedLabel="標準"
          />
        </section>
        <section className="reader-panel-card reader-panel-card--compact reader-settings-section">
          <p className="reader-panel-section-label">本文校正</p>
          <p className="reader-panel-section-description">この作品だけに適用します。取得した原文は変更しません。</p>
          <ReaderSettingsSwitch
            checked={quoteNormalizationEnabled}
            disabled={isReaderCorrectionSaving}
            label="引用符を〝〟へ置換"
            onChange={onQuoteNormalizationChange}
          />
          <ReaderSettingsSwitch
            checked={hyphenDashNormalizationEnabled}
            disabled={isReaderCorrectionSaving}
            label="連続ハイフンをダッシュへ置換"
            onChange={onHyphenDashNormalizationChange}
          />
          <ReaderSettingsSwitch
            checked={parenthesisNormalizationEnabled}
            disabled={isReaderCorrectionSaving}
            label="半角括弧を全角へ置換"
            onChange={onParenthesisNormalizationChange}
          />
          <ReaderSettingsSwitch
            checked={halfwidthAlnumPunctuationNormalizationEnabled}
            disabled={isReaderCorrectionSaving}
            label="半角英数字・!?を全角へ置換"
            onChange={onHalfwidthAlnumPunctuationNormalizationChange}
          />
          <ReaderSettingsSwitch
            checked={tildeNormalizationEnabled}
            disabled={isReaderCorrectionSaving}
            label="半角チルダを波ダッシュへ置換"
            onChange={onTildeNormalizationChange}
          />
          <ReaderSettingsSwitch
            checked={consecutivePeriodNormalizationEnabled}
            disabled={isReaderCorrectionSaving}
            label="連続ピリオドを……へ置換"
            onChange={onConsecutivePeriodNormalizationChange}
          />
          <p className="reader-panel-section-description">
            半角ピリオドが2個以上続く箇所を一律で……へ置換するため、表示上の文字数が変わります。
          </p>
          <div className="reader-settings-custom-replacements">
            <div>
              <span className="reader-panel-section-label">単語の置換</span>
              <p className="reader-panel-section-description">
                完全一致する文字列を登録順に置換します。この作品だけに適用され、原文は変更しません。
              </p>
              <div className="reader-panel-chip-row reader-settings-replacement-example">
                <span className="reader-panel-chip">例：表記ゆれ → 統一表記</span>
              </div>
            </div>
            {customReplacementDraft.length > 0 ? (
              <div className="reader-settings-replacement-list">
                {customReplacementDraft.map((rule, index) => (
                  <div className="reader-settings-replacement-row" key={rule.id}>
                    <label>
                      <span>置換前</span>
                      <input
                        aria-label={`置換前 ${index + 1}`}
                        disabled={isReaderCorrectionSaving}
                        maxLength={CUSTOM_REPLACEMENT_MAX_LENGTH}
                        onInput={(event) => updateCustomReplacement(rule.id, "from", event.currentTarget.value)}
                        placeholder="置換する文字列"
                        type="text"
                        value={rule.from}
                      />
                    </label>
                    <span aria-hidden="true" className="reader-settings-replacement-arrow">
                      →
                    </span>
                    <label>
                      <span>置換後</span>
                      <input
                        aria-label={`置換後 ${index + 1}`}
                        disabled={isReaderCorrectionSaving}
                        maxLength={CUSTOM_REPLACEMENT_MAX_LENGTH}
                        onInput={(event) => updateCustomReplacement(rule.id, "to", event.currentTarget.value)}
                        placeholder="置換後の文字列（空欄で削除）"
                        type="text"
                        value={rule.to}
                      />
                    </label>
                    <button
                      aria-label={`置換ルール ${index + 1} を削除`}
                      className="reader-settings-replacement-remove"
                      disabled={isReaderCorrectionSaving}
                      onClick={() =>
                        setCustomReplacementDraft((current) =>
                          current.filter((candidate) => candidate.id !== rule.id)
                        )
                      }
                      type="button"
                    >
                      削除
                    </button>
                  </div>
                ))}
              </div>
            ) : (
              <p className="reader-settings-replacement-empty">置換ルールはまだありません。</p>
            )}
            {duplicateFromValues.size > 0 ? (
              <p className="reader-settings-replacement-error" role="alert">
                同じ置換前文字列は1件だけ登録できます。
              </p>
            ) : hasIncompleteCustomReplacement ? (
              <p className="reader-settings-replacement-empty">置換前を入力すると保存できます。</p>
            ) : null}
            <div className="reader-settings-replacement-actions">
              <button
                disabled={isReaderCorrectionSaving || customReplacementDraft.length >= CUSTOM_REPLACEMENT_MAX_RULES}
                onClick={() =>
                  setCustomReplacementDraft((current) => [
                    ...current,
                    { id: ++nextCustomReplacementDraftId, from: "", to: "" }
                  ])
                }
                type="button"
              >
                置換ルールを追加
              </button>
              <button
                disabled={isReaderCorrectionSaving || !isCustomReplacementDirty || hasInvalidCustomReplacement}
                onClick={() => onCustomReplacementsChange(draftReplacementRules)}
                type="button"
              >
                {isReaderCorrectionSaving ? "保存中..." : "置換ルールを保存"}
              </button>
            </div>
            <p className="reader-panel-section-description">最大50件。置換後を空欄にすると該当文字列を削除します。</p>
          </div>
          <div className="reader-settings-ai-proofread">
            <div>
              <span className="reader-panel-section-label">AIによる読みやすさ補正</span>
              <p className="reader-panel-section-description">AI校正版は表示用です。取得した原文は変更されません。</p>
            </div>
            {hasAIProofread ? (
              <>
                <label className="reader-settings-field">
                  <span>表示する本文</span>
                  <select
                    disabled={readerAIProofreadState === "deleting"}
                    onChange={(event) => onShowingAIProofreadChange(event.target.value === "proofread")}
                    value={isShowingAIProofread ? "proofread" : "original"}
                  >
                    <option value="original">原文</option>
                    <option value="proofread">AI校正版</option>
                  </select>
                </label>
                <div className="reader-settings-ai-proofread-actions">
                  <button
                    disabled={readerAIProofreadState === "generating" || readerAIProofreadState === "deleting"}
                    onClick={onGenerateAIProofread}
                    type="button"
                  >
                    {readerAIProofreadState === "generating" ? "再生成中..." : "再生成"}
                  </button>
                  <button
                    disabled={readerAIProofreadState === "generating" || readerAIProofreadState === "deleting"}
                    onClick={onDeleteAIProofread}
                    type="button"
                  >
                    {readerAIProofreadState === "deleting" ? "削除中..." : "校正版を削除"}
                  </button>
                </div>
              </>
            ) : (
              <button
                className="reader-panel-link reader-panel-link-button"
                disabled={readerAIProofreadState === "loading" || readerAIProofreadState === "generating"}
                onClick={onGenerateAIProofread}
                type="button"
              >
                {readerAIProofreadState === "loading"
                  ? "状態を確認中..."
                  : readerAIProofreadState === "generating"
                    ? "AI校正中..."
                    : "この話をAI校正"}
              </button>
            )}
          </div>
        </section>
        <section className="reader-panel-card reader-panel-card--compact reader-settings-section">
          <p className="reader-panel-section-label">デバッグ</p>
          <p className="reader-panel-section-description">ページからあふれる列を通常は隠し、確認したい時だけ色付きで残します。</p>
          <ReaderSettingsSwitch
            checked={debugPageOverflow}
            checkedLabel="緑で可視化"
            label="あふれる列を緑で可視化"
            onChange={onDebugPageOverflowChange}
            uncheckedLabel="非表示"
          />
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
