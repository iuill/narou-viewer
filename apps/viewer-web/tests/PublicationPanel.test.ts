import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement, type ComponentProps } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import { PublicationPanel } from "../src/PublicationPanel";
import type { PublicationEntry } from "../src/features/publications/types";

type PanelProps = ComponentProps<typeof PublicationPanel>;

const novelEntry: PublicationEntry = {
  id: "novel-9784040000008",
  kind: "novel",
  status: "manual",
  override: "isbn",
  isbn13: "9784040000008",
  title: "書籍版タイトル",
  authors: ["著者A"],
  publisher: "出版社",
  publishedDate: "2026-01-01",
  imageUrl: "https://example.test/cover.jpg",
  source: "NDLサーチ",
  sourceUrl: "https://ndl.example.test/detail",
  coverSource: "Google Books",
  coverSourceUrl: "https://books.google.test/volume",
  updatedAt: "2026-06-28T12:00:00Z"
};

function createProps(overrides: Partial<PanelProps> = {}): PanelProps {
  return {
    displayCoverEntryId: "novel-9784040000008",
    entries: [novelEntry],
    isLoading: false,
    savingEntryId: null,
    onCreateISBN: vi.fn(),
    onSaveISBN: vi.fn(),
    onClear: vi.fn(),
    onDisable: vi.fn(),
    onRedisplay: vi.fn(),
    onSetDisplayCover: vi.fn(),
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
  vi.stubGlobal("HTMLFormElement", dom.window.HTMLFormElement);
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
    root.render(createElement(PublicationPanel, props));
  });
  return { container, root, dom };
}

async function changeInput(input: HTMLInputElement, value: string, dom: JSDOM): Promise<void> {
  await act(async () => {
    const descriptor = Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value");
    descriptor?.set?.call(input, value);
    input.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
  });
}

async function click(element: Element, dom: JSDOM): Promise<void> {
  await act(async () => {
    element.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
  });
}

async function submit(form: HTMLFormElement, dom: JSDOM): Promise<void> {
  await act(async () => {
    form.dispatchEvent(new dom.window.Event("submit", { bubbles: true, cancelable: true }));
  });
}

function getButtonByText(container: HTMLElement, text: string): HTMLButtonElement {
  const button = Array.from(container.querySelectorAll("button")).find((candidate) => candidate.textContent === text);
  if (!(button instanceof HTMLButtonElement)) {
    throw new Error(`button not found: ${text}`);
  }
  return button;
}

