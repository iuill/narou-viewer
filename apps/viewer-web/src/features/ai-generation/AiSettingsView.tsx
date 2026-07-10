import {
  formatCount,
  getAiGenerationApiKeyStatusLabel,
  getAiGenerationSharedApiKeyStatusLabel,
  type AiGenerationHelpKey,
  type AiGenerationProfileDraft,
  type AiGenerationSharedProviderDraft,
  type ExtractionStrategyModelsDraft
} from "./model";
import type { AiGenerationSettingsResponse } from "./types";

type AiGenerationSettingsResponseLike = Pick<AiGenerationSettingsResponse, "masterPassphraseConfigured">;

export type AiSettingsViewProps = {
  aiGenerationSettingsError: string | null;
  aiGenerationPreferredMode: "llm" | "heuristic";
  aiGenerationSettings: AiGenerationSettingsResponseLike | null;
  isAiGenerationModeSaving: boolean;
  openAiGenerationHelpKey: AiGenerationHelpKey | null;
  onToggleAiGenerationHelp: (key: AiGenerationHelpKey) => void;
  onAiGenerationPreferredModeChange: (mode: "llm" | "heuristic") => void | Promise<void>;
  aiGenerationSharedOpenRouterDraft: AiGenerationSharedProviderDraft;
  onUpdateAiGenerationSharedOpenRouterDraft: (updater: (current: AiGenerationSharedProviderDraft) => AiGenerationSharedProviderDraft) => void;
  aiGenerationSharedGoogleBooksDraft: AiGenerationSharedProviderDraft;
  onUpdateAiGenerationSharedGoogleBooksDraft: (updater: (current: AiGenerationSharedProviderDraft) => AiGenerationSharedProviderDraft) => void;
  aiGenerationProfileDrafts: AiGenerationProfileDraft[];
  extractionStrategyModelsDraft: ExtractionStrategyModelsDraft;
  onSetExtractionStrategyModelsDraft: (draft: ExtractionStrategyModelsDraft) => void;
  defaultAiGenerationProfileDraft: AiGenerationProfileDraft | null;
  editingAiGenerationProfileId: string;
  onSelectEditingAiGenerationProfile: (profileId: string) => void;
  editingAiGenerationProfileDraft: AiGenerationProfileDraft | null;
  onUpdateAiGenerationProfileDraft: (profileId: string, updater: (current: AiGenerationProfileDraft) => AiGenerationProfileDraft) => void;
  onAddAiGenerationProfile: () => void;
  onRemoveAiGenerationProfile: (profileId: string) => void;
  selectedAiGenerationProfileId: string;
  onSetSelectedAiGenerationProfileId: (profileId: string) => void;
  isAiGenerationSettingsLoading: boolean;
  isAiGenerationSettingsSaving: boolean;
  onSaveAiGenerationSettings: () => void | Promise<void>;
};

