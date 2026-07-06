import type { ButtonHTMLAttributes, ReactNode } from "react";
import { ReaderFloatingPanel } from "./ReaderFloatingPanel";
import type { ReaderSpeechVoiceOption } from "./readerSpeech";

type Props = {
  isSpeechSupported: boolean;
  speechEnabled: boolean;
  speechRate: number;
  speechVoiceUri: string | null;
  speechPreferRubyText: boolean;
  speechDebugHighlight: boolean;
  speechVoices: ReaderSpeechVoiceOption[];
  isPlaying: boolean;
  isPaused: boolean;
  hasSpeechContent: boolean;
  hasActiveChunk: boolean;
  onClose: () => void;
  onSpeechEnabledChange: (speechEnabled: boolean) => void;
  onSpeechRateChange: (speechRate: number) => void;
  onSpeechVoiceUriChange: (speechVoiceUri: string | null) => void;
  onSpeechPreferRubyTextChange: (speechPreferRubyText: boolean) => void;
  onSpeechDebugHighlightChange: (speechDebugHighlight: boolean) => void;
  onPlay: () => void;
  onPause: () => void;
  onResume: () => void;
  onStop: () => void;
  onReset: () => void;
};

const READER_SPEECH_RATE_MIN = 0.5;
const READER_SPEECH_RATE_MAX = 2;
const READER_SPEECH_RATE_STEP = 0.05;

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

function roundToStep(value: number, digits: number) {
  return Number(value.toFixed(digits));
}

