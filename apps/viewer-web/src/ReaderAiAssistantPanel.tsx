import { Fragment, type FormEvent, type KeyboardEvent, type ReactNode, useCallback, useEffect, useRef, useState } from "react";
import { postReaderAiAssistantChatStream, readReaderAiAssistantStream } from "./features/reader/api";
import type {
  ReaderAiAssistantHistoryMessage,
  ReaderAiAssistantResponse,
  ReaderAiAssistantStreamEvent
} from "./features/reader/types";
import { ReaderFloatingPanel } from "./ReaderFloatingPanel";

export type {
  ReaderAiAssistantResponse,
  ReaderAiAssistantToolRequest,
  ReaderAiAssistantToolResult
} from "./features/reader/types";

export type ReaderAiAssistantMessage = {
  createdAt: string;
  id: string;
  role: "user" | "assistant";
  text: string;
  turnId: string;
};

export type ReaderAiAssistantProgressEvent = Exclude<ReaderAiAssistantStreamEvent, { type: "result" | "error" }> & {
  occurredAt: string;
  sequence: number;
  turnId: string;
};

export type ReaderAiAssistantState = {
  draft: string;
  error: string | null;
  isSubmitting: boolean;
  lastResponse: ReaderAiAssistantResponse | null;
  messages: ReaderAiAssistantMessage[];
  progressEvents: ReaderAiAssistantProgressEvent[];
};

type ReaderAiAssistantStateUpdater = (updater: (current: ReaderAiAssistantState) => ReaderAiAssistantState) => void;

type Props = {
  assistantState?: ReaderAiAssistantState;
  currentEpisodeIndex: string | null;
  disabledReason?: string | null;
  formatEpisodeOrderLabel: (episodeIndex: string) => string;
  getCurrentPosition?: () => number | null;
  novelId: string | null;
  onAssistantStateChange?: ReaderAiAssistantStateUpdater;
  onClose: () => void;
};

const SUGGESTED_PROMPTS = ["直近5話の流れを要約して", "前話で何があった？"];

export function createEmptyReaderAiAssistantState(): ReaderAiAssistantState {
  return {
    draft: "",
    error: null,
    isSubmitting: false,
    lastResponse: null,
    messages: [],
    progressEvents: []
  };
}

