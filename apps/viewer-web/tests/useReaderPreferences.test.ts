import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import { fetchReaderPreferences, putReaderPreferences } from "../src/features/reader/api";
import { useReaderPreferences } from "../src/hooks/useReaderPreferences";
import { DEFAULT_READER_LOCAL_PREFERENCES } from "../src/readerPreferences";

vi.mock("../src/features/reader/api", () => ({
  fetchReaderPreferences: vi.fn(),
  putReaderPreferences: vi.fn()
}));

type HookResult = ReturnType<typeof useReaderPreferences>;

function installDom(): JSDOM {
  const dom = new JSDOM("<!doctype html><html><head></head><body><div id=\"root\"></div></body></html>", {
    url: "http://localhost/"
  });
  vi.stubGlobal("window", dom.window);
  vi.stubGlobal("document", dom.window.document);
  vi.stubGlobal("navigator", dom.window.navigator);
  return dom;
}

async function flushAsyncWork(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
}

function renderHookHarness(onRender: (result: HookResult, errors: string[], notices: string[]) => void): Root {
  const rootElement = document.getElementById("root");
  if (!rootElement) {
    throw new Error("root element is missing");
  }
  const errors: string[] = [];
  const notices: string[] = [];

  function Harness() {
    const result = useReaderPreferences({
      initialLocalPreferences: {
        ...DEFAULT_READER_LOCAL_PREFERENCES,
        fontSizePx: 24,
        experimentalFontId: "noto-serif-jp"
      },
      setError: (nextError) => {
        if (typeof nextError === "function") {
          return;
        }
        if (nextError) {
          errors.push(nextError);
        }
      },
      setReaderNotice: (nextNotice) => {
        if (typeof nextNotice === "function") {
          return;
        }
        if (nextNotice) {
          notices.push(nextNotice);
        }
      }
    });
    onRender(result, errors, notices);
    return null;
  }

  const root = createRoot(rootElement);
  root.render(createElement(Harness));
  return root;
}

describe("useReaderPreferences", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("loads server preferences, persists local preferences, and reports save failures", async () => {
    installDom();
    vi.mocked(fetchReaderPreferences).mockResolvedValue({
      readingMode: "horizontal",
      fontFamily: "gothic",
      theme: "forest",
      updatedAt: "2026-07-01T00:00:00.000Z"
    });
    vi.mocked(putReaderPreferences).mockRejectedValue(new Error("save failed"));

    let latest: HookResult | null = null;
    let latestErrors: string[] = [];
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness((result, errors) => {
        latest = result;
        latestErrors = errors;
      });
      await flushAsyncWork();
    });

    expect(latest?.readingMode).toBe("horizontal");
    expect(latest?.readerFontFamily).toBe("gothic");
    expect(latest?.readerTheme).toBe("forest");

    await act(async () => {
      latest?.setReaderTheme("ocean");
      await flushAsyncWork();
    });

    expect(putReaderPreferences).toHaveBeenCalledWith({
      readingMode: "horizontal",
      fontFamily: "gothic",
      theme: "ocean"
    });
    expect(latestErrors).toContain("save failed");

    await act(async () => {
      latest?.setReaderFontSizePx(30);
      await flushAsyncWork();
    });
    expect(window.localStorage.getItem("narou-viewer.reader-local-preferences.v1")).toContain("\"fontSizePx\":30");

    await act(async () => {
      root?.unmount();
    });
  });

  it("tracks experimental font loading, retry, and reset transitions", async () => {
    const dom = installDom();
    vi.mocked(fetchReaderPreferences).mockResolvedValue({
      readingMode: "vertical",
      fontFamily: "mincho",
      theme: "classic",
      updatedAt: null
    });
    vi.mocked(putReaderPreferences).mockResolvedValue({
      readingMode: "vertical",
      fontFamily: "mincho",
      theme: "classic",
      updatedAt: null
    });

    let latest: HookResult | null = null;
    let latestNotices: string[] = [];
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness((result, _errors, notices) => {
        latest = result;
        latestNotices = notices;
      });
      await flushAsyncWork();
    });

    expect(latest?.readerExperimentalFontLoadStatus).toBe("loading");
    const link = dom.window.document.head.querySelector<HTMLLinkElement>('link[data-reader-experimental-font="true"]');
    expect(link?.href).toContain("Noto+Serif+JP");

    await act(async () => {
      link?.dispatchEvent(new dom.window.Event("error"));
      await flushAsyncWork();
    });
    expect(latest?.readerExperimentalFontLoadStatus).toBe("error");
    const versionAfterError = latest?.readerExperimentalFontLayoutVersion;

    await act(async () => {
      latest?.handleRetryReaderExperimentalFontLoad();
      await flushAsyncWork();
    });
    const retriedLink = dom.window.document.head.querySelector<HTMLLinkElement>('link[data-reader-experimental-font="true"]');
    expect(retriedLink).not.toBe(link);

    await act(async () => {
      retriedLink?.dispatchEvent(new dom.window.Event("load"));
      await flushAsyncWork();
    });
    expect(latest?.readerExperimentalFontLoadStatus).toBe("ready");
    expect(latest?.readerExperimentalFontLayoutVersion).toBe((versionAfterError ?? 0) + 1);

    await act(async () => {
      latest?.handleResetReaderPreferences();
      await flushAsyncWork();
    });
    expect(latest?.readerExperimentalFontId).toBe("none");
    expect(latest?.readerExperimentalFontLoadStatus).toBe("idle");
    expect(latestNotices).toContain("読書設定を初期化しました。");

    await act(async () => {
      root?.unmount();
    });
  });
});
