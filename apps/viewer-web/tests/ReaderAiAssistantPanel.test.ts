import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement, type ComponentProps } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import {
  ReaderAiAssistantPanel,
  createEmptyReaderAiAssistantState,
  isReaderAiSubmitShortcut
} from "../src/ReaderAiAssistantPanel";

type PanelProps = ComponentProps<typeof ReaderAiAssistantPanel>;

function createProps(overrides: Partial<PanelProps> = {}): PanelProps {
  return {
    currentEpisodeIndex: "2",
    previousEpisodeIndex: "1",
    formatEpisodeOrderLabel: (episodeIndex) => episodeIndex,
    getCurrentPosition: () => 123,
    novelId: "novel-a",
    onClose: vi.fn(),
    ...overrides
  };
}

function installDom(): JSDOM {
  const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
    url: "http://localhost/"
  });

  vi.stubGlobal("window", dom.window);
  vi.stubGlobal("document", dom.window.document);
  vi.stubGlobal("navigator", dom.window.navigator);
  vi.stubGlobal("HTMLElement", dom.window.HTMLElement);
  vi.stubGlobal("HTMLTextAreaElement", dom.window.HTMLTextAreaElement);
  vi.stubGlobal("HTMLButtonElement", dom.window.HTMLButtonElement);
  vi.stubGlobal("HTMLFormElement", dom.window.HTMLFormElement);
  vi.stubGlobal("Node", dom.window.Node);
  vi.stubGlobal("Event", dom.window.Event);
  vi.stubGlobal("KeyboardEvent", dom.window.KeyboardEvent);
  vi.stubGlobal("MouseEvent", dom.window.MouseEvent);
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);

  return dom;
}

async function renderPanel(props: PanelProps): Promise<{ container: HTMLElement; root: Root; dom: JSDOM }> {
  const dom = installDom();
  const container = dom.window.document.getElementById("root");

  if (!container) {
    throw new Error("root container not found");
  }

  const root = createRoot(container);

  await act(async () => {
    root.render(createElement(ReaderAiAssistantPanel, props));
  });

  return { container, root, dom };
}

