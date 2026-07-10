import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement, type ComponentProps } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import { AiGenerationWorkspace } from "../src/AiGenerationWorkspace";
import type { AiGenerationProfileDraft, AiGenerationSharedProviderDraft } from "../src/features/ai-generation/model";

type WorkspaceProps = ComponentProps<typeof AiGenerationWorkspace>;
type WorkspaceOverrides = Partial<WorkspaceProps> &
  Partial<WorkspaceProps["settingsViewProps"]> &
  Partial<WorkspaceProps["playgroundViewProps"]> &
  Partial<WorkspaceProps["jobsViewProps"]> &
  Partial<WorkspaceProps["usageViewProps"]>;

function createProfile(overrides: Partial<AiGenerationProfileDraft> = {}): AiGenerationProfileDraft {
  return {
    id: "default",
    label: "Default",
    provider: "openrouter",
    apiKeySource: "custom",
    apiKeyInput: "",
    apiKeyMasked: "sk-****",
    hasApiKey: true,
    credentialsUpdatedAt: "2026-03-22T10:00:00Z",
    modelId: "openai/gpt-5-mini",
    providerOrder: "openai,anthropic",
    allowFallbacks: true,
    requireParameters: false,
    ...overrides
  };
}

function createProps(overrides: WorkspaceOverrides = {}): WorkspaceProps {
  const defaultProfile = createProfile();
  const sharedOpenRouterDraft: AiGenerationSharedProviderDraft = {
    provider: "openrouter",
    apiKeyInput: "",
    apiKeyMasked: "sk-shared-****",
    hasApiKey: true,
    updatedAt: "2026-03-22T10:00:00Z"
  };
  const sharedGoogleBooksDraft: AiGenerationSharedProviderDraft = {
    provider: "googleBooks",
    apiKeyInput: "",
    apiKeyMasked: "gb-****",
    hasApiKey: true,
    updatedAt: "2026-03-22T10:00:00Z"
  };
  const secondaryProfile = createProfile({
    id: "profile-2",
    label: "Profile 2",
    hasApiKey: false,
    apiKeyMasked: null,
    modelId: "openai/gpt-5-nano",
    providerOrder: ""
  });

  return {
    activeView: overrides.activeView ?? "settings",
    aiGenerationSummaryLabel: overrides.aiGenerationSummaryLabel ?? "既定: Default",
    aiGenerationNotice: overrides.aiGenerationNotice ?? "AI機能設定を見直してください。",
    onOpenView: overrides.onOpenView ?? vi.fn(),
    onClose: overrides.onClose ?? vi.fn(),
    settingsViewProps: {
      aiGenerationSettingsError: "設定の読み込みに失敗しました。",
      aiGenerationPreferredMode: "llm",
      aiGenerationSettings: {
        masterPassphraseConfigured: false
      },
      isAiGenerationModeSaving: false,
      openAiGenerationHelpKey: "preferredMode",
      onToggleAiGenerationHelp: vi.fn(),
      onAiGenerationPreferredModeChange: vi.fn(),
      aiGenerationSharedOpenRouterDraft: sharedOpenRouterDraft,
      onUpdateAiGenerationSharedOpenRouterDraft: vi.fn(),
      aiGenerationSharedGoogleBooksDraft: sharedGoogleBooksDraft,
      onUpdateAiGenerationSharedGoogleBooksDraft: vi.fn(),
      aiGenerationProfileDrafts: [defaultProfile, secondaryProfile],
      extractionStrategyModelsDraft: {
        nameDiscoveryModelId: "openai/gpt-5-nano"
      },
      onSetExtractionStrategyModelsDraft: vi.fn(),
      defaultAiGenerationProfileDraft: defaultProfile,
      editingAiGenerationProfileId: defaultProfile.id,
      onSelectEditingAiGenerationProfile: vi.fn(),
      editingAiGenerationProfileDraft: defaultProfile,
      onUpdateAiGenerationProfileDraft: vi.fn(),
      onAddAiGenerationProfile: vi.fn(),
      onRemoveAiGenerationProfile: vi.fn(),
      selectedAiGenerationProfileId: defaultProfile.id,
      onSetSelectedAiGenerationProfileId: vi.fn(),
      isAiGenerationSettingsLoading: false,
      isAiGenerationSettingsSaving: false,
      onSaveAiGenerationSettings: vi.fn(),
      ...overrides,
      ...overrides.settingsViewProps
    },
    playgroundViewProps: {
      aiGenerationPlaygroundError: null,
      aiGenerationPlaygroundNovelId: "n1",
      onSetAiGenerationPlaygroundNovelId: vi.fn(),
      aiGenerationPlaygroundProfileId: defaultProfile.id,
      onSetAiGenerationPlaygroundProfileId: vi.fn(),
      aiGenerationPlaygroundUpToEpisodeIndex: "12",
      onSetAiGenerationPlaygroundUpToEpisodeIndex: vi.fn(),
      aiGenerationPlaygroundMaxEpisodeIndex: "20",
      isAiGenerationPlaygroundRunning: false,
      onRunAiGenerationPlayground: vi.fn(),
      novels: [
        {
          novelId: "n1",
          title: "小説A",
          totalEpisodes: 20
        }
      ],
      aiGenerationProfileDrafts: [defaultProfile, secondaryProfile],
      aiGenerationPlaygroundProgress: null,
      aiGenerationPlaygroundResult: null,
      aiGenerationPlaygroundPromptPreview: null,
      aiGenerationPlaygroundBatchTimings: [],
      aiGenerationPlaygroundResponseJson: "{}",
      ...overrides,
      ...overrides.playgroundViewProps
    },
    jobsViewProps: {
      aiGenerationJobsError: null,
      isAiGenerationJobsLoading: false,
      hasAiGenerationJobs: false,
      aiGenerationJobFilter: "active",
      onSetAiGenerationJobFilter: vi.fn(),
      aiGenerationActiveJobsCount: 1,
      aiGenerationFailedJobsCount: 0,
      aiGenerationCompletedJobsCount: 2,
      visibleAiGenerationJobs: [],
      onOpenNovelFromJob: vi.fn(),
      ...overrides,
      ...overrides.jobsViewProps
    },
    usageViewProps: {
      aiUsage: null,
      aiUsageError: null,
      isAiUsageLoading: false,
      ...overrides,
      ...overrides.usageViewProps
    }
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
  vi.stubGlobal("Node", dom.window.Node);
  vi.stubGlobal("Event", dom.window.Event);
  vi.stubGlobal("InputEvent", dom.window.InputEvent);
  vi.stubGlobal("MouseEvent", dom.window.MouseEvent);
  vi.stubGlobal("HTMLInputElement", dom.window.HTMLInputElement);
  vi.stubGlobal("HTMLButtonElement", dom.window.HTMLButtonElement);
  vi.stubGlobal("HTMLSelectElement", dom.window.HTMLSelectElement);
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);

  return dom;
}