export function ReaderAiAssistantPanel({
  assistantState: controlledAssistantState,
  currentEpisodeIndex,
  disabledReason = null,
  formatEpisodeOrderLabel,
  getCurrentPosition,
  novelId,
  onAssistantStateChange,
  onClose
}: Props) {
  const [localAssistantState, setLocalAssistantState] = useState<ReaderAiAssistantState>(() => createEmptyReaderAiAssistantState());
  const assistantState = controlledAssistantState ?? localAssistantState;
  const updateAssistantState = onAssistantStateChange ?? setLocalAssistantState;
  const { draft, error, isSubmitting, lastResponse, messages, progressEvents } = assistantState;
  const progressEventSequenceRef = useRef(0);
  const activeRequestRef = useRef<{
    controller: AbortController;
    requestId: string;
  } | null>(null);
  const readerContextRef = useRef({ currentEpisodeIndex, novelId });

  useEffect(() => {
    progressEventSequenceRef.current = Math.max(progressEventSequenceRef.current, getNextProgressEventSequence(progressEvents));
  }, [progressEvents]);

  const cancelActiveRequest = useCallback(
    (options: { removeTurn: boolean }) => {
      const activeRequest = activeRequestRef.current;
      if (!activeRequest) {
        return;
      }

      activeRequest.controller.abort();
      activeRequestRef.current = null;
      updateAssistantState((current) => ({
        ...current,
        isSubmitting: false,
        messages: options.removeTurn
          ? current.messages.filter((message) => message.turnId !== activeRequest.requestId)
          : current.messages,
        progressEvents: options.removeTurn
          ? current.progressEvents.filter((event) => event.turnId !== activeRequest.requestId)
          : current.progressEvents
      }));
    },
    [updateAssistantState]
  );

  useEffect(() => {
    if (
      readerContextRef.current.currentEpisodeIndex === currentEpisodeIndex &&
      readerContextRef.current.novelId === novelId
    ) {
      return;
    }

    readerContextRef.current = { currentEpisodeIndex, novelId };
    cancelActiveRequest({ removeTurn: true });
  }, [cancelActiveRequest, currentEpisodeIndex, novelId]);

  useEffect(
    () => () => {
      cancelActiveRequest({ removeTurn: true });
    },
    [cancelActiveRequest]
  );

  function updateDraft(nextDraft: string) {
    updateAssistantState((current) => ({
      ...current,
      draft: nextDraft
    }));
  }

  function updateMessages(updater: (current: ReaderAiAssistantMessage[]) => ReaderAiAssistantMessage[]) {
    updateAssistantState((current) => ({
      ...current,
      messages: updater(current.messages)
    }));
  }

  function updateProgressEvents(updater: (current: ReaderAiAssistantProgressEvent[]) => ReaderAiAssistantProgressEvent[]) {
    updateAssistantState((current) => ({
      ...current,
      progressEvents: updater(current.progressEvents)
    }));
  }

  function updateAssistantFields(fields: Partial<ReaderAiAssistantState>) {
    updateAssistantState((current) => ({
      ...current,
      ...fields
    }));
  }

  async function submitMessage(message: string) {
    const trimmed = message.trim();
    if (disabledReason || !novelId || !currentEpisodeIndex || trimmed.length === 0 || activeRequestRef.current || isSubmitting) {
      return;
    }

    let currentPosition = 0;
    try {
      const measuredPosition = getCurrentPosition?.() ?? 0;
      currentPosition = Number.isInteger(measuredPosition) && measuredPosition >= 0 ? measuredPosition : 0;
    } catch {
      currentPosition = 0;
    }

    const requestId = crypto.randomUUID();
    const controller = new AbortController();
    activeRequestRef.current = {
      controller,
      requestId
    };
    const userMessage: ReaderAiAssistantMessage = {
      createdAt: new Date().toISOString(),
      id: crypto.randomUUID(),
      role: "user",
      text: trimmed,
      turnId: requestId
    };
    const completedTurnIds = new Set(messages.filter((message) => message.role === "assistant").map((message) => message.turnId));
    const history: ReaderAiAssistantHistoryMessage[] = messages
      .filter((message) => completedTurnIds.has(message.turnId))
      .map(({ role, text }) => ({ role, text }));
    const isActiveRequest = () => activeRequestRef.current?.requestId === requestId;
    const updateActiveAssistantFields = (fields: Partial<ReaderAiAssistantState>) => {
      if (!isActiveRequest()) {
        return;
      }
      updateAssistantFields(fields);
    };
    updateMessages((current) => [...current, userMessage]);
    updateAssistantFields({
      draft: "",
      error: null,
      isSubmitting: true
    });

    try {
      const response = await postReaderAiAssistantChatStream(
        novelId,
        {
          message: trimmed,
          currentEpisodeIndex,
          position: currentPosition,
          history
        },
        controller.signal
      );
      if (!isActiveRequest()) {
        return;
      }

      const streamResult: { value?: ReaderAiAssistantResponse } = {};
      await readReaderAiAssistantStream(response, (event) => {
        if (event.type === "result") {
          streamResult.value = event.response;
          return;
        }

        if (event.type === "error") {
          throw new Error(event.error);
        }

        if (!isActiveRequest()) {
          return;
        }
        updateProgressEvents((current) => [
          ...current,
          {
            ...event,
            occurredAt: new Date().toISOString(),
            sequence: takeNextProgressEventSequence(progressEventSequenceRef, current),
            turnId: requestId
          }
        ]);
      });
      if (!isActiveRequest()) {
        return;
      }

      const finalResult = streamResult.value;
      if (!finalResult) {
        throw new Error("読書AIの応答取得に失敗しました。");
      }

      updateActiveAssistantFields({
        lastResponse: finalResult
      });
      updateMessages((current) => [
        ...current,
        {
          id: `assistant-${Date.now()}`,
          createdAt: new Date().toISOString(),
          role: "assistant",
          text: finalResult.answer,
          turnId: requestId
        }
      ]);
    } catch (submitError) {
      if (submitError instanceof Error && submitError.name === "AbortError") {
        return;
      }
      updateActiveAssistantFields({
        error: submitError instanceof Error ? submitError.message : "Unknown error"
      });
    } finally {
      updateActiveAssistantFields({
        isSubmitting: false
      });
      if (isActiveRequest()) {
        activeRequestRef.current = null;
      }
    }
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void submitMessage(draft);
  }

  function handleDraftKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (isReaderAiSubmitShortcut(event)) {
      event.preventDefault();
      void submitMessage(draft);
    }
  }

  function exportConversation() {
    const exportedAt = new Date().toISOString();
    const payload = {
      schemaVersion: 1,
      exportedAt,
      novelId,
      readingContext: {
        currentEpisodeIndex,
        currentEpisodeLabel: currentEpisodeIndex ? formatEpisodeOrderLabel(currentEpisodeIndex) : null,
        spoilerBoundaryEpisodeIndex: currentEpisodeIndex
      },
      messages: messages.map(({ createdAt, role, text, turnId }) => ({ createdAt, role, text, turnId })),
      timeline: buildReaderAiTimeline(messages, progressEvents),
      progressEvents,
      lastResponse,
      error
    };
    const blob = new Blob([`${JSON.stringify(payload, null, 2)}\n`], {
      type: "application/json"
    });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `reader-ai-chat-${sanitizeFileName(novelId ?? "unknown")}-${exportedAt.replace(/[:.]/g, "-")}.json`;
    link.click();
    URL.revokeObjectURL(url);
  }

  function resetConversation() {
    updateAssistantState(() => createEmptyReaderAiAssistantState());
  }

  const hasConversationState = messages.length > 0 || progressEvents.length > 0 || lastResponse !== null || error !== null || draft.length > 0;
  const isInputDisabled = Boolean(disabledReason) || !novelId || !currentEpisodeIndex || isSubmitting;

  return (
    <ReaderFloatingPanel
      ariaLabel="読書AI"
      className="reader-ai-panel reader-overlay-panel--ai"
      description={
        disabledReason
          ? disabledReason
          : currentEpisodeIndex
            ? `第${formatEpisodeOrderLabel(currentEpisodeIndex)}話までの情報だけを使います。`
            : "本文を開くと利用できます。"
      }
      onClose={onClose}
      title="読書AI"
    >
      <div className="reader-ai-panel-body">
        <div className="reader-panel-chip-row">
          <span className="reader-panel-chip">ネタバレ境界 第{currentEpisodeIndex ? formatEpisodeOrderLabel(currentEpisodeIndex) : "?"}話</span>
          <span className="reader-panel-chip">{disabledReason ? "LLM未設定" : lastResponse ? "AI応答" : "待機中"}</span>
        </div>

        {disabledReason ? (
          <section className="reader-panel-card reader-panel-card--compact reader-ai-unavailable-card">
            <p className="reader-panel-section-label">利用できません</p>
            <p>{disabledReason}</p>
          </section>
        ) : null}

        <div className="reader-actions reader-ai-debug-actions">
          <button disabled={messages.length === 0 && progressEvents.length === 0 && !lastResponse && !error} onClick={exportConversation} type="button">
            会話ログ
          </button>
          <button disabled={!hasConversationState || isSubmitting} onClick={resetConversation} type="button">
            新規会話
          </button>
        </div>

        {messages.length === 0 ? (
          <section className="reader-panel-card reader-panel-card--compact">
            <p className="reader-panel-section-label">質問例</p>
            <div className="reader-actions reader-ai-suggestions">
              {SUGGESTED_PROMPTS.map((prompt) => (
                <button
                  disabled={isInputDisabled}
                  key={prompt}
                  onClick={() => {
                    void submitMessage(prompt);
                  }}
                  type="button"
                >
                  {prompt}
                </button>
              ))}
            </div>
          </section>
        ) : null}

        <div aria-live="polite" className="reader-ai-messages">
          {buildReaderAiTurns(messages, progressEvents).map((turn) => (
            <section className="reader-ai-turn" key={turn.turnId}>
              {turn.messages
                .filter((message) => message.role === "user")
                .map((message) => (
                  <ReaderAiMessageArticle key={message.id} message={message} />
                ))}
              {turn.progressEvents.length > 0 ? (
                <ReaderAiProgressLog
                  isComplete={turn.messages.some((message) => message.role === "assistant")}
                  progressEvents={turn.progressEvents}
                />
              ) : null}
              {turn.messages
                .filter((message) => message.role === "assistant")
                .map((message) => (
                  <ReaderAiMessageArticle key={message.id} message={message} />
                ))}
            </section>
          ))}
          {isSubmitting ? <p className="message">読書AIが作品内を確認しています...</p> : null}
        </div>

        {error ? <p className="message error">{error}</p> : null}

        <form className="reader-ai-form" onSubmit={handleSubmit}>
          <textarea
            aria-label="読書AIへの質問"
            disabled={isInputDisabled}
            onChange={(event) => updateDraft(event.target.value)}
            onKeyDown={handleDraftKeyDown}
            placeholder="気になる人物、用語、状況を聞く"
            rows={3}
            value={draft}
          />
          <button
            className="reader-ai-submit-button"
            disabled={isInputDisabled || draft.trim().length === 0}
            type="submit"
          >
            <span>送信</span>
            <span className="reader-ai-submit-shortcut">Ctrl+Enter</span>
          </button>
        </form>
      </div>
    </ReaderFloatingPanel>
  );
}

