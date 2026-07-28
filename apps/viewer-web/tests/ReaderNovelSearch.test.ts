import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement, type ComponentProps } from "react";
import { createRoot } from "react-dom/client";
import { JSDOM } from "jsdom";

import { ReaderNovelSearch } from "../src/features/reader/ReaderNovelSearch";
import { searchNovelText } from "../src/features/reader/api";

vi.mock("../src/features/reader/api", () => ({
  searchNovelText: vi.fn()
}));

type SearchProps = ComponentProps<typeof ReaderNovelSearch>;

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
  vi.stubGlobal("HTMLFormElement", dom.window.HTMLFormElement);
  vi.stubGlobal("Node", dom.window.Node);
  vi.stubGlobal("Event", dom.window.Event);
  vi.stubGlobal("InputEvent", dom.window.InputEvent);
  vi.stubGlobal("MouseEvent", dom.window.MouseEvent);
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  return dom;
}

async function renderSearch(overrides: Partial<SearchProps> = {}) {
  const dom = installDom();
  const container = dom.window.document.getElementById("root");
  if (!container) {
    throw new Error("root container not found");
  }
  const root = createRoot(container);
  const props: SearchProps = {
    children: createElement("p", null, "目次本文"),
    formatEpisodeLabel: (episodeIndex) => `第${episodeIndex}話`,
    novelId: "novel-1",
    onRead: vi.fn(),
    ...overrides
  };
  await act(async () => root.render(createElement(ReaderNovelSearch, props)));
  return { container, dom, props, root };
}

async function inputAndSubmit(container: HTMLElement, dom: JSDOM, query: string) {
  const input = container.querySelector("input");
  const form = container.querySelector("form");
  if (!(input instanceof dom.window.HTMLInputElement) || !form) {
    throw new Error("search form not found");
  }
  await act(async () => {
    const descriptor = Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value");
    descriptor?.set?.call(input, query);
    input.dispatchEvent(new dom.window.InputEvent("input", { bubbles: true, inputType: "insertText", data: query }));
    input.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
  });
  await act(async () => form.dispatchEvent(new dom.window.Event("submit", { bubbles: true, cancelable: true })));
}

async function clickButton(container: HTMLElement, dom: JSDOM, text: string) {
  const button = Array.from(container.querySelectorAll("button")).find((candidate) =>
    candidate.textContent?.includes(text)
  );
  if (!button) {
    throw new Error(`button not found: ${text}`);
  }
  await act(async () => button.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true })));
}

afterEach(() => {
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});

describe("ReaderNovelSearch", () => {
  it("結果をプレビューし、明示操作でだけ本文を開く", async () => {
    vi.mocked(searchNovelText).mockResolvedValue({
      query: "探索語",
      candidateCount: 2,
      matchedEpisodeCount: 1,
      truncated: false,
      matches: [{ episodeIndex: "2", episodeNumber: 2, title: "第二話", position: 42, snippet: "前後の探索語本文" }]
    });
    const onRead = vi.fn();
    const { container, dom, root } = await renderSearch({ onRead });

    expect(container.textContent).toContain("目次本文");
    await inputAndSubmit(container, dom, "  探索語  ");
    expect(searchNovelText).toHaveBeenCalledWith("novel-1", "探索語");
    expect(container.textContent).toContain("1件の検索結果");
    await clickButton(container, dom, "第二話");
    expect(container.textContent).toContain("プレビューでは最終既読位置を変更しません");
    expect(onRead).not.toHaveBeenCalled();
    await clickButton(container, dom, "検索結果に戻る");
    await clickButton(container, dom, "第二話");
    await clickButton(container, dom, "この位置から読む");
    expect(onRead).toHaveBeenCalledWith(expect.objectContaining({ episodeIndex: "2", position: 42 }));

    await act(async () => root.unmount());
  });

  it("0件と検索失敗を表示できる", async () => {
    vi.mocked(searchNovelText)
      .mockResolvedValueOnce({
        query: "なし",
        candidateCount: 0,
        matchedEpisodeCount: 0,
        truncated: false,
        matches: []
      })
      .mockRejectedValueOnce(new Error("検索できません"));
    const { container, dom, root } = await renderSearch();

    await inputAndSubmit(container, dom, "なし");
    expect(container.textContent).toContain("一致する本文がありません");
    await clickButton(container, dom, "目次に戻る");
    expect(container.textContent).toContain("目次本文");
    await inputAndSubmit(container, dom, "失敗");
    expect(container.textContent).toContain("検索できません");

    await act(async () => root.unmount());
  });
});
