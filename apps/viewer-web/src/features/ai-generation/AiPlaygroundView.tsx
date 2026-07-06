import { formatElapsedMs } from "../../shared/date";
import { getAiGenerationModeLabel, getCharacterGenerationStrategyLabel, type AiGenerationProfileDraft } from "./model";
import type { AiGenerationPlaygroundBatchTiming, AiGenerationPlaygroundProgress, AiGenerationPlaygroundPromptPreview, AiGenerationPlaygroundResponse } from "./types";

type NovelOption = { novelId: string; title: string; totalEpisodes: number };

export type AiPlaygroundViewProps = {
  aiGenerationPlaygroundError: string | null;
  aiGenerationPlaygroundNovelId: string;
  onSetAiGenerationPlaygroundNovelId: (novelId: string) => void;
  aiGenerationPlaygroundProfileId: string;
  onSetAiGenerationPlaygroundProfileId: (profileId: string) => void;
  aiGenerationPlaygroundUpToEpisodeIndex: string;
  onSetAiGenerationPlaygroundUpToEpisodeIndex: (episodeIndex: string) => void;
  aiGenerationPlaygroundMaxEpisodeIndex: string;
  isAiGenerationPlaygroundRunning: boolean;
  onRunAiGenerationPlayground: () => void | Promise<void>;
  novels: NovelOption[];
  aiGenerationProfileDrafts: AiGenerationProfileDraft[];
  aiGenerationPlaygroundProgress: AiGenerationPlaygroundProgress | null;
  aiGenerationPlaygroundResult: AiGenerationPlaygroundResponse | null;
  aiGenerationPlaygroundPromptPreview: AiGenerationPlaygroundPromptPreview | null;
  aiGenerationPlaygroundBatchTimings: AiGenerationPlaygroundBatchTiming[];
  aiGenerationPlaygroundResponseJson: string;
};