export function isReaderAiSubmitShortcut(event: Pick<KeyboardEvent<HTMLTextAreaElement>, "ctrlKey" | "key">): boolean {
  return event.ctrlKey && event.key === "Enter";
}

function ReaderAiMessageArticle({ message }: { message: ReaderAiAssistantMessage }) {
  return (
    <article className={`reader-ai-message reader-ai-message--${message.role}`}>
      <p className="reader-panel-section-label">
        <span className="reader-ai-message-speaker">
          <ReaderAiMessageIcon role={message.role} />
          <span>{message.role === "user" ? "あなた" : "読書AI"}</span>
        </span>
        <time dateTime={message.createdAt}>{formatLogTime(message.createdAt)}</time>
      </p>
      {message.role === "assistant" ? <ReaderAiAssistantText text={message.text} /> : <p>{message.text}</p>}
    </article>
  );
}

function ReaderAiMessageIcon({ role }: { role: ReaderAiAssistantMessage["role"] }) {
  if (role === "user") {
    return (
      <span aria-hidden="true" className="reader-ai-message-icon reader-ai-message-icon--user">
        <svg aria-hidden="true" focusable="false" viewBox="0 0 24 24">
          <path d="M12 12.2a4.1 4.1 0 1 0 0-8.2 4.1 4.1 0 0 0 0 8.2Zm-7 7.05c0-3.12 3.14-5.65 7-5.65s7 2.53 7 5.65c0 .42-.34.75-.75.75H5.75a.75.75 0 0 1-.75-.75Z" />
        </svg>
      </span>
    );
  }

  return (
    <span aria-hidden="true" className="reader-ai-message-icon reader-ai-message-icon--assistant">
      <svg aria-hidden="true" focusable="false" viewBox="0 0 24 24">
        <path d="M11 3h2v2h-2V3ZM8.1 6h7.8A3.1 3.1 0 0 1 19 9.1v4.8a3.1 3.1 0 0 1-3.1 3.1h-1.2l-2.15 2.35a.75.75 0 0 1-1.1 0L9.3 17H8.1A3.1 3.1 0 0 1 5 13.9V9.1A3.1 3.1 0 0 1 8.1 6Zm.35 2A1.45 1.45 0 0 0 7 9.45v4.1A1.45 1.45 0 0 0 8.45 15h7.1A1.45 1.45 0 0 0 17 13.55v-4.1A1.45 1.45 0 0 0 15.55 8h-7.1Zm.9 3a1.15 1.15 0 1 1 2.3 0 1.15 1.15 0 0 1-2.3 0Zm5.3 0a1.15 1.15 0 1 0-2.3 0 1.15 1.15 0 0 0 2.3 0Zm-4.9 2.45h4.5v1.2h-4.5v-1.2Z" />
      </svg>
    </span>
  );
}

