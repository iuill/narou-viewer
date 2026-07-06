import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement, type ComponentProps } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import { ReaderSettingsPanel } from "../src/ReaderSettingsPanel";

type PanelProps = ComponentProps<typeof ReaderSettingsPanel>;

function createProps(overrides: Partial<PanelProps> = {}): PanelProps {
  return {
    onClose: vi.fn(),
    onDebugPageOverflowChange: vi.fn(),
    onReaderFontFamilyChange: vi.fn(),
    onReaderFontSizeChange: vi.fn(),
    onReaderLetterSpacingChange: vi.fn(),
    onHyphenDashNormalizationChange: vi.fn(),
    onParenthesisNormalizationChange: vi.fn(),
    onHalfwidthAlnumPunctuationNormalizationChange: vi.fn(),
    onQuoteNormalizationChange: vi.fn(),
    onReverseTapPageNavigationChange: vi.fn(),
    onReaderThemeChange: vi.fn(),
    onReadingModeChange: vi.fn(),
    onReset: vi.fn(),
    debugPageOverflow: false,
    isReaderCorrectionSaving: false,
    hyphenDashNormalizationEnabled: false,
    parenthesisNormalizationEnabled: false,
    halfwidthAlnumPunctuationNormalizationEnabled: false,
    quoteNormalizationEnabled: false,
    readerFontFamily: "mincho",
    readerFontSizePx: 20,
    readerLetterSpacingEm: 0.08,
    reverseTapPageNavigation: false,
    readerTheme: "classic",
    readingMode: "vertical",
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
    root.render(createElement(ReaderSettingsPanel, props));
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

describe("ReaderSettingsPanel", () => {
  it("読書設定を描画して主要ハンドラを呼び出す", async () => {
    const props = createProps();
    const { container, root, dom } = await renderPanel(props);

    expect(container.textContent).toContain("読書設定");
    expect(container.textContent).toContain("表示");
    expect(container.textContent).toContain("操作");
    expect(container.textContent).toContain("本文校正");
    expect(container.textContent).toContain("引用符を〝〟へ置換");
    expect(container.textContent).toContain("連続ハイフンをダッシュへ置換");
    expect(container.textContent).toContain("半角括弧を全角へ置換");
    expect(container.textContent).toContain("半角英数字・!?を全角へ置換");
    expect(container.textContent).toContain("デバッグ");
    expect(container.textContent).toContain("文字サイズ: 20px");
    expect(container.textContent).toContain("文字間隔: 0.08em");
    expect(container.textContent).toContain("左右端タップ");

    await click(getButtonByText(container, "横書き"), dom);
    await click(getButtonByText(container, "読書設定を初期化"), dom);

    const selects = container.querySelectorAll("select");
    await changeSelect(selects[0] as HTMLSelectElement, "gothic", dom);
    await changeSelect(selects[1] as HTMLSelectElement, "forest", dom);
    await changeSelect(selects[2] as HTMLSelectElement, "reversed", dom);
    await changeSelect(selects[3] as HTMLSelectElement, "enabled", dom);
    await changeSelect(selects[4] as HTMLSelectElement, "enabled", dom);
    await changeSelect(selects[5] as HTMLSelectElement, "enabled", dom);
    await changeSelect(selects[6] as HTMLSelectElement, "enabled", dom);
    await changeSelect(selects[7] as HTMLSelectElement, "debug", dom);
    await click(container.querySelector('button[aria-label="読書設定を閉じる"]') as Element, dom);

    expect(props.onReadingModeChange).toHaveBeenCalledWith("horizontal");
    expect(props.onReset).toHaveBeenCalledTimes(1);
    expect(props.onReverseTapPageNavigationChange).toHaveBeenCalledWith(true);
    expect(props.onQuoteNormalizationChange).toHaveBeenCalledWith(true);
    expect(props.onHyphenDashNormalizationChange).toHaveBeenCalledWith(true);
    expect(props.onParenthesisNormalizationChange).toHaveBeenCalledWith(true);
    expect(props.onHalfwidthAlnumPunctuationNormalizationChange).toHaveBeenCalledWith(true);
    expect(props.onDebugPageOverflowChange).toHaveBeenCalledWith(true);
    expect(props.onReaderFontFamilyChange).toHaveBeenCalledWith("gothic");
    expect(props.onReaderThemeChange).toHaveBeenCalledWith("forest");
    expect(props.onClose).toHaveBeenCalledTimes(1);

    await act(async () => {
      root.unmount();
    });
  });

  it("選択状態に応じて active クラスを切り替える", async () => {
    const props = createProps({
      readingMode: "horizontal"
    });
    const { container, root } = await renderPanel(props);

    const buttons = Array.from(container.querySelectorAll(".mode-toggle button"));
    expect(buttons[0]?.className ?? "").not.toContain("active");
    expect(buttons[1]?.className ?? "").toContain("active");

    await act(async () => {
      root.unmount();
    });
  });

  it("縦書き切り替えを処理する", async () => {
    const props = createProps({
      readingMode: "horizontal"
    });
    const { container, root, dom } = await renderPanel(props);

    await click(getButtonByText(container, "縦書き"), dom);

    expect(props.onReadingModeChange).toHaveBeenCalledWith("vertical");

    await act(async () => {
      root.unmount();
    });
  });

  it("文字サイズと文字間隔を +/- ボタンで調整できる", async () => {
    const props = createProps();
    const { container, root, dom } = await renderPanel(props);

    await click(container.querySelector('button[aria-label="文字サイズを小さくする"]') as Element, dom);
    await click(container.querySelector('button[aria-label="文字サイズを大きくする"]') as Element, dom);
    await click(container.querySelector('button[aria-label="文字間隔を狭くする"]') as Element, dom);
    await click(container.querySelector('button[aria-label="文字間隔を広くする"]') as Element, dom);

    expect(props.onReaderFontSizeChange).toHaveBeenNthCalledWith(1, 19);
    expect(props.onReaderFontSizeChange).toHaveBeenNthCalledWith(2, 21);
    expect(props.onReaderLetterSpacingChange).toHaveBeenNthCalledWith(1, 0.07);
    expect(props.onReaderLetterSpacingChange).toHaveBeenNthCalledWith(2, 0.09);

    await act(async () => {
      root.unmount();
    });
  });

  it("上下限に達していると +/- ボタンを無効化する", async () => {
    const props = createProps({
      readerFontSizePx: 14,
      readerLetterSpacingEm: 0
    });
    const { container, root } = await renderPanel(props);

    expect((container.querySelector('button[aria-label="文字サイズを小さくする"]') as HTMLButtonElement).disabled).toBe(true);
    expect((container.querySelector('button[aria-label="文字間隔を狭くする"]') as HTMLButtonElement).disabled).toBe(true);
    expect((container.querySelector('button[aria-label="文字サイズを大きくする"]') as HTMLButtonElement).disabled).toBe(false);
    expect((container.querySelector('button[aria-label="文字間隔を広くする"]') as HTMLButtonElement).disabled).toBe(false);

    await act(async () => {
      root.unmount();
    });
  });

  it("左右端タップの反転設定を選択状態に応じて描画する", async () => {
    const props = createProps({
      reverseTapPageNavigation: true
    });
    const { container, root } = await renderPanel(props);

    const tapNavigationSelect = container.querySelectorAll("select")[2] as HTMLSelectElement;
    expect(tapNavigationSelect.value).toBe("reversed");

    await act(async () => {
      root.unmount();
    });
  });

  it("列はみ出しデバッグ設定を選択状態に応じて描画する", async () => {
    const props = createProps({
      debugPageOverflow: true
    });
    const { container, root } = await renderPanel(props);

    const overflowDebugSelect = container.querySelectorAll("select")[7] as HTMLSelectElement;
    expect(overflowDebugSelect.value).toBe("debug");

    await act(async () => {
      root.unmount();
    });
  });

  it("本文校正が利用できない間は引用符置換の選択を無効化する", async () => {
    const props = createProps({
      isReaderCorrectionSaving: true
    });
    const { container, root } = await renderPanel(props);

    const quoteNormalizationSelect = container.querySelectorAll("select")[3] as HTMLSelectElement;
    const hyphenDashNormalizationSelect = container.querySelectorAll("select")[4] as HTMLSelectElement;
    const parenthesisNormalizationSelect = container.querySelectorAll("select")[5] as HTMLSelectElement;
    const halfwidthAlnumPunctuationNormalizationSelect = container.querySelectorAll("select")[6] as HTMLSelectElement;
    expect(quoteNormalizationSelect.disabled).toBe(true);
    expect(hyphenDashNormalizationSelect.disabled).toBe(true);
    expect(parenthesisNormalizationSelect.disabled).toBe(true);
    expect(halfwidthAlnumPunctuationNormalizationSelect.disabled).toBe(true);
    expect(getButtonByText(container, "読書設定を初期化").disabled).toBe(true);

    await act(async () => {
      root.unmount();
    });
  });
});