function SpeechActionButton({
  children,
  icon,
  kind = "secondary",
  className,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { icon: ReactNode; kind?: "primary" | "secondary" }) {
  return (
    <button
      {...props}
      className={`reader-speech-action-button reader-speech-action-button--${kind}${className ? ` ${className}` : ""}`}
      type={props.type ?? "button"}
    >
      <span aria-hidden="true" className="reader-speech-action-icon">
        {icon}
      </span>
      <span>{children}</span>
    </button>
  );
}

export function ReaderSpeechPanel({
  isSpeechSupported,
  speechEnabled,
  speechRate,
  speechVoiceUri,
  speechPreferRubyText,
  speechDebugHighlight,
  speechVoices,
  isPlaying,
  isPaused,
  hasSpeechContent,
  hasActiveChunk,
  onClose,
  onSpeechEnabledChange,
  onSpeechRateChange,
  onSpeechVoiceUriChange,
  onSpeechPreferRubyTextChange,
  onSpeechDebugHighlightChange,
  onPlay,
  onPause,
  onResume,
  onStop,
  onReset
}: Props) {
  const canDecreaseSpeechRate = speechRate > READER_SPEECH_RATE_MIN;
  const canIncreaseSpeechRate = speechRate < READER_SPEECH_RATE_MAX;
  const selectedVoice = speechVoices.find((voice) => voice.voiceURI === speechVoiceUri) ?? null;
  const speechStatusLabel = !speechEnabled ? "オフ" : isPlaying ? "再生中" : isPaused ? "一時停止中" : "待機中";
  const speechStatusDescription = !isSpeechSupported
    ? "このブラウザでは読み上げを利用できません。"
    : !speechEnabled
      ? "読み上げを有効にすると、このパネルから再生と設定をまとめて操作できます。"
      : !hasSpeechContent
        ? "この話では、読み上げできる本文がまだ見つかっていません。"
        : isPlaying
          ? "読み上げ中です。必要になったらここで一時停止や停止ができます。"
          : isPaused
            ? "一時停止中です。再開か停止を選べます。"
            : "現在位置から読み上げを始められます。";

  function handleAdjustSpeechRate(delta: number) {
    onSpeechRateChange(roundToStep(clamp(speechRate + delta, READER_SPEECH_RATE_MIN, READER_SPEECH_RATE_MAX), 2));
  }

  return (
    <ReaderFloatingPanel
      className="reader-speech-panel reader-overlay-panel--speech"
      description="読み上げの再生状態と音声設定をまとめて調整します。"
      onClose={onClose}
      title="読み上げ"
    >
      <section className="reader-panel-card reader-panel-card--hero">
        <p className="reader-panel-section-label">状態</p>
        <p className="reader-speech-status">{speechStatusLabel}</p>
        <p className="reader-panel-section-description">{speechStatusDescription}</p>
        <div className="reader-panel-chip-row">
          <span className="reader-panel-chip">速度 {speechRate.toFixed(2)}x</span>
          <span
            className="reader-panel-chip reader-speech-chip reader-speech-chip--voice"
            title={selectedVoice ? `音声 ${selectedVoice.name}` : "音声 標準"}
          >
            音声 {selectedVoice ? selectedVoice.name : "標準"}
          </span>
          <span className="reader-panel-chip">{speechPreferRubyText ? "ルビを読む" : "本文を読む"}</span>
          {speechDebugHighlight ? <span className="reader-panel-chip">チャンク表示</span> : null}
        </div>
      </section>

      <section className="reader-panel-card reader-panel-card--compact reader-settings-section">
        <p className="reader-panel-section-label">再生</p>
        <p className="reader-panel-section-description">速度や音声を変えると、読み上げはいったん停止します。</p>
        <div className="reader-actions reader-speech-actions">
          {isPlaying ? (
            <>
              <SpeechActionButton
                icon={
                  <svg aria-hidden="true" focusable="false" viewBox="0 0 24 24">
                    <path d="M6 5h4v14H6zm8 0h4v14h-4z" />
                  </svg>
                }
                onClick={onPause}
                kind="secondary"
              >
                一時停止
              </SpeechActionButton>
              <SpeechActionButton
                icon={
                  <svg aria-hidden="true" focusable="false" viewBox="0 0 24 24">
                    <path d="M6 6h12v12H6z" />
                  </svg>
                }
                onClick={onStop}
                kind="secondary"
              >
                停止
              </SpeechActionButton>
            </>
          ) : isPaused ? (
            <>
              <SpeechActionButton
                icon={
                  <svg aria-hidden="true" focusable="false" viewBox="0 0 24 24">
                    <path d="M8 5.5v13l10-6.5Z" />
                  </svg>
                }
                onClick={onResume}
                kind="primary"
              >
                再開
              </SpeechActionButton>
              <SpeechActionButton
                icon={
                  <svg aria-hidden="true" focusable="false" viewBox="0 0 24 24">
                    <path d="M6 6h12v12H6z" />
                  </svg>
                }
                onClick={onStop}
                kind="secondary"
              >
                停止
              </SpeechActionButton>
            </>
          ) : (
            <SpeechActionButton
              disabled={!isSpeechSupported || !speechEnabled || !hasSpeechContent}
              icon={
                <svg aria-hidden="true" focusable="false" viewBox="0 0 24 24">
                  <path d="M7 5.5v13l10-6.5Z" />
                </svg>
              }
              kind="primary"
              onClick={onPlay}
            >
              {hasActiveChunk ? "現在位置からやり直す" : "現在位置から再生"}
            </SpeechActionButton>
          )}
        </div>
      </section>

      <section className="reader-panel-card reader-panel-card--compact reader-settings-section">
        <p className="reader-panel-section-label">設定</p>
        <label className="reader-settings-field">
          <span>読み上げ</span>
          <select onChange={(event) => onSpeechEnabledChange(event.target.value === "enabled")} value={speechEnabled ? "enabled" : "disabled"}>
            <option value="enabled">有効</option>
            <option value="disabled">無効</option>
          </select>
        </label>
        <label className="reader-settings-field">
          <span>読み上げ速度: {speechRate.toFixed(2)}x</span>
          <div className="reader-settings-range-control">
            <button
              aria-label="読み上げ速度を下げる"
              className="reader-settings-step-button"
              disabled={!isSpeechSupported || !speechEnabled || !canDecreaseSpeechRate}
              onClick={() => handleAdjustSpeechRate(-READER_SPEECH_RATE_STEP)}
              type="button"
            >
              -
            </button>
            <input
              aria-label={`読み上げ速度: ${speechRate.toFixed(2)}x`}
              disabled={!isSpeechSupported || !speechEnabled}
              max={READER_SPEECH_RATE_MAX}
              min={READER_SPEECH_RATE_MIN}
              onChange={(event) => onSpeechRateChange(Number.parseFloat(event.target.value))}
              step={READER_SPEECH_RATE_STEP}
              type="range"
              value={speechRate}
            />
            <button
              aria-label="読み上げ速度を上げる"
              className="reader-settings-step-button"
              disabled={!isSpeechSupported || !speechEnabled || !canIncreaseSpeechRate}
              onClick={() => handleAdjustSpeechRate(READER_SPEECH_RATE_STEP)}
              type="button"
            >
              +
            </button>
          </div>
        </label>
        <label className="reader-settings-field">
          <span>音声</span>
          <select
            disabled={!isSpeechSupported || !speechEnabled || speechVoices.length === 0}
            onChange={(event) => onSpeechVoiceUriChange(event.target.value.length > 0 ? event.target.value : null)}
            value={speechVoiceUri ?? ""}
          >
            <option value="">標準の音声</option>
            {speechVoices.map((voice) => (
              <option key={`${voice.voiceURI}:${voice.lang}:${voice.name}:${voice.default ? "default" : "custom"}`} value={voice.voiceURI}>
                {voice.name} ({voice.lang}){voice.default ? " [既定]" : ""}
              </option>
            ))}
          </select>
        </label>
        <label className="reader-settings-field">
          <span>ルビの読み方</span>
          <select
            disabled={!isSpeechSupported || !speechEnabled}
            onChange={(event) => onSpeechPreferRubyTextChange(event.target.value === "ruby")}
            value={speechPreferRubyText ? "ruby" : "text"}
          >
            <option value="ruby">ルビを読む</option>
            <option value="text">本文を読む</option>
          </select>
        </label>
        <label className="reader-settings-field">
          <span>デバッグ表示</span>
          <select
            disabled={!isSpeechSupported || !speechEnabled}
            onChange={(event) => onSpeechDebugHighlightChange(event.target.value === "enabled")}
            value={speechDebugHighlight ? "enabled" : "disabled"}
          >
            <option value="disabled">表示しない</option>
            <option value="enabled">読み上げチャンクを表示</option>
          </select>
        </label>
      </section>

      <div className="reader-settings-actions">
        <button className="reader-settings-reset" onClick={onReset} type="button">
          読み上げ設定を初期化
        </button>
      </div>
    </ReaderFloatingPanel>
  );
}