function ReaderAiAssistantText({ text }: { text: string }) {
  return <div className="reader-ai-message-content">{renderReaderAiMarkdown(text)}</div>;
}

function renderReaderAiMarkdown(text: string): ReactNode[] {
  const blocks: ReactNode[] = [];
  const listItems: ReactNode[] = [];

  function flushList() {
    if (listItems.length === 0) {
      return;
    }

    blocks.push(<ul key={`list-${blocks.length}`}>{listItems.splice(0, listItems.length)}</ul>);
  }

  for (const line of text.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (trimmed.length === 0) {
      flushList();
      continue;
    }

    const listMatch = /^[-*]\s+(.+)$/.exec(trimmed);
    if (listMatch) {
      listItems.push(<li key={`item-${blocks.length}-${listItems.length}`}>{renderReaderAiInlineMarkdown(listMatch[1])}</li>);
      continue;
    }

    flushList();
    blocks.push(<p key={`paragraph-${blocks.length}`}>{renderReaderAiInlineMarkdown(trimmed)}</p>);
  }

  flushList();
  return blocks;
}

function renderReaderAiInlineMarkdown(text: string): ReactNode[] {
  const nodes: ReactNode[] = [];
  const strongPattern = /\*\*([^*\n]+)\*\*/g;
  let lastIndex = 0;
  let match = strongPattern.exec(text);

  while (match !== null) {
    if (match.index > lastIndex) {
      nodes.push(<Fragment key={`text-${nodes.length}`}>{text.slice(lastIndex, match.index)}</Fragment>);
    }
    nodes.push(<strong key={`strong-${nodes.length}`}>{match[1]}</strong>);
    lastIndex = match.index + match[0].length;
    match = strongPattern.exec(text);
  }

  if (lastIndex < text.length) {
    nodes.push(<Fragment key={`text-${nodes.length}`}>{text.slice(lastIndex)}</Fragment>);
  }

  return nodes;
}