async function renderWorkspace(props: WorkspaceProps): Promise<{ container: HTMLElement; root: Root; dom: JSDOM }> {
  const dom = installDom();
  const container = dom.window.document.getElementById("root");

  if (!container) {
    throw new Error("root container not found");
  }

  const root = createRoot(container);

  await act(async () => {
    root.render(createElement(AiGenerationWorkspace, props));
  });

  return { container, root, dom };
}

function getButtonByText(container: HTMLElement, text: string): HTMLButtonElement {
  const normalizedTarget = text.replace(/\s+/g, " ").trim();
  const button = Array.from(container.querySelectorAll("button")).find((candidate) => {
    const normalizedText = candidate.textContent?.replace(/\s+/g, " ").trim() ?? "";

    return normalizedText === normalizedTarget || normalizedText.includes(normalizedTarget);
  });

  if (!button) {
    throw new Error(`button not found: ${text}`);
  }

  return button as HTMLButtonElement;
}

function getInputByPlaceholder(container: HTMLElement, placeholder: string): HTMLInputElement {
  const input = Array.from(container.querySelectorAll("input")).find((candidate) => candidate.getAttribute("placeholder") === placeholder);

  if (!input) {
    throw new Error(`input not found: ${placeholder}`);
  }

  return input as HTMLInputElement;
}

