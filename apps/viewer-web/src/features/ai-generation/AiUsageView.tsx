import { Fragment, type CSSProperties } from "react";
import { formatDate, formatElapsedMs } from "../../shared/date";
import { fetchAiUsageRunJson } from "./api";
import { formatCount } from "./model";
import type { AiUsageRequestSummary, AiUsageResponse, AiUsageRunSummary } from "./types";

export type AiUsageViewProps = {
  aiUsage: AiUsageResponse | null;
  aiUsageError: string | null;
  isAiUsageLoading: boolean;
};

export function AiUsageView({ aiUsage, aiUsageError, isAiUsageLoading }: AiUsageViewProps) {
  async function downloadUsageRunJson(run: AiUsageRunSummary): Promise<void> {
    if (!run.hasSnapshot) {
      window.alert("この run には保存済み JSON がありません。");
      return;
    }

    try {
      const detail = await fetchAiUsageRunJson(run.runId);
      const blob = new Blob([`${JSON.stringify(detail, null, 2)}\n`], { type: "application/json" });
      const objectUrl = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = objectUrl;
      anchor.download = `ai-usage-run-${sanitizeFilePart(run.runId)}.json`;
      document.body.append(anchor);
      anchor.click();
      anchor.remove();
      URL.revokeObjectURL(objectUrl);
    } catch (error) {
      window.alert(error instanceof Error ? error.message : "AI使用量JSONの取得に失敗しました。");
    }
  }

  return <div className="ai-workspace-body">
          {aiUsageError ? <p className="message error">{aiUsageError}</p> : null}
          {isAiUsageLoading && !aiUsage ? <p className="message">AI使用量を読み込み中...</p> : null}
          {aiUsage ? (
            <>
              <div className="ai-usage-summary-grid">
                <article className="ai-usage-summary-card">
                  <span>Run</span>
                  <strong>{formatCount(aiUsage.summary.runCount)}</strong>
                  <small>{formatCount(aiUsage.summary.requestCount)} requests</small>
                </article>
                <article className="ai-usage-summary-card">
                  <span>Total tokens</span>
                  <strong>{formatCount(aiUsage.summary.totalTokens)}</strong>
                  <small>
                    in {formatCount(aiUsage.summary.inputTokens)} / out {formatCount(aiUsage.summary.outputTokens)}
                  </small>
                </article>
                <article className="ai-usage-summary-card">
                  <span>Cache</span>
                  <strong>{formatCount(aiUsage.summary.cachedInputTokens)}</strong>
                  <small>cached input tokens</small>
                </article>
                <article className="ai-usage-summary-card">
                  <span>Cost</span>
                  <strong>{formatCost(aiUsage.summary.totalCost)}</strong>
                  <small>OpenRouter reported</small>
                </article>
              </div>
              <section className="ai-usage-run-section">
                <div className="panel-header compact">
                  <div>
                    <h3>読書AI利用統計</h3>
                    <p>直近 {aiUsage.runs.length} 件の Agents SDK run を表示します。</p>
                  </div>
                  <p>平均 {formatCount(Math.round(aiUsage.summary.averageTotalTokens))} tokens/run</p>
                </div>
                {aiUsage.runs.length > 0 ? (
                  <div className="ai-usage-table-wrap">
                    <table className="ai-usage-table">
                      <thead>
                        <tr>
                          <th scope="col">時刻</th>
                          <th scope="col" className="numeric">
                            Tokens
                          </th>
                          <th scope="col" className="numeric">
                            In / Out
                          </th>
                          <th scope="col" className="numeric">
                            Cache
                          </th>
                          <th scope="col" className="numeric">
                            Req
                          </th>
                          <th scope="col" className="numeric">
                            Tools
                          </th>
                          <th scope="col" className="numeric">
                            Time
                          </th>
                          <th scope="col" className="numeric">
                            Cost
                          </th>
                          <th scope="col">Context</th>
                          <th scope="col">Status</th>
                        </tr>
                      </thead>
                      <tbody>
                        {aiUsage.runs.map((run) => (
                          <Fragment key={run.runId}>
                            <tr>
                              <td>{formatDate(run.startedAt)}</td>
                              <td className="numeric strong">{formatCount(run.totalTokens)}</td>
                              <td className="numeric">
                                {formatCount(run.inputTokens)} / {formatCount(run.outputTokens)}
                              </td>
                              <td className="numeric">{formatCount(run.cachedInputTokens)}</td>
                              <td className="numeric">{run.requestCount}</td>
                              <td className="numeric">{run.toolCallCount}</td>
                              <td className="numeric">{formatElapsedMs(run.elapsedMs)}</td>
                              <td className="numeric strong">{formatCost(run.totalCost)}</td>
                              <td>
                                <div className="ai-usage-context">
                                  <span title={run.novelTitle ?? run.novelId ?? undefined}>
                                    {run.novelTitle ?? run.novelId ?? "読書AI"}
                                  </span>
                                  <small>
                                    {run.currentEpisodeIndex ? `第${run.currentEpisodeIndex}話 / ` : ""}
                                    {run.profileLabel ? `${run.profileLabel} / ` : ""}
                                    {run.modelId ?? "model 未記録"}
                                  </small>
                                  <small title={run.runId}>
                                    Run {formatRunId(run.runId)}
                                    {run.hasSnapshot ? " / JSON保存あり" : ""}
                                  </small>
                                  {run.reasoningOutputTokens > 0 ? (
                                    <small>reasoning {formatCount(run.reasoningOutputTokens)}</small>
                                  ) : null}
                                  {run.errorMessage ? <small className="error">{run.errorMessage}</small> : null}
                                </div>
                              </td>
                              <td>
                                <span className={`queue-task-badge status-${run.status}`}>{run.status}</span>
                              </td>
                            </tr>
                            <tr className="ai-usage-request-row">
                              <td colSpan={10}>
                                <div className="ai-usage-request-toolbar">
                                  <button
                                    aria-label={`${run.novelTitle ?? run.novelId ?? "読書AI"} ${formatDate(run.startedAt)} の usage JSON をダウンロード`}
                                    className="ai-usage-json-button"
                                    disabled={!run.hasSnapshot}
                                    onClick={() => void downloadUsageRunJson(run)}
                                    type="button"
                                  >
                                    JSON
                                  </button>
                                </div>
                                <details className="ai-usage-request-details">
                                  <summary
                                    aria-label={`${run.novelTitle ?? run.novelId ?? "読書AI"} ${formatDate(run.startedAt)} の request ごとの費消状況`}
                                    title={run.runId}
                                  >
                                    request ごとの費消状況
                                  </summary>
                                  {run.requests.length > 0 ? (
                                    <table className="ai-usage-request-table">
                                      <thead>
                                        <tr>
                                          <th scope="col">Req</th>
                                          <th scope="col">Kind</th>
                                          <th scope="col" className="numeric">
                                            Tokens
                                          </th>
                                          <th scope="col" className="numeric">
                                            In
                                          </th>
                                          <th scope="col" className="numeric">
                                            Out
                                          </th>
                                          <th scope="col" className="numeric">
                                            Cache
                                          </th>
                                          <th scope="col" className="numeric">
                                            Reasoning
                                          </th>
                                          <th scope="col" className="numeric">
                                            Cost
                                          </th>
                                        </tr>
                                      </thead>
                                      <tbody>
                                        {(() => {
                                          const maxTokens = Math.max(...run.requests.map((item) => item.totalTokens), 0);
                                          const maxCost = Math.max(...run.requests.map((item) => item.cost), 0);
                                          return run.requests.map((request) => {
                                            const toolNames = request.toolNames ?? [];
                                            const toolSummaries = request.toolSummaries ?? [];
                                            const tokenRatio = getRatioPercent(request.totalTokens, maxTokens);
                                            const costRatio = getRatioPercent(request.cost, maxCost);
                                            const isMaxTokens = maxTokens > 0 && request.totalTokens === maxTokens;
                                            const isMaxCost = maxCost > 0 && request.cost === maxCost;
                                            return (
                                              <tr key={request.requestIndex}>
                                                <td>#{request.requestIndex}</td>
                                                <td>
                                                  <div className="ai-usage-request-kind">
                                                    <span>{formatRequestKind(request.kind)}</span>
                                                    {request.parentRequestIndex ? (
                                                      <small>parent #{request.parentRequestIndex}</small>
                                                    ) : null}
                                                    {toolSummaries.length > 0 ? (
                                                      <small>{toolSummaries.join(", ")}</small>
                                                    ) : toolNames.length > 0 ? (
                                                      <small>{toolNames.join(", ")}</small>
                                                    ) : null}
                                                  </div>
                                                </td>
                                                <td className="numeric strong">
                                                  <div className="ai-usage-meter-cell">
                                                    <span>
                                                      {formatCount(request.totalTokens)}
                                                      {isMaxTokens ? <b>max</b> : null}
                                                    </span>
                                                    <i style={{ "--meter-ratio": `${tokenRatio}%` } as CSSProperties} />
                                                  </div>
                                                </td>
                                                <td className="numeric">{formatCount(request.inputTokens)}</td>
                                                <td className="numeric">{formatCount(request.outputTokens)}</td>
                                                <td className="numeric">{formatCount(request.cachedInputTokens)}</td>
                                                <td className="numeric">{formatCount(request.reasoningOutputTokens)}</td>
                                                <td className="numeric strong">
                                                  <div className="ai-usage-meter-cell">
                                                    <span>
                                                      {formatCost(request.cost)}
                                                      {isMaxCost ? <b>max</b> : null}
                                                    </span>
                                                    <i style={{ "--meter-ratio": `${costRatio}%` } as CSSProperties} />
                                                  </div>
                                                </td>
                                              </tr>
                                            );
                                          });
                                        })()}
                                      </tbody>
                                    </table>
                                  ) : (
                                    <p className="message">request 明細は記録されていません。</p>
                                  )}
                                </details>
                              </td>
                            </tr>
                          </Fragment>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <p className="message">まだ読書AIの使用量は記録されていません。</p>
                )}
              </section>
            </>
          ) : null}

  </div>;
}

function formatCost(value: number): string {
  if (value <= 0) {
    return "$0";
  }

  return `$${value.toFixed(value >= 0.01 ? 4 : 6)}`;
}

function formatRunId(value: string): string {
  return value.length > 12 ? `${value.slice(0, 8)}...${value.slice(-4)}` : value;
}

function sanitizeFilePart(value: string): string {
  return value.replace(/[^a-z0-9._-]/gi, "_").slice(0, 80) || "unknown";
}

function getRatioPercent(value: number, max: number): number {
  if (value <= 0 || max <= 0) {
    return 0;
  }

  return Math.max(4, Math.min(100, Math.round((value / max) * 100)));
}

function formatRequestKind(kind: AiUsageRequestSummary["kind"]): string {
  switch (kind) {
    case "tool_call":
      return "tool call";
    case "handoff":
      return "handoff";
    case "final_answer":
      return "final answer";
    case "sub_request":
      return "sub request";
    default:
      return "other";
  }
}