function ReaderAiProgressLog({
  isComplete,
  progressEvents
}: {
  isComplete: boolean;
  progressEvents: ReaderAiAssistantProgressEvent[];
}) {
  if (progressEvents.length === 0) {
    return null;
  }

  if (isComplete) {
    return (
      <details className="reader-ai-progress-details">
        <summary>実行ログ {progressEvents.length} 件</summary>
        <ReaderAiProgressList progressEvents={progressEvents} />
      </details>
    );
  }

  const latestEvent = progressEvents.at(-1);
  const previousEvents = progressEvents.slice(0, -1);

  return (
    <div className="reader-ai-progress-live">
      {latestEvent ? <ReaderAiProgressList ariaLabel="読書AIの最新実行ログ" progressEvents={[latestEvent]} /> : null}
      {previousEvents.length > 0 ? (
        <details className="reader-ai-progress-details">
          <summary>以前の実行ログ {previousEvents.length} 件</summary>
          <ReaderAiProgressList progressEvents={previousEvents} />
        </details>
      ) : null}
    </div>
  );
}

function ReaderAiProgressList({
  ariaLabel = "読書AIの実行ログ",
  progressEvents
}: {
  ariaLabel?: string;
  progressEvents: ReaderAiAssistantProgressEvent[];
}) {
  return (
    <ol aria-label={ariaLabel} className="reader-ai-progress">
      {progressEvents.map((event) => (
        <li
          className={`reader-ai-progress-item reader-ai-progress-item--${event.type}`}
          key={`${event.turnId}-${event.sequence}`}
        >
          <time dateTime={event.occurredAt}>{formatLogTime(event.occurredAt)}</time>
          <span>{formatProgressEvent(event)}</span>
        </li>
      ))}
    </ol>
  );
}

function getNextProgressEventSequence(progressEvents: ReaderAiAssistantProgressEvent[]): number {
  return progressEvents.reduce(
    (nextSequence, event) => (Number.isInteger(event.sequence) ? Math.max(nextSequence, event.sequence + 1) : nextSequence),
    0
  );
}

function takeNextProgressEventSequence(
  sequenceRef: { current: number },
  progressEvents: ReaderAiAssistantProgressEvent[]
): number {
  const nextSequence = Math.max(sequenceRef.current, getNextProgressEventSequence(progressEvents));
  sequenceRef.current = nextSequence + 1;
  return nextSequence;
}

function buildReaderAiTurns(messages: ReaderAiAssistantMessage[], progressEvents: ReaderAiAssistantProgressEvent[]) {
  const turnIds = Array.from(new Set([...messages.map((message) => message.turnId), ...progressEvents.map((event) => event.turnId)]));
  return turnIds.map((turnId) => ({
    turnId,
    messages: messages.filter((message) => message.turnId === turnId),
    progressEvents: progressEvents.filter((event) => event.turnId === turnId)
  }));
}

function buildReaderAiTimeline(messages: ReaderAiAssistantMessage[], progressEvents: ReaderAiAssistantProgressEvent[]) {
  return [
    ...messages.map((message) => ({
      kind: "message" as const,
      occurredAt: message.createdAt,
      role: message.role,
      text: message.text,
      turnId: message.turnId
    })),
    ...progressEvents.map((event) => ({
      kind: "progress" as const,
      ...event
    }))
  ].sort((left, right) => left.occurredAt.localeCompare(right.occurredAt));
}

function formatLogTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "";
  }

  return date.toLocaleTimeString("ja-JP", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit"
  });
}

function formatProgressEvent(event: ReaderAiAssistantProgressEvent): string {
  if (event.type === "tool_call") {
    return `${event.toolName}を実行: ${event.message}`;
  }

  if (event.type === "tool_result") {
    return `${event.toolName}が完了: ${event.message}`;
  }

  if (event.type === "status") {
    return event.message;
  }

  return "";
}

function sanitizeFileName(value: string): string {
  return value.replace(/[^A-Za-z0-9._-]+/g, "_").slice(0, 80) || "unknown";
}
