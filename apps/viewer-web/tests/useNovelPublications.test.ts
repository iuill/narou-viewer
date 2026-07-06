import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import { useNovelPublications } from "../src/hooks/useNovelPublications";

type HookResult = ReturnType<typeof useNovelPublications>;

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

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status
  });
}

async function flushAsyncWork(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
}

function renderHookHarness(props: {
  novelId: string | null;
  onError?: (message: string | null) => void;
  onSaved?: () => void;
  onRender: (result: HookResult) => void;
}): Root {
  const rootElement = document.getElementById("root");
  if (!rootElement) {
    throw new Error("root element is missing");
  }

  function Harness() {
    const result = useNovelPublications({
      novelId: props.novelId,
      onError: props.onError ?? vi.fn(),
      onSaved: props.onSaved
    });
    props.onRender(result);
    return null;
  }

  const root = createRoot(rootElement);
  root.render(createElement(Harness));
  return root;
}

function renderMutableHookHarness(props: {
  novelId: string | null;
  onError?: (message: string | null) => void;
  onSaved?: () => void;
  onRender: (result: HookResult) => void;
}): { root: Root; rerender: (novelId: string | null) => void } {
  const rootElement = document.getElementById("root");
  if (!rootElement) {
    throw new Error("root element is missing");
  }
  const state = { ...props };

  function Harness() {
    const result = useNovelPublications({
      novelId: state.novelId,
      onError: state.onError ?? vi.fn(),
      onSaved: state.onSaved
    });
    state.onRender(result);
    return null;
  }

  const root = createRoot(rootElement);
  root.render(createElement(Harness));
  return {
    root,
    rerender: (novelId) => {
      state.novelId = novelId;
      root.render(createElement(Harness));
    }
  };
}

function deferred<T>(): { promise: Promise<T>; resolve: (value: T) => void; reject: (error: unknown) => void } {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve;
    reject = promiseReject;
  });
  return { promise, resolve, reject };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("useNovelPublications", () => {
  it("保存に成功したら onSaved を呼ぶ", async () => {
    installDom();
    const onSaved = vi.fn();
    const onError = vi.fn();
    let latest: HookResult | null = null;
    const fetchMock = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      const requestUrl = new URL(url.toString(), "http://localhost");
      if (requestUrl.pathname === "/api/library/novels/n1/publications" && !init?.method) {
        return jsonResponse({
          novelId: "n1",
          displayCoverEntryId: "",
          entries: []
        });
      }
      if (requestUrl.pathname === "/api/library/novels/n1/publications/entries" && init?.method === "POST") {
        return jsonResponse({
          novelId: "n1",
          displayCoverEntryId: "",
          entries: [
            { id: "novel-9784040000008", kind: "novel", status: "manual", override: "isbn", isbn13: "9784040000008" }
          ]
        });
      }
      throw new Error(`Unhandled fetch: ${requestUrl.pathname}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    const root = renderHookHarness({
      novelId: "n1",
      onError,
      onSaved,
      onRender: (result) => {
        latest = result;
      }
    });

    await act(async () => {
      await flushAsyncWork();
    });
    if (!latest) {
      throw new Error("hook did not render");
    }

    await act(async () => {
      await latest?.createEntry({ kind: "novel", mode: "isbn", isbn13: "9784040000008" });
    });

    expect(onSaved).toHaveBeenCalledTimes(1);
    expect(onError).toHaveBeenLastCalledWith(null);
    expect(latest.entries[0]?.isbn13).toBe("9784040000008");

    await act(async () => {
      root.unmount();
    });
  });

  it("保存中に作品が切り替わったら古い保存結果を反映しない", async () => {
    installDom();
    const onSaved = vi.fn();
    const onError = vi.fn();
    let latest: HookResult | null = null;
    const n1Create = deferred<Response>();
    const fetchMock = vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      const requestUrl = new URL(url.toString(), "http://localhost");
      if (requestUrl.pathname === "/api/library/novels/n1/publications" && !init?.method) {
        return jsonResponse({ novelId: "n1", displayCoverEntryId: "", entries: [] });
      }
      if (requestUrl.pathname === "/api/library/novels/n2/publications" && !init?.method) {
        return jsonResponse({
          novelId: "n2",
          displayCoverEntryId: "",
          entries: [{ id: "comic-9784040000015", kind: "comic", status: "manual", override: "isbn", isbn13: "9784040000015" }]
        });
      }
      if (requestUrl.pathname === "/api/library/novels/n1/publications/entries" && init?.method === "POST") {
        return n1Create.promise;
      }
      throw new Error(`Unhandled fetch: ${requestUrl.pathname}`);
    });
    vi.stubGlobal("fetch", fetchMock);

    const { root, rerender } = renderMutableHookHarness({
      novelId: "n1",
      onError,
      onSaved,
      onRender: (result) => {
        latest = result;
      }
    });

    await act(async () => {
      await flushAsyncWork();
    });
    const savePromise = latest?.createEntry({ kind: "novel", mode: "isbn", isbn13: "9784040000008" });
    await act(async () => {
      rerender("n2");
      await flushAsyncWork();
    });

    n1Create.resolve(
      jsonResponse({
        novelId: "n1",
        displayCoverEntryId: "",
        entries: [{ id: "novel-9784040000008", kind: "novel", status: "manual", override: "isbn", isbn13: "9784040000008" }]
      })
    );
    await act(async () => {
      await savePromise;
      await flushAsyncWork();
    });

    expect(onSaved).not.toHaveBeenCalled();
    expect(latest?.data?.novelId).toBe("n2");
    expect(latest?.entries[0]?.isbn13).toBe("9784040000015");

    await act(async () => {
      root.unmount();
    });
  });
});