function getButtonByAriaLabel(container: HTMLElement, label: string): HTMLButtonElement {
  const button = container.querySelector(`button[aria-label="${label}"]`);

  if (!(button instanceof HTMLButtonElement)) {
    throw new Error(`button not found: ${label}`);
  }

  return button;
}

async function click(element: Element, dom: JSDOM): Promise<void> {
  await act(async () => {
    element.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
  });
}

async function changeTextInput(input: HTMLInputElement, value: string, dom: JSDOM): Promise<void> {
  await act(async () => {
    const descriptor = Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value");
    descriptor?.set?.call(input, value);
    input.dispatchEvent(new dom.window.InputEvent("input", { bubbles: true, data: value }));
    input.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
  });
}

async function changeCheckbox(input: HTMLInputElement, checked: boolean, dom: JSDOM): Promise<void> {
  await act(async () => {
    const descriptor = Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "checked");
    descriptor?.set?.call(input, checked);
    input.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
    input.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
  });
}

async function changeSelect(select: HTMLSelectElement, value: string, dom: JSDOM): Promise<void> {
  await act(async () => {
    const descriptor = Object.getOwnPropertyDescriptor(dom.window.HTMLSelectElement.prototype, "value");
    descriptor?.set?.call(select, value);
    select.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
  });
}

async function submitForm(form: HTMLFormElement, dom: JSDOM): Promise<void> {
  await act(async () => {
    form.dispatchEvent(new dom.window.Event("submit", { bubbles: true, cancelable: true }));
  });
}