export function AiSettingsView({
  aiGenerationSettingsError,
  aiGenerationPreferredMode,
  aiGenerationSettings,
  isAiGenerationModeSaving,
  openAiGenerationHelpKey,
  onToggleAiGenerationHelp,
  onAiGenerationPreferredModeChange,
  aiGenerationSharedOpenRouterDraft,
  onUpdateAiGenerationSharedOpenRouterDraft,
  aiGenerationSharedGoogleBooksDraft,
  onUpdateAiGenerationSharedGoogleBooksDraft,
  aiGenerationProfileDrafts,
  extractionStrategyModelsDraft,
  onSetExtractionStrategyModelsDraft,
  defaultAiGenerationProfileDraft,
  editingAiGenerationProfileId,
  onSelectEditingAiGenerationProfile,
  editingAiGenerationProfileDraft,
  onUpdateAiGenerationProfileDraft,
  onAddAiGenerationProfile,
  onRemoveAiGenerationProfile,
  selectedAiGenerationProfileId,
  onSetSelectedAiGenerationProfileId,
  isAiGenerationSettingsLoading,
  isAiGenerationSettingsSaving,
  onSaveAiGenerationSettings
}: AiSettingsViewProps) {
  return <div className="ai-workspace-body">
          {aiGenerationSettingsError ? <p className="message error">{aiGenerationSettingsError}</p> : null}
          {aiGenerationSettings && !aiGenerationSettings.masterPassphraseConfigured ? (
            <p className="message error ai-settings-alert">
              `AI_GENERATION_SETTINGS_MASTER_PASSPHRASE` が未設定です。保存した APIキーを暗号化できないため、
              OpenRouter / Google Books の APIキー保存はこのままでは正しく動きません。.env.local などにマスターパスフレーズを設定してください。
            </p>
          ) : null}
          <section className="library-queue-section">
            <div className="panel-header compact library-queue-header">
              <div>
                <h3>連携モード</h3>
                <p>LLM 連携を使うか、ローカルのヒューリスティック生成に切り替えるかを選びます。</p>
              </div>
            </div>
            <div className="mode-toggle ai-job-filter-tabs">
              <button
                className={aiGenerationPreferredMode === "llm" ? "active" : ""}
                disabled={isAiGenerationModeSaving}
                onClick={() => void onAiGenerationPreferredModeChange("llm")}
                type="button"
              >
                LLM連携
              </button>
              <button
                className={aiGenerationPreferredMode === "heuristic" ? "active" : ""}
                disabled={isAiGenerationModeSaving}
                onClick={() => void onAiGenerationPreferredModeChange("heuristic")}
                type="button"
              >
                ヒューリスティック
              </button>
              <button
                aria-expanded={openAiGenerationHelpKey === "preferredMode"}
                aria-label="連携モードの説明を表示"
                className="field-help"
                onClick={() => onToggleAiGenerationHelp("preferredMode")}
                type="button"
              >
                i
              </button>
            </div>
            {openAiGenerationHelpKey === "preferredMode" ? (
              <p className="field-help-text">
                `LLM連携` は viewer-api 内の Go internal AI module から OpenRouter のモデルを使います。`ヒューリスティック` は外部
                LLM を使わず、viewer-api 内の簡易抽出で動きます。
              </p>
            ) : null}
            {aiGenerationPreferredMode === "llm" ? (
              <p className="message">
                LLM連携を有効にすると、本文またはその抜粋・要約用テキストが設定した外部 provider に送信される場合があります。各サイト規約、権利者条件、provider 規約を確認してから使ってください。
              </p>
            ) : null}
            <p className="message">{isAiGenerationModeSaving ? "連携モードを反映中です。" : "連携モードは切り替えるとすぐに反映されます。"}</p>
          </section>
          <section className="library-queue-section">
            <div className="panel-header compact library-queue-header">
              <div>
                <h3>共通 OpenRouter APIキー</h3>
                <p>shared を使うプロファイルから参照される共通資格情報です。</p>
              </div>
            </div>
            <label className="download-form-field">
              <span>
                APIキー
                <button
                  aria-expanded={openAiGenerationHelpKey === "sharedApiKey"}
                  aria-label="共通APIキーの説明を表示"
                  className="field-help"
                  onClick={() => onToggleAiGenerationHelp("sharedApiKey")}
                  type="button"
                >
                  i
                </button>
              </span>
              <input
                autoComplete="off"
                onChange={(event) =>
                  onUpdateAiGenerationSharedOpenRouterDraft((current) => ({
                    ...current,
                    apiKeyInput: event.target.value
                  }))
                }
                placeholder={getAiGenerationSharedApiKeyStatusLabel(aiGenerationSharedOpenRouterDraft)}
                type="password"
                value={aiGenerationSharedOpenRouterDraft.apiKeyInput}
              />
              {aiGenerationSharedOpenRouterDraft.apiKeyInput.trim().length === 0 ? (
                <p className="field-help-text ai-api-key-status">
                  現在の保存状態: {getAiGenerationSharedApiKeyStatusLabel(aiGenerationSharedOpenRouterDraft)}
                </p>
              ) : null}
              {openAiGenerationHelpKey === "sharedApiKey" ? (
                <p className="field-help-text">
                  OpenRouter に接続するための共通キーです。各プロファイルで shared を選ぶとこのキーを使います。LLM連携時は本文またはその抜粋・要約用テキストが外部 provider に送信される場合があります。空のまま保存すると既存キーを維持します。
                </p>
              ) : null}
            </label>
          </section>
          <section className="library-queue-section">
            <div className="panel-header compact library-queue-header">
              <div>
                <h3>Google Books APIキー</h3>
                <p>ISBN 登録時の表紙画像と補助書誌の取得に使います。</p>
              </div>
            </div>
            <label className="download-form-field">
              <span>
                APIキー
                <button
                  aria-expanded={openAiGenerationHelpKey === "googleBooksApiKey"}
                  aria-label="Google Books APIキーの説明を表示"
                  className="field-help"
                  onClick={() => onToggleAiGenerationHelp("googleBooksApiKey")}
                  type="button"
                >
                  i
                </button>
              </span>
              <input
                autoComplete="off"
                onChange={(event) =>
                  onUpdateAiGenerationSharedGoogleBooksDraft((current) => ({
                    ...current,
                    apiKeyInput: event.target.value
                  }))
                }
                placeholder={getAiGenerationSharedApiKeyStatusLabel(aiGenerationSharedGoogleBooksDraft)}
                type="password"
                value={aiGenerationSharedGoogleBooksDraft.apiKeyInput}
              />
              {aiGenerationSharedGoogleBooksDraft.apiKeyInput.trim().length === 0 ? (
                <p className="field-help-text ai-api-key-status">
                  現在の保存状態: {getAiGenerationSharedApiKeyStatusLabel(aiGenerationSharedGoogleBooksDraft)}
                </p>
              ) : null}
              {openAiGenerationHelpKey === "googleBooksApiKey" ? (
                <p className="field-help-text">
                  Google Books API に接続するためのキーです。空のまま保存すると既存キーを維持します。.env の GOOGLE_BOOKS_API_KEY
                  は後方互換の fallback として扱います。
                </p>
              ) : null}
            </label>
          </section>
          <div className="panel-header compact">
            <div>
              <h3>プロファイル</h3>
              <p>
                {aiGenerationProfileDrafts.length} 件を保存できます。既定: {defaultAiGenerationProfileDraft?.label ?? "未設定"}
              </p>
            </div>
            <div className="panel-header-actions">
              <button className="summary-action-button" onClick={onAddAiGenerationProfile} type="button">
                プロファイル追加
              </button>
            </div>
          </div>
          <div className="ai-profile-list">
            {aiGenerationProfileDrafts.map((profile) => (
              <button
                key={profile.id}
                className={`ai-profile-pill ${profile.id === editingAiGenerationProfileId ? "active" : ""}`}
                onClick={() => onSelectEditingAiGenerationProfile(profile.id)}
                type="button"
              >
                <span>{profile.label}</span>
                <small>
                  {profile.modelId || "モデル未設定"}
                  {profile.id === selectedAiGenerationProfileId ? " / 既定" : ""}
                </small>
              </button>
            ))}
          </div>
          <section className="library-queue-section">
            <div className="panel-header compact library-queue-header">
              <div>
                <h3>抽出モデル</h3>
                <p>名前発見 + 並列抽出 + 補正で使う補助モデルを指定します。</p>
              </div>
            </div>
            <label className="download-form-field">
              <span>
                名前発見モデル
                <button
                  aria-expanded={openAiGenerationHelpKey === "nameDiscoveryModelId"}
                  aria-label="名前発見モデルの説明を表示"
                  className="field-help"
                  onClick={() => onToggleAiGenerationHelp("nameDiscoveryModelId")}
                  type="button"
                >
                  i
                </button>
              </span>
              <input
                onChange={(event) =>
                  onSetExtractionStrategyModelsDraft({
                    ...extractionStrategyModelsDraft,
                    nameDiscoveryModelId: event.target.value
                  })
                }
                placeholder="未指定なら既定プロファイルのモデル"
                type="text"
                value={extractionStrategyModelsDraft.nameDiscoveryModelId}
              />
              {openAiGenerationHelpKey === "nameDiscoveryModelId" ? (
                <p className="field-help-text">
                  D方式の名前候補発見だけに使う OpenRouter の実モデルIDです。空なら通常のプロファイルモデルをそのまま使い、詳細抽出と補正も通常のプロファイルモデルを使います。
                </p>
              ) : null}
            </label>
          </section>
          {editingAiGenerationProfileDraft ? (
            <form
              className="download-form ai-settings-form"
              onSubmit={(event) => {
                event.preventDefault();
                void onSaveAiGenerationSettings();
              }}
            >
              <label className="download-form-field">
                <span>
                  プロファイル名
                  <button
                    aria-expanded={openAiGenerationHelpKey === "profileLabel"}
                    aria-label="プロファイル名の説明を表示"
                    className="field-help"
                    onClick={() => onToggleAiGenerationHelp("profileLabel")}
                    type="button"
                  >
                    i
                  </button>
                </span>
                <input
                  onChange={(event) =>
                    onUpdateAiGenerationProfileDraft(editingAiGenerationProfileDraft.id, (current) => ({
                      ...current,
                      label: event.target.value
                    }))
                  }
                  placeholder="Default"
                  type="text"
                  value={editingAiGenerationProfileDraft.label}
                />
                {openAiGenerationHelpKey === "profileLabel" ? (
                  <p className="field-help-text">設定一覧と生成テストで見分けるための表示名です。</p>
                ) : null}
              </label>
              <label className="download-form-field ai-api-key-custom-field">
                <div className="ai-api-key-field-header">
                  <span className="ai-api-key-field-label">
                    APIキー
                    <button
                      aria-expanded={openAiGenerationHelpKey === "apiKey"}
                      aria-label="APIキーの説明を表示"
                      className="field-help"
                      onClick={() => onToggleAiGenerationHelp("apiKey")}
                      type="button"
                    >
                      i
                    </button>
                  </span>
                  <fieldset
                    aria-label="APIキーの参照元"
                    className="mode-toggle ai-api-key-source-toggle"
                    style={{ border: 0, margin: 0, padding: 0 }}
                  >
                    <button
                      aria-pressed={editingAiGenerationProfileDraft.apiKeySource === "shared"}
                      className={editingAiGenerationProfileDraft.apiKeySource === "shared" ? "active" : ""}
                      onClick={() =>
                        onUpdateAiGenerationProfileDraft(editingAiGenerationProfileDraft.id, (current) => ({
                          ...current,
                          apiKeySource: "shared",
                          apiKeyInput: ""
                        }))
                      }
                      type="button"
                    >
                      共通を使う
                    </button>
                    <button
                      aria-pressed={editingAiGenerationProfileDraft.apiKeySource === "custom"}
                      className={editingAiGenerationProfileDraft.apiKeySource === "custom" ? "active" : ""}
                      onClick={() =>
                        onUpdateAiGenerationProfileDraft(editingAiGenerationProfileDraft.id, (current) => ({
                          ...current,
                          apiKeySource: "custom"
                        }))
                      }
                      type="button"
                    >
                      プロファイル専用
                    </button>
                  </fieldset>
                </div>
                <input
                  autoComplete="off"
                  disabled={editingAiGenerationProfileDraft.apiKeySource !== "custom"}
                  onChange={(event) =>
                    onUpdateAiGenerationProfileDraft(editingAiGenerationProfileDraft.id, (current) => ({
                      ...current,
                      apiKeyInput: event.target.value
                    }))
                  }
                  placeholder={getAiGenerationApiKeyStatusLabel(editingAiGenerationProfileDraft)}
                  type="password"
                  value={editingAiGenerationProfileDraft.apiKeyInput}
                />
                {openAiGenerationHelpKey === "apiKey" ? (
                  <p className="field-help-text">
                    `共通を使う` は共通 OpenRouter APIキーを参照します。`プロファイル専用` を選ぶと、このプロファイルだけの APIキーを保存できます。LLM連携時は本文またはその抜粋・要約用テキストが外部 provider に送信される場合があります。空のまま保存すると、既存キーを維持します。
                  </p>
                ) : null}
              </label>
              <label className="download-form-field">
                <span>
                  モデル
                  <button
                    aria-expanded={openAiGenerationHelpKey === "modelId"}
                    aria-label="モデルの説明を表示"
                    className="field-help"
                    onClick={() => onToggleAiGenerationHelp("modelId")}
                    type="button"
                  >
                    i
                  </button>
                </span>
                <input
                  onChange={(event) =>
                    onUpdateAiGenerationProfileDraft(editingAiGenerationProfileDraft.id, (current) => ({
                      ...current,
                      modelId: event.target.value,
                      modelInfo: undefined
                    }))
                  }
                  placeholder="openai/gpt-5-mini"
                  type="text"
                  value={editingAiGenerationProfileDraft.modelId}
                />
                {openAiGenerationHelpKey === "modelId" ? (
                  <p className="field-help-text">
                    人物と用語の抽出に使う OpenRouter の実モデルIDです。例: `openai/gpt-5-mini`
                  </p>
                ) : null}
                {editingAiGenerationProfileDraft.modelInfo ? (
                  <p className="field-help-text ai-model-info">
                    OpenRouter: context {formatCount(editingAiGenerationProfileDraft.modelInfo.contextLength)} tokens / max output{" "}
                    {formatCount(editingAiGenerationProfileDraft.modelInfo.maxCompletionTokens)} tokens
                  </p>
                ) : null}
              </label>
              <label className="download-form-field">
                <span>
                  provider order
                  <button
                    aria-expanded={openAiGenerationHelpKey === "providerOrder"}
                    aria-label="provider order の説明を表示"
                    className="field-help"
                    onClick={() => onToggleAiGenerationHelp("providerOrder")}
                    type="button"
                  >
                    i
                  </button>
                </span>
                <input
                  onChange={(event) =>
                    onUpdateAiGenerationProfileDraft(editingAiGenerationProfileDraft.id, (current) => ({
                      ...current,
                      providerOrder: event.target.value
                    }))
                  }
                  placeholder="openai,anthropic"
                  type="text"
                  value={editingAiGenerationProfileDraft.providerOrder}
                />
                {openAiGenerationHelpKey === "providerOrder" ? (
                  <p className="field-help-text">
                    OpenRouter の provider 優先順です。カンマ区切りで指定します。未指定なら OpenRouter 側に任せます。
                  </p>
                ) : null}
              </label>
              <div className="download-form-options ai-settings-options">
                <label className="download-form-checkbox">
                  <input
                    checked={editingAiGenerationProfileDraft.allowFallbacks}
                    onChange={(event) =>
                      onUpdateAiGenerationProfileDraft(editingAiGenerationProfileDraft.id, (current) => ({
                        ...current,
                        allowFallbacks: event.target.checked
                      }))
                    }
                    type="checkbox"
                  />
                  <span>fallback を許可</span>
                  <button
                    aria-expanded={openAiGenerationHelpKey === "allowFallbacks"}
                    aria-label="fallback の説明を表示"
                    className="field-help"
                    onClick={() => onToggleAiGenerationHelp("allowFallbacks")}
                    type="button"
                  >
                    i
                  </button>
                </label>
                <label className="download-form-checkbox">
                  <input
                    checked={editingAiGenerationProfileDraft.requireParameters}
                    onChange={(event) =>
                      onUpdateAiGenerationProfileDraft(editingAiGenerationProfileDraft.id, (current) => ({
                        ...current,
                        requireParameters: event.target.checked
                      }))
                    }
                    type="checkbox"
                  />
                  <span>parameters 必須</span>
                  <button
                    aria-expanded={openAiGenerationHelpKey === "requireParameters"}
                    aria-label="parameters 必須の説明を表示"
                    className="field-help"
                    onClick={() => onToggleAiGenerationHelp("requireParameters")}
                    type="button"
                  >
                    i
                  </button>
                </label>
              </div>
              {openAiGenerationHelpKey === "allowFallbacks" ? (
                <p className="field-help-text ai-settings-options">
                  有効にすると、指定 provider が使えないときに別 provider へフォールバックできます。
                </p>
              ) : null}
              {openAiGenerationHelpKey === "requireParameters" ? (
                <p className="field-help-text ai-settings-options">
                  有効にすると、provider 側で追加パラメータが必要な場合に失敗扱いにします。再現性を優先したいときに向いています。
                </p>
              ) : null}
              <div className="download-form-actions ai-settings-actions">
                <button
                  className="download-cancel-button"
                  disabled={aiGenerationProfileDrafts.length <= 1}
                  onClick={() => onRemoveAiGenerationProfile(editingAiGenerationProfileDraft.id)}
                  type="button"
                >
                  このプロファイルを削除
                </button>
                <button
                  className={selectedAiGenerationProfileId === editingAiGenerationProfileDraft.id ? "active" : ""}
                  disabled={selectedAiGenerationProfileId === editingAiGenerationProfileDraft.id}
                  onClick={() => onSetSelectedAiGenerationProfileId(editingAiGenerationProfileDraft.id)}
                  type="button"
                >
                  既定プロファイルに設定
                </button>
                <button disabled={isAiGenerationSettingsLoading || isAiGenerationSettingsSaving || isAiGenerationModeSaving} type="submit">
                  {isAiGenerationSettingsSaving ? "保存中..." : "プロファイル設定を保存"}
                </button>
              </div>
            </form>
          ) : (
            <p className="message">編集するプロファイルを選択してください。</p>
          )}
          <p className="message">
            `プロファイル設定を保存` で、共通 APIキー と編集中プロファイル、既定プロファイル、抽出モデルの指定が反映されます。APIキー欄を空のまま保存すると、既存のキーは変更しません。
          </p>

  </div>;
}
