import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement, type ComponentProps } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import { ReaderSpeechPanel } from "../src/ReaderSpeechPanel";

type PanelProps = ComponentProps<typeof ReaderSpeechPanel>;

function createProps(overrides: Partial<PanelProps> = {}): PanelProps {
  return {
    onClose: vi.fn(),
    onPause: vi.fn(),
    onPlay: vi.fn(),
    onReset: vi.fn(),
    onResume: vi.fn(),
    onSpeechEnabledChange: vi.fn(),
    onSpeechDebugHighlightChange: vi.fn(),
    onSpeechPreferRubyTextChange: vi.fn(),
    onSpeechRateChange: vi.fn(),
    onSpeechVoiceUriChange: vi.fn(),
    onStop: vi.fn(),
    hasActiveChunk: false,
    hasSpeechContent: true,
    isPaused: false,
    isPlaying: false,
    isSpeechSupported: true,
    speechEnabled: true,
    speechDebugHighlight: false,
    speechPreferRubyText: true,
    speechRate: 1,
    speechVoiceUri: null,
    speechVoices: [
      {
        voiceURI: "voice:ja-jp",
        name: "日本語音声",
        lang: "ja-JP",
        default: true,
        localService: true
      }
    ],
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
  vi.stubGlobal("HTMLSelectElement", dom.window.HTMLSelectElement);
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
    root.render(createElement(ReaderSpeechPanel, props));
  });

  return { container, root, dom };
}

function getButtonByText(container: HTMLElement, text: string): HTMLButtonElement {
  const button = Array.from(container.querySelectorAll("button")).find((candidate) => candidate.textContent?.trim() === text);

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

describe("ReaderSpeechPanel", () => {
  it("読み上げ panel を描画して主要ハンドラを呼び出す", async () => {
    const props = createProps();
    const { container, root, dom } = await renderPanel(props);

    expect(container.textContent).toContain("読み上げ");
    expect(container.textContent).toContain("待機中");
    expect(container.textContent).toContain("再生");
    expect(container.textContent).toContain("設定");
    expect(container.textContent).toContain("読み上げ速度: 1.00x");
    expect(container.textContent).toContain("ルビを読む");
    expect(container.textContent).toContain("デバッグ表示");

    await click(getButtonByText(container, "現在位置から再生"), dom);
    await click(getButtonByText(container, "読み上げ設定を初期化"), dom);

    const selects = container.querySelectorAll("select");
    await changeSelect(selects[0] as HTMLSelectElement, "disabled", dom);
    await changeSelect(selects[1] as HTMLSelectElement, "voice:ja-jp", dom);
    await changeSelect(selects[2] as HTMLSelectElement, "text", dom);
    await changeSelect(selects[3] as HTMLSelectElement, "enabled", dom);
    await click(container.querySelector('button[aria-label="読み上げ速度を下げる"]') as Element, dom);
    await click(container.querySelector('button[aria-label="読み上げ速度を上げる"]') as Element, dom);
    await click(container.querySelector('button[aria-label="読み上げを閉じる"]') as Element, dom);

    expect(props.onPlay).toHaveBeenCalledTimes(1);
    expect(props.onReset).toHaveBeenCalledTimes(1);
    expect(props.onSpeechEnabledChange).toHaveBeenCalledWith(false);
    expect(props.onSpeechVoiceUriChange).toHaveBeenCalledWith("voice:ja-jp");
    expect(props.onSpeechPreferRubyTextChange).toHaveBeenCalledWith(false);
    expect(props.onSpeechDebugHighlightChange).toHaveBeenCalledWith(true);
    expect(props.onSpeechRateChange).toHaveBeenNthCalledWith(1, 0.95);
    expect(props.onSpeechRateChange).toHaveBeenNthCalledWith(2, 1.05);
    expect(props.onClose).toHaveBeenCalledTimes(1);

    await act(async () => {
      root.unmount();
    });
  });

  it("再生中は一時停止と停止を表示する", async () => {
    const props = createProps({
      isPlaying: true
    });
    const { container, root, dom } = await renderPanel(props);

    expect(container.textContent).toContain("再生中");
    await click(getButtonByText(container, "一時停止"), dom);
    await click(getButtonByText(container, "停止"), dom);

    expect(props.onPause).toHaveBeenCalledTimes(1);
    expect(props.onStop).toHaveBeenCalledTimes(1);

    await act(async () => {
      root.unmount();
    });
  });

  it("一時停止中は再開と停止を表示する", async () => {
    const props = createProps({
      isPaused: true
    });
    const { container, root, dom } = await renderPanel(props);

    expect(container.textContent).toContain("一時停止中");
    await click(getButtonByText(container, "再開"), dom);
    await click(getButtonByText(container, "停止"), dom);

    expect(props.onResume).toHaveBeenCalledTimes(1);
    expect(props.onStop).toHaveBeenCalledTimes(1);

    await act(async () => {
      root.unmount();
    });
  });

  it("読み上げ非対応ブラウザでは設定入力を無効化する", async () => {
    const props = createProps({
      isSpeechSupported: false
    });
    const { container, root } = await renderPanel(props);

    expect((container.querySelector('button[aria-label="読み上げ速度を下げる"]') as HTMLButtonElement).disabled).toBe(true);
    expect((container.querySelector('button[aria-label="読み上げ速度を上げる"]') as HTMLButtonElement).disabled).toBe(true);

    const selects = container.querySelectorAll("select");
    expect((selects[1] as HTMLSelectElement).disabled).toBe(true);
    expect((selects[2] as HTMLSelectElement).disabled).toBe(true);
    expect(getButtonByText(container, "現在位置から再生").disabled).toBe(true);

    await act(async () => {
      root.unmount();
    });
  });

  it("無効時は設定だけ触れて再生はできない", async () => {
    const props = createProps({
      speechEnabled: false
    });
    const { container, root } = await renderPanel(props);

    expect(container.textContent).toContain("オフ");
    expect((container.querySelector('button[aria-label="読み上げ速度を下げる"]') as HTMLButtonElement).disabled).toBe(true);
    expect(getButtonByText(container, "現在位置から再生").disabled).toBe(true);

    await act(async () => {
      root.unmount();
    });
  });

  it("再開待ちがあると再生ボタン文言を切り替える", async () => {
    const props = createProps({
      hasActiveChunk: true
    });
    const { container, root } = await renderPanel(props);

    expect(container.textContent).toContain("現在位置からやり直す");

    await act(async () => {
      root.unmount();
    });
  });
});