function createNdjsonResponse(events: unknown[]): Response {
  const encoder = new TextEncoder();
  return new Response(
    new ReadableStream({
      start(controller) {
        for (const event of events) {
          controller.enqueue(encoder.encode(`${JSON.stringify(event)}\n`));
        }
        controller.close();
      }
    }),
    {
      status: 200,
      headers: {
        "content-type": "application/x-ndjson"
      }
    }
  );
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("ReaderAiAssistantPanel", () => {
  it("shows concise suggested prompts for resuming reading", async () => {
    const { container, root, dom } = await renderPanel(createProps());

    try {
      const checkbox = container.querySelector('input[type="checkbox"]');
      expect(checkbox).toBeInstanceOf(dom.window.HTMLInputElement);
      expect((checkbox as HTMLInputElement).checked).toBe(false);
      expect(container.textContent).toContain("新規参照上限 第1話");
      expect(container.textContent).toContain("直近5話の流れを要約して");
      expect(container.textContent).toContain("前話で何があった？");
      expect(container.textContent).toContain("Ctrl+Enter");
      expect(container.textContent).not.toContain("この人物は誰？");
      expect(container.textContent).not.toContain("前に出た場面を探して");
    } finally {
      await act(async () => {
        root.unmount();
      });
    }
  });

  it("disables reader assistant input when LLM settings are unavailable", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const { container, root, dom } = await renderPanel(
      createProps({
        disabledReason: "読書AIはLLM連携時のみ利用できます。"
      })
    );

    try {
      expect(container.textContent).toContain("利用できません");
      expect(container.textContent).toContain("読書AIはLLM連携時のみ利用できます。");
      const promptButton = Array.from(container.querySelectorAll("button")).find(
        (button) => button.textContent === "直近5話の流れを要約して"
      );
      const textarea = container.querySelector("textarea");
      const submitButton = container.querySelector(".reader-ai-submit-button");

      expect(promptButton?.hasAttribute("disabled")).toBe(true);
      expect(textarea?.hasAttribute("disabled")).toBe(true);
      expect(submitButton?.hasAttribute("disabled")).toBe(true);

      if (!(promptButton instanceof dom.window.HTMLButtonElement)) {
        throw new Error("prompt button not found");
      }

      await act(async () => {
        promptButton.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
      });

      expect(fetchMock).not.toHaveBeenCalled();
    } finally {
      await act(async () => {
        root.unmount();
      });
    }
  });

  it("submits reader assistant questions with the current reading context", async () => {
    const fetchMock = vi.fn(async () =>
      createNdjsonResponse([
        {
          type: "status",
          message: "AI機能の設定を確認しています。"
        },
        {
          type: "tool_call",
          toolName: "get_current_episode",
          message: "現在位置の本文を確認しています。"
        },
        {
          type: "result",
          response: {
            answer: "第2話までの情報だけを使って確認しました。",
            maxEpisodeIndex: "2",
            novelId: "novel-a",
            runId: null,
            toolRequests: [],
            generationMode: "remote",
            toolResults: [
              {
                name: "get_current_episode",
                result: {}
              }
            ]
          }
        }
      ])
    );
    vi.stubGlobal("fetch", fetchMock);

    const { container, root, dom } = await renderPanel(createProps());

    try {
      const checkbox = container.querySelector('input[type="checkbox"]');
      if (!(checkbox instanceof dom.window.HTMLInputElement)) {
        throw new Error("boundary checkbox not found");
      }
      await act(async () => checkbox.click());

      const promptButton = Array.from(container.querySelectorAll("button")).find(
        (button) => button.textContent === "直近5話の流れを要約して"
      );

      if (!(promptButton instanceof dom.window.HTMLButtonElement)) {
        throw new Error("prompt button not found");
      }

      await act(async () => {
        promptButton.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
      });

      expect(fetchMock).toHaveBeenCalledWith(
        "/api/library/novels/novel-a/reader-assistant/chat/stream",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({
            message: "直近5話の流れを要約して",
            currentEpisodeIndex: "2",
            position: 123,
            history: []
          })
        })
      );
      expect(container.textContent).toContain("AI機能の設定を確認しています。");
      expect(container.textContent).toContain("get_current_episodeを実行: 現在位置の本文を確認しています。");
      expect(container.textContent).toContain("実行ログ 2 件");
      expect(container.querySelector(".reader-ai-progress-details")?.hasAttribute("open")).toBe(false);
      expect(container.textContent).toContain("第2話までの情報だけを使って確認しました。");
    } finally {
      await act(async () => {
        root.unmount();
      });
    }
  });

  it("uses the previous episode as the boundary when the current episode is excluded", async () => {
    const fetchMock = vi.fn(async () =>
      createNdjsonResponse([
        {
          type: "result",
          response: {
            answer: "第1話までを確認しました。",
            maxEpisodeIndex: "1",
            novelId: "novel-a",
            runId: null,
            toolRequests: [],
            generationMode: "remote",
            toolResults: []
          }
        }
      ])
    );
    vi.stubGlobal("fetch", fetchMock);
    const { container, root, dom } = await renderPanel(createProps());

    try {
      expect(container.textContent).toContain("新規参照上限 第1話");

      const promptButton = Array.from(container.querySelectorAll("button")).find(
        (button) => button.textContent === "直近5話の流れを要約して"
      );
      if (!(promptButton instanceof dom.window.HTMLButtonElement)) {
        throw new Error("prompt button not found");
      }
      await act(async () => {
        promptButton.click();
      });

      expect(fetchMock).toHaveBeenCalledWith(
        "/api/library/novels/novel-a/reader-assistant/chat/stream",
        expect.objectContaining({
          body: JSON.stringify({
            message: "直近5話の流れを要約して",
            currentEpisodeIndex: "1",
            position: 0,
            history: []
          })
        })
      );
    } finally {
      await act(async () => root.unmount());
    }
  });

  it("disables questions when the current episode is excluded on episode one", async () => {
    const { container, root } = await renderPanel(
      createProps({ currentEpisodeIndex: "1", previousEpisodeIndex: null })
    );

    try {
      expect(container.textContent).toContain("第1話より前には参照できる話がありません。");
      expect(container.querySelector("textarea")?.hasAttribute("disabled")).toBe(true);
    } finally {
      await act(async () => root.unmount());
    }
  });

  it("assigns unique sequences to progress events from the same stream", async () => {
    const fetchMock = vi.fn(async () =>
      createNdjsonResponse([
        {
          type: "tool_call",
          toolName: "get_current_episode",
          message: "現在位置の本文を確認しています。"
        },
        {
          type: "tool_call",
          toolName: "get_current_episode",
          message: "現在位置の本文を確認しています。"
        },
        {
          type: "result",
          response: {
            answer: "確認しました。",
            maxEpisodeIndex: "2",
            novelId: "novel-a",
            runId: null,
            toolRequests: [],
            generationMode: "remote",
            toolResults: []
          }
        }
      ])
    );
    vi.stubGlobal("fetch", fetchMock);
    let assistantState = createEmptyReaderAiAssistantState();
    const onAssistantStateChange = vi.fn((updater: (current: typeof assistantState) => typeof assistantState) => {
      assistantState = updater(assistantState);
    });

    const { container, root, dom } = await renderPanel(
      createProps({
        assistantState,
        onAssistantStateChange
      })
    );

    try {
      const promptButton = Array.from(container.querySelectorAll("button")).find(
        (button) => button.textContent === "直近5話の流れを要約して"
      );

      if (!(promptButton instanceof dom.window.HTMLButtonElement)) {
        throw new Error("prompt button not found");
      }

      await act(async () => {
        promptButton.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
      });

      expect(assistantState.progressEvents).toHaveLength(2);
      expect(assistantState.progressEvents.map((event) => event.sequence)).toEqual([0, 1]);
      expect(new Set(assistantState.progressEvents.map((event) => `${event.turnId}-${event.sequence}`)).size).toBe(2);
    } finally {
      await act(async () => {
        root.unmount();
      });
    }
  });

  it("falls back to position zero when measuring the current reader position fails", async () => {
    const fetchMock = vi.fn(async () =>
      createNdjsonResponse([
        {
          type: "result",
          response: {
            answer: "位置なしで回答しました。",
            maxEpisodeIndex: "2",
            novelId: "novel-a",
            runId: null,
            toolRequests: [],
            generationMode: "remote",
            toolResults: []
          }
        }
      ])
    );
    vi.stubGlobal("fetch", fetchMock);

    const { container, root, dom } = await renderPanel(
      createProps({
        getCurrentPosition: () => {
          throw new Error("measurement failed");
        }
      })
    );

    try {
      const promptButton = Array.from(container.querySelectorAll("button")).find(
        (button) => button.textContent === "直近5話の流れを要約して"
      );

      if (!(promptButton instanceof dom.window.HTMLButtonElement)) {
        throw new Error("prompt button not found");
      }

      await act(async () => {
        promptButton.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
      });

      expect(fetchMock).toHaveBeenCalledWith(
        "/api/library/novels/novel-a/reader-assistant/chat/stream",
        expect.objectContaining({
          body: JSON.stringify({
            message: "直近5話の流れを要約して",
            currentEpisodeIndex: "1",
            position: 0,
            history: []
          })
        })
      );
      expect(container.textContent).toContain("位置なしで回答しました。");
    } finally {
      await act(async () => {
        root.unmount();
      });
    }
  });

  it("sends previous messages as reader assistant history for follow-up questions", async () => {
    const fetchMock = vi.fn(async () =>
      createNdjsonResponse([
        {
          type: "result",
          response: {
            answer: "追質問に回答しました。",
            maxEpisodeIndex: "2",
            novelId: "novel-a",
            runId: null,
            toolRequests: [],
            generationMode: "remote",
            toolResults: []
          }
        }
      ])
    );
    vi.stubGlobal("fetch", fetchMock);

    const { container, root, dom } = await renderPanel(
      createProps({
        assistantState: {
          ...createEmptyReaderAiAssistantState(),
          draft: "それは誰？",
          messages: [
            {
              createdAt: "2026-05-10T00:00:00.000Z",
              id: "user-1",
              role: "user",
              text: "アリスって誰？",
              turnId: "turn-1"
            },
            {
              createdAt: "2026-05-10T00:00:01.000Z",
              id: "assistant-1",
              role: "assistant",
              text: "第一話時点では案内役です。",
              turnId: "turn-1"
            },
            {
              createdAt: "2026-05-10T00:00:02.000Z",
              id: "user-2",
              role: "user",
              text: "未回答の質問",
              turnId: "turn-2"
            }
          ]
        }
      })
    );

    try {
      const form = container.querySelector("form");
      if (!(form instanceof dom.window.HTMLFormElement)) {
        throw new Error("form not found");
      }

      await act(async () => {
        form.dispatchEvent(new dom.window.Event("submit", { bubbles: true, cancelable: true }));
      });

      expect(fetchMock).toHaveBeenCalledWith(
        "/api/library/novels/novel-a/reader-assistant/chat/stream",
        expect.objectContaining({
          body: JSON.stringify({
            message: "それは誰？",
            currentEpisodeIndex: "1",
            position: 0,
            history: [
              {
                role: "user",
                text: "アリスって誰？"
              },
              {
                role: "assistant",
                text: "第一話時点では案内役です。"
              }
            ]
          })
        })
      );
    } finally {
      await act(async () => {
        root.unmount();
      });
    }
  });

  it("does not start duplicate reader assistant requests while one is active", async () => {
    let resolveResponse: ((response: Response) => void) | null = null;
    const fetchMock = vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          resolveResponse = resolve;
        })
    );
    vi.stubGlobal("fetch", fetchMock);
    const { container, root, dom } = await renderPanel(createProps());

    try {
      const promptButton = Array.from(container.querySelectorAll("button")).find(
        (button) => button.textContent === "直近5話の流れを要約して"
      );
      if (!(promptButton instanceof dom.window.HTMLButtonElement)) {
        throw new Error("prompt button not found");
      }

      await act(async () => {
        promptButton.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
        promptButton.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
      });

      expect(fetchMock).toHaveBeenCalledTimes(1);
      await act(async () => {
        resolveResponse?.(
          createNdjsonResponse([
            {
              type: "result",
              response: {
                answer: "回答",
                maxEpisodeIndex: "2",
                novelId: "novel-a",
                runId: null,
                toolRequests: [],
                generationMode: "remote",
                toolResults: []
              }
            }
          ])
        );
      });
    } finally {
      await act(async () => {
        root.unmount();
      });
    }
  });

  it("aborts an active reader assistant request when the reading context changes", async () => {
    let capturedSignal: AbortSignal | null = null;
    const fetchMock = vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
      capturedSignal = init?.signal ?? null;
      return new Promise<Response>(() => undefined);
    });
    vi.stubGlobal("fetch", fetchMock);
    let assistantState = createEmptyReaderAiAssistantState();
    const onAssistantStateChange = vi.fn((updater: (current: typeof assistantState) => typeof assistantState) => {
      assistantState = updater(assistantState);
    });
    const { container, root, dom } = await renderPanel(
      createProps({
        assistantState,
        onAssistantStateChange
      })
    );

    try {
      const promptButton = Array.from(container.querySelectorAll("button")).find(
        (button) => button.textContent === "直近5話の流れを要約して"
      );
      if (!(promptButton instanceof dom.window.HTMLButtonElement)) {
        throw new Error("prompt button not found");
      }

      await act(async () => {
        promptButton.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
      });
      expect(capturedSignal?.aborted).toBe(false);

      await act(async () => {
        root.render(
          createElement(
            ReaderAiAssistantPanel,
            createProps({
              assistantState,
              currentEpisodeIndex: "1",
              novelId: "novel-b",
              onAssistantStateChange
            })
          )
        );
      });

      expect(capturedSignal?.aborted).toBe(true);
      expect(assistantState.isSubmitting).toBe(false);
      expect(assistantState.messages).toEqual([]);
    } finally {
      await act(async () => {
        root.unmount();
      });
    }
  });

  it("preserves completed conversation when moving within the same novel", async () => {
    let assistantState = {
      ...createEmptyReaderAiAssistantState(),
      messages: [
        {
          createdAt: "2026-05-10T00:00:00.000Z",
          id: "user-1",
          role: "user" as const,
          spoilerBoundaryEpisodeIndex: "1",
          text: "ここまでの流れは？",
          turnId: "turn-1"
        },
        {
          createdAt: "2026-05-10T00:00:01.000Z",
          id: "assistant-1",
          role: "assistant" as const,
          spoilerBoundaryEpisodeIndex: "1",
          text: "第1話までの流れです。",
          turnId: "turn-1"
        }
      ]
    };
    const onAssistantStateChange = vi.fn((updater: (current: typeof assistantState) => typeof assistantState) => {
      assistantState = updater(assistantState);
    });
    const { container, root } = await renderPanel(
      createProps({
        assistantState,
        onAssistantStateChange
      })
    );

    try {
      await act(async () => {
        root.render(
          createElement(
            ReaderAiAssistantPanel,
            createProps({
              assistantState,
              currentEpisodeIndex: "3",
              previousEpisodeIndex: "2",
              onAssistantStateChange
            })
          )
        );
      });

      expect(assistantState.messages).toHaveLength(2);
      expect(container.textContent).toContain("第1話までの流れです。");
      expect(container.textContent).toContain("参照上限 第1話");
      expect(container.textContent).toContain("新規参照上限 第2話");
    } finally {
      await act(async () => root.unmount());
    }
  });

  it("preserves conversation when changing the new-reference boundary", async () => {
    let assistantState = {
      ...createEmptyReaderAiAssistantState(),
      messages: [
        {
          createdAt: "2026-05-10T00:00:00.000Z",
          id: "assistant-1",
          role: "assistant" as const,
          spoilerBoundaryEpisodeIndex: "1",
          text: "残す回答",
          turnId: "turn-1"
        }
      ]
    };
    const onAssistantStateChange = vi.fn((updater: (current: typeof assistantState) => typeof assistantState) => {
      assistantState = updater(assistantState);
    });
    const { container, root, dom } = await renderPanel(
      createProps({
        assistantState,
        onAssistantStateChange
      })
    );

    try {
      const checkbox = container.querySelector('input[type="checkbox"]');
      if (!(checkbox instanceof dom.window.HTMLInputElement)) {
        throw new Error("boundary checkbox not found");
      }
      await act(async () => checkbox.click());

      expect(assistantState.includeCurrentEpisode).toBe(true);
      expect(assistantState.messages).toHaveLength(1);
      expect(assistantState.messages[0]?.text).toBe("残す回答");
    } finally {
      await act(async () => root.unmount());
    }
  });

  it("aborts an active reader assistant request when the panel unmounts", async () => {
    let capturedSignal: AbortSignal | null = null;
    const fetchMock = vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
      capturedSignal = init?.signal ?? null;
      return new Promise<Response>(() => undefined);
    });
    vi.stubGlobal("fetch", fetchMock);
    let assistantState = createEmptyReaderAiAssistantState();
    const onAssistantStateChange = vi.fn((updater: (current: typeof assistantState) => typeof assistantState) => {
      assistantState = updater(assistantState);
    });
    const { container, root, dom } = await renderPanel(
      createProps({
        assistantState,
        onAssistantStateChange
      })
    );

    const promptButton = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent === "直近5話の流れを要約して"
    );
    if (!(promptButton instanceof dom.window.HTMLButtonElement)) {
      throw new Error("prompt button not found");
    }

    await act(async () => {
      promptButton.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
    });
    expect(capturedSignal?.aborted).toBe(false);

    await act(async () => {
      root.unmount();
    });

    expect(capturedSignal?.aborted).toBe(true);
    expect(assistantState.isSubmitting).toBe(false);
    expect(assistantState.messages).toEqual([]);
  });

  it("shows reader assistant API error details", async () => {
    const fetchMock = vi.fn(async () =>
      createNdjsonResponse([
        {
          type: "error",
          error: "読書AIは未設定です。OPENROUTER_API_KEY と LLM_MODEL_ID を設定してください。"
        }
      ])
    );
    vi.stubGlobal("fetch", fetchMock);

    const { container, root, dom } = await renderPanel(createProps());

    try {
      const promptButton = Array.from(container.querySelectorAll("button")).find(
        (button) => button.textContent === "直近5話の流れを要約して"
      );

      if (!(promptButton instanceof dom.window.HTMLButtonElement)) {
        throw new Error("prompt button not found");
      }

      await act(async () => {
        promptButton.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
      });

      expect(container.textContent).toContain("読書AIは未設定です。OPENROUTER_API_KEY と LLM_MODEL_ID を設定してください。");
    } finally {
      await act(async () => {
        root.unmount();
      });
    }
  });

  it("treats Ctrl+Enter as the submit shortcut", () => {
    expect(isReaderAiSubmitShortcut({ ctrlKey: true, key: "Enter" })).toBe(true);
    expect(isReaderAiSubmitShortcut({ ctrlKey: false, key: "Enter" })).toBe(false);
    expect(isReaderAiSubmitShortcut({ ctrlKey: true, key: "a" })).toBe(false);
  });

  it("can preserve conversation state after the panel is unmounted", async () => {
    const fetchMock = vi.fn(async () =>
      createNdjsonResponse([
        {
          type: "result",
          response: {
            answer: "閉じても残る回答です。",
            maxEpisodeIndex: "2",
            novelId: "novel-a",
            runId: null,
            toolRequests: [],
            generationMode: "remote",
            toolResults: []
          }
        }
      ])
    );
    vi.stubGlobal("fetch", fetchMock);

    let assistantState = createEmptyReaderAiAssistantState();
    const onAssistantStateChange = vi.fn((updater: (current: typeof assistantState) => typeof assistantState) => {
      assistantState = updater(assistantState);
    });

    const firstRender = await renderPanel(
      createProps({
        assistantState,
        onAssistantStateChange
      })
    );

    try {
      const promptButton = Array.from(firstRender.container.querySelectorAll("button")).find(
        (button) => button.textContent === "直近5話の流れを要約して"
      );

      if (!(promptButton instanceof firstRender.dom.window.HTMLButtonElement)) {
        throw new Error("prompt button not found");
      }

      await act(async () => {
        promptButton.dispatchEvent(new firstRender.dom.window.MouseEvent("click", { bubbles: true }));
      });
    } finally {
      await act(async () => {
        firstRender.root.unmount();
      });
    }

    const secondRender = await renderPanel(
      createProps({
        assistantState,
        onAssistantStateChange
      })
    );

    try {
      expect(secondRender.container.textContent).toContain("直近5話の流れを要約して");
      expect(secondRender.container.textContent).toContain("閉じても残る回答です。");
    } finally {
      await act(async () => {
        secondRender.root.unmount();
      });
    }
  });

  it("can reset reader assistant conversation state", async () => {
    let assistantState = {
      ...createEmptyReaderAiAssistantState(),
      draft: "残っている下書き",
      error: "残っているエラー",
      lastResponse: {
        answer: "古い回答",
        novelId: "novel-a",
        maxEpisodeIndex: "2",
        runId: null,
        toolRequests: [],
        generationMode: "remote" as const,
        toolResults: []
      },
      messages: [
        {
          createdAt: "2026-05-10T00:00:00.000Z",
          id: "message-1",
          role: "user" as const,
          text: "古い質問",
          turnId: "turn-1"
        }
      ]
    };
    const onAssistantStateChange = vi.fn((updater: (current: typeof assistantState) => typeof assistantState) => {
      assistantState = updater(assistantState);
    });
    const { container, root, dom } = await renderPanel(
      createProps({
        assistantState,
        onAssistantStateChange
      })
    );

    try {
      const resetButton = Array.from(container.querySelectorAll("button")).find((button) => button.textContent === "新規会話");
      if (!(resetButton instanceof dom.window.HTMLButtonElement)) {
        throw new Error("reset button not found");
      }

      await act(async () => {
        resetButton.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
      });

      expect(assistantState).toEqual(createEmptyReaderAiAssistantState());
    } finally {
      await act(async () => {
        root.unmount();
      });
    }
  });

  it("renders common markdown in assistant messages", async () => {
    const { container, root } = await renderPanel(
      createProps({
        assistantState: {
          ...createEmptyReaderAiAssistantState(),
          messages: [
            {
              createdAt: "2026-05-10T00:00:00.000Z",
              id: "assistant-1",
              role: "assistant",
              text: "- **第1話**：カップ麺の場面\n- **第2話**：風呂の場面",
              turnId: "turn-1"
            }
          ]
        }
      })
    );

    try {
      expect(container.textContent).toContain("第1話：カップ麺の場面");
      expect(container.textContent).not.toContain("**第1話**");
      expect(container.querySelectorAll(".reader-ai-message-content li")).toHaveLength(2);
      expect(container.querySelector(".reader-ai-message-content strong")?.textContent).toBe("第1話");
    } finally {
      await act(async () => {
        root.unmount();
      });
    }
  });

  it("shows speaker icons for user and assistant messages", async () => {
    const { container, root } = await renderPanel(
      createProps({
        assistantState: {
          ...createEmptyReaderAiAssistantState(),
          messages: [
            {
              createdAt: "2026-05-10T00:00:00.000Z",
              id: "user-1",
              role: "user",
              text: "質問",
              turnId: "turn-1"
            },
            {
              createdAt: "2026-05-10T00:00:01.000Z",
              id: "assistant-1",
              role: "assistant",
              text: "回答",
              turnId: "turn-1"
            }
          ]
        }
      })
    );

    try {
      expect(container.querySelector(".reader-ai-message-icon--user")).not.toBeNull();
      expect(container.querySelector(".reader-ai-message-icon--assistant")).not.toBeNull();
    } finally {
      await act(async () => {
        root.unmount();
      });
    }
  });

  it("exports reader assistant conversation logs for debugging", async () => {
    const fetchMock = vi.fn(async () =>
      createNdjsonResponse([
        {
          type: "status",
          message: "AI機能の設定を確認しています。"
        },
        {
          type: "tool_result",
          toolName: "get_current_episode",
          message: "現在位置の本文を確認しました。"
        },
        {
          type: "result",
          response: {
            answer: "第2話までの情報だけを使って確認しました。",
            maxEpisodeIndex: "2",
            novelId: "novel-a",
            runId: null,
            toolRequests: [],
            generationMode: "remote",
            toolResults: [
              {
                name: "get_current_episode",
                result: {
                  excerpt: "本文断片"
                }
              }
            ]
          }
        }
      ])
    );
    vi.stubGlobal("fetch", fetchMock);

    const { container, root, dom } = await renderPanel(createProps());
    let exportedBlob: Blob | null = null;
    const createObjectUrl = vi.fn((blob: Blob) => {
      exportedBlob = blob;
      return "blob:reader-ai-log";
    });
    const revokeObjectUrl = vi.fn();
    vi.stubGlobal("URL", {
      ...URL,
      createObjectURL: createObjectUrl,
      revokeObjectURL: revokeObjectUrl
    });
    const clickSpy = vi.spyOn(dom.window.HTMLAnchorElement.prototype, "click").mockImplementation(() => undefined);

    try {
      const promptButton = Array.from(container.querySelectorAll("button")).find(
        (button) => button.textContent === "直近5話の流れを要約して"
      );

      if (!(promptButton instanceof dom.window.HTMLButtonElement)) {
        throw new Error("prompt button not found");
      }

      await act(async () => {
        promptButton.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
      });

      const exportButton = Array.from(container.querySelectorAll("button")).find((button) => button.textContent === "会話ログ");
      if (!(exportButton instanceof dom.window.HTMLButtonElement)) {
        throw new Error("export button not found");
      }

      exportButton.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));

      expect(clickSpy).toHaveBeenCalled();
      expect(createObjectUrl).toHaveBeenCalled();
      expect(revokeObjectUrl).toHaveBeenCalledWith("blob:reader-ai-log");
      expect(exportedBlob).not.toBeNull();
      const exported = JSON.parse(await exportedBlob.text()) as {
        schemaVersion: number;
        novelId: string;
        readingContext: { currentEpisodeIndex: string };
        messages: Array<{ role: string; text: string }>;
        progressEvents: Array<{ type: string; message: string }>;
        lastResponse: { answer: string; toolResults: Array<{ name: string }> };
      };

      expect(exported).toMatchObject({
        schemaVersion: 1,
        novelId: "novel-a",
        readingContext: {
          currentEpisodeIndex: "2"
        },
        messages: [
          {
            role: "user",
            text: "直近5話の流れを要約して"
          },
          {
            role: "assistant",
            text: "第2話までの情報だけを使って確認しました。"
          }
        ],
        lastResponse: {
          answer: "第2話までの情報だけを使って確認しました。",
          toolResults: [
            {
              name: "get_current_episode"
            }
          ]
        }
      });
      expect(exported.progressEvents).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            type: "status",
            message: "AI機能の設定を確認しています。"
          })
        ])
      );
    } finally {
      await act(async () => {
        root.unmount();
      });
    }
  });

  it("disables input until a reader episode is available", async () => {
    const { container, root } = await renderPanel(
      createProps({
        currentEpisodeIndex: null,
        novelId: null
      })
    );
    const textarea = container.querySelector("textarea");
    const submitButton = container.querySelector(".reader-ai-submit-button");

    expect(textarea).toBeInstanceOf(HTMLTextAreaElement);
    expect(textarea?.hasAttribute("disabled")).toBe(true);
    expect(submitButton?.hasAttribute("disabled")).toBe(true);

    await act(async () => {
      root.unmount();
    });
  });
});