function getCheckboxes(container: HTMLElement): HTMLInputElement[] {
  return Array.from(container.querySelectorAll('input[type="checkbox"]')) as HTMLInputElement[];
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("AiGenerationWorkspace", () => {
  it("設定ビューを描画して主要ハンドラを呼び出す", async () => {
    const props = createProps({
      selectedAiGenerationProfileId: "profile-2"
    });
    const { container, root, dom } = await renderWorkspace(props);

    expect(container.textContent).toContain("AI機能");
    expect(container.textContent).toContain("`AI_GENERATION_SETTINGS_MASTER_PASSPHRASE` が未設定です。");
    expect(container.textContent).toContain("`LLM連携` は viewer-api 内の Go internal AI module から OpenRouter のモデルを使います。");

    await click(getButtonByText(container, "生成テスト"), dom);
    await click(getButtonByText(container, "閉じる"), dom);
    await click(getButtonByText(container, "ヒューリスティック"), dom);
    await click(getButtonByAriaLabel(container, "連携モードの説明を表示"), dom);
    await click(getButtonByText(container, "プロファイル追加"), dom);
    await click(getButtonByText(container, "Profile 2"), dom);
    await click(getButtonByAriaLabel(container, "APIキーの説明を表示"), dom);
    await click(getButtonByAriaLabel(container, "モデルの説明を表示"), dom);
    await click(getButtonByAriaLabel(container, "provider order の説明を表示"), dom);
    await click(getButtonByAriaLabel(container, "fallback の説明を表示"), dom);
    await click(getButtonByAriaLabel(container, "parameters 必須の説明を表示"), dom);
    expect(getInputByPlaceholder(container, "Default").value).toBe("Default");
    const passwordInputs = Array.from(container.querySelectorAll('input[type="password"]')) as HTMLInputElement[];
    await changeTextInput(passwordInputs.at(-1) as HTMLInputElement, "sk-test", dom);
    await changeTextInput(getInputByPlaceholder(container, "openai/gpt-5-mini"), "openai/gpt-5.4-mini", dom);
    await changeTextInput(getInputByPlaceholder(container, "openai,anthropic"), "openai,google", dom);
    const checkboxes = getCheckboxes(container);
    await changeCheckbox(checkboxes[0] as HTMLInputElement, false, dom);
    await changeCheckbox(checkboxes[1] as HTMLInputElement, true, dom);
    await click(getButtonByText(container, "このプロファイルを削除"), dom);
    await click(getButtonByText(container, "既定プロファイルに設定"), dom);

    const form = container.querySelector("form");
    if (!(form instanceof dom.window.HTMLFormElement)) {
      throw new Error("settings form not found");
    }
    await submitForm(form, dom);

    expect(props.onOpenView).toHaveBeenCalledWith("playground");
    expect(props.onClose).toHaveBeenCalledTimes(1);
    expect(props.settingsViewProps.onAiGenerationPreferredModeChange).toHaveBeenCalledWith("heuristic");
    expect(props.settingsViewProps.onToggleAiGenerationHelp).toHaveBeenCalledWith("preferredMode");
    expect(props.settingsViewProps.onAddAiGenerationProfile).toHaveBeenCalledTimes(1);
    expect(props.settingsViewProps.onSelectEditingAiGenerationProfile).toHaveBeenCalledWith("profile-2");
    expect(props.settingsViewProps.onRemoveAiGenerationProfile).toHaveBeenCalledWith("default");
    expect(props.settingsViewProps.onSetSelectedAiGenerationProfileId).toHaveBeenCalledWith("default");
    expect(props.settingsViewProps.onSaveAiGenerationSettings).toHaveBeenCalledTimes(1);

    await act(async () => {
      root.unmount();
    });
  });

  it("編集中プロファイルがないときは補助メッセージを出す", async () => {
    const props = createProps({
      editingAiGenerationProfileDraft: null,
      openAiGenerationHelpKey: null
    });
    const { container, root } = await renderWorkspace(props);

    expect(container.textContent).toContain("編集するプロファイルを選択してください。");

    await act(async () => {
      root.unmount();
    });
  });

  it("生成テストビューを描画して入力変更と実行を処理する", async () => {
    const props = createProps({
      activeView: "playground",
      aiGenerationPlaygroundError: "生成テストエラー",
      isAiGenerationPlaygroundRunning: true,
      aiGenerationPlaygroundProgress: {
        message: "生成中",
        progress: 45,
        step: 2,
        stepCount: 4,
        batchIndex: 1,
        batchCount: 3
      },
      aiGenerationPlaygroundResult: {
        novelTitle: "小説A",
        profileLabel: "Default",
        generationMode: "openrouter",
        modelId: "openai/gpt-5-mini",
        processedUpToEpisodeIndex: "12",
        characters: [
          {
            characterId: "c1",
            canonicalName: "主人公",
            fullName: "主人公 太郎",
            gender: "male",
            firstAppearanceEpisodeIndex: "1",
            aliases: [],
            appearance: "長身",
            personality: "冷静",
            summary: "主人公です。",
            importance: {
              category: "main",
              score: 0.91
            }
          }
        ],
        terms: [
          {
            term: "聖剣",
            reading: "せいけん",
            category: "item",
            description: "王家に伝わる剣。"
          }
        ]
      },
      aiGenerationPlaygroundPromptPreview: {
        systemPrompt: "system prompt",
        batches: [
          {
            batchIndex: 1,
            batchCount: 3,
            episodeIndexes: ["1", "2"],
            chunkCount: 1,
            chunks: [
              {
                episodeIndex: "1",
                title: "第一話",
                chapter: null,
                subchapter: null,
                chunkIndex: 1,
                chunkCount: 1,
                text: "本文"
              }
            ]
          }
        ]
      },
      aiGenerationPlaygroundBatchTimings: [
        {
          batchIndex: 1,
          batchCount: 3,
          episodeIndexes: ["1", "2"],
          chunkCount: 1,
          elapsedMs: 1234,
          generatedCharacterCount: 2,
          mergedCharacterCount: 1,
          message: "完了"
        }
      ],
      aiGenerationPlaygroundResponseJson: "{\"ok\":true}"
    });
    const { container, root, dom } = await renderWorkspace(props);

    expect(container.textContent).toContain("生成テストエラー");
    expect(container.textContent).toContain("生成中");
    expect(container.textContent).toContain("送信プロンプト");
    expect(container.textContent).toContain("batch処理時間");
    expect(container.textContent).toContain("主人公");
    expect(container.textContent).toContain("レスポンスJSON");

    const selects = container.querySelectorAll("select");
    await changeSelect(selects[0] as HTMLSelectElement, "n1", dom);
    await changeSelect(selects[1] as HTMLSelectElement, "profile-2", dom);

    const episodeInput = container.querySelector('input[type="number"]');
    if (!(episodeInput instanceof dom.window.HTMLInputElement)) {
      throw new Error("episode input not found");
    }
    await changeTextInput(episodeInput, "15", dom);

    const form = container.querySelector("form");
    if (!(form instanceof dom.window.HTMLFormElement)) {
      throw new Error("playground form not found");
    }
    await submitForm(form, dom);

    expect(props.playgroundViewProps.onSetAiGenerationPlaygroundNovelId).toHaveBeenCalledWith("n1");
    expect(props.playgroundViewProps.onSetAiGenerationPlaygroundProfileId).toHaveBeenCalledWith("profile-2");
    expect(props.playgroundViewProps.onRunAiGenerationPlayground).toHaveBeenCalledTimes(1);

    await act(async () => {
      root.unmount();
    });
  });

  it("生成テストで結果待ち状態と空作品時の無効化を表示する", async () => {
    const props = createProps({
      activeView: "playground",
      novels: [],
      aiGenerationPlaygroundNovelId: "",
      aiGenerationPlaygroundProfileId: "default",
      aiGenerationPlaygroundUpToEpisodeIndex: "",
      isAiGenerationPlaygroundRunning: false,
      aiGenerationPlaygroundResult: null,
      aiGenerationPlaygroundPromptPreview: {
        systemPrompt: "system prompt",
        batches: []
      },
      aiGenerationPlaygroundBatchTimings: []
    });
    const { container, root } = await renderWorkspace(props);

    expect(container.textContent).toContain("送信プロンプトと batch 進捗を表示しています。結果を待っています。");
    const submitButton = getButtonByText(container, "プレビュー実行");
    expect(submitButton.disabled).toBe(true);

    await act(async () => {
      root.unmount();
    });
  });

  it("キャラ生成履歴ビューを描画してフィルタ変更と作品オープンを処理する", async () => {
    const props = createProps({
      activeView: "jobs",
      aiGenerationJobsError: "キャラ生成履歴の取得に失敗しました。",
      hasAiGenerationJobs: true,
      visibleAiGenerationJobs: [
        {
          jobId: "job-1",
          novelId: "n1",
          novelTitle: "小説A",
          profileLabel: "Default",
          requestedUpToEpisodeIndex: "12",
          generationMode: "openrouter",
          modelId: "openai/gpt-5-mini",
          status: "failed",
          createdAt: "2026-03-22T10:00:00Z",
          startedAt: "2026-03-22T10:01:00Z",
          finishedAt: "2026-03-22T10:02:00Z",
          errorMessage: "rate limited"
        }
      ],
      aiGenerationJobFilter: "failed",
      aiGenerationActiveJobsCount: 2,
      aiGenerationFailedJobsCount: 1,
      aiGenerationCompletedJobsCount: 3
    });
    const { container, root, dom } = await renderWorkspace(props);

    expect(container.textContent).toContain("キャラ生成履歴の取得に失敗しました。");
    expect(container.textContent).toContain("小説A");
    expect(container.textContent).toContain("rate limited");

    await click(getButtonByText(container, "進行中 2"), dom);
    await click(getButtonByText(container, "失敗 1"), dom);
    await click(getButtonByText(container, "完了 3"), dom);
    await click(getButtonByText(container, "作品を開く"), dom);

    expect(props.jobsViewProps.onSetAiGenerationJobFilter).toHaveBeenNthCalledWith(1, "active");
    expect(props.jobsViewProps.onSetAiGenerationJobFilter).toHaveBeenNthCalledWith(2, "failed");
    expect(props.jobsViewProps.onSetAiGenerationJobFilter).toHaveBeenNthCalledWith(3, "completed");
    expect(props.jobsViewProps.onOpenNovelFromJob).toHaveBeenCalledWith("n1");

    await act(async () => {
      root.unmount();
    });
  });

  it("キャラ生成履歴が空なら状態に応じたメッセージを出す", async () => {
    const props = createProps({
      activeView: "jobs",
      aiGenerationJobFilter: "failed"
    });
    const { container, root } = await renderWorkspace(props);

    expect(container.textContent).toContain("失敗したキャラ生成はありません。");

    await act(async () => {
      root.unmount();
    });
  });

  it("読書AI利用統計ビューで usage と JSON ダウンロードを表示する", async () => {
    const fetchMock = vi.fn(async () =>
      new Response(JSON.stringify({ runId: "run-1234", snapshot: { ok: true } }), {
        status: 200,
        headers: {
          "content-type": "application/json"
        }
      })
    );
    const createObjectUrlMock = vi.fn(() => "blob:usage-json");
    const revokeObjectUrlMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("URL", {
      createObjectURL: createObjectUrlMock,
      revokeObjectURL: revokeObjectUrlMock
    });

    const props = createProps({
      activeView: "usage",
      aiUsage: {
        summary: {
          runCount: 1,
          requestCount: 2,
          inputTokens: 1500,
          outputTokens: 200,
          totalTokens: 1700,
          cachedInputTokens: 300,
          reasoningOutputTokens: 0,
          totalCost: 0.0012,
          averageTotalTokens: 1700
        },
        runs: [
          {
            runId: "run-1234",
            feature: "reader-assistant",
            workflowName: "reader-ai-assistant",
            status: "completed",
            startedAt: "2026-05-11T03:43:24.000Z",
            finishedAt: "2026-05-11T03:43:28.000Z",
            elapsedMs: 4000,
            novelId: "n1",
            novelTitle: "小説A",
            currentEpisodeIndex: "102",
            modelId: "openai/gpt-5-mini",
            profileLabel: "Default",
            generationMode: "remote",
            answerChars: 123,
            requestCount: 2,
            inputTokens: 1500,
            outputTokens: 200,
            totalTokens: 1700,
            cachedInputTokens: 300,
            reasoningOutputTokens: 0,
            totalCost: 0.0012,
            toolCallCount: 1,
            toolResultCount: 1,
            hasSnapshot: true,
            errorMessage: null,
            requests: [
              {
                requestIndex: 1,
                kind: "tool_call",
                parentRequestIndex: null,
                toolNames: ["summarize_episode_range"],
                toolSummaries: ["summarize_episode_range"],
                inputTokens: 700,
                outputTokens: 40,
                totalTokens: 740,
                cachedInputTokens: 100,
                reasoningOutputTokens: 0,
                cost: 0.0005
              },
              {
                requestIndex: 2,
                kind: "sub_request",
                parentRequestIndex: 1,
                toolNames: null as unknown as string[],
                toolSummaries: null as unknown as string[],
                inputTokens: 800,
                outputTokens: 160,
                totalTokens: 960,
                cachedInputTokens: 200,
                reasoningOutputTokens: 0,
                cost: 0
              }
            ]
          }
        ]
      }
    });
    const { container, root, dom } = await renderWorkspace(props);

    expect(container.textContent).toContain("読書AI利用統計");
    expect(container.textContent).toContain("Run run-1234 / JSON保存あり");
    expect(container.textContent).toContain("request ごとの費消状況");
    expect(container.textContent).not.toContain("run run-1234");
    expect(container.textContent).toContain("sub request");
    expect(container.textContent).toContain("parent #1");
    expect(container.querySelector('button[aria-label^="小説A "]')?.getAttribute("aria-label")).toContain("usage JSON をダウンロード");
    expect(container.querySelector("summary")?.getAttribute("aria-label")).toContain("小説A ");
    expect(container.querySelector("summary")?.getAttribute("aria-label")).toContain("request ごとの費消状況");
    expect(Array.from(container.querySelectorAll(".ai-usage-meter-cell i")).at(-1)?.getAttribute("style")).toContain("--meter-ratio: 0%");

    await click(getButtonByText(container, "JSON"), dom);

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/ai-generation/usage/run-1234",
      expect.objectContaining({
        headers: expect.any(Headers)
      })
    );
    const headers = fetchMock.mock.calls[0]?.[1]?.headers as Headers;
    expect(headers.get("x-narou-viewer-api-contract-version")).toBe("1");
    expect(createObjectUrlMock).toHaveBeenCalledTimes(1);
    expect(revokeObjectUrlMock).toHaveBeenCalledWith("blob:usage-json");

    await act(async () => {
      root.unmount();
    });
  });
});