function getInputByLabel(container: HTMLElement, label: string): HTMLInputElement {
  const input = container.querySelector(`input[aria-label="${label}"]`);
  if (!(input instanceof HTMLInputElement)) {
    throw new Error(`input not found: ${label}`);
  }
  return input;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("PublicationPanel", () => {
  it("登録済み書籍情報と Google Books リンクを描画する", async () => {
    const props = createProps();
    const { container, root } = await renderPanel(props);

    expect(container.textContent).toContain("書籍情報");
    expect(container.textContent).toContain("書籍版タイトル");
    expect(container.textContent).toContain("著者A");
    expect(container.textContent).toContain("NDLサーチで見る");
    expect(container.textContent).toContain("Google Books で見る");
    expect(container.textContent).toContain("Cover data powered by Google.");
    expect(container.textContent).toContain("書籍メタデータの一部は NDLサーチ API から取得しています");
    expect(container.querySelector("img")?.getAttribute("src")).toBe("https://example.test/cover.jpg");
    expect(getButtonByText(container, "一覧表紙に設定中").disabled).toBe(true);

    await act(async () => {
      root.unmount();
    });
  });

  it("既存書籍情報の ISBN 更新、解除、非表示を呼び出す", async () => {
    const props = createProps();
    const { container, root, dom } = await renderPanel(props);

    const novelInput = getInputByLabel(container, "小説版 ISBN13");
    await changeInput(novelInput, "9784040000015", dom);

    const novelCard = novelInput.closest(".publication-card");
    if (!novelCard) {
      throw new Error("novel card not found");
    }
    const form = novelCard.querySelector("form");
    if (!(form instanceof HTMLFormElement)) {
      throw new Error("novel form not found");
    }
    await submit(form, dom);
    await click(Array.from(novelCard.querySelectorAll("button")).find((button) => button.textContent === "解除") as Element, dom);
    await click(Array.from(novelCard.querySelectorAll("button")).find((button) => button.textContent === "非表示") as Element, dom);

    expect(props.onSaveISBN).toHaveBeenCalledWith("novel-9784040000008", "9784040000015");
    expect(props.onClear).toHaveBeenCalledWith(novelEntry);
    expect(props.onDisable).toHaveBeenCalledWith(novelEntry);

    await act(async () => {
      root.unmount();
    });
  });

  it("同じ種別の書籍情報を追加できる", async () => {
    const props = createProps();
    const { container, root, dom } = await renderPanel(props);

    const addInput = getInputByLabel(container, "小説版を追加 ISBN13");
    await changeInput(addInput, "9784040000008", dom);
    const addForm = addInput.closest("form");
    if (!(addForm instanceof HTMLFormElement)) {
      throw new Error("add form not found");
    }
    await submit(addForm, dom);

    expect(props.onCreateISBN).toHaveBeenCalledWith("novel", "9784040000008");

    await act(async () => {
      root.unmount();
    });
  });

  it("一覧用表紙をエントリ単位で選べる", async () => {
    const comicEntry: PublicationEntry = {
      id: "comic-9784040000009",
      kind: "comic",
      status: "manual",
      override: "isbn",
      isbn13: "9784040000009",
      title: "コミック版タイトル",
      imageUrl: "https://example.test/comic-cover.jpg"
    };
    const props = createProps({
      displayCoverEntryId: "novel-9784040000008",
      entries: [novelEntry, comicEntry]
    });
    const { container, root, dom } = await renderPanel(props);

    await click(getButtonByText(container, "一覧表紙にする"), dom);
    expect(props.onSetDisplayCover).toHaveBeenCalledWith("comic-9784040000009");

    await act(async () => {
      root.unmount();
    });
  });

  it("fallback で一覧表示中の表紙は固定できる文言にする", async () => {
    const props = createProps({
      displayCoverEntryId: ""
    });
    const { container, root, dom } = await renderPanel(props);

    await click(getButtonByText(container, "自動表示中・固定する"), dom);
    expect(props.onSetDisplayCover).toHaveBeenCalledWith("novel-9784040000008");

    await act(async () => {
      root.unmount();
    });
  });

  it("不正な ISBN13 は登録ボタンを無効にする", async () => {
    const props = createProps();
    const { container, root, dom } = await renderPanel(props);

    const comicInput = getInputByLabel(container, "コミック版を追加 ISBN13");
    await changeInput(comicInput, "978-4866993910", dom);

    const comicForm = comicInput.closest("form");
    if (!comicForm) {
      throw new Error("comic add form not found");
    }
    const registerButton = Array.from(comicForm.querySelectorAll("button")).find((button) => button.textContent === "コミック版を追加");
    if (!(registerButton instanceof HTMLButtonElement)) {
      throw new Error("register button not found");
    }
    expect(registerButton.disabled).toBe(true);

    await act(async () => {
      root.unmount();
    });
  });

  it("非表示中は再表示ボタンを表示する", async () => {
    const hiddenEntry: PublicationEntry = {
      id: "novel-9784040000008",
      kind: "novel",
      status: "disabled",
      override: "disabled",
      isbn13: "9784040000008",
      title: "非表示中タイトル",
      imageUrl: "https://example.test/hidden-cover.jpg",
      source: "NDLサーチ",
      sourceUrl: "https://ndl.example.test/detail",
      coverSource: "Google Books",
      coverSourceUrl: "https://books.google.test/volume",
      warnings: ["google_books_lookup_failed"]
    };
    const props = createProps({
      displayCoverEntryId: "",
      entries: [hiddenEntry]
    });
    const { container, root, dom } = await renderPanel(props);

    const novelCard = Array.from(container.querySelectorAll(".publication-card")).at(0);
    if (!novelCard) {
      throw new Error("novel card not found");
    }
    const redisplayButton = Array.from(novelCard.querySelectorAll("button")).find((button) => button.textContent === "再表示");
    if (!(redisplayButton instanceof HTMLButtonElement)) {
      throw new Error("redisplay button not found");
    }
    expect(novelCard.textContent).toContain("表示しない");
    expect(novelCard.textContent).toContain("この書籍情報は非表示に設定されています。");
    expect(novelCard.textContent).not.toContain("非表示中タイトル");
    expect(novelCard.textContent).not.toContain("NDLサーチで見る");
    expect(novelCard.textContent).not.toContain("Google Books で見る");
    expect(novelCard.textContent).not.toContain("Cover data powered by Google.");
    expect(novelCard.textContent).not.toContain("ISBN は保存しました。");
    expect(novelCard.querySelector("img")).toBeNull();
    expect(Array.from(novelCard.querySelectorAll("button")).some((button) => button.textContent === "非表示")).toBe(false);
    await click(redisplayButton, dom);
    expect(props.onRedisplay).toHaveBeenCalledWith(hiddenEntry);
    expect(props.onClear).not.toHaveBeenCalled();

    await act(async () => {
      root.unmount();
    });
  });

  it("登録済みの非表示状態では解除も選べる", async () => {
    const hiddenEntry: PublicationEntry = {
      id: "novel-9784040000008",
      kind: "novel",
      status: "disabled",
      override: "disabled",
      isbn13: "9784040000008",
      title: "非表示中タイトル"
    };
    const props = createProps({
      displayCoverEntryId: "",
      entries: [hiddenEntry]
    });
    const { container, root, dom } = await renderPanel(props);

    await click(getButtonByText(container, "解除"), dom);
    expect(props.onClear).toHaveBeenCalledWith(hiddenEntry);

    await act(async () => {
      root.unmount();
    });
  });

  it("provider ごとの補完失敗 warning を表示する", async () => {
    const props = createProps({
      entries: [
        {
          id: "novel-9784040000008",
          kind: "novel",
          status: "manual",
          override: "isbn",
          isbn13: "9784040000008",
          title: "NDLタイトル",
          source: "NDLサーチ",
          sourceUrl: "https://ndl.example.test/detail",
          warnings: ["google_books_lookup_failed"]
        }
      ]
    });
    const { container, root } = await renderPanel(props);

    expect(container.textContent).toContain("ISBN は保存しました。Google Books から表紙画像を取得できませんでした。");
    expect(container.textContent).not.toContain("書誌補完は一部取得できませんでした");

    await act(async () => {
      root.unmount();
    });
  });

  it("Google Books API key 未設定 warning を表示する", async () => {
    const props = createProps({
      entries: [
        {
          id: "novel-9784040000008",
          kind: "novel",
          status: "manual",
          override: "isbn",
          isbn13: "9784040000008",
          title: "NDLタイトル",
          source: "NDLサーチ",
          sourceUrl: "https://ndl.example.test/detail",
          warnings: ["google_books_api_key_missing"]
        }
      ]
    });
    const { container, root } = await renderPanel(props);

    expect(container.textContent).toContain("Google Books API key が未設定のため、表紙画像を取得しませんでした。");

    await act(async () => {
      root.unmount();
    });
  });

  it("表紙がない Google Books レコードでは book data credit を表示する", async () => {
    const props = createProps({
      entries: [
        {
          id: "novel-9784040000008",
          kind: "novel",
          status: "manual",
          override: "isbn",
          isbn13: "9784040000008",
          title: "NDLタイトル",
          source: "NDLサーチ",
          sourceUrl: "https://ndl.example.test/detail",
          coverSource: "Google Books",
          coverSourceUrl: "https://books.google.test/volume",
          warnings: ["google_books_cover_missing"]
        }
      ]
    });
    const { container, root } = await renderPanel(props);

    expect(container.textContent).toContain("Book data powered by Google.");
    expect(container.textContent).toContain("Google Books に書誌はありますが、表紙画像は提供されていません。");
    expect(container.textContent).not.toContain("Cover data powered by Google.");

    await act(async () => {
      root.unmount();
    });
  });
});
