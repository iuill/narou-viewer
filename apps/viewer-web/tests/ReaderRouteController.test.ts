import { JSDOM } from "jsdom";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ReaderRouteController } from "../src/routes/ReaderRouteController";

let mountedRoot: Root | null = null;

function installDom(): JSDOM {
  const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
    url: "http://localhost/"
  });

  vi.stubGlobal("window", dom.window);
  vi.stubGlobal("document", dom.window.document);
  vi.stubGlobal("navigator", dom.window.navigator);
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);

  return dom;
}

function renderController(props: Parameters<typeof ReaderRouteController>[0]): Root {
  const rootElement = document.getElementById("root");
  if (!rootElement) {
    throw new Error("root element is missing");
  }

  const root = createRoot(rootElement);
  act(() => {
    root.render(createElement(ReaderRouteController, props));
  });
  mountedRoot = root;
  return root;
}

afterEach(() => {
  if (mountedRoot) {
    act(() => mountedRoot?.unmount());
    mountedRoot = null;
  }
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("ReaderRouteController", () => {
  it("repairs a missing desktop selection to the first filtered novel", () => {
    installDom();
    const onSelectNovel = vi.fn();

    renderController({
      filteredNovels: [{ novelId: "n1" }, { novelId: "n2" }],
      isInitialLoading: false,
      isMobileLibraryViewport: false,
      onClearSelection: vi.fn(),
      onSelectNovel,
      onShowMobileLibraryPanel: vi.fn(),
      selectedNovelId: "missing"
    });

    expect(onSelectNovel).toHaveBeenCalledWith("n1");
  });

  it("clears mobile selection and returns to the library panel", () => {
    installDom();
    const onClearSelection = vi.fn();
    const onShowMobileLibraryPanel = vi.fn();

    renderController({
      filteredNovels: [{ novelId: "n1" }],
      isInitialLoading: false,
      isMobileLibraryViewport: true,
      onClearSelection,
      onSelectNovel: vi.fn(),
      onShowMobileLibraryPanel,
      selectedNovelId: "missing"
    });

    expect(onClearSelection).toHaveBeenCalledTimes(1);
    expect(onShowMobileLibraryPanel).toHaveBeenCalledTimes(1);
  });

  it("does not repair selection while the initial library load is pending", () => {
    installDom();
    const onSelectNovel = vi.fn();

    renderController({
      filteredNovels: [{ novelId: "n1" }],
      isInitialLoading: true,
      isMobileLibraryViewport: false,
      onClearSelection: vi.fn(),
      onSelectNovel,
      onShowMobileLibraryPanel: vi.fn(),
      selectedNovelId: "missing"
    });

    expect(onSelectNovel).not.toHaveBeenCalled();
  });
});