export function AiPlaygroundView({
  aiGenerationPlaygroundError,
  aiGenerationPlaygroundNovelId,
  onSetAiGenerationPlaygroundNovelId,
  aiGenerationPlaygroundProfileId,
  onSetAiGenerationPlaygroundProfileId,
  aiGenerationPlaygroundUpToEpisodeIndex,
  onSetAiGenerationPlaygroundUpToEpisodeIndex,
  aiGenerationPlaygroundMaxEpisodeIndex,
  isAiGenerationPlaygroundRunning,
  onRunAiGenerationPlayground,
  novels,
  aiGenerationProfileDrafts,
  aiGenerationPlaygroundProgress,
  aiGenerationPlaygroundResult,
  aiGenerationPlaygroundPromptPreview,
  aiGenerationPlaygroundBatchTimings,
  aiGenerationPlaygroundResponseJson
}: AiPlaygroundViewProps) {
  return <div className="ai-workspace-body">
          {aiGenerationPlaygroundError ? <p className="message error">{aiGenerationPlaygroundError}</p> : null}
          <form
            className="download-form ai-settings-form"
            onSubmit={(event) => {
              event.preventDefault();
              void onRunAiGenerationPlayground();
            }}
          >
            <label className="download-form-field">
              <span>作品</span>
              <select onChange={(event) => onSetAiGenerationPlaygroundNovelId(event.target.value)} value={aiGenerationPlaygroundNovelId}>
                {novels.map((novel) => (
                  <option key={novel.novelId} value={novel.novelId}>
                    {novel.title}
                  </option>
                ))}
              </select>
            </label>
            <label className="download-form-field">
              <span>プロファイル</span>
              <select
                onChange={(event) => onSetAiGenerationPlaygroundProfileId(event.target.value)}
                value={aiGenerationPlaygroundProfileId}
              >
                {aiGenerationProfileDrafts.map((profile) => (
                  <option key={profile.id} value={profile.id}>
                    {profile.label}
                  </option>
                ))}
              </select>
            </label>
            <label className="download-form-field">
              <span>対象話数</span>
              <input
                inputMode="numeric"
                max={aiGenerationPlaygroundMaxEpisodeIndex || undefined}
                min={1}
                onChange={(event) => onSetAiGenerationPlaygroundUpToEpisodeIndex(event.target.value)}
                type="number"
                value={aiGenerationPlaygroundUpToEpisodeIndex}
              />
            </label>
            <div className="download-form-actions">
              <button disabled={isAiGenerationPlaygroundRunning || novels.length === 0} type="submit">
                {isAiGenerationPlaygroundRunning ? "実行中..." : "プレビュー実行"}
              </button>
            </div>
          </form>
          {isAiGenerationPlaygroundRunning && aiGenerationPlaygroundProgress ? (
            <section aria-live="polite" className="ai-playground-progress">
              <div className="queue-inline-progress">
                <div className="queue-inline-progress-copy">
                  <strong>{aiGenerationPlaygroundProgress.message}</strong>
                  <span>
                    ステップ {aiGenerationPlaygroundProgress.step}/{aiGenerationPlaygroundProgress.stepCount}
                    {aiGenerationPlaygroundProgress.batchIndex && aiGenerationPlaygroundProgress.batchCount
                      ? ` / batch ${aiGenerationPlaygroundProgress.batchIndex}/${aiGenerationPlaygroundProgress.batchCount}`
                      : ""}
                  </span>
                </div>
                <div aria-hidden="true" className="queue-inline-progress-bar">
                  <span style={{ width: `${aiGenerationPlaygroundProgress.progress}%` }} />
                </div>
              </div>
              <p className="field-help-text ai-playground-progress-hint">
                生成テスト専用の段階進捗です。AI 問い合わせ中はこの段階でしばらく待機することがあります。
              </p>
            </section>
          ) : null}
          {aiGenerationPlaygroundResult || aiGenerationPlaygroundPromptPreview || aiGenerationPlaygroundBatchTimings.length > 0 ? (
            <section className="ai-playground-result">
              <div className="ai-playground-result-main">
                {aiGenerationPlaygroundResult ? (
                  <>
                    <div className="panel-header compact">
                      <div>
                        <h3>{aiGenerationPlaygroundResult.novelTitle}</h3>
                        <p>
                          {aiGenerationPlaygroundResult.profileLabel ? `${aiGenerationPlaygroundResult.profileLabel} / ` : ""}
                          {getAiGenerationModeLabel(aiGenerationPlaygroundResult.generationMode)} /{" "}
                          {getCharacterGenerationStrategyLabel(aiGenerationPlaygroundResult.generationStrategy)}
                          {aiGenerationPlaygroundResult.modelId ? ` / ${aiGenerationPlaygroundResult.modelId}` : ""}
                        </p>
                      </div>
                      <p>
                        第{aiGenerationPlaygroundResult.processedUpToEpisodeIndex}話時点 / {aiGenerationPlaygroundResult.characters.length} 人
                      </p>
                    </div>
                    {aiGenerationPlaygroundResult.characters.length > 0 ? (
                      <div className="reader-character-cards">
                        {aiGenerationPlaygroundResult.characters.map((character) => (
                          <article className="reader-character-card" key={character.characterId}>
                            <header>
                              <strong>{character.canonicalName}</strong>
                              <span>
                                {character.fullName ?? "フルネーム未確定"}
                                {character.gender ? ` / ${character.gender}` : ""}
                              </span>
                            </header>
                            <dl>
                              <div>
                                <dt>初登場</dt>
                                <dd>第{character.firstAppearanceEpisodeIndex}話</dd>
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
                      <p className="message">キャラクターは抽出されませんでした。</p>
                    )}
                  </>
                ) : (
                  <p className="message">送信プロンプトと batch 進捗を表示しています。結果を待っています。</p>
                )}
              </div>
              <aside className="ai-playground-json-panels">
                {aiGenerationPlaygroundPromptPreview ? (
                  <section className="ai-playground-json-panel ai-playground-prompt-panel">
                    <div className="panel-header compact">
                      <div>
                        <h3>送信プロンプト</h3>
                        <p>system prompt と batch ごとの対象本文を表示します。</p>
                      </div>
                      <p>{aiGenerationPlaygroundPromptPreview.batches.length} batch</p>
                    </div>
                    <details className="ai-playground-json-panel-details">
                      <summary>
                        <span>システムプロンプト</span>
                        <span>OpenRouter 宛て</span>
                      </summary>
                      <pre>{aiGenerationPlaygroundPromptPreview.systemPrompt}</pre>
                    </details>
                    {aiGenerationPlaygroundPromptPreview.batches.map((batch) => (
                      <details className="ai-playground-json-panel-details" key={`prompt-batch-${batch.batchIndex}`}>
                        <summary>
                          <span>対象本文 batch {batch.batchIndex}/{batch.batchCount}</span>
                          <span>{batch.episodeIndexes.map((episodeIndex) => `第${episodeIndex}話`).join(", ")}</span>
                        </summary>
                        <div className="ai-playground-prompt-chunks">
                          {batch.chunks.map((chunk) => (
                            <section className="ai-playground-prompt-chunk" key={`${batch.batchIndex}-${chunk.episodeIndex}-${chunk.chunkIndex}`}>
                              <p className="ai-playground-prompt-chunk-label">
                                第{chunk.episodeIndex}話 {chunk.title}
                                {chunk.chunkCount > 1 ? ` / chunk ${chunk.chunkIndex}/${chunk.chunkCount}` : ""}
                              </p>
                              <pre>{chunk.text}</pre>
                            </section>
                          ))}
                        </div>
                      </details>
                    ))}
                  </section>
                ) : null}
                {aiGenerationPlaygroundBatchTimings.length > 0 ? (
                  <section className="ai-playground-json-panel">
                    <div className="panel-header compact">
                      <div>
                        <h3>batch処理時間</h3>
                        <p>各 batch のリクエストからレスポンスまでの所要時間です。</p>
                      </div>
                      <p>{aiGenerationPlaygroundBatchTimings.length} 件</p>
                    </div>
                    <div className="ai-playground-batch-timings">
                      {aiGenerationPlaygroundBatchTimings.map((timing) => (
                        <article className="ai-playground-batch-timing" key={`timing-${timing.batchIndex}`}>
                          <div className="ai-playground-batch-timing-header">
                            <strong>
                              batch {timing.batchIndex}/{timing.batchCount}
                            </strong>
                            <span>{formatElapsedMs(timing.elapsedMs)}</span>
                          </div>
                          <p>{timing.episodeIndexes.map((episodeIndex) => `第${episodeIndex}話`).join(", ")}</p>
                        </article>
                      ))}
                    </div>
                  </section>
                ) : null}
                <details className="ai-playground-json-panel">
                  <summary>
                    <span>レスポンスJSON</span>
                    <span>受信内容</span>
                  </summary>
                  <pre>{aiGenerationPlaygroundResponseJson}</pre>
                </details>
              </aside>
            </section>
          ) : null}

  </div>;
}
