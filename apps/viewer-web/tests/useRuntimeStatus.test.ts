import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import type { RuntimeStatusResponse } from "../src/features/runtime/types";
import { findRuntimeService, getRuntimeStatusLabel, useRuntimeStatus } from "../src/hooks/useRuntimeStatus";

type HookResult = ReturnType<typeof useRuntimeStatus>;

function installDom(): JSDOM {
  const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
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
}

function renderHookHarness(props: {
  fetchStatus?: () => Promise<RuntimeStatusResponse>;
  onRender: (result: HookResult) => void;
}): Root {
  const rootElement = document.getElementById("root");
  if (!rootElement) {
    throw new Error("root element is missing");
  }

  function Harness() {
    const result = useRuntimeStatus({ fetchStatus: props.fetchStatus });
    props.onRender(result);
    return null;
  }

  const root = createRoot(rootElement);
  root.render(createElement(Harness));
  return root;
}

function createRuntimeStatus(overrides: Partial<RuntimeStatusResponse> = {}): RuntimeStatusResponse {
  return {
    status: "ok",
    checkedAt: "2026-06-15T00:00:00.000Z",
    services: [
      {
        id: "go-internal-ai",
        label: "AI機能",
        status: "ok",
        summary: "利用可能",
        detail: "OpenRouter API key is configured."
      },
      {
        id: "novel-fetcher",
        label: "取得サービス",
        status: "warn",
        summary: "更新あり",
        detail: "A newer novel-fetcher version is available."
      }
    ],
    ...overrides
  };
}

describe("runtime status helpers", () => {
  it("formats status labels", () => {
    expect(getRuntimeStatusLabel(null)).toBe("確認中");
    expect(getRuntimeStatusLabel(createRuntimeStatus({ status: "ok" }))).toBe("正常");
    expect(getRuntimeStatusLabel(createRuntimeStatus({ status: "warn" }))).toBe("一部制限あり");
    expect(getRuntimeStatusLabel(createRuntimeStatus({ status: "error" }))).toBe("要確認");
  });

  it("finds services by id", () => {
    const status = createRuntimeStatus();

    expect(findRuntimeService(status, "go-internal-ai")?.summary).toBe("利用可能");
    expect(findRuntimeService(status, "missing")).toBeNull();
    expect(findRuntimeService(null, "go-internal-ai")).toBeNull();
  });
});

describe("useRuntimeStatus", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("accepts an initial runtime status from the app bootstrap flow", async () => {
    installDom();
    const initialStatus = createRuntimeStatus({ status: "warn" });

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    await act(async () => {
      latest?.setStatus(initialStatus);
      await flushAsyncWork();
    });

    expect(latest?.status).toBe(initialStatus);
    expect(latest?.statusLabel).toBe("一部制限あり");
    expect(latest?.aiGenerationService?.id).toBe("go-internal-ai");
    expect(latest?.fetcherService?.id).toBe("novel-fetcher");

    await act(async () => {
      root?.unmount();
    });
  });

  it("refreshes runtime status through the API boundary", async () => {
    installDom();
    const refreshedStatus = createRuntimeStatus({ status: "error" });
    const fetchStatus = vi.fn().mockResolvedValue(refreshedStatus);

    let latest: HookResult | null = null;
    let root: Root | null = null;
    await act(async () => {
      root = renderHookHarness({
        fetchStatus,
        onRender: (result) => {
          latest = result;
        }
      });
      await flushAsyncWork();
    });

    await act(async () => {
      await latest?.refreshStatus();
      await flushAsyncWork();
    });

    expect(fetchStatus).toHaveBeenCalledTimes(1);
    expect(latest?.status).toBe(refreshedStatus);
    expect(latest?.statusLabel).toBe("要確認");

    await act(async () => {
      root?.unmount();
    });
  });
});
