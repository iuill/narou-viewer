import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement, type ComponentProps } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import { ReaderCharacterSummaryPanel } from "../src/ReaderCharacterSummaryPanel";

type PanelProps = ComponentProps<typeof ReaderCharacterSummaryPanel>;

function createProps(overrides: Partial<PanelProps> = {}): PanelProps {
  return {
    activeJobs: [],
    canClear: true,
    canGenerate: true,
    completedJobs: [],
    data: null,
    defaultUpToEpisodeIndex: "12",
    error: null,
    formatEpisodeOrderLabel: (episodeIndex) => episodeIndex,
    isClearing: false,
    includeCurrentEpisode: false,
    isLoading: false,
    isSubmitting: false,
    notice: null,
    onClear: vi.fn(),
    onIncludeCurrentEpisodeChange: vi.fn(),
    onClose: vi.fn(),
    onRequestedGenerationStrategyChange: vi.fn(),
    onRequestedUpToEpisodeIndexChange: vi.fn(),
    onShowTerms: vi.fn(),
    onSubmit: vi.fn(),
    requestedGenerationStrategy: "parallel_identity",
    requestedUpToEpisodeIndex: "12",
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
  vi.stubGlobal("HTMLInputElement", dom.window.HTMLInputElement);
  vi.stubGlobal("HTMLButtonElement", dom.window.HTMLButtonElement);
  vi.stubGlobal("Node", dom.window.Node);
  vi.stubGlobal("Event", dom.window.Event);
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
    root.render(createElement(ReaderCharacterSummaryPanel, props));
  });

  return { container, root, dom };
}

function getButtonByText(container: HTMLElement, text: string): HTMLButtonElement {
  const normalizedTarget = text.replace(/\s+/g, " ").trim();
  const button = Array.from(container.querySelectorAll("button")).find((candidate) => {
    const normalizedText = candidate.textContent?.replace(/\s+/g, " ").trim() ?? "";

    return normalizedText === normalizedTarget || normalizedText.includes(normalizedTarget);
  });

  if (!(button instanceof HTMLButtonElement)) {
    throw new Error(`button not found: ${text}`);
  }

  return button;
}

async function click(element: Element, dom: JSDOM): Promise<void> {
  await act(async () => {
    element.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
  });
}

async function submitForm(form: HTMLFormElement, dom: JSDOM): Promise<void> {
  await act(async () => {
    form.dispatchEvent(new dom.window.Event("submit", { bubbles: true, cancelable: true }));
  });
}

