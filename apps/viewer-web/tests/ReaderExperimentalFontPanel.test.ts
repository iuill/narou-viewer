import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement, type ComponentProps } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import { ReaderExperimentalFontPanel } from "../src/ReaderExperimentalFontPanel";

type PanelProps = ComponentProps<typeof ReaderExperimentalFontPanel>;

function createProps(overrides: Partial<PanelProps> = {}): PanelProps {
  return {
    loadStatus: "idle",
    onClose: vi.fn(),
    onReaderExperimentalFontChange: vi.fn(),
    onReaderExperimentalFontWeightChange: vi.fn(),
    onRetryRemoteFontLoad: vi.fn(),
    previewFontFamilyCss: "var(--font-serif-ja)",
    previewFontWeight: 400,
    readerExperimentalFontId: "none",
    readerExperimentalFontWeight: 400,
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
  vi.stubGlobal("HTMLSelectElement", dom.window.HTMLSelectElement);
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
    root.render(createElement(ReaderExperimentalFontPanel, props));
  });

  return { container, root, dom };
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

describe("ReaderExperimentalFontPanel", () => {
  it("不正な select 値を none に丸める", async () => {
    const props = createProps();
    const { container, root, dom } = await renderPanel(props);

    await changeSelect(container.querySelectorAll("select")[0] as HTMLSelectElement, "invalid-font", dom);

    expect(props.onReaderExperimentalFontChange).toHaveBeenCalledWith("none");

    await act(async () => {
      root.unmount();
    });
  });

  it("不正な太さの select 値を 400 に丸める", async () => {
    const props = createProps();
    const { container, root, dom } = await renderPanel(props);

    await changeSelect(container.querySelectorAll("select")[1] as HTMLSelectElement, "700", dom);

    expect(props.onReaderExperimentalFontWeightChange).toHaveBeenCalledWith(400);

    await act(async () => {
      root.unmount();
    });
  });

  it("Google Fonts 読み込み失敗時に再読み込み操作を表示する", async () => {
    const props = createProps({
      loadStatus: "error",
      readerExperimentalFontId: "noto-serif-jp"
    });
    const { container, root } = await renderPanel(props);

    const retryButton = Array.from(container.querySelectorAll("button")).find((button) =>
      button.textContent?.includes("Google Fonts を再読み込み")
    );

    expect(retryButton).toBeTruthy();

    await act(async () => {
      retryButton?.dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
    });

    expect(props.onRetryRemoteFontLoad).toHaveBeenCalledTimes(1);

    await act(async () => {
      root.unmount();
    });
  });

  it("Google Fonts 利用時の状態ラベルを表示できる", async () => {
    const props = createProps({
      loadStatus: "loading",
      readerExperimentalFontId: "noto-sans-jp"
    });
    const { container, root } = await renderPanel(props);

    expect(container.textContent).toContain("Google Fonts を読み込み中");
    expect(container.querySelector(".reader-experimental-font-kind.kind-gothic")?.textContent).toContain("ゴシック系");

    await act(async () => {
      root.render(createElement(ReaderExperimentalFontPanel, { ...props, loadStatus: "ready" }));
    });

    expect(container.textContent).toContain("Google Fonts 読み込み済み");

    await act(async () => {
      root.render(createElement(ReaderExperimentalFontPanel, { ...props, loadStatus: "idle" }));
    });

    expect(container.textContent).toContain("Google Fonts を必要時のみ読み込み");

    await act(async () => {
      root.unmount();
    });
  });
});