async function changeSelect(select: HTMLSelectElement, value: string, dom: JSDOM): Promise<void> {
  await act(async () => {
    const descriptor = Object.getOwnPropertyDescriptor(dom.window.HTMLSelectElement.prototype, "value");
    descriptor?.set?.call(select, value);
    select.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("ReaderCharacterSummaryPanel", () => {
  it("ready 状態の一覧とジョブを描画して主要操作を受ける", async () => {
    const props = createProps({
      activeJobs: [
        {
          jobId: "job-running",
          requestedUpToEpisodeIndex: "12",
          generationStrategy: "parallel_identity",
          status: "running",
          progress: 62,
          progressStage: "batch",
          currentBatchIndex: 2,
          batchCount: 4,
          completedBatchCount: 2,
          generatedCharacterCount: 5,
          createdAt: "2026-03-22T10:00:00Z",
          errorMessage: null
        }
      ],
      completedJobs: [
        {
          jobId: "job-failed",
          requestedUpToEpisodeIndex: "8",
          generationStrategy: "serial",
          status: "failed",
          createdAt: "2026-03-22T09:00:00Z",
          errorMessage: "timeout"
        }
      ],
      data: {
        status: "partial",
        upToEpisodeIndex: "12",
        processedUpToEpisodeIndex: "11",
        characters: [
          {
            characterId: "c1",
            canonicalName: "主人公",
            fullName: "主人公 太郎",
            gender: "male",
            firstAppearanceEpisodeIndex: "1",
            aliases: ["タロウ"],
            appearance: "長身",
            personality: "冷静",
            summary: "主人公です。",
            importance: {
              category: "main",
              score: 0.912
            }
          },
          {
            characterId: "c2",
            canonicalName: "相棒",
            fullName: null,
            gender: null,
            firstAppearanceEpisodeIndex: "2",
            aliases: [],
            appearance: null,
            personality: null,
            summary: "同行する仲間です。",
            importance: {
              category: "regular",
              score: 0.441
            }
          }
        ]
      },
      notice:
        "第11話時点までの生成済み一覧を表示しています。第12話時点の一覧はまだ生成されていません。"
    });
    const { container, root, dom } = await renderPanel(props);

    expect(container.textContent).toContain("人物・用語一覧");
    expect(container.textContent).toContain("第12話時点までの情報を確認します。");
    expect(container.textContent).toContain(
      "第11話時点までの生成済み一覧を表示しています。第12話時点の一覧はまだ生成されていません。"
    );
    expect(container.textContent).toContain("主人公");
    expect(container.textContent).toContain("メインキャラ");
    expect(container.textContent).toContain("初登場第1話");
    expect(container.textContent).toContain("進行中の生成");
    expect(container.textContent).toContain("並列抽出 + 人物・用語統合");
    expect(container.textContent).toContain("事前発見 + 並列抽出 + 補正");
    expect(container.textContent).toContain("人物・用語一覧を生成中");
    expect(container.textContent).toContain("全体 62% / batch 完了 2/4 / 5 人まで反映");
    expect(container.textContent).toContain("過去の生成履歴");
    expect(container.textContent).toContain("順次抽出");
    expect(container.textContent).toContain("timeout");

    const form = container.querySelector("form");
    if (!(form instanceof dom.window.HTMLFormElement)) {
      throw new Error("form not found");
    }

    await submitForm(form, dom);
    const termsTab = container.querySelector(".reader-extraction-tabs button:last-child");
    if (!(termsTab instanceof dom.window.HTMLButtonElement)) {
      throw new Error("terms tab not found");
    }
    await click(termsTab, dom);
    await click(getButtonByText(container, "×"), dom);

    expect(props.onSubmit).toHaveBeenCalledTimes(1);
    expect(props.onShowTerms).toHaveBeenCalledTimes(1);
    expect(props.onClose).toHaveBeenCalledTimes(1);

    await act(async () => {
      root.unmount();
    });
  });

  it("抽出方式の変更を通知する", async () => {
    const onRequestedGenerationStrategyChange = vi.fn();
    const props = createProps({ onRequestedGenerationStrategyChange });
    const { container, root, dom } = await renderPanel(props);
    const select = Array.from(container.querySelectorAll("select")).find((candidate) =>
      candidate.textContent?.includes("順次抽出")
    );

    if (!(select instanceof dom.window.HTMLSelectElement)) {
      throw new Error("generation strategy select not found");
    }

    await changeSelect(select, "discovery_parallel_correction", dom);

    expect(onRequestedGenerationStrategyChange).toHaveBeenCalledWith("discovery_parallel_correction");

    await act(async () => {
      root.unmount();
    });
  });

  it("通知とエラーを同時に表示できる", async () => {
    const props = createProps({
      error: "キャラクター一覧の再取得に失敗しました。",
      notice: "第11話時点までの生成済み一覧を表示しています。"
    });
    const { container, root } = await renderPanel(props);

    expect(container.textContent).toContain("キャラクター一覧の再取得に失敗しました。");
    expect(container.textContent).toContain("第11話時点までの生成済み一覧を表示しています。");

    await act(async () => {
      root.unmount();
    });
  });

  it("キャラクターが空なら再生成メッセージを出す", async () => {
    const props = createProps({
      data: {
        status: "ready",
        upToEpisodeIndex: "5",
        processedUpToEpisodeIndex: null,
        characters: []
      }
    });
    const { container, root } = await renderPanel(props);

    expect(container.textContent).toContain("キャラクターは抽出されませんでした。必要なら対象話数を変えて再生成できます。");

    await act(async () => {
      root.unmount();
    });
  });

  it("カテゴリで一覧を絞り込める", async () => {
    const props = createProps({
      data: {
        status: "ready",
        upToEpisodeIndex: "12",
        processedUpToEpisodeIndex: "12",
        characters: [
          {
            characterId: "c1",
            canonicalName: "主人公",
            fullName: null,
            gender: null,
            firstAppearanceEpisodeIndex: "1",
            aliases: [],
            appearance: null,
            personality: null,
            summary: "主人公です。",
            importance: {
              category: "main",
              score: 0.82
            }
          },
          {
            characterId: "c2",
            canonicalName: "同級生",
            fullName: null,
            gender: null,
            firstAppearanceEpisodeIndex: "3",
            aliases: [],
            appearance: null,
            personality: null,
            summary: "友人です。",
            importance: {
              category: "regular",
              score: 0.31
            }
          }
        ]
      }
    });
    const { container, root, dom } = await renderPanel(props);

    const select = Array.from(container.querySelectorAll("select")).find((candidate) =>
      candidate.textContent?.includes("メインキャラ")
    );
    if (!(select instanceof dom.window.HTMLSelectElement)) {
      throw new Error("select not found");
    }

    await changeSelect(select, "main", dom);

    expect(container.textContent).toContain("主人公");
    expect(container.textContent).not.toContain("同級生");

    await act(async () => {
      root.unmount();
    });
  });

  it("未生成かつ第1話では生成不可メッセージと disabled 状態を出す", async () => {
    const props = createProps({
      canGenerate: false,
      defaultUpToEpisodeIndex: null,
      includeCurrentEpisode: false
    });
    const { container, root, dom } = await renderPanel(props);

    expect(container.textContent).toContain("第1話より前には生成対象がありません。");
    expect(container.textContent).toContain("「現在話を含む」を有効にすると第1話を生成できます。");

    const input = container.querySelector('input[type="number"]');
    if (!(input instanceof dom.window.HTMLInputElement)) {
      throw new Error("number input not found");
    }

    const submitButton = getButtonByText(container, "人物と用語を抽出");
    expect(input.disabled).toBe(true);
    expect(submitButton.disabled).toBe(true);

    await act(async () => {
      root.unmount();
    });
  });

  it("読み込み中と登録中の状態を表示する", async () => {
    const props = createProps({
      canGenerate: true,
      isLoading: true,
      isSubmitting: true
    });
    const { container, root } = await renderPanel(props);

    expect(container.textContent).toContain("人物と用語の抽出情報を読み込み中...");
    expect(container.textContent).toContain("登録中...");

    await act(async () => {
      root.unmount();
    });
  });

  it("episode ID ではなく話順ラベルで表示できる", async () => {
    const props = createProps({
      data: {
        status: "ready",
        upToEpisodeIndex: "16818622175778745166",
        processedUpToEpisodeIndex: "16818622175778503554",
        characters: [
          {
            characterId: "c1",
            canonicalName: "みみちゃん",
            fullName: null,
            gender: null,
            firstAppearanceEpisodeIndex: "16818622175778404653",
            aliases: [],
            appearance: null,
            personality: null,
            summary: "登場人物です。",
            importance: null
          }
        ]
      },
      activeJobs: [
        {
          jobId: "job-running",
          requestedUpToEpisodeIndex: "16818622175778745166",
          status: "running",
          createdAt: "2026-03-20T00:00:00.000Z",
          errorMessage: null
        }
      ],
      completedJobs: [
        {
          jobId: "job-completed",
          requestedUpToEpisodeIndex: "16818622175778503554",
          status: "completed",
          createdAt: "2026-03-20T01:00:00.000Z",
          errorMessage: null
        }
      ],
      formatEpisodeOrderLabel: (episodeIndex) =>
        ({
          "16818622175778404653": "1",
          "16818622175778503554": "3",
          "16818622175778745166": "2"
        })[episodeIndex] ?? episodeIndex
    });
    const { container, root } = await renderPanel(props);

    expect(container.textContent).toContain("第2話時点 / 1 / 1 人");
    expect(container.textContent).toContain("初登場第1話");
    expect(container.textContent).toContain("第2話まで");
    expect(container.textContent).toContain("第3話まで");

    await act(async () => {
      root.unmount();
    });
  });
});
