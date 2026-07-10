import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";

import App from "../src/App";
import { formatDate } from "../src/shared/date";
import { API_CLIENT_UPDATE_REQUIRED_EVENT } from "../src/api/contract";

type FetchHandler = (url: string, init?: RequestInit) => Promise<Response>;

class MockAppSpeechSynthesisUtterance {
  lang = "";
  rate = 1;
  voice: SpeechSynthesisVoice | undefined;
  onstart: (() => void) | null = null;
  onresume: (() => void) | null = null;
  onboundary: ((event: { charIndex: number; elapsedTime?: number }) => void) | null = null;
  onend: (() => void) | null = null;
  onerror: ((event: { error?: string }) => void) | null = null;

  constructor(public readonly text: string) {}
}

function createAppMockVoice(overrides: Partial<SpeechSynthesisVoice> = {}): SpeechSynthesisVoice {
  return {
    default: true,
    lang: "ja-JP",
    localService: true,
    name: "日本語音声",
    voiceURI: "voice:ja",
    ...overrides
  } as SpeechSynthesisVoice;
}

function createAppMockSpeechSynthesis(options?: {
  voices?: SpeechSynthesisVoice[];
  autoStart?: boolean;
}) {
  const voices = options?.voices ?? [createAppMockVoice()];
  const autoStart = options?.autoStart ?? true;
  const listeners = new Map<string, Set<() => void>>();
  const mock = {
    utterances: [] as MockAppSpeechSynthesisUtterance[],
    onvoiceschanged: null as (() => void) | null,
    getVoices: vi.fn(() => voices),
    speak: vi.fn((utterance: MockAppSpeechSynthesisUtterance) => {
      mock.utterances.push(utterance);
      if (autoStart) {
        utterance.onstart?.();
      }
    }),
    pause: vi.fn(),
    resume: vi.fn(),
    cancel: vi.fn(),
    addEventListener: vi.fn((type: string, listener: () => void) => {
      const bucket = listeners.get(type) ?? new Set<() => void>();
      bucket.add(listener);
      listeners.set(type, bucket);
    }),
    removeEventListener: vi.fn((type: string, listener: () => void) => {
      listeners.get(type)?.delete(listener);
    }),
    dispatch(type: string) {
      for (const listener of listeners.get(type) ?? []) {
        listener();
      }
      if (type === "voiceschanged") {
        mock.onvoiceschanged?.();
      }
    }
  };

  return mock;
}

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: {
      "content-type": "application/json"
    },
    ...init
  });
}

function createMatchMedia(options?: { viewportWidth?: number; coarsePointer?: boolean }) {
  const viewportWidth = options?.viewportWidth ?? 1280;
  const coarsePointer = options?.coarsePointer ?? false;

  return (query: string) => {
    const minWidthMatches = [...query.matchAll(/\(min-width:\s*(\d+)px\)/g)].every(
      (match) => viewportWidth >= Number.parseInt(match[1] ?? "0", 10)
    );
    const maxWidthMatches = [...query.matchAll(/\(max-width:\s*(\d+)px\)/g)].every(
      (match) => viewportWidth <= Number.parseInt(match[1] ?? "0", 10)
    );
    const pointerMatches = query.includes("(pointer: coarse)") ? coarsePointer : true;

    return {
      matches: minWidthMatches && maxWidthMatches && pointerMatches,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn()
    };
  };
}

function installDom(
  fetchHandler: FetchHandler,
  options?: {
    url?: string;
    matchesMobile?: boolean;
    viewportWidth?: number;
    coarsePointer?: boolean;
    maxTouchPoints?: number;
    speechSynthesis?: unknown;
    speechSynthesisUtterance?: unknown;
  }
): JSDOM {
  const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
    url: options?.url ?? "http://localhost/"
  });
  const viewportWidth = options?.viewportWidth ?? (options?.matchesMobile ? 720 : 1280);
  const coarsePointer = options?.coarsePointer ?? false;
  const maxTouchPoints = options?.maxTouchPoints ?? 0;

  Object.defineProperty(dom.window.document, "hidden", {
    configurable: true,
    value: false
  });

  Object.defineProperty(dom.window.navigator, "maxTouchPoints", {
    configurable: true,
    value: maxTouchPoints
  });

  Object.defineProperty(dom.window.HTMLElement.prototype, "clientWidth", {
    configurable: true,
    get() {
      return 800;
    }
  });
  Object.defineProperty(dom.window.HTMLElement.prototype, "clientHeight", {
    configurable: true,
    get() {
      return 600;
    }
  });
  Object.defineProperty(dom.window.HTMLElement.prototype, "scrollWidth", {
    configurable: true,
    get() {
      return 1600;
    }
  });
  Object.defineProperty(dom.window.HTMLElement.prototype, "scrollHeight", {
    configurable: true,
    get() {
      return 1200;
    }
  });
  Object.defineProperty(dom.window.HTMLElement.prototype, "offsetWidth", {
    configurable: true,
    get() {
      return 120;
    }
  });
  Object.defineProperty(dom.window.HTMLImageElement.prototype, "complete", {
    configurable: true,
    get() {
      return true;
    }
  });
  Object.defineProperty(dom.window.HTMLImageElement.prototype, "naturalWidth", {
    configurable: true,
    get() {
      return 640;
    }
  });
  Object.defineProperty(dom.window.HTMLImageElement.prototype, "naturalHeight", {
    configurable: true,
    get() {
      return 480;
    }
  });
  dom.window.HTMLElement.prototype.getBoundingClientRect = function getBoundingClientRect() {
    return {
      x: 0,
      y: 0,
      width: viewportWidth,
      height: 600,
      top: 0,
      right: viewportWidth,
      bottom: 600,
      left: 0,
      toJSON() {
        return {};
      }
    };
  };
  dom.window.HTMLElement.prototype.getClientRects = function getClientRects() {
    return [
      {
        x: 0,
        y: 0,
        width: 400,
        height: 24,
        top: 0,
        right: 400,
        bottom: 24,
        left: 0,
        toJSON() {
          return {};
        }
      }
    ] as unknown as DOMRectList;
  };
  dom.window.HTMLElement.prototype.scrollIntoView = function scrollIntoView() {};
  dom.window.Range.prototype.getBoundingClientRect = function getBoundingClientRect() {
    return {
      x: 0,
      y: 0,
      width: 24,
      height: 24,
      top: 0,
      right: 24,
      bottom: 24,
      left: 0,
      toJSON() {
        return {};
      }
    };
  };
  dom.window.Range.prototype.getClientRects = function getClientRects() {
    return [
      {
        x: 0,
        y: 0,
        width: 24,
        height: 24,
        top: 0,
        right: 24,
        bottom: 24,
        left: 0,
        toJSON() {
          return {};
        }
      }
    ] as unknown as DOMRectList;
  };

  Object.defineProperty(dom.window, "innerWidth", {
    configurable: true,
    value: viewportWidth
  });
  dom.window.matchMedia = createMatchMedia({ viewportWidth, coarsePointer }) as typeof dom.window.matchMedia;
  dom.window.setTimeout = globalThis.setTimeout.bind(globalThis) as typeof dom.window.setTimeout;
  dom.window.clearTimeout = globalThis.clearTimeout.bind(globalThis) as typeof dom.window.clearTimeout;
  dom.window.requestAnimationFrame = ((callback: FrameRequestCallback) => dom.window.setTimeout(() => callback(0), 0)) as typeof dom.window.requestAnimationFrame;
  dom.window.cancelAnimationFrame = ((id: number) => dom.window.clearTimeout(id)) as typeof dom.window.cancelAnimationFrame;

  const speechSynthesis = options?.speechSynthesis;
  const speechSynthesisUtterance = options?.speechSynthesisUtterance;
  if (speechSynthesis) {
    Object.defineProperty(dom.window, "speechSynthesis", {
      configurable: true,
      value: speechSynthesis
    });
  }
  if (speechSynthesisUtterance) {
    Object.defineProperty(dom.window, "SpeechSynthesisUtterance", {
      configurable: true,
      value: speechSynthesisUtterance
    });
  }

  class ResizeObserverMock {
    observe() {}
    disconnect() {}
    unobserve() {}
  }

  vi.stubGlobal("window", dom.window);
  vi.stubGlobal("document", dom.window.document);
  vi.stubGlobal("navigator", dom.window.navigator);
  vi.stubGlobal("HTMLElement", dom.window.HTMLElement);
  vi.stubGlobal("Element", dom.window.Element);
  vi.stubGlobal("HTMLInputElement", dom.window.HTMLInputElement);
  vi.stubGlobal("HTMLButtonElement", dom.window.HTMLButtonElement);
  vi.stubGlobal("HTMLFormElement", dom.window.HTMLFormElement);
  vi.stubGlobal("HTMLSelectElement", dom.window.HTMLSelectElement);
  vi.stubGlobal("HTMLImageElement", dom.window.HTMLImageElement);
  vi.stubGlobal("HTMLAnchorElement", dom.window.HTMLAnchorElement);
  vi.stubGlobal("Node", dom.window.Node);
  vi.stubGlobal("Range", dom.window.Range);
  vi.stubGlobal("Text", dom.window.Text);
  vi.stubGlobal("Event", dom.window.Event);
  vi.stubGlobal("InputEvent", dom.window.InputEvent);
  vi.stubGlobal("MouseEvent", dom.window.MouseEvent);
  vi.stubGlobal("PointerEvent", dom.window.MouseEvent);
  vi.stubGlobal("ResizeObserver", ResizeObserverMock);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: RequestInfo | URL, init?: RequestInit) => {
      try {
        return await fetchHandler(url, init);
      } catch (error) {
        const requestUrl = new URL(url, "http://localhost");
        if (error instanceof Error && error.message.startsWith("Unhandled fetch:") && isReaderSettingsRequest(requestUrl)) {
          return jsonResponse(defaultNovelReaderSettings(requestUrl, init));
        }
        if (error instanceof Error && error.message.startsWith("Unhandled fetch:") && isPublicationsRequest(requestUrl)) {
          return jsonResponse(defaultNovelPublications(requestUrl));
        }
        throw error;
      }
    })
  );
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);

  return dom;
}

function isReaderSettingsRequest(requestUrl: URL): boolean {
  return /^\/api\/library\/novels\/[^/]+\/reader-settings$/.test(requestUrl.pathname);
}

function isPublicationsRequest(requestUrl: URL): boolean {
  return /^\/api\/library\/novels\/[^/]+\/publications$/.test(requestUrl.pathname);
}

function defaultNovelPublications(requestUrl: URL): Record<string, unknown> {
  const match = requestUrl.pathname.match(/^\/api\/library\/novels\/([^/]+)\/publications$/);
  return {
    novelId: match?.[1] ?? "",
    displayCoverEntryId: "",
    entries: []
  };
}

function defaultNovelReaderSettings(requestUrl: URL, init?: RequestInit): Record<string, unknown> {
  const match = requestUrl.pathname.match(/^\/api\/library\/novels\/([^/]+)\/reader-settings$/);
  const body = typeof init?.body === "string" ? (JSON.parse(init.body) as Record<string, unknown>) : {};
  const correction =
    typeof body.correction === "object" && body.correction !== null
      ? {
          quoteNormalization: true,
          hyphenDashNormalization: true,
          parenthesisNormalization: true,
          halfwidthAlnumPunctuationNormalization: true,
          ...(body.correction as Record<string, unknown>)
        }
      : {
          quoteNormalization: true,
          hyphenDashNormalization: true,
          parenthesisNormalization: true,
          halfwidthAlnumPunctuationNormalization: true
        };
  return {
    novelId: match ? decodeURIComponent(match[1] ?? "") : "",
    correction,
    updatedAt: null
  };
}

async function renderApp(
  fetchHandler: FetchHandler,
  options?: {
    url?: string;
    matchesMobile?: boolean;
    viewportWidth?: number;
    coarsePointer?: boolean;
    maxTouchPoints?: number;
    speechSynthesis?: unknown;
    speechSynthesisUtterance?: unknown;
    beforeRender?: (dom: JSDOM) => void;
  }
): Promise<{ container: HTMLElement; root: Root; dom: JSDOM }> {
  const dom = installDom(fetchHandler, options);
  const container = dom.window.document.getElementById("root");

  if (!container) {
    throw new Error("root container not found");
  }

  const root = createRoot(container);
  options?.beforeRender?.(dom);

  await act(async () => {
    root.render(createElement(App));
  });

  return { container, root, dom };
}

function createReaderFetchHandler(options?: {
  readerDocument?: Record<string, unknown>;
  plainTextLength?: number;
  contentEtag?: string;
  episodes?: Array<Record<string, unknown>>;
  totalEpisodes?: number;
  episodeResponses?: Record<string, Record<string, unknown>>;
  readingMode?: "vertical" | "horizontal";
  readerSettingsResponse?: Response | Promise<Response>;
}) {
  const readerDocument = options?.readerDocument ?? {
    version: 1,
    blocks: [
      { type: "title", text: "第1話" },
      {
        type: "paragraph",
        section: "body",
        inlines: [{ type: "text", text: "本文です。" }]
      }
    ]
  };
  const plainTextLength = options?.plainTextLength ?? 10;
  const contentEtag = options?.contentEtag ?? "etag-1";
  const readingMode = options?.readingMode ?? "vertical";
  const readerSettingsResponse = options?.readerSettingsResponse;
  const episodes = options?.episodes ?? [
    {
      episodeIndex: "1",
      title: "第1話",
      chapter: "第一章",
      subchapter: null,
      sourceUrl: "https://example.com/n1/1/",
      updatedAt: "2026-03-20T10:00:00Z",
      contentEtag
    },
    {
      episodeIndex: "2",
      title: "第2話",
      chapter: "第一章",
      subchapter: null,
      sourceUrl: "https://example.com/n1/2/",
      updatedAt: "2026-03-21T10:00:00Z",
      contentEtag: "etag-2"
    }
  ];
  const totalEpisodes = options?.totalEpisodes ?? episodes.length;
  const episodeResponses = options?.episodeResponses ?? {};

  const fetchHandler: FetchHandler = async (url, init) => {
    const requestUrl = new URL(url, "http://localhost");

    if (requestUrl.pathname === "/api/system/status") {
      return jsonResponse({
        status: "ok",
        checkedAt: "2026-03-22T10:00:00Z",
        services: [{ id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" }]
      });
    }

    if (requestUrl.pathname === "/api/library/novels") {
      return jsonResponse({
        novels: [
          {
            novelId: "n1",
            fetcherWorkId: "work-1",
            title: "小説A",
            author: "作者A",
            siteName: "小説家になろう",
            tocUrl: "https://example.com/n1",
            updatedAt: "2026-03-22T10:00:00Z",
            lastReadEpisodeIndex: "1",
            lastReadEpisodeTitle: "第1話",
            latestBookmarkEpisodeIndex: "1",
            bookmarkCount: 0,
            totalEpisodes: 2
          }
        ]
      });
    }

    if (requestUrl.pathname === "/api/reader/preferences") {
      return jsonResponse({
        readingMode,
        fontFamily: "mincho",
        theme: "classic",
        updatedAt: "2026-03-22T10:00:00Z"
      });
    }

    if (requestUrl.pathname === "/api/fetcher/queue") {
      return jsonResponse({ total: 0, webWorker: 0, worker: 0, running: false });
    }

    if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
      return jsonResponse({
        current: null,
        queued: [],
        recentCompleted: [],
        recentFailed: [],
        completedCount: 0,
        failedCount: 0,
        convertCurrent: null,
        convertQueued: []
      });
    }

    if (requestUrl.pathname === "/api/ai-generation/settings") {
      return jsonResponse({
        apiBaseUrlConfigured: true,
        masterPassphraseConfigured: true,
        preferredMode: "heuristic",
        effectiveGenerationMode: "heuristic",
        settings: {
          selectedProfileId: "default",
          profiles: [
            {
              id: "default",
              label: "Default",
              hasApiKey: false,
              apiKeyMasked: null,
              modelId: null,
              providerOrder: [],
              allowFallbacks: true,
              requireParameters: false,
              updatedAt: "2026-03-22T10:00:00Z"
            }
          ]
        }
      });
    }

    if (requestUrl.pathname === "/api/ai-generation/jobs") {
      return jsonResponse({ jobs: [] });
    }

    if (requestUrl.pathname === "/api/library/novels/n1/reader-settings") {
      return readerSettingsResponse ?? jsonResponse({
        novelId: "n1",
        correction: {
          quoteNormalization: false,
          hyphenDashNormalization: false,
          parenthesisNormalization: false,
          halfwidthAlnumPunctuationNormalization: false
        },
        updatedAt: null
      });
    }

    if (requestUrl.pathname === "/api/library/novels/n1/toc") {
      return jsonResponse({
        novelId: "n1",
        fetcherWorkId: "work-1",
        title: "小説A",
        author: "作者A",
        siteName: "小説家になろう",
        tocUrl: "https://example.com/n1",
        updatedAt: "2026-03-22T10:00:00Z",
        totalEpisodes,
        story: "reader speech test story",
        episodes
      });
    }

    if (requestUrl.pathname === "/api/reader/state") {
      if (init?.method === "PUT") {
        return jsonResponse({
          novelId: "n1",
          lastReadEpisodeIndex: "1",
          position: 0,
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      return jsonResponse({
        novelId: "n1",
        lastReadEpisodeIndex: "1",
        position: 0,
        updatedAt: "2026-03-22T10:00:00Z"
      });
    }

    if (requestUrl.pathname === "/api/bookmarks") {
      return jsonResponse({ bookmarks: [] });
    }

    const episodeMatch = requestUrl.pathname.match(/^\/api\/library\/novels\/n1\/episodes\/(.+)$/);
    if (episodeMatch) {
      const episodeIndex = episodeMatch[1] ?? "1";
      const response = episodeResponses[episodeIndex];
      if (response) {
        return jsonResponse(response);
      }
      if (episodeIndex !== "1") {
        return new Response("not found", { status: 404 });
      }
      return jsonResponse({
        novelId: "n1",
        episodeIndex: "1",
        title: "第1話",
        chapter: "第一章",
        subchapter: null,
        sourceUrl: "https://example.com/n1/1/",
        html: "",
        plainTextLength,
        updatedAt: "2026-03-22T10:00:00Z",
        contentEtag,
        readerDocument
      });
    }

    throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
  };

  return fetchHandler;
}

async function waitFor(predicate: () => boolean, timeoutMs = 3000): Promise<void> {
  const start = Date.now();

  while (Date.now() - start < timeoutMs) {
    await act(async () => {
      await Promise.resolve();
      await new Promise((resolve) => setTimeout(resolve, 10));
    });

    if (predicate()) {
      return;
    }
  }

  throw new Error("timed out waiting for condition");
}

function getButtonByText(container: HTMLElement, text: string, className?: string): HTMLButtonElement {
  const normalizedTarget = text.replace(/\s+/g, " ").trim();
  const button = Array.from(container.querySelectorAll("button")).find((candidate) => {
    const normalizedText = candidate.textContent?.replace(/\s+/g, " ").trim() ?? "";
    const classMatches = className ? candidate.className.includes(className) : true;

    return classMatches && (normalizedText === normalizedTarget || normalizedText.includes(normalizedTarget));
  });

  if (!(button instanceof HTMLButtonElement)) {
    throw new Error(`button not found: ${text}`);
  }

  return button;
}

function getButtonByLabel(container: HTMLElement, label: string): HTMLButtonElement {
  const button = container.querySelector(`button[aria-label="${label}"]`);
  if (!(button instanceof HTMLButtonElement)) {
    throw new Error(`button not found: ${label}`);
  }
  return button;
}

async function click(element: Element, dom: JSDOM, init?: MouseEventInit): Promise<void> {
  await act(async () => {
    element.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true, clientX: 400, clientY: 40, ...init }));
  });
}

async function keyDown(target: Window, key: string): Promise<void> {
  await act(async () => {
    target.dispatchEvent(new target.KeyboardEvent("keydown", { bubbles: true, key }));
  });
}

async function keyDownElement(element: Element, dom: JSDOM, key: string, init?: KeyboardEventInit): Promise<void> {
  await act(async () => {
    element.dispatchEvent(new dom.window.KeyboardEvent("keydown", { bubbles: true, key, ...init }));
  });
}

async function changeSelect(select: HTMLSelectElement, value: string, dom: JSDOM): Promise<void> {
  await act(async () => {
    const descriptor = Object.getOwnPropertyDescriptor(dom.window.HTMLSelectElement.prototype, "value");
    descriptor?.set?.call(select, value);
    select.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
  });
}

async function submitForm(form: HTMLFormElement, dom: JSDOM): Promise<void> {
  await act(async () => {
    form.dispatchEvent(new dom.window.Event("submit", { bubbles: true, cancelable: true }));
  });
}

async function dropText(target: Element, text: string, dom: JSDOM): Promise<void> {
  await act(async () => {
    const event = new dom.window.Event("drop", { bubbles: true, cancelable: true }) as Event & {
      dataTransfer: { getData: (type: string) => string };
    };
    Object.defineProperty(event, "dataTransfer", {
      value: {
        getData(type: string) {
          if (type === "text/plain") {
            return text;
          }
          return "";
        }
      }
    });
    target.dispatchEvent(event);
  });
}

async function pointerDown(element: Element, dom: JSDOM): Promise<void> {
  await act(async () => {
    element.dispatchEvent(new dom.window.MouseEvent("pointerdown", { bubbles: true }));
  });
}

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("App", () => {
  it("配信中のビルドが変わっていると起動時に更新要求を表示する", async () => {
    const baseFetchHandler = createReaderFetchHandler();
    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");
      if (requestUrl.pathname === "/build-info.json") {
        return jsonResponse({
          version: "0.1.0",
          gitHash: "newer-build",
          gitShortHash: "newer-b",
          gitCommitDate: "2026-03-23T10:00:00Z"
        });
      }

      return baseFetchHandler(url, init);
    };

    const { container, root } = await renderApp(fetchHandler);

    await waitFor(() => Boolean(container.querySelector(".client-update-required")));
    const dialog = container.querySelector(".client-update-required");
    expect(dialog?.getAttribute("role")).toBe("alertdialog");
    expect(dialog?.textContent).toContain("アプリの更新が必要です");
    expect(dialog?.textContent).toContain("再読み込み");

    await act(async () => {
      root.unmount();
    });
  });

  it("配信中のビルドが同じなら起動時に更新要求を表示しない", async () => {
    const baseFetchHandler = createReaderFetchHandler();
    let buildInfoRequested = false;
    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");
      if (requestUrl.pathname === "/build-info.json") {
        buildInfoRequested = true;
        return jsonResponse({
          version: "0.0.0",
          gitHash: null,
          gitShortHash: null,
          gitCommitDate: null
        });
      }

      return baseFetchHandler(url, init);
    };

    const { container, root } = await renderApp(fetchHandler);

    await waitFor(() => buildInfoRequested && container.textContent?.includes("小説A") === true);
    expect(container.querySelector(".client-update-required")).toBeNull();

    await act(async () => {
      root.unmount();
    });
  });

  it("API契約更新が必要なときに閉じられない更新要求を表示する", async () => {
    const { container, root, dom } = await renderApp(createReaderFetchHandler());

    await waitFor(() => container.textContent?.includes("小説A") === true);

    await act(async () => {
      dom.window.dispatchEvent(
        new dom.window.CustomEvent(API_CLIENT_UPDATE_REQUIRED_EVENT, {
          detail: {
            message: "Client update required.",
            code: "CLIENT_UPDATE_REQUIRED",
            status: 426,
            minApiContractVersion: "1"
          }
        })
      );
    });

    await waitFor(() => Boolean(container.querySelector(".client-update-required")));
    const dialog = container.querySelector(".client-update-required");
    expect(dialog?.getAttribute("role")).toBe("alertdialog");
    expect(dialog?.getAttribute("aria-modal")).toBe("true");
    expect(dialog?.textContent).toContain("アプリの更新が必要です");
    expect(dialog?.textContent).toContain("再読み込み");
    expect(dialog?.querySelector('[aria-label*="閉じる"]')).toBeNull();

    await act(async () => {
      root.unmount();
    });
  });

  it("初期ロード後に主要 popover と AI 設定ビューを開ける", async () => {
    const fetchHandler: FetchHandler = async (url) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return jsonResponse({
          status: "warn",
          checkedAt: "2026-03-22T10:00:00Z",
          services: [
            { id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" },
            {
              id: "novel-fetcher",
              label: "novel-fetcher",
              status: "ok",
              summary: "ok",
              detail: "稼働中",
              versionInfo: {
                current: "3.8.0",
                latest: "3.8.1",
                updateAvailable: true
              }
            },
            {
              id: "google-books",
              label: "Google Books",
              status: "warn",
              summary: "API key未設定",
              detail: "GOOGLE_BOOKS_API_KEY が未設定です。Google Books による表紙画像と補助書誌の取得は行いません。"
            },
            { id: "go-internal-ai", label: "Go internal AI", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "library", label: "library", status: "ok", summary: "ok", detail: "2 作品" }
          ]
        });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: [
            {
              novelId: "n1",
              fetcherWorkId: "101",
              title: "小説A",
              author: "作者A",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n1",
              story: "これはタブレット向けのあらすじです。".repeat(8),
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: "2",
              lastReadEpisodeTitle: "第2話",
              latestBookmarkEpisodeIndex: "2",
              bookmarkCount: 1,
              totalEpisodes: 12
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        return jsonResponse({
          readingMode: "vertical",
          fontFamily: "mincho",
          theme: "classic",
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({
          total: 1,
          webWorker: 0,
          worker: 1,
          running: true
        });
      }

      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        return jsonResponse({
          current: {
            id: "task-1",
            type: "download",
            novelId: "n1",
            novelTitle: "小説A",
            novelAuthor: "作者A",
            status: "running",
            message: "取得中",
            createdAt: "2026-03-22T10:00:00Z",
            startedAt: "2026-03-22T10:01:00Z",
            progress: 50,
            totalSteps: 10,
            currentStep: 5
          },
          queued: [],
          recentCompleted: [],
          recentFailed: [],
          completedCount: 0,
          failedCount: 0,
          convertCurrent: null,
          convertQueued: []
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        return jsonResponse({
          apiBaseUrlConfigured: true,
          masterPassphraseConfigured: true,
          preferredMode: "heuristic",
          effectiveGenerationMode: "heuristic",
          settings: {
            selectedProfileId: "default",
            profiles: [
              {
                id: "default",
                label: "Default",
                hasApiKey: false,
                apiKeyMasked: null,
                modelId: null,
                providerOrder: [],
                allowFallbacks: true,
                requireParameters: false,
                updatedAt: "2026-03-22T10:00:00Z"
              }
            ]
          }
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/toc") {
        return jsonResponse({
          novelId: "n1",
          fetcherWorkId: "work-1",
          title: "小説A",
          author: "作者A",
          siteName: "小説家になろう",
          tocUrl: "https://example.com/n1",
          updatedAt: "2026-03-22T10:00:00Z",
          totalEpisodes: 12,
          story: "これはとても長いあらすじです。".repeat(10),
          episodes: [
            {
              episodeIndex: "1",
              title: "第1話",
              chapter: "第一章",
              subchapter: null,
              sourceUrl: "https://example.com/n1/1/",
              updatedAt: "2026-03-20T10:00:00Z",
              contentEtag: "etag-1"
            },
            {
              episodeIndex: "2",
              title: "第2話",
              chapter: "第一章",
              subchapter: null,
              updatedAt: "2026-03-21T10:00:00Z",
              contentEtag: "etag-2"
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/state") {
        return jsonResponse({
          novelId: "n1",
          lastReadEpisodeIndex: "2",
          position: 0,
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/bookmarks") {
        return jsonResponse({
          bookmarks: [
            {
              id: "b1",
              novelId: "n1",
              episodeIndex: "2",
              position: 0,
              label: "しおり",
              createdAt: "2026-03-22T10:00:00Z"
            }
          ]
        });
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { container, root, dom } = await renderApp(fetchHandler);

    await waitFor(() => container.textContent?.includes("Library") === true && container.textContent?.includes("小説A") === true);

    expect(container.textContent).toContain("最終既読");
    expect(container.textContent).toContain("第2話");
    expect(container.textContent).toContain("novel-fetcher 3.8.1 が利用できます。現在は 3.8.0 を使用中です。");
    expect(container.textContent).toContain("GOOGLE_BOOKS_API_KEY が未設定です。");

    await click(container.querySelector('button[aria-label="小説を追加"]') as Element, dom);
    await waitFor(() => container.textContent?.includes("ダウンロード") === true);
    await click(getButtonByText(container, "閉じる"), dom);

    await click(getButtonByText(container, "動作状況"), dom);
    await waitFor(() => container.textContent?.includes("サービスの状態") === true);
    await pointerDown(dom.window.document.body, dom);

    await click(getButtonByText(container, "取得状況"), dom);
    await waitFor(() => container.textContent?.includes("取得状況") === true);

    await click(getButtonByText(container, "AI機能"), dom);
    await waitFor(() => Boolean(container.querySelector(".ai-generation-popover")));
    await click(getButtonByText(container, "設定", "ai-generation-nav-button"), dom);
    await waitFor(() => Boolean(container.querySelector("#ai-generation-workspace")));
    await click(container.querySelector(".ai-workspace-close") as Element, dom);

    await act(async () => {
      root.unmount();
    });
  });

  it("モバイルの状況タブからAI設定ビューを開くとワークスペースへスクロールとフォーカスを移す", async () => {
    let settingsRequestCount = 0;
    let releaseDelayedSettings: (() => void) | null = null;
    const settingsResponse = () =>
      jsonResponse({
        apiBaseUrlConfigured: true,
        masterPassphraseConfigured: true,
        preferredMode: "heuristic",
        effectiveGenerationMode: "heuristic",
        settings: {
          selectedProfileId: "default",
          profiles: [
            {
              id: "default",
              label: "Default",
              hasApiKey: false,
              apiKeyMasked: null,
              modelId: null,
              providerOrder: [],
              allowFallbacks: true,
              requireParameters: false,
              updatedAt: "2026-03-22T10:00:00Z"
            }
          ]
        }
      });
    const fetchHandler: FetchHandler = async (url) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return jsonResponse({
          status: "ok",
          checkedAt: "2026-03-22T10:00:00Z",
          services: [
            { id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "novel-fetcher", label: "novel-fetcher", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "go-internal-ai", label: "Go internal AI", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "library", label: "library", status: "ok", summary: "ok", detail: "1 作品" }
          ]
        });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: [
            {
              novelId: "n1",
              fetcherWorkId: "101",
              title: "小説A",
              author: "作者A",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n1",
              story: "これはモバイル向けのあらすじです。",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: null,
              lastReadEpisodeTitle: null,
              latestBookmarkEpisodeIndex: null,
              bookmarkCount: 0,
              totalEpisodes: 12
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        return jsonResponse({
          readingMode: "vertical",
          fontFamily: "mincho",
          theme: "classic",
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({ total: 0, webWorker: 0, worker: 0, running: false });
      }

      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        return jsonResponse({ current: null, queued: [], recentCompleted: [], recentFailed: [], completedCount: 0, failedCount: 0 });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        settingsRequestCount += 1;
        if (settingsRequestCount >= 2) {
          await new Promise<void>((resolve) => {
            releaseDelayedSettings = resolve;
          });
        }

        return settingsResponse();
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      coarsePointer: true,
      matchesMobile: true,
      maxTouchPoints: 1,
      viewportWidth: 390
    });
    const scrollIntoView = vi.spyOn(dom.window.HTMLElement.prototype, "scrollIntoView");
    scrollIntoView.mockClear();

    await waitFor(() => container.textContent?.includes("Library") === true && container.textContent?.includes("小説A") === true);

    const statusTab = Array.from(container.querySelectorAll(".mobile-home-tabs button")).find((button) =>
      button.textContent?.includes("状況")
    );
    expect(statusTab).toBeInstanceOf(dom.window.HTMLButtonElement);
    if (!(statusTab instanceof dom.window.HTMLButtonElement)) {
      throw new Error("mobile status tab not found");
    }

    await click(statusTab, dom);
    await waitFor(() => Boolean(container.querySelector(".mobile-status-panel")));
    await click(getButtonByText(container, "設定", "ai-generation-nav-button"), dom);
    await waitFor(() => Boolean(container.querySelector("#ai-generation-workspace")));
    await waitFor(() => settingsRequestCount >= 2);

    const workspaceHost = container.querySelector(".ai-generation-workspace-host");
    expect(workspaceHost).toBeInstanceOf(dom.window.HTMLElement);
    if (!(workspaceHost instanceof dom.window.HTMLElement)) {
      throw new Error("AI workspace host not found");
    }

    await waitFor(() => scrollIntoView.mock.contexts.includes(workspaceHost) && dom.window.document.activeElement === workspaceHost);
    expect(workspaceHost.tabIndex).toBe(-1);
    const scrollCallIndex = scrollIntoView.mock.contexts.indexOf(workspaceHost);
    expect(scrollIntoView.mock.calls[scrollCallIndex]).toEqual([{ behavior: "smooth", block: "start" }]);

    releaseDelayedSettings?.();
    await waitFor(() => Boolean(container.querySelector("#ai-generation-workspace")));

    await act(async () => {
      root.unmount();
    });
  });

  it("モバイル取得タブでは同一作品の実行中タスクがある更新と再開を無効化する", async () => {
    const fetchHandler: FetchHandler = async (url) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return jsonResponse({
          status: "ok",
          checkedAt: "2026-03-22T10:00:00Z",
          services: [
            { id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "novel-fetcher", label: "novel-fetcher", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "go-internal-ai", label: "Go internal AI", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "library", label: "library", status: "ok", summary: "ok", detail: "2 作品" }
          ]
        });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: [
            {
              novelId: "n1",
              fetcherWorkId: "101",
              title: "更新対象",
              author: "作者A",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n1",
              story: "更新対象のあらすじです。",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: null,
              lastReadEpisodeTitle: null,
              latestBookmarkEpisodeIndex: null,
              bookmarkCount: 0,
              totalEpisodes: 12
            },
            {
              novelId: "n2",
              fetcherWorkId: "102",
              title: "再開対象",
              author: "作者B",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n2",
              story: "再開対象のあらすじです。",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: null,
              lastReadEpisodeTitle: null,
              latestBookmarkEpisodeIndex: null,
              bookmarkCount: 0,
              totalEpisodes: 12,
              savedEpisodes: 4,
              fetchStatus: "partial"
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        return jsonResponse({
          readingMode: "vertical",
          fontFamily: "mincho",
          theme: "classic",
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({ total: 2, webWorker: 0, worker: 1, running: true });
      }

      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        return jsonResponse({
          current: {
            id: "update-running",
            type: "update",
            novelIds: ["101"],
            novelTitle: "更新対象",
            status: "running",
            message: "更新中"
          },
          queued: [
            {
              id: "resume-queued",
              type: "resume",
              novelIds: ["102"],
              novelTitle: "再開対象",
              status: "queued",
              message: "再開待ち"
            }
          ],
          recentCompleted: [],
          recentFailed: [],
          completedCount: 0,
          failedCount: 0,
          convertCurrent: null,
          convertQueued: []
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        return jsonResponse({
          apiBaseUrlConfigured: true,
          masterPassphraseConfigured: true,
          preferredMode: "heuristic",
          effectiveGenerationMode: "heuristic",
          settings: { selectedProfileId: "default", profiles: [] }
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      coarsePointer: true,
      matchesMobile: true,
      maxTouchPoints: 1,
      viewportWidth: 390
    });

    await waitFor(() => container.textContent?.includes("Library") === true && container.textContent?.includes("更新対象") === true);

    const downloadTab = Array.from(container.querySelectorAll(".mobile-home-tabs button")).find((button) =>
      button.textContent?.includes("取得")
    );
    expect(downloadTab).toBeInstanceOf(dom.window.HTMLButtonElement);
    if (!(downloadTab instanceof dom.window.HTMLButtonElement)) {
      throw new Error("mobile download tab not found");
    }

    await click(downloadTab, dom);
    await waitFor(() => container.textContent?.includes("保存済み作品の更新") === true);

    const rows = Array.from(container.querySelectorAll(".library-maintenance-row"));
    const updateButton = rows.find((row) => row.textContent?.includes("更新対象"))?.querySelector("button");
    const resumeButton = rows
      .filter((row) => row.textContent?.includes("再開対象"))
      .at(-1)
      ?.querySelector("button");

    expect(updateButton).toBeInstanceOf(dom.window.HTMLButtonElement);
    expect(resumeButton).toBeInstanceOf(dom.window.HTMLButtonElement);
    if (!(updateButton instanceof dom.window.HTMLButtonElement) || !(resumeButton instanceof dom.window.HTMLButtonElement)) {
      throw new Error("busy action buttons not found");
    }

    await waitFor(() => updateButton.disabled && resumeButton.disabled);
    expect(updateButton.textContent).toContain("更新中...");
    expect(resumeButton.textContent).toContain("再開中...");

    await act(async () => {
      root.unmount();
    });
  });

  it("モバイル取得タブでは古い空ステータスで送信中の更新ボタンを再有効化しない", async () => {
    const emptyTaskSummary = {
      current: null,
      queued: [],
      recentCompleted: [],
      recentFailed: [],
      completedCount: 0,
      failedCount: 0,
      convertCurrent: null,
      convertQueued: []
    };
    let releaseInitialTasks: (() => void) | null = null;
    const initialTasksResponse = new Promise<Response>((resolve) => {
      releaseInitialTasks = () => resolve(jsonResponse(emptyTaskSummary));
    });
    let releaseUpdate: (() => void) | null = null;
    let updateRequested = false;
    let taskSummaryRequests = 0;

    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return jsonResponse({
          status: "ok",
          checkedAt: "2026-03-22T10:00:00Z",
          services: [
            { id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "novel-fetcher", label: "novel-fetcher", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "library", label: "library", status: "ok", summary: "ok", detail: "1 作品" }
          ]
        });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: [
            {
              novelId: "n1",
              fetcherWorkId: "101",
              title: "更新対象",
              author: "作者A",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n1",
              story: "更新対象のあらすじです。",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: null,
              lastReadEpisodeTitle: null,
              latestBookmarkEpisodeIndex: null,
              bookmarkCount: 0,
              totalEpisodes: 12
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        return jsonResponse({
          readingMode: "vertical",
          fontFamily: "mincho",
          theme: "classic",
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({ total: 0, webWorker: 0, worker: 0, running: false });
      }

      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        taskSummaryRequests += 1;
        if (taskSummaryRequests === 1) {
          return initialTasksResponse;
        }
        return jsonResponse(emptyTaskSummary);
      }

      if (requestUrl.pathname === "/api/fetcher/works/update") {
        expect(init?.method).toBe("POST");
        updateRequested = true;
        return new Promise<Response>((resolve) => {
          releaseUpdate = () =>
            resolve(
              jsonResponse({
                taskIds: ["update-task-1"],
                message: "更新を開始しました。"
              })
            );
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        return jsonResponse({
          apiBaseUrlConfigured: true,
          masterPassphraseConfigured: true,
          preferredMode: "heuristic",
          effectiveGenerationMode: "heuristic",
          settings: { selectedProfileId: "default", profiles: [] }
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      coarsePointer: true,
      matchesMobile: true,
      maxTouchPoints: 1,
      viewportWidth: 390
    });

    await waitFor(() => container.textContent?.includes("Library") === true && container.textContent?.includes("更新対象") === true);

    const downloadTab = Array.from(container.querySelectorAll(".mobile-home-tabs button")).find((button) =>
      button.textContent?.includes("取得")
    );
    expect(downloadTab).toBeInstanceOf(dom.window.HTMLButtonElement);
    if (!(downloadTab instanceof dom.window.HTMLButtonElement)) {
      throw new Error("mobile download tab not found");
    }

    await click(downloadTab, dom);
    await waitFor(() => container.textContent?.includes("保存済み作品の更新") === true);

    const getUpdateButton = () => {
      const row = Array.from(container.querySelectorAll(".library-maintenance-row")).find((item) =>
        item.textContent?.includes("更新対象")
      );
      const button = row?.querySelector("button");
      if (!(button instanceof dom.window.HTMLButtonElement)) {
        throw new Error("update button not found");
      }
      return button;
    };

    await click(getUpdateButton(), dom);
    await waitFor(() => updateRequested && getUpdateButton().disabled);

    await act(async () => {
      releaseInitialTasks?.();
    });
    await waitFor(() => taskSummaryRequests >= 1);
    expect(getUpdateButton().disabled).toBe(true);

    await act(async () => {
      releaseUpdate?.();
    });
    await waitFor(() => container.textContent?.includes("更新を開始しました。taskId: update-task-1") === true);
    expect(getUpdateButton().disabled).toBe(true);

    await act(async () => {
      root.unmount();
    });
  });

  it("モバイル取得タブではactive検出後のdegraded空ステータスで更新ボタンを再有効化しない", async () => {
    const emptyTaskSummary = {
      current: null,
      queued: [],
      recentCompleted: [],
      recentFailed: [],
      completedCount: 0,
      failedCount: 0,
      convertCurrent: null,
      convertQueued: []
    };
    let updateAccepted = false;
    let taskSummaryRequestsAfterUpdate = 0;

    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return jsonResponse({
          status: "ok",
          checkedAt: "2026-03-22T10:00:00Z",
          services: [
            { id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "novel-fetcher", label: "novel-fetcher", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "library", label: "library", status: "ok", summary: "ok", detail: "1 作品" }
          ]
        });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: [
            {
              novelId: "n1",
              fetcherWorkId: "101",
              title: "更新対象",
              author: "作者A",
              siteName: "小説家になろう",
              tocUrl: "https://ncode.syosetu.com/n1234ab/",
              story: "更新対象のあらすじです。",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: null,
              lastReadEpisodeTitle: null,
              latestBookmarkEpisodeIndex: null,
              bookmarkCount: 0,
              totalEpisodes: 12
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        return jsonResponse({
          readingMode: "vertical",
          fontFamily: "mincho",
          theme: "classic",
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({
          total: updateAccepted ? 1 : 0,
          webWorker: 0,
          worker: updateAccepted ? 1 : 0,
          running: updateAccepted
        });
      }

      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        if (!updateAccepted) {
          return jsonResponse(emptyTaskSummary);
        }
        taskSummaryRequestsAfterUpdate += 1;
        if (taskSummaryRequestsAfterUpdate === 1) {
          return jsonResponse({
            current: {
              id: "update-task-1",
              type: "update",
              novelIds: ["101"],
              novelTitle: "更新対象",
              status: "running",
              message: "更新中"
            },
            queued: [],
            recentCompleted: [],
            recentFailed: [],
            completedCount: 0,
            failedCount: 0,
            convertCurrent: null,
            convertQueued: []
          });
        }
        return jsonResponse({
          ...emptyTaskSummary,
          available: false,
          degraded: true
        });
      }

      if (requestUrl.pathname === "/api/fetcher/works/update") {
        expect(init?.method).toBe("POST");
        updateAccepted = true;
        return jsonResponse({
          taskIds: ["update-task-1"],
          message: "更新を開始しました。"
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        return jsonResponse({
          apiBaseUrlConfigured: true,
          masterPassphraseConfigured: true,
          preferredMode: "heuristic",
          effectiveGenerationMode: "heuristic",
          settings: { selectedProfileId: "default", profiles: [] }
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      coarsePointer: true,
      matchesMobile: true,
      maxTouchPoints: 1,
      viewportWidth: 390
    });

    await waitFor(() => container.textContent?.includes("Library") === true && container.textContent?.includes("更新対象") === true);

    const downloadTab = Array.from(container.querySelectorAll(".mobile-home-tabs button")).find((button) =>
      button.textContent?.includes("取得")
    );
    expect(downloadTab).toBeInstanceOf(dom.window.HTMLButtonElement);
    if (!(downloadTab instanceof dom.window.HTMLButtonElement)) {
      throw new Error("mobile download tab not found");
    }

    await click(downloadTab, dom);
    await waitFor(() => container.textContent?.includes("保存済み作品の更新") === true);

    const getUpdateButton = () => {
      const row = Array.from(container.querySelectorAll(".library-maintenance-row")).find((item) =>
        item.textContent?.includes("更新対象")
      );
      const button = row?.querySelector("button");
      if (!(button instanceof dom.window.HTMLButtonElement)) {
        throw new Error("update button not found");
      }
      return button;
    };

    await click(getUpdateButton(), dom);
    await waitFor(() => taskSummaryRequestsAfterUpdate >= 1 && getUpdateButton().disabled);
    expect(getUpdateButton().textContent).toContain("更新中...");

    await waitFor(() => taskSummaryRequestsAfterUpdate >= 2, 4000);
    expect(getUpdateButton().disabled).toBe(true);
    expect(getUpdateButton().textContent).toContain("更新中...");

    await act(async () => {
      root.unmount();
    });
  });

  it("モバイル取得タブでは外部実行中タスクがdegraded空ステータス後も更新ボタンをロックする", async () => {
    const emptyTaskSummary = {
      current: null,
      queued: [],
      recentCompleted: [],
      recentFailed: [],
      completedCount: 0,
      failedCount: 0,
      convertCurrent: null,
      convertQueued: []
    };
    let taskSummaryRequests = 0;

    const fetchHandler: FetchHandler = async (url) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return jsonResponse({
          status: "ok",
          checkedAt: "2026-03-22T10:00:00Z",
          services: [
            { id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "novel-fetcher", label: "novel-fetcher", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "library", label: "library", status: "ok", summary: "ok", detail: "1 作品" }
          ]
        });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: [
            {
              novelId: "n1",
              fetcherWorkId: "101",
              title: "更新対象",
              author: "作者A",
              siteName: "小説家になろう",
              tocUrl: "https://ncode.syosetu.com/n1234ab/",
              story: "更新対象のあらすじです。",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: null,
              lastReadEpisodeTitle: null,
              latestBookmarkEpisodeIndex: null,
              bookmarkCount: 0,
              totalEpisodes: 12
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        return jsonResponse({
          readingMode: "vertical",
          fontFamily: "mincho",
          theme: "classic",
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({ total: 1, webWorker: 0, worker: 1, running: true });
      }

      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        taskSummaryRequests += 1;
        if (taskSummaryRequests === 1) {
          return jsonResponse({
            current: {
              id: "update-task-1",
              type: "update",
              novelIds: ["101"],
              novelTitle: "更新対象",
              status: "running",
              message: "更新中"
            },
            queued: [],
            recentCompleted: [],
            recentFailed: [],
            completedCount: 0,
            failedCount: 0,
            convertCurrent: null,
            convertQueued: []
          });
        }
        return jsonResponse({
          ...emptyTaskSummary,
          available: false,
          degraded: true
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        return jsonResponse({
          apiBaseUrlConfigured: true,
          masterPassphraseConfigured: true,
          preferredMode: "heuristic",
          effectiveGenerationMode: "heuristic",
          settings: { selectedProfileId: "default", profiles: [] }
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      coarsePointer: true,
      matchesMobile: true,
      maxTouchPoints: 1,
      viewportWidth: 390
    });

    await waitFor(() => container.textContent?.includes("Library") === true && container.textContent?.includes("更新対象") === true);

    const downloadTab = Array.from(container.querySelectorAll(".mobile-home-tabs button")).find((button) =>
      button.textContent?.includes("取得")
    );
    expect(downloadTab).toBeInstanceOf(dom.window.HTMLButtonElement);
    if (!(downloadTab instanceof dom.window.HTMLButtonElement)) {
      throw new Error("mobile download tab not found");
    }

    await click(downloadTab, dom);
    await waitFor(() => container.textContent?.includes("保存済み作品の更新") === true);

    const getUpdateButton = () => {
      const row = Array.from(container.querySelectorAll(".library-maintenance-row")).find((item) =>
        item.textContent?.includes("更新対象")
      );
      const button = row?.querySelector("button");
      if (!(button instanceof dom.window.HTMLButtonElement)) {
        throw new Error("update button not found");
      }
      return button;
    };

    await waitFor(() => taskSummaryRequests >= 1 && getUpdateButton().disabled);
    expect(getUpdateButton().textContent).toContain("更新中...");

    await waitFor(() => taskSummaryRequests >= 2, 5000);
    expect(getUpdateButton().disabled).toBe(true);
    expect(getUpdateButton().textContent).toContain("更新中...");

    await act(async () => {
      root.unmount();
    });
  });

  it("モバイル取得タブではbare Nコードのdownload開始後も既存作品の更新ボタンを無効化する", async () => {
    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return jsonResponse({
          status: "ok",
          checkedAt: "2026-03-22T10:00:00Z",
          services: [
            { id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "novel-fetcher", label: "novel-fetcher", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "library", label: "library", status: "ok", summary: "ok", detail: "1 作品" }
          ]
        });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: [
            {
              novelId: "n1",
              fetcherWorkId: "101",
              title: "更新対象",
              author: "作者A",
              siteName: "小説家になろう",
              tocUrl: "https://ncode.syosetu.com/n1234ab/",
              story: "更新対象のあらすじです。",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: null,
              lastReadEpisodeTitle: null,
              latestBookmarkEpisodeIndex: null,
              bookmarkCount: 0,
              totalEpisodes: 12
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        return jsonResponse({
          readingMode: "vertical",
          fontFamily: "mincho",
          theme: "classic",
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({ total: 0, webWorker: 0, worker: 0, running: false });
      }

      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        return jsonResponse({
          current: null,
          queued: [],
          recentCompleted: [],
          recentFailed: [],
          completedCount: 0,
          failedCount: 0,
          convertCurrent: null,
          convertQueued: []
        });
      }

      if (requestUrl.pathname === "/api/fetcher/works/download") {
        expect(init?.method).toBe("POST");
        expect(JSON.parse(String(init?.body))).toEqual({
          targets: ["n1234ab"],
          force: false,
          convertAfterDownload: false,
          mail: false
        });
        return jsonResponse({
          taskIds: ["download-task-1"],
          message: "ダウンロードを開始しました。"
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        return jsonResponse({
          apiBaseUrlConfigured: true,
          masterPassphraseConfigured: true,
          preferredMode: "heuristic",
          effectiveGenerationMode: "heuristic",
          settings: { selectedProfileId: "default", profiles: [] }
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      coarsePointer: true,
      matchesMobile: true,
      maxTouchPoints: 1,
      viewportWidth: 390
    });

    await waitFor(() => container.textContent?.includes("Library") === true && container.textContent?.includes("更新対象") === true);

    const downloadTab = Array.from(container.querySelectorAll(".mobile-home-tabs button")).find((button) =>
      button.textContent?.includes("取得")
    );
    expect(downloadTab).toBeInstanceOf(dom.window.HTMLButtonElement);
    if (!(downloadTab instanceof dom.window.HTMLButtonElement)) {
      throw new Error("mobile download tab not found");
    }

    await click(downloadTab, dom);
    await waitFor(() => container.textContent?.includes("保存済み作品の更新") === true);

    const composer = container.querySelector(".library-download-composer");
    if (!(composer instanceof dom.window.HTMLDivElement)) {
      throw new Error("download composer not found");
    }
    await dropText(composer, "n1234ab", dom);
    const downloadForm = container.querySelector(".download-form");
    expect(downloadForm).toBeInstanceOf(dom.window.HTMLFormElement);
    if (!(downloadForm instanceof dom.window.HTMLFormElement)) {
      throw new Error("download form not found");
    }
    await submitForm(downloadForm, dom);
    await waitFor(() => container.textContent?.includes("ダウンロードを開始しました。taskId: download-task-1") === true);

    const row = Array.from(container.querySelectorAll(".library-maintenance-row")).find((item) =>
      item.textContent?.includes("更新対象")
    );
    const updateButton = row?.querySelector("button");
    expect(updateButton).toBeInstanceOf(dom.window.HTMLButtonElement);
    if (!(updateButton instanceof dom.window.HTMLButtonElement)) {
      throw new Error("update button not found");
    }
    expect(updateButton.disabled).toBe(true);
    expect(updateButton.textContent).toContain("更新中...");

    await act(async () => {
      root.unmount();
    });
  });

  it("初期ロード失敗時はエラーメッセージを表示する", async () => {
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
    const fetchHandler: FetchHandler = async (url) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return new Response("boom", { status: 500 });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({ novels: [] });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        return new Response("boom", { status: 500 });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        return new Response("boom", { status: 500 });
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return new Response("boom", { status: 500 });
      }

      if (requestUrl.pathname === "/api/fetcher/queue" || requestUrl.pathname === "/api/fetcher/tasks/summary") {
        return new Response("boom", { status: 500 });
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}`);
    };

    const { container, root } = await renderApp(fetchHandler);

    await waitFor(() => container.textContent?.includes("初期データの取得に失敗しました。") === true);
    expect(container.textContent).toContain("初期データの取得に失敗しました。");

    await act(async () => {
      root.unmount();
    });

    consoleError.mockRestore();
  });

  it("モバイル表示では作品選択後に最終既読の本文へ直接遷移する", async () => {
    const fetchHandler: FetchHandler = async (url) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return jsonResponse({
          status: "ok",
          checkedAt: "2026-03-22T10:00:00Z",
          services: [
            { id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "novel-fetcher", label: "novel-fetcher", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "library", label: "library", status: "ok", summary: "ok", detail: "1 作品" }
          ]
        });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: [
            {
              novelId: "n1",
              fetcherWorkId: "work-1",
              title: "小説A",
              author: "作者A",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n1",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: "2",
              lastReadEpisodeTitle: "第2話",
              latestBookmarkEpisodeIndex: "2",
              bookmarkCount: 1,
              totalEpisodes: 12
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        return jsonResponse({
          readingMode: "vertical",
          fontFamily: "mincho",
          theme: "classic",
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({ total: 0, webWorker: 0, worker: 0, running: false });
      }

      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        return jsonResponse({
          current: null,
          queued: [],
          recentCompleted: [],
          recentFailed: [],
          completedCount: 0,
          failedCount: 0,
          convertCurrent: null,
          convertQueued: []
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        return jsonResponse({
          apiBaseUrlConfigured: true,
          masterPassphraseConfigured: true,
          preferredMode: "heuristic",
          effectiveGenerationMode: "heuristic",
          settings: {
            selectedProfileId: "default",
            profiles: []
          }
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/toc") {
        return jsonResponse({
          novelId: "n1",
          fetcherWorkId: "work-1",
          title: "小説A",
          author: "作者A",
          siteName: "小説家になろう",
          tocUrl: "https://example.com/n1",
          updatedAt: "2026-03-22T10:00:00Z",
          totalEpisodes: 12,
          story: "あらすじ",
          episodes: [
            {
              episodeIndex: "1",
              title: "第1話",
              chapter: "第一章",
              subchapter: null,
              updatedAt: "2026-03-20T10:00:00Z",
              contentEtag: "etag-1"
            },
            {
              episodeIndex: "2",
              title: "第2話",
              chapter: "第一章",
              subchapter: null,
              updatedAt: "2026-03-21T10:00:00Z",
              contentEtag: "etag-2"
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/state") {
        return jsonResponse({
          novelId: "n1",
          lastReadEpisodeIndex: "2",
          position: 7,
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/bookmarks") {
        return jsonResponse({
          bookmarks: [
            {
              id: "b1",
              novelId: "n1",
              episodeIndex: "2",
              position: 4,
              label: "しおり",
              createdAt: "2026-03-22T10:00:00Z"
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/episodes/2") {
        return jsonResponse({
          novelId: "n1",
          episodeIndex: "2",
          title: "第2話",
          chapter: "第一章",
          subchapter: null,
          html: "",
          plainTextLength: 10,
          updatedAt: "2026-03-22T10:00:00Z",
          contentEtag: "etag-2",
          readerDocument: {
            version: 1,
            blocks: [
              { type: "title", text: "第2話" },
              {
                type: "paragraph",
                section: "body",
                inlines: [{ type: "text", text: "本文2です。" }]
              }
            ]
          }
        });
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      matchesMobile: true,
      coarsePointer: true,
      maxTouchPoints: 5
    });

    await waitFor(() => container.textContent?.includes("Library") === true && container.textContent?.includes("小説A") === true);

    await click(getButtonByText(container, "小説A"), dom);

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("本文2です。") === true);
    expect(dom.window.location.search).toContain("novelId=n1");
    expect(dom.window.location.search).toContain("episode=2");
    expect(dom.window.location.search).toContain("pos=7");

    await click(container.querySelector('button[aria-label="栞"]') as Element, dom);
    await waitFor(() => container.textContent?.includes("この位置に栞を追加") === true);
    expect(container.textContent).toContain("しおり");
    await click(container.querySelector('button[aria-label="栞を閉じる"]') as Element, dom);

    await click(container.querySelector('button[aria-label="一覧へ戻る"]') as Element, dom);
    await waitFor(() => Boolean(container.querySelector(".library-panel")) && container.textContent?.includes("小説A") === true);

    await click(getButtonByText(container, "小説A"), dom);
    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("本文2です。") === true);

    await act(async () => {
      root.unmount();
    });
  });

  it("タブレット幅のタッチ端末ではトップページに作品詳細を表示しない", async () => {
    const fetchHandler: FetchHandler = async (url) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return jsonResponse({
          status: "ok",
          checkedAt: "2026-03-22T10:00:00Z",
          services: [
            { id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "novel-fetcher", label: "novel-fetcher", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "library", label: "library", status: "ok", summary: "ok", detail: "1 作品" }
          ]
        });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: [
            {
              novelId: "n1",
              fetcherWorkId: "work-1",
              title: "小説A",
              author: "作者A",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n1",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: "2",
              lastReadEpisodeTitle: "第2話",
              latestBookmarkEpisodeIndex: "2",
              bookmarkCount: 1,
              totalEpisodes: 12
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        return jsonResponse({
          readingMode: "vertical",
          fontFamily: "mincho",
          theme: "classic",
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({ total: 0, webWorker: 0, worker: 0, running: false });
      }

      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        return jsonResponse({
          current: null,
          queued: [],
          recentCompleted: [],
          recentFailed: [],
          completedCount: 0,
          failedCount: 0,
          convertCurrent: null,
          convertQueued: []
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        return jsonResponse({
          apiBaseUrlConfigured: true,
          masterPassphraseConfigured: true,
          preferredMode: "heuristic",
          effectiveGenerationMode: "heuristic",
          settings: {
            selectedProfileId: "default",
            profiles: []
          }
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/toc") {
        return jsonResponse({
          novelId: "n1",
          fetcherWorkId: "work-1",
          title: "小説A",
          author: "作者A",
          siteName: "小説家になろう",
          tocUrl: "https://example.com/n1",
          updatedAt: "2026-03-22T10:00:00Z",
          totalEpisodes: 12,
          story: "あらすじ",
          episodes: [
            {
              episodeIndex: "1",
              title: "第1話",
              chapter: "第一章",
              subchapter: null,
              updatedAt: "2026-03-20T10:00:00Z",
              contentEtag: "etag-1"
            },
            {
              episodeIndex: "2",
              title: "第2話",
              chapter: "第一章",
              subchapter: null,
              updatedAt: "2026-03-21T10:00:00Z",
              contentEtag: "etag-2"
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/state") {
        return jsonResponse({
          novelId: "n1",
          lastReadEpisodeIndex: "2",
          position: 7,
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/bookmarks") {
        return jsonResponse({
          bookmarks: [
            {
              id: "b1",
              novelId: "n1",
              episodeIndex: "2",
              position: 4,
              label: "しおり",
              createdAt: "2026-03-22T10:00:00Z"
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/episodes/2") {
        return jsonResponse({
          novelId: "n1",
          episodeIndex: "2",
          title: "第2話",
          chapter: "第一章",
          subchapter: null,
          html: "",
          plainTextLength: 10,
          updatedAt: "2026-03-22T10:00:00Z",
          contentEtag: "etag-2",
          readerDocument: {
            version: 1,
            blocks: [
              { type: "title", text: "第2話" },
              {
                type: "paragraph",
                section: "body",
                inlines: [{ type: "text", text: "本文2です。" }]
              }
            ]
          }
        });
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { container, root } = await renderApp(fetchHandler, {
      viewportWidth: 1024,
      coarsePointer: true,
      maxTouchPoints: 5
    });

    await waitFor(() => container.textContent?.includes("Library") === true && container.textContent?.includes("小説A") === true);

    expect(container.querySelector(".toc-panel")).toBeNull();
    expect(container.textContent).not.toContain("作品詳細");
    expect(container.textContent).toContain("あらすじ");
    expect(container.querySelector(".reader-shell")).toBeNull();

    await act(async () => {
      root.unmount();
    });
  });

  it("reader 画面で主要パネルと画像ビューアを開ける", async () => {
    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return jsonResponse({
          status: "ok",
          checkedAt: "2026-03-22T10:00:00Z",
          services: [
            { id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" }
          ]
        });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: [
            {
              novelId: "n1",
              fetcherWorkId: "work-1",
              title: "小説A",
              author: "作者A",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n1",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: "1",
              lastReadEpisodeTitle: "第1話",
              latestBookmarkEpisodeIndex: "1",
              bookmarkCount: 1,
              totalEpisodes: 2
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        if (init?.method === "PUT") {
          return jsonResponse({
            readingMode: "vertical",
            fontFamily: "mincho",
            theme: "classic",
            updatedAt: "2026-03-22T10:00:00Z"
          });
        }

        return jsonResponse({
          readingMode: "vertical",
          fontFamily: "mincho",
          theme: "classic",
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({ total: 0, webWorker: 0, worker: 0, running: false });
      }

      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        return jsonResponse({
          current: null,
          queued: [],
          recentCompleted: [],
          recentFailed: [],
          completedCount: 0,
          failedCount: 0,
          convertCurrent: null,
          convertQueued: []
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        return jsonResponse({
          apiBaseUrlConfigured: true,
          masterPassphraseConfigured: true,
          preferredMode: "heuristic",
          effectiveGenerationMode: "heuristic",
          settings: {
            selectedProfileId: "default",
            profiles: [
              {
                id: "default",
                label: "Default",
                hasApiKey: false,
                apiKeyMasked: null,
                modelId: null,
                providerOrder: [],
                allowFallbacks: true,
                requireParameters: false,
                updatedAt: "2026-03-22T10:00:00Z"
              }
            ]
          }
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/toc") {
        return jsonResponse({
          novelId: "n1",
          fetcherWorkId: "work-1",
          title: "小説A",
          author: "作者A",
          siteName: "小説家になろう",
          tocUrl: "https://example.com/n1",
          updatedAt: "2026-03-22T10:00:00Z",
          totalEpisodes: 3,
          story: "reader test story",
          episodes: [
            {
              episodeIndex: "1",
              title: "第1話",
              chapter: "第一章",
              subchapter: null,
              updatedAt: "2026-03-20T10:00:00Z",
              contentEtag: "etag-1"
            },
            {
              episodeIndex: "2",
              title: "第2話",
              chapter: "第一章",
              subchapter: null,
              sourceUrl: "https://example.com/n1/2/",
              updatedAt: "2026-03-21T10:00:00Z",
              contentEtag: "etag-2",
              bodyStatus: "partial"
            },
            {
              episodeIndex: "3",
              title: "第3話",
              chapter: "第一章",
              subchapter: null,
              sourceUrl: "https://example.com/n1/3/",
              updatedAt: "2026-03-22T10:00:00Z",
              contentEtag: "etag-3",
              bodyStatus: "partial"
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/state") {
        if (init?.method === "PUT") {
          return jsonResponse({
            novelId: "n1",
            lastReadEpisodeIndex: "1",
            position: 0,
            updatedAt: "2026-03-22T10:00:00Z"
          });
        }

        return jsonResponse({
          novelId: "n1",
          lastReadEpisodeIndex: "1",
          position: 0,
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/bookmarks") {
        return jsonResponse({
          bookmarks: [
            {
              id: "b1",
              novelId: "n1",
              episodeIndex: "1",
              position: 0,
              label: "しおり",
              createdAt: "2026-03-22T10:00:00Z"
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/episodes/1") {
        return jsonResponse({
          novelId: "n1",
          episodeIndex: "1",
          title: "第1話",
          chapter: "第一章",
          subchapter: null,
          sourceUrl: "https://example.com/n1/1/",
          html: "",
          plainTextLength: 10,
          updatedAt: "2026-03-22T10:00:00Z",
          contentEtag: "etag-1",
          readerDocument: {
            version: 1,
            blocks: [
              { type: "title", text: "第1話" },
              {
                type: "paragraph",
                section: "body",
                inlines: [{ type: "text", text: "本文です。" }]
              },
              {
                type: "image",
                section: "body",
                src: "/cover.jpg",
                alt: "挿絵",
                originalUrl: "https://example.com/cover.jpg",
                title: "挿絵タイトル"
              }
            ]
          }
        });
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=1"
    });

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("第1話") === true);
    const visibleReaderActionLabels = Array.from(container.querySelectorAll(".reader-bottom-controls > button")).map((button) =>
      button.getAttribute("aria-label")
    );
    expect(visibleReaderActionLabels).toEqual([
      "一覧へ戻る",
      "次の話へ進む",
      "次のページへ進む",
      "前のページへ戻る",
      "前の話へ戻る",
      "目次",
      "栞を追加",
      "読書設定",
      "実験フォント",
      "キャラクター一覧",
      "読書AI",
      "情報",
      "フルスクリーン表示"
    ]);
    const nextEpisodeButton = container.querySelector('button[aria-label="次の話へ進む"]');
    expect(nextEpisodeButton).toBeInstanceOf(dom.window.HTMLButtonElement);
    expect((nextEpisodeButton as HTMLButtonElement).disabled).toBe(true);

    await click(container.querySelector('button[aria-label="読書設定"]') as Element, dom);
    await waitFor(() => Boolean(container.querySelector(".reader-settings-panel")));
    expect(container.querySelector(".reader-info-panel")).toBeNull();
    expect(container.querySelector(".reader-toc-panel")).toBeNull();

    await click(container.querySelector('button[aria-label="情報"]') as Element, dom);
    await waitFor(() => Boolean(container.querySelector(".reader-info-panel")));
    expect(container.querySelector(".reader-settings-panel")).toBeNull();
    expect(container.querySelector(".reader-toc-panel")).toBeNull();
    expect(container.querySelector(".reader-info-panel")?.textContent).toContain("小説家になろう");
    expect(container.querySelector(".reader-info-panel")?.textContent).toContain("ページ");
    expect(container.querySelector(".reader-info-panel")?.textContent).toContain("https://example.com/n1/1/");

    await click(container.querySelector('button[aria-label="目次"]') as Element, dom);
    await waitFor(() => Boolean(container.querySelector(".reader-toc-panel")));
    expect(container.querySelector(".reader-settings-panel")).toBeNull();
    expect(container.querySelector(".reader-info-panel")).toBeNull();
    const unfetchedTocButton = container.querySelector(
      '.reader-toc-panel button[data-reader-panel-item="toc-episode"][data-episode-index="2"]'
    );
    expect(unfetchedTocButton).toBeInstanceOf(dom.window.HTMLButtonElement);
    expect((unfetchedTocButton as HTMLButtonElement).disabled).toBe(true);
    expect(unfetchedTocButton?.textContent).toContain("未取得");

    await pointerDown(dom.window.document.body, dom);
    await waitFor(() => container.querySelector(".reader-toc-panel") === null);

    await click(container.querySelector('button[aria-label="目次"]') as Element, dom);
    await waitFor(() => Boolean(container.querySelector(".reader-toc-panel")));

    const image = container.querySelector(".reader-prose-paged img");
    if (!(image instanceof dom.window.HTMLImageElement)) {
      throw new Error("reader image not found");
    }

    await click(image, dom, { clientX: 400, clientY: 40 });
    await waitFor(() => Boolean(container.querySelector(".reader-image-viewer")));
    await click(container.querySelector('button[aria-label="画像情報"]') as Element, dom);
    await waitFor(() => container.textContent?.includes("オリジナル画像を開く") === true);
    await click(container.querySelector('button[aria-label="画像拡大表示を閉じる"]') as Element, dom);

    await act(async () => {
      root.unmount();
    });
  });

  it("reader 画面の目次を開くと別ページから現在話の行へ戻ってスクロールする", async () => {
    const episodes = Array.from({ length: 75 }, (_, index) => {
      const episodeNumber = index + 1;
      return {
        episodeIndex: String(episodeNumber),
        title: `第${episodeNumber}話`,
        chapter: "第一章",
        subchapter: null,
        sourceUrl: `https://example.com/n1/${episodeNumber}/`,
        updatedAt: "2026-03-22T10:00:00Z",
        contentEtag: `etag-${episodeNumber}`
      };
    });
    const { container, root, dom } = await renderApp(
      createReaderFetchHandler({
        episodes,
        episodeResponses: {
          "75": {
            novelId: "n1",
            episodeIndex: "75",
            title: "第75話",
            chapter: "第一章",
            subchapter: null,
            sourceUrl: "https://example.com/n1/75/",
            html: "",
            plainTextLength: 10,
            updatedAt: "2026-03-22T10:00:00Z",
            contentEtag: "etag-75",
            readerDocument: {
              version: 1,
              blocks: [
                { type: "title", text: "第75話" },
                { type: "paragraph", section: "body", inlines: [{ type: "text", text: "七十五話の本文です。" }] }
              ]
            }
          }
        }
      }),
      {
        url: "http://localhost/?novelId=n1&episode=75"
      }
    );

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("第75話") === true);
    const scrollIntoView = vi.spyOn(dom.window.HTMLElement.prototype, "scrollIntoView");
    scrollIntoView.mockClear();

    await click(getButtonByLabel(container, "目次"), dom);
    await waitFor(() => container.querySelector(".reader-toc-panel .list-pagination-summary")?.textContent?.includes("51-75 / 75 件") === true);
    const currentTocButton = container.querySelector(
      '.reader-toc-panel button[data-reader-panel-item="toc-episode"][data-episode-index="75"]'
    );

    expect(currentTocButton).toBeInstanceOf(dom.window.HTMLButtonElement);
    await waitFor(() => scrollIntoView.mock.contexts.includes(currentTocButton));
    const scrollCallIndex = scrollIntoView.mock.contexts.indexOf(currentTocButton);
    expect(scrollIntoView.mock.calls[scrollCallIndex]).toEqual([{ block: "center", inline: "nearest" }]);

    await click(getButtonByText(container, "前へ"), dom);
    await waitFor(() => container.querySelector(".reader-toc-panel .list-pagination-summary")?.textContent?.includes("1-50 / 75 件") === true);
    expect(container.querySelector('.reader-toc-panel button[data-reader-panel-item="toc-episode"][data-episode-index="75"]')).toBeNull();

    scrollIntoView.mockClear();
    await click(getButtonByLabel(container, "目次を閉じる"), dom);
    await waitFor(() => container.querySelector(".reader-toc-panel") === null);

    await click(getButtonByLabel(container, "目次"), dom);
    await waitFor(() => container.querySelector(".reader-toc-panel .list-pagination-summary")?.textContent?.includes("51-75 / 75 件") === true);
    const reopenedCurrentTocButton = container.querySelector(
      '.reader-toc-panel button[data-reader-panel-item="toc-episode"][data-episode-index="75"]'
    );

    expect(reopenedCurrentTocButton).toBeInstanceOf(dom.window.HTMLButtonElement);
    await waitFor(() => scrollIntoView.mock.contexts.includes(reopenedCurrentTocButton));
    const reopenedScrollCallIndex = scrollIntoView.mock.contexts.indexOf(reopenedCurrentTocButton);
    expect(scrollIntoView.mock.calls[reopenedScrollCallIndex]).toEqual([{ block: "center", inline: "nearest" }]);

    await act(async () => {
      root.unmount();
    });
  });

  it("本文校正の変更は対象フィールドだけを保存リクエストへ送る", async () => {
    const baseFetchHandler = createReaderFetchHandler();
    const savedBodies: Array<Record<string, unknown>> = [];
    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");
      if (requestUrl.pathname === "/api/library/novels/n1/reader-settings" && init?.method === "PUT") {
        savedBodies.push(JSON.parse(String(init.body)) as Record<string, unknown>);
        return jsonResponse(defaultNovelReaderSettings(requestUrl, init));
      }
      return baseFetchHandler(url, init);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=1"
    });

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("第1話") === true);
    await click(getButtonByLabel(container, "読書設定"), dom);
    await waitFor(() => Boolean(container.querySelector(".reader-settings-panel")));

    await changeSelect(container.querySelectorAll(".reader-settings-panel select")[3] as HTMLSelectElement, "enabled", dom);
    await waitFor(() => savedBodies.length === 1);

    expect(savedBodies[0]).toEqual({ correction: { quoteNormalization: true } });

    await act(async () => {
      root.unmount();
    });
  });

  it("別作品へ移動後に前作品の読書設定保存レスポンスを反映しない", async () => {
    let resolveN1SettingsPut: ((value: Response) => void) | null = null;
    const pendingN1SettingsPut = new Promise<Response>((resolve) => {
      resolveN1SettingsPut = resolve;
    });
    const requestedEpisodes: string[] = [];

    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return jsonResponse({
          status: "ok",
          checkedAt: "2026-03-22T10:00:00Z",
          services: [{ id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" }]
        });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: [
            {
              novelId: "n1",
              fetcherWorkId: "work-1",
              title: "小説A",
              author: "作者A",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n1",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: "1",
              lastReadEpisodeTitle: "第1話",
              latestBookmarkEpisodeIndex: null,
              bookmarkCount: 0,
              totalEpisodes: 1
            },
            {
              novelId: "n2",
              fetcherWorkId: "work-2",
              title: "小説B",
              author: "作者B",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n2",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: "1",
              lastReadEpisodeTitle: "第1話",
              latestBookmarkEpisodeIndex: null,
              bookmarkCount: 0,
              totalEpisodes: 1
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        return jsonResponse({
          readingMode: "vertical",
          fontFamily: "mincho",
          theme: "classic",
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({ total: 0, webWorker: 0, worker: 0, running: false });
      }

      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        return jsonResponse({
          current: null,
          queued: [],
          recentCompleted: [],
          recentFailed: [],
          completedCount: 0,
          failedCount: 0,
          convertCurrent: null,
          convertQueued: []
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        return jsonResponse({
          apiBaseUrlConfigured: true,
          masterPassphraseConfigured: true,
          preferredMode: "heuristic",
          effectiveGenerationMode: "heuristic",
          settings: { selectedProfileId: "default", profiles: [] }
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }

      const tocMatch = requestUrl.pathname.match(/^\/api\/library\/novels\/(n[12])\/toc$/);
      if (tocMatch) {
        const novelId = tocMatch[1] ?? "n1";
        return jsonResponse({
          novelId,
          fetcherWorkId: novelId === "n1" ? "work-1" : "work-2",
          title: novelId === "n1" ? "小説A" : "小説B",
          author: novelId === "n1" ? "作者A" : "作者B",
          siteName: "小説家になろう",
          tocUrl: `https://example.com/${novelId}`,
          updatedAt: "2026-03-22T10:00:00Z",
          story: "",
          totalEpisodes: 1,
          episodes: [
            {
              episodeIndex: "1",
              title: "第1話",
              chapter: null,
              subchapter: null,
              sourceUrl: `https://example.com/${novelId}/1/`,
              updatedAt: "2026-03-22T10:00:00Z",
              contentEtag: `${novelId}-toc-1`,
              bodyStatus: "complete"
            }
          ]
        });
      }

      const readerSettingsMatch = requestUrl.pathname.match(/^\/api\/library\/novels\/(n[12])\/reader-settings$/);
      if (readerSettingsMatch) {
        const novelId = readerSettingsMatch[1] ?? "n1";
        if (init?.method === "PUT" && novelId === "n1") {
          return pendingN1SettingsPut;
        }
        return jsonResponse({
          novelId,
          correction: {
            quoteNormalization: false,
            hyphenDashNormalization: false,
            parenthesisNormalization: false,
            halfwidthAlnumPunctuationNormalization: false
          },
          updatedAt: null
        });
      }

      if (requestUrl.pathname === "/api/reader/state") {
        const novelId = requestUrl.searchParams.get("novelId") ?? "n1";
        return jsonResponse({
          novelId,
          lastReadEpisodeIndex: "1",
          position: 0,
          updatedAt: "2026-03-22T10:00:00Z",
          stateVersion: 1,
          updatedByClientId: "server"
        });
      }

      if (requestUrl.pathname === "/api/bookmarks") {
        return jsonResponse({ bookmarks: [] });
      }

      const episodeMatch = requestUrl.pathname.match(/^\/api\/library\/novels\/(n[12])\/episodes\/(.+)$/);
      if (episodeMatch) {
        const novelId = episodeMatch[1] ?? "n1";
        const episodeIndex = episodeMatch[2] ?? "1";
        requestedEpisodes.push(`${novelId}:${episodeIndex}`);
        return jsonResponse({
          novelId,
          episodeIndex,
          title: `${novelId === "n1" ? "小説A" : "小説B"} 第${episodeIndex}話`,
          chapter: null,
          subchapter: null,
          sourceUrl: `https://example.com/${novelId}/${episodeIndex}/`,
          html: "",
          plainTextLength: 4,
          updatedAt: "2026-03-22T10:00:00Z",
          contentEtag: `${novelId}-episode-${episodeIndex}`,
          readerDocument: {
            version: 1,
            blocks: [
              { type: "title", text: `${novelId === "n1" ? "小説A" : "小説B"} 第${episodeIndex}話` },
              { type: "paragraph", section: "body", inlines: [{ type: "text", text: `${novelId}本文` }] }
            ]
          }
        });
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=1"
    });

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("小説A 第1話") === true);
    if (!container.querySelector(".reader-settings-panel")) {
      await click(getButtonByLabel(container, "読書設定"), dom);
    }
    await waitFor(() => Boolean(container.querySelector(".reader-settings-panel")));

    await changeSelect(container.querySelectorAll(".reader-settings-panel select")[3] as HTMLSelectElement, "enabled", dom);
    await waitFor(() => (container.querySelectorAll(".reader-settings-panel select")[3] as HTMLSelectElement).disabled);

    await act(async () => {
      dom.window.history.pushState(null, "", "/?novelId=n2&episode=1");
      dom.window.dispatchEvent(new dom.window.PopStateEvent("popstate"));
    });
    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("小説B 第1話") === true);

    resolveN1SettingsPut?.(
      jsonResponse({
        novelId: "n1",
        correction: {
          quoteNormalization: true,
          hyphenDashNormalization: false,
          parenthesisNormalization: false,
          halfwidthAlnumPunctuationNormalization: false
        },
        updatedAt: "2026-03-22T10:00:00Z"
      })
    );
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    if (!container.querySelector(".reader-settings-panel")) {
      await click(getButtonByLabel(container, "読書設定"), dom);
    }
    await waitFor(() => Boolean(container.querySelector(".reader-settings-panel")));

    const quoteNormalizationSelect = container.querySelectorAll(".reader-settings-panel select")[3] as HTMLSelectElement;
    expect(quoteNormalizationSelect.value).toBe("disabled");
    expect(requestedEpisodes.filter((entry) => entry === "n2:1")).toHaveLength(1);

    await act(async () => {
      root.unmount();
    });
  });

  it("前作品へ戻った後なら前作品の読書設定保存レスポンスを反映する", async () => {
    let resolveN1SettingsPut: ((value: Response) => void) | null = null;
    const pendingN1SettingsPut = new Promise<Response>((resolve) => {
      resolveN1SettingsPut = resolve;
    });

    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return jsonResponse({
          status: "ok",
          checkedAt: "2026-03-22T10:00:00Z",
          services: [{ id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" }]
        });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: [
            {
              novelId: "n1",
              fetcherWorkId: "work-1",
              title: "小説A",
              author: "作者A",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n1",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: "1",
              lastReadEpisodeTitle: "第1話",
              latestBookmarkEpisodeIndex: null,
              bookmarkCount: 0,
              totalEpisodes: 1
            },
            {
              novelId: "n2",
              fetcherWorkId: "work-2",
              title: "小説B",
              author: "作者B",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n2",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: "1",
              lastReadEpisodeTitle: "第1話",
              latestBookmarkEpisodeIndex: null,
              bookmarkCount: 0,
              totalEpisodes: 1
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        return jsonResponse({
          readingMode: "vertical",
          fontFamily: "mincho",
          theme: "classic",
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({ total: 0, webWorker: 0, worker: 0, running: false });
      }

      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        return jsonResponse({
          current: null,
          queued: [],
          recentCompleted: [],
          recentFailed: [],
          completedCount: 0,
          failedCount: 0,
          convertCurrent: null,
          convertQueued: []
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        return jsonResponse({
          apiBaseUrlConfigured: true,
          masterPassphraseConfigured: true,
          preferredMode: "heuristic",
          effectiveGenerationMode: "heuristic",
          settings: { selectedProfileId: "default", profiles: [] }
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }

      const tocMatch = requestUrl.pathname.match(/^\/api\/library\/novels\/(n[12])\/toc$/);
      if (tocMatch) {
        const novelId = tocMatch[1] ?? "n1";
        return jsonResponse({
          novelId,
          fetcherWorkId: novelId === "n1" ? "work-1" : "work-2",
          title: novelId === "n1" ? "小説A" : "小説B",
          author: novelId === "n1" ? "作者A" : "作者B",
          siteName: "小説家になろう",
          tocUrl: `https://example.com/${novelId}`,
          updatedAt: "2026-03-22T10:00:00Z",
          story: "",
          totalEpisodes: 1,
          episodes: [
            {
              episodeIndex: "1",
              title: "第1話",
              chapter: null,
              subchapter: null,
              sourceUrl: `https://example.com/${novelId}/1/`,
              updatedAt: "2026-03-22T10:00:00Z",
              contentEtag: `${novelId}-toc-1`,
              bodyStatus: "complete"
            }
          ]
        });
      }

      const readerSettingsMatch = requestUrl.pathname.match(/^\/api\/library\/novels\/(n[12])\/reader-settings$/);
      if (readerSettingsMatch) {
        const novelId = readerSettingsMatch[1] ?? "n1";
        if (init?.method === "PUT" && novelId === "n1") {
          return pendingN1SettingsPut;
        }
        return jsonResponse({
          novelId,
          correction: {
            quoteNormalization: false,
            hyphenDashNormalization: false,
            parenthesisNormalization: false,
            halfwidthAlnumPunctuationNormalization: false
          },
          updatedAt: null
        });
      }

      if (requestUrl.pathname === "/api/reader/state") {
        const novelId = requestUrl.searchParams.get("novelId") ?? "n1";
        return jsonResponse({
          novelId,
          lastReadEpisodeIndex: "1",
          position: 0,
          updatedAt: "2026-03-22T10:00:00Z",
          stateVersion: 1,
          updatedByClientId: "server"
        });
      }

      if (requestUrl.pathname === "/api/bookmarks") {
        return jsonResponse({ bookmarks: [] });
      }

      const episodeMatch = requestUrl.pathname.match(/^\/api\/library\/novels\/(n[12])\/episodes\/(.+)$/);
      if (episodeMatch) {
        const novelId = episodeMatch[1] ?? "n1";
        const episodeIndex = episodeMatch[2] ?? "1";
        return jsonResponse({
          novelId,
          episodeIndex,
          title: `${novelId === "n1" ? "小説A" : "小説B"} 第${episodeIndex}話`,
          chapter: null,
          subchapter: null,
          sourceUrl: `https://example.com/${novelId}/${episodeIndex}/`,
          html: "",
          plainTextLength: 4,
          updatedAt: "2026-03-22T10:00:00Z",
          contentEtag: `${novelId}-episode-${episodeIndex}`,
          readerDocument: {
            version: 1,
            blocks: [
              { type: "title", text: `${novelId === "n1" ? "小説A" : "小説B"} 第${episodeIndex}話` },
              { type: "paragraph", section: "body", inlines: [{ type: "text", text: `${novelId}本文` }] }
            ]
          }
        });
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=1"
    });

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("小説A 第1話") === true);
    await click(getButtonByLabel(container, "読書設定"), dom);
    await waitFor(() => Boolean(container.querySelector(".reader-settings-panel")));

    await changeSelect(container.querySelectorAll(".reader-settings-panel select")[3] as HTMLSelectElement, "enabled", dom);
    await waitFor(() => (container.querySelectorAll(".reader-settings-panel select")[3] as HTMLSelectElement).disabled);

    await act(async () => {
      dom.window.history.pushState(null, "", "/?novelId=n2&episode=1");
      dom.window.dispatchEvent(new dom.window.PopStateEvent("popstate"));
    });
    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("小説B 第1話") === true);

    await act(async () => {
      dom.window.history.pushState(null, "", "/?novelId=n1&episode=1");
      dom.window.dispatchEvent(new dom.window.PopStateEvent("popstate"));
    });
    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("小説A 第1話") === true);

    resolveN1SettingsPut?.(
      jsonResponse({
        novelId: "n1",
        correction: {
          quoteNormalization: true,
          hyphenDashNormalization: false,
          parenthesisNormalization: false,
          halfwidthAlnumPunctuationNormalization: false
        },
        updatedAt: "2026-03-22T10:00:00Z"
      })
    );

    await waitFor(() => {
      if (!container.querySelector(".reader-settings-panel")) {
        return false;
      }
      return (container.querySelectorAll(".reader-settings-panel select")[3] as HTMLSelectElement).value === "enabled";
    });

    await act(async () => {
      root.unmount();
    });
  });

  it("reader 画面の縦書き keyboard forward で最終ページから次話確認を出す", async () => {
    const { container, root, dom } = await renderApp(createReaderFetchHandler(), {
      url: "http://localhost/?novelId=n1&episode=1"
    });

    await waitFor(() => getButtonByLabel(container, "次のページへ進む").disabled === false);

    await keyDown(dom.window, "ArrowLeft");
    await keyDown(dom.window, "ArrowLeft");

    await waitFor(() => container.textContent?.includes("次の話へ進みますか？") === true);
    const confirmPanel = container.querySelector(".reader-next-episode-confirm");
    expect(confirmPanel?.textContent).toContain("「第2話」を開きます。");
    expect(confirmPanel?.querySelectorAll("button")).toHaveLength(2);
    expect(getButtonByLabel(container, "次の話への移動を閉じる").textContent?.trim()).toBe("×");
    expect(confirmPanel?.textContent).not.toContain("キャンセル");
    await waitFor(() => dom.window.document.activeElement?.textContent?.trim() === "進む");
    expect(getButtonByLabel(container, "一覧へ戻る").disabled).toBe(true);

    await keyDown(dom.window, "ArrowRight");
    expect(container.querySelector(".reader-next-episode-confirm")).not.toBeNull();
    expect(new URL(dom.window.location.href).searchParams.get("episode")).toBe("1");

    if (!(confirmPanel instanceof dom.window.Element)) {
      throw new Error("next episode confirm panel not found");
    }
    await click(confirmPanel, dom);
    expect(container.querySelector(".reader-next-episode-confirm")).not.toBeNull();

    const confirmBackdrop = container.querySelector(".reader-next-episode-confirm-backdrop");
    if (!(confirmBackdrop instanceof dom.window.Element)) {
      throw new Error("next episode confirm backdrop not found");
    }
    await click(confirmBackdrop, dom);
    expect(container.querySelector(".reader-next-episode-confirm")).toBeNull();

    await keyDown(dom.window, "ArrowLeft");
    await waitFor(() => container.textContent?.includes("次の話へ進みますか？") === true);
    await click(getButtonByLabel(container, "次の話への移動を閉じる"), dom);
    expect(container.querySelector(".reader-next-episode-confirm")).toBeNull();

    await keyDown(dom.window, "ArrowLeft");
    await waitFor(() => container.textContent?.includes("次の話へ進みますか？") === true);

    const focusedElement = dom.window.document.activeElement;
    if (!(focusedElement instanceof dom.window.Element)) {
      throw new Error("focused element is missing");
    }
    await keyDownElement(focusedElement, dom, "Escape");
    expect(container.querySelector(".reader-next-episode-confirm")).toBeNull();

    await act(async () => {
      root.unmount();
    });
  });

  it("reader 画面の横書き forward ボタンで最終ページから次話確認を出す", async () => {
    const { container, root, dom } = await renderApp(createReaderFetchHandler({ readingMode: "horizontal" }), {
      url: "http://localhost/?novelId=n1&episode=1"
    });

    await waitFor(() => getButtonByLabel(container, "次のページへ進む").disabled === false);

    await click(getButtonByLabel(container, "次のページへ進む"), dom);
    await click(getButtonByLabel(container, "次のページへ進む"), dom);

    await waitFor(() => container.textContent?.includes("次の話へ進みますか？") === true);
    const confirmPanel = container.querySelector(".reader-next-episode-confirm");
    const confirmButton = Array.from(confirmPanel?.querySelectorAll("button") ?? []).find((button) => button.textContent?.trim() === "進む");
    if (!(confirmButton instanceof HTMLButtonElement)) {
      throw new Error("next episode confirm button not found");
    }
    await click(confirmButton, dom);
    await waitFor(() => new URL(dom.window.location.href).searchParams.get("episode") === "2");
    expect(container.querySelector(".reader-next-episode-confirm")).toBeNull();

    await act(async () => {
      root.unmount();
    });
  });

  it("reader 画面の最終話最終ページでは到達 notice を出す", async () => {
    const { container, root, dom } = await renderApp(
      createReaderFetchHandler({
        episodes: [
          {
            episodeIndex: "1",
            title: "第1話",
            chapter: "第一章",
            subchapter: null,
            sourceUrl: "https://example.com/n1/1/",
            updatedAt: "2026-03-20T10:00:00Z",
            contentEtag: "etag-1"
          }
        ]
      }),
      {
        url: "http://localhost/?novelId=n1&episode=1"
      }
    );

    await waitFor(() => getButtonByLabel(container, "次のページへ進む").disabled === false);

    await keyDown(dom.window, "ArrowLeft");
    await keyDown(dom.window, "ArrowLeft");

    await waitFor(() => container.textContent?.includes("最終話の最終ページに到達しました。") === true);
    expect(container.querySelector(".reader-notice")?.textContent).toContain("最終話の最終ページに到達しました。");

    await act(async () => {
      root.unmount();
    });
  });

  it("reader 画面の部分取得末尾では最終話ではなく未取得の続き notice を出す", async () => {
    const { container, root, dom } = await renderApp(
      createReaderFetchHandler({
        totalEpisodes: 2,
        episodes: [
          {
            episodeIndex: "1",
            title: "第1話",
            chapter: "第一章",
            subchapter: null,
            sourceUrl: "https://example.com/n1/1/",
            updatedAt: "2026-03-20T10:00:00Z",
            contentEtag: "etag-1"
          }
        ]
      }),
      {
        url: "http://localhost/?novelId=n1&episode=1"
      }
    );

    await waitFor(() => getButtonByLabel(container, "次のページへ進む").disabled === false);

    await keyDown(dom.window, "ArrowLeft");
    await keyDown(dom.window, "ArrowLeft");

    await waitFor(() => container.textContent?.includes("続きの話はまだ取得されていません。再開して取得してください。") === true);
    expect(container.querySelector(".reader-notice")?.textContent).toContain(
      "続きの話はまだ取得されていません。再開して取得してください。"
    );
    expect(container.textContent).not.toContain("最終話の最終ページに到達しました。");

    await act(async () => {
      root.unmount();
    });
  });

  it("reader 画面の次話未取得時は最終話ではなく未取得 notice を出す", async () => {
    const { container, root, dom } = await renderApp(
      createReaderFetchHandler({
        episodes: [
          {
            episodeIndex: "1",
            title: "第1話",
            chapter: "第一章",
            subchapter: null,
            sourceUrl: "https://example.com/n1/1/",
            updatedAt: "2026-03-20T10:00:00Z",
            contentEtag: "etag-1"
          },
          {
            episodeIndex: "2",
            title: "第2話",
            chapter: "第一章",
            subchapter: null,
            sourceUrl: "https://example.com/n1/2/",
            updatedAt: "2026-03-21T10:00:00Z",
            contentEtag: "etag-2",
            bodyStatus: "failed"
          }
        ]
      }),
      {
        url: "http://localhost/?novelId=n1&episode=1"
      }
    );

    await waitFor(() => getButtonByLabel(container, "次のページへ進む").disabled === false);

    await keyDown(dom.window, "ArrowLeft");
    await keyDown(dom.window, "ArrowLeft");

    await waitFor(() => container.textContent?.includes("次の話はまだ本文が取得されていません。再開して取得してください。") === true);
    expect(container.querySelector(".reader-notice")?.textContent).toContain(
      "次の話はまだ本文が取得されていません。再開して取得してください。"
    );

    await act(async () => {
      root.unmount();
    });
  });

  it("読書設定のrange操作中は左右矢印キーでページ移動しない", async () => {
    const confirm = vi.fn(() => false);
    const { container, root, dom } = await renderApp(createReaderFetchHandler(), {
      url: "http://localhost/?novelId=n1&episode=1"
    });
    dom.window.confirm = confirm;

    await waitFor(() => getButtonByLabel(container, "次のページへ進む").disabled === false);
    await click(getButtonByLabel(container, "読書設定"), dom);
    await waitFor(() => Boolean(container.querySelector(".reader-settings-panel")));

    const fontSizeSlider = container.querySelector('.reader-settings-panel input[type="range"]');
    expect(fontSizeSlider).toBeInstanceOf(dom.window.HTMLInputElement);

    await keyDownElement(fontSizeSlider as Element, dom, "ArrowLeft");
    await keyDownElement(fontSizeSlider as Element, dom, "ArrowLeft");

    expect(confirm).not.toHaveBeenCalled();

    await act(async () => {
      root.unmount();
    });
  });

  it("画像ビューア表示中は左右矢印キーで背景のページ移動をしない", async () => {
    const confirm = vi.fn(() => false);
    const { container, root, dom } = await renderApp(
      createReaderFetchHandler({
        readerDocument: {
          version: 1,
          blocks: [
            { type: "title", text: "第1話" },
            {
              type: "paragraph",
              section: "body",
              inlines: [{ type: "text", text: "本文です。" }]
            },
            {
              type: "image",
              src: "https://example.com/image.jpg",
              alt: "挿絵",
              width: 640,
              height: 480
            }
          ]
        }
      }),
      {
        url: "http://localhost/?novelId=n1&episode=1"
      }
    );
    dom.window.confirm = confirm;

    await waitFor(() => getButtonByLabel(container, "次のページへ進む").disabled === false);

    const image = container.querySelector(".reader-prose-paged img");
    expect(image).toBeInstanceOf(dom.window.HTMLImageElement);
    await click(image as Element, dom, { clientX: 400, clientY: 40 });
    await waitFor(() => Boolean(container.querySelector(".reader-image-viewer")));

    await keyDown(dom.window, "ArrowLeft");
    await keyDown(dom.window, "ArrowLeft");

    expect(confirm).not.toHaveBeenCalled();
    expect(container.querySelector(".reader-image-viewer")).not.toBeNull();

    await act(async () => {
      root.unmount();
    });
  });

  it("URL で未取得話を指定したら取得済み話へフォールバックする", async () => {
    const requestedPaths: string[] = [];
    const fetchHandler: FetchHandler = async (url) => {
      const requestUrl = new URL(url, "http://localhost");
      requestedPaths.push(requestUrl.pathname);

      if (requestUrl.pathname === "/api/runtime/status") {
        return jsonResponse({ services: [] });
      }
      if (requestUrl.pathname === "/api/fetcher/status") {
        return jsonResponse({ version: null, queue: null, tasks: null });
      }
      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({ total: 0, webWorker: 0, worker: 0, running: false });
      }
      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        return jsonResponse({ current: null, queued: [], recentCompleted: [], recentFailed: [], completedCount: 0, failedCount: 0 });
      }
      if (requestUrl.pathname === "/api/ai-generation/settings") {
        return jsonResponse({ preferredMode: "heuristic", effectiveGenerationMode: "heuristic", settings: { selectedProfileId: "default", profiles: [] } });
      }
      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }
      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: [
            {
              novelId: "n1",
              fetcherWorkId: "work-1",
              title: "小説A",
              author: "作者A",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n1",
              story: "",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: null,
              lastReadEpisodeTitle: null,
              bookmarkCount: 0,
              totalEpisodes: 2,
              savedEpisodes: 1,
              fetchStatus: "failed",
              resumeEpisodeId: "2"
            }
          ]
        });
      }
      if (requestUrl.pathname === "/api/library/novels/n1/toc") {
        return jsonResponse({
          novelId: "n1",
          fetcherWorkId: "work-1",
          title: "小説A",
          author: "作者A",
          siteName: "小説家になろう",
          tocUrl: "https://example.com/n1",
          updatedAt: "2026-03-22T10:00:00Z",
          totalEpisodes: 2,
          story: "",
          episodes: [
            { episodeIndex: "1", title: "第1話", chapter: null, subchapter: null, updatedAt: "2026-03-20T10:00:00Z", contentEtag: "etag-1" },
            { episodeIndex: "2", title: "第2話", chapter: null, subchapter: null, updatedAt: "2026-03-21T10:00:00Z", contentEtag: "etag-2", bodyStatus: "failed" }
          ]
        });
      }
      if (requestUrl.pathname === "/api/reader/state") {
        return jsonResponse({ novelId: "n1", lastReadEpisodeIndex: null, position: 0, updatedAt: "2026-03-22T10:00:00Z" });
      }
      if (requestUrl.pathname === "/api/reader/preferences") {
        return jsonResponse({ preferences: null });
      }
      if (requestUrl.pathname === "/api/bookmarks") {
        return jsonResponse({ bookmarks: [] });
      }
      if (requestUrl.pathname === "/api/library/novels/n1/episodes/1") {
        return jsonResponse({
          novelId: "n1",
          episodeIndex: "1",
          title: "第1話",
          chapter: null,
          subchapter: null,
          html: "",
          plainTextLength: 10,
          updatedAt: "2026-03-20T10:00:00Z",
          contentEtag: "etag-1",
          readerDocument: {
            version: 1,
            blocks: [
              { type: "title", text: "第1話" },
              { type: "paragraph", section: "body", inlines: [{ type: "text", text: "本文です。" }] }
            ]
          }
        });
      }
      if (requestUrl.pathname === "/api/library/novels/n1/episodes/2") {
        return jsonResponse({ error: "should not fetch" }, { status: 500 });
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { root } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=2"
    });

    await waitFor(() => requestedPaths.includes("/api/library/novels/n1/episodes/1"));
    expect(requestedPaths).not.toContain("/api/library/novels/n1/episodes/2");

    await act(async () => {
      root.unmount();
    });
  });

  it("reader 画面で読み上げ再生中に設定を変えると停止する", async () => {
    const speechSynthesis = createAppMockSpeechSynthesis();
    const { container, root, dom } = await renderApp(createReaderFetchHandler(), {
      url: "http://localhost/?novelId=n1&episode=1",
      speechSynthesis,
      speechSynthesisUtterance: MockAppSpeechSynthesisUtterance
    });

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("第1話") === true);

    await click(container.querySelector('button[aria-label="読み上げ"]') as Element, dom);
    await waitFor(() => Boolean(container.querySelector(".reader-speech-panel")));

    await click(getButtonByText(container, "現在位置から再生"), dom);
    await waitFor(() => container.textContent?.includes("再生中") === true);

    expect(speechSynthesis.speak).toHaveBeenCalledTimes(1);
    const cancelCountBeforeSettingChange = speechSynthesis.cancel.mock.calls.length;

    await click(container.querySelector('button[aria-label="読み上げ速度を上げる"]') as Element, dom);

    await waitFor(() => container.textContent?.includes("読み上げ設定が変わったため停止しました。") === true);
    expect(container.querySelector(".reader-notice")?.textContent).toContain("読み上げ設定が変わったため停止しました。");
    expect(speechSynthesis.cancel.mock.calls.length).toBeGreaterThan(cancelCountBeforeSettingChange);

    await act(async () => {
      root.unmount();
    });
  });

  it("reader 画面の読み上げ完了イベントで次 chunk へ進み最後に通知する", async () => {
    const speechSynthesis = createAppMockSpeechSynthesis();
    const longText = "あ".repeat(170);
    const { container, root, dom } = await renderApp(
      createReaderFetchHandler({
        plainTextLength: 340,
        readerDocument: {
          version: 1,
          blocks: [
            {
              type: "paragraph",
              section: "body",
              inlines: [{ type: "text", text: longText }]
            },
            {
              type: "paragraph",
              section: "body",
              inlines: [{ type: "text", text: longText }]
            }
          ]
        }
      }),
      {
        url: "http://localhost/?novelId=n1&episode=1",
        speechSynthesis,
        speechSynthesisUtterance: MockAppSpeechSynthesisUtterance
      }
    );

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.querySelector('button[aria-label="読み上げ"]') !== null);

    await click(container.querySelector('button[aria-label="読み上げ"]') as Element, dom);
    await waitFor(() => Boolean(container.querySelector(".reader-speech-panel")));
    await click(getButtonByText(container, "現在位置から再生"), dom);
    await waitFor(() => speechSynthesis.utterances.length === 1);

    await act(async () => {
      speechSynthesis.utterances[0]?.onend?.();
      await Promise.resolve();
    });

    await waitFor(() => speechSynthesis.utterances.length === 2);

    await act(async () => {
      speechSynthesis.utterances[1]?.onend?.();
      await Promise.resolve();
    });

    await waitFor(() => container.textContent?.includes("読み上げが終わりました。") === true);
    expect(speechSynthesis.utterances).toHaveLength(2);

    await act(async () => {
      root.unmount();
    });
  });

  it("reader 画面で読み上げ boundary progress を現在位置に反映する", async () => {
    const speechSynthesis = createAppMockSpeechSynthesis();
    const consoleDebug = vi.spyOn(console, "debug").mockImplementation(() => {});
    const longText = "あ".repeat(100);
    const { container, root, dom } = await renderApp(
      createReaderFetchHandler({
        plainTextLength: 100,
        readerDocument: {
          version: 1,
          blocks: [
            {
              type: "paragraph",
              section: "body",
              inlines: [{ type: "text", text: longText }]
            }
          ]
        }
      }),
      {
        url: "http://localhost/?novelId=n1&episode=1",
        speechSynthesis,
        speechSynthesisUtterance: MockAppSpeechSynthesisUtterance
      }
    );

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.querySelector('button[aria-label="読み上げ"]') !== null);

    await click(container.querySelector('button[aria-label="読み上げ"]') as Element, dom);
    await waitFor(() => Boolean(container.querySelector(".reader-speech-panel")));
    await changeSelect(container.querySelectorAll(".reader-speech-panel select")[3] as HTMLSelectElement, "enabled", dom);
    await click(getButtonByText(container, "現在位置から再生"), dom);
    await waitFor(() => speechSynthesis.utterances.length === 1);
    await waitFor(() => container.querySelector(".reader-speech-chunk-debug") !== null);
    const scrollIntoView = vi.spyOn(dom.window.HTMLElement.prototype, "scrollIntoView");
    scrollIntoView.mockClear();

    await act(async () => {
      speechSynthesis.utterances[0]?.onboundary?.({ charIndex: 50, elapsedTime: 1.25 });
      speechSynthesis.utterances[0]?.onboundary?.({ charIndex: 51, elapsedTime: 1.35 });
      await Promise.resolve();
    });

    await waitFor(() => dom.window.location.search.includes("pos=51"));
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(container.querySelector(".reader-speech-chunk-debug")).not.toBeNull();
    expect(scrollIntoView).not.toHaveBeenCalled();
    const progressLog = consoleDebug.mock.calls
      .map((call) => (typeof call[0] === "string" && call[0].startsWith("[reader-tts] progress ") ? call[0] : null))
      .filter((log): log is string => log !== null)
      .map((log) => JSON.parse(log.replace("[reader-tts] progress ", "")))
      .find((payload) => payload.source === "boundary" && payload.charIndex === 50);
    expect(progressLog).toEqual(
      expect.objectContaining({
        source: "boundary",
        chunkIndex: 0,
        charIndex: 50,
        elapsedTimeMs: 1250,
        position: 50
      })
    );

    await act(async () => {
      root.unmount();
    });
  });

  it("reader 画面で voiceschanged 後に未存在 voice をリセットする", async () => {
    const voices = [createAppMockVoice(), createAppMockVoice({ default: false, name: "別音声", voiceURI: "voice:alt" })];
    const speechSynthesis = createAppMockSpeechSynthesis({ voices });
    const { container, root, dom } = await renderApp(createReaderFetchHandler(), {
      url: "http://localhost/?novelId=n1&episode=1",
      speechSynthesis,
      speechSynthesisUtterance: MockAppSpeechSynthesisUtterance
    });

    await waitFor(() => Boolean(container.querySelector('button[aria-label="読み上げ"]')));
    await click(container.querySelector('button[aria-label="読み上げ"]') as Element, dom);
    await waitFor(() => Boolean(container.querySelector(".reader-speech-panel")));

    const selects = container.querySelectorAll(".reader-speech-panel select");
    const voiceSelect = selects[1] as HTMLSelectElement;
    await changeSelect(voiceSelect, "voice:alt", dom);
    expect(voiceSelect.value).toBe("voice:alt");

    voices.splice(1, 1);
    await act(async () => {
      speechSynthesis.dispatch("voiceschanged");
      await Promise.resolve();
    });

    await waitFor(() => voiceSelect.value === "");

    await act(async () => {
      root.unmount();
    });
  });

  it("reader 画面で読み上げ pause 失敗を表示できる", async () => {
    const speechSynthesis = createAppMockSpeechSynthesis();
    speechSynthesis.pause.mockImplementation(() => {
      throw new Error("pause failed");
    });
    const { container, root, dom } = await renderApp(createReaderFetchHandler(), {
      url: "http://localhost/?novelId=n1&episode=1",
      speechSynthesis,
      speechSynthesisUtterance: MockAppSpeechSynthesisUtterance
    });

    await waitFor(() => Boolean(container.querySelector('button[aria-label="読み上げ"]')));
    await click(container.querySelector('button[aria-label="読み上げ"]') as Element, dom);
    await waitFor(() => Boolean(container.querySelector(".reader-speech-panel")));
    await click(getButtonByText(container, "現在位置から再生"), dom);
    await waitFor(() => container.textContent?.includes("再生中") === true);

    await click(getButtonByText(container, "一時停止"), dom);
    await waitFor(() => container.querySelector(".reader-error-alert")?.textContent?.includes("pause failed") === true);
    await pointerDown(getButtonByLabel(container, "エラーを閉じる"), dom);
    await click(getButtonByLabel(container, "エラーを閉じる"), dom);
    await waitFor(() => container.querySelector(".reader-error-alert") === null);
    expect(container.querySelector(".reader-speech-panel")).not.toBeNull();

    await act(async () => {
      root.unmount();
    });
  });

  it("reader 画面で読み上げ resume 失敗を表示できる", async () => {
    const speechSynthesis = createAppMockSpeechSynthesis();
    const { container, root, dom } = await renderApp(createReaderFetchHandler(), {
      url: "http://localhost/?novelId=n1&episode=1",
      speechSynthesis,
      speechSynthesisUtterance: MockAppSpeechSynthesisUtterance
    });

    await waitFor(() => Boolean(container.querySelector('button[aria-label="読み上げ"]')));
    await click(container.querySelector('button[aria-label="読み上げ"]') as Element, dom);
    await waitFor(() => Boolean(container.querySelector(".reader-speech-panel")));
    await click(getButtonByText(container, "現在位置から再生"), dom);
    await waitFor(() => container.textContent?.includes("再生中") === true);

    await click(getButtonByText(container, "一時停止"), dom);
    await waitFor(() => container.textContent?.includes("一時停止中") === true);

    speechSynthesis.resume.mockImplementation(() => {
      throw new Error("resume failed");
    });
    await click(getButtonByText(container, "再開"), dom);
    await waitFor(() => container.querySelector(".reader-error-alert")?.textContent?.includes("resume failed") === true);

    await act(async () => {
      root.unmount();
    });
  });

  it("reader 画面で読み上げ engine error を表示できる", async () => {
    const speechSynthesis = createAppMockSpeechSynthesis({ autoStart: false });
    const { container, root, dom } = await renderApp(createReaderFetchHandler(), {
      url: "http://localhost/?novelId=n1&episode=1",
      speechSynthesis,
      speechSynthesisUtterance: MockAppSpeechSynthesisUtterance
    });

    await waitFor(() => Boolean(container.querySelector('button[aria-label="読み上げ"]')));
    await click(container.querySelector('button[aria-label="読み上げ"]') as Element, dom);
    await waitFor(() => Boolean(container.querySelector(".reader-speech-panel")));
    await click(getButtonByText(container, "現在位置から再生"), dom);
    await waitFor(() => speechSynthesis.utterances.length === 1);

    await act(async () => {
      speechSynthesis.utterances[0]?.onerror?.({ error: "engine failed" });
      await Promise.resolve();
    });

    await waitFor(() => container.querySelector(".reader-error-alert")?.textContent?.includes("engine failed") === true);

    await act(async () => {
      root.unmount();
    });
  });

  it("reader 画面で読み上げ開始失敗を表示できる", async () => {
    const speechSynthesis = createAppMockSpeechSynthesis();
    speechSynthesis.speak.mockImplementation(() => {
      throw new Error("speak failed");
    });
    const { container, root, dom } = await renderApp(createReaderFetchHandler(), {
      url: "http://localhost/?novelId=n1&episode=1",
      speechSynthesis,
      speechSynthesisUtterance: MockAppSpeechSynthesisUtterance
    });

    await waitFor(() => Boolean(container.querySelector('button[aria-label="読み上げ"]')));
    await click(container.querySelector('button[aria-label="読み上げ"]') as Element, dom);
    await waitFor(() => Boolean(container.querySelector(".reader-speech-panel")));
    await click(getButtonByText(container, "現在位置から再生"), dom);

    await waitFor(() => container.querySelector(".reader-error-alert")?.textContent?.includes("speak failed") === true);

    await act(async () => {
      root.unmount();
    });
  });

  it("reader の読み込みオーバーレイを最低 1 秒表示する", async () => {
    vi.useFakeTimers();

    let resolveEpisodeResponse: ((response: Response) => void) | null = null;
    const episodeResponsePromise = new Promise<Response>((resolve) => {
      resolveEpisodeResponse = resolve;
    });

    const fetchHandler: FetchHandler = async (url) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return jsonResponse({
          status: "ok",
          checkedAt: "2026-03-22T10:00:00Z",
          services: [{ id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" }]
        });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: [
            {
              novelId: "n1",
              fetcherWorkId: "work-1",
              title: "小説A",
              author: "作者A",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n1",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: "1",
              lastReadEpisodeTitle: "第1話",
              latestBookmarkEpisodeIndex: "1",
              bookmarkCount: 0,
              totalEpisodes: 1
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        return jsonResponse({
          readingMode: "vertical",
          fontFamily: "mincho",
          theme: "classic",
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({ total: 0, webWorker: 0, worker: 0, running: false });
      }

      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        return jsonResponse({
          current: null,
          queued: [],
          recentCompleted: [],
          recentFailed: [],
          completedCount: 0,
          failedCount: 0,
          convertCurrent: null,
          convertQueued: []
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        return jsonResponse({
          apiBaseUrlConfigured: true,
          masterPassphraseConfigured: true,
          preferredMode: "heuristic",
          effectiveGenerationMode: "heuristic",
          settings: {
            selectedProfileId: "default",
            profiles: [
              {
                id: "default",
                label: "Default",
                hasApiKey: false,
                apiKeyMasked: null,
                modelId: null,
                providerOrder: [],
                allowFallbacks: true,
                requireParameters: false,
                updatedAt: "2026-03-22T10:00:00Z"
              }
            ]
          }
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/toc") {
        return jsonResponse({
          novelId: "n1",
          fetcherWorkId: "work-1",
          title: "小説A",
          author: "作者A",
          siteName: "小説家になろう",
          tocUrl: "https://example.com/n1",
          updatedAt: "2026-03-22T10:00:00Z",
          totalEpisodes: 1,
          story: "reader loading overlay test",
          episodes: [
            {
              episodeIndex: "1",
              title: "第1話",
              chapter: "第一章",
              subchapter: null,
              updatedAt: "2026-03-20T10:00:00Z",
              contentEtag: "etag-1"
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/state") {
        return jsonResponse({
          novelId: "n1",
          lastReadEpisodeIndex: "1",
          position: 0,
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/bookmarks") {
        return jsonResponse({ bookmarks: [] });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/episodes/1") {
        return episodeResponsePromise;
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { container, root } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=1"
    });

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(container.querySelector(".reader-loading-overlay")).not.toBeNull();
    expect(container.querySelector(".reader-loading-overlay")?.textContent).toContain("小説A");
    expect(container.querySelector(".reader-loading-overlay")?.textContent).toContain("第一章 - 第1話");

    resolveEpisodeResponse?.(
      jsonResponse({
        novelId: "n1",
        episodeIndex: "1",
        title: "第1話",
        chapter: "第一章",
        subchapter: null,
        html: "",
        plainTextLength: 10,
        updatedAt: "2026-03-22T10:00:00Z",
        contentEtag: "etag-1",
        readerDocument: {
          version: 1,
          blocks: [
            { type: "title", text: "第1話" },
            {
              type: "paragraph",
              section: "body",
              inlines: [{ type: "text", text: "本文です。" }]
            }
          ]
        }
      })
    );

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(container.querySelector(".reader-loading-overlay")).not.toBeNull();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(999);
    });
    expect(container.querySelector(".reader-loading-overlay")).not.toBeNull();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
      await Promise.resolve();
    });
    expect(container.querySelector(".reader-loading-overlay")).toBeNull();
    expect(container.textContent).toContain("第1話");

    await act(async () => {
      root.unmount();
    });
  });

  it("話移動中の読み込みオーバーレイは移動先の完全な話ラベルを表示し続ける", async () => {
    const episode2Response = {
      novelId: "n1",
      episodeIndex: "2",
      title: "第2話",
      chapter: "第一章",
      subchapter: null,
      sourceUrl: "https://example.com/n1/2/",
      html: "",
      plainTextLength: 10,
      updatedAt: "2026-03-22T10:06:00Z",
      contentEtag: "etag-2",
      readerDocument: {
        version: 1,
        blocks: [
          { type: "title", text: "第2話" },
          { type: "paragraph", section: "body", inlines: [{ type: "text", text: "本文2です。" }] }
        ]
      }
    };
    let resolveEpisode2: ((response: Response) => void) | null = null;
    const pendingEpisode2 = new Promise<Response>((resolve) => {
      resolveEpisode2 = resolve;
    });
    const baseFetchHandler = createReaderFetchHandler({
      episodeResponses: { "2": episode2Response }
    });
    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");
      if (requestUrl.pathname === "/api/library/novels/n1/episodes/2") {
        return pendingEpisode2;
      }
      return baseFetchHandler(url, init);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=1"
    });

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("本文です。") === true);
    await waitFor(() => getButtonByLabel(container, "次のページへ進む").disabled === false);
    await keyDown(dom.window, "ArrowLeft");
    await keyDown(dom.window, "ArrowLeft");
    await waitFor(() => Boolean(container.querySelector(".reader-next-episode-confirm")));
    await click(getButtonByText(container, "進む"), dom);
    await waitFor(() => Boolean(container.querySelector(".reader-loading-overlay")));

    const loadingOverlay = container.querySelector(".reader-loading-overlay");
    expect(loadingOverlay?.textContent).toContain("第一章 - 第2話");
    expect(loadingOverlay?.textContent).not.toContain("第一章 - 第1話");
    expect(loadingOverlay?.textContent).not.toContain("第1話");

    resolveEpisode2?.(jsonResponse(episode2Response));

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    const remainingOverlay = container.querySelector(".reader-loading-overlay");
    expect(remainingOverlay).not.toBeNull();
    expect(remainingOverlay?.textContent).toContain("第一章 - 第2話");
    expect(remainingOverlay?.textContent).not.toContain("第1話");

    await waitFor(() => container.querySelector(".reader-loading-overlay") === null);
    expect(container.textContent).toContain("本文2です。");

    await act(async () => {
      root.unmount();
    });
  });

  it("別作品への履歴遷移中の読み込みオーバーレイは旧作品の TOC ラベルを表示しない", async () => {
    const pendingN1Episode = new Promise<Response>(() => {});
    const pendingN2Toc = new Promise<Response>(() => {});
    const fetchHandler: FetchHandler = async (url) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return jsonResponse({
          status: "ok",
          checkedAt: "2026-03-22T10:00:00Z",
          services: [{ id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" }]
        });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: [
            {
              novelId: "n1",
              fetcherWorkId: "work-1",
              title: "小説A",
              author: "作者A",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n1",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: "1",
              lastReadEpisodeTitle: "Aだけの第1話",
              latestBookmarkEpisodeIndex: null,
              bookmarkCount: 0,
              totalEpisodes: 1
            },
            {
              novelId: "n2",
              fetcherWorkId: "work-2",
              title: "小説B",
              author: "作者B",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n2",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: "1",
              lastReadEpisodeTitle: "Bの第1話",
              latestBookmarkEpisodeIndex: null,
              bookmarkCount: 0,
              totalEpisodes: 1
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        return jsonResponse({
          readingMode: "vertical",
          fontFamily: "mincho",
          theme: "classic",
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({ total: 0, webWorker: 0, worker: 0, running: false });
      }

      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        return jsonResponse({
          current: null,
          queued: [],
          recentCompleted: [],
          recentFailed: [],
          completedCount: 0,
          failedCount: 0,
          convertCurrent: null,
          convertQueued: []
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        return jsonResponse({
          apiBaseUrlConfigured: true,
          masterPassphraseConfigured: true,
          preferredMode: "heuristic",
          effectiveGenerationMode: "heuristic",
          settings: { selectedProfileId: "default", profiles: [] }
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/toc") {
        return jsonResponse({
          novelId: "n1",
          fetcherWorkId: "work-1",
          title: "小説A",
          author: "作者A",
          siteName: "小説家になろう",
          tocUrl: "https://example.com/n1",
          updatedAt: "2026-03-22T10:00:00Z",
          story: "",
          totalEpisodes: 1,
          episodes: [
            {
              episodeIndex: "1",
              title: "Aだけの第1話",
              chapter: "旧章",
              subchapter: null,
              sourceUrl: "https://example.com/n1/1/",
              updatedAt: "2026-03-22T10:00:00Z",
              contentEtag: "n1-toc-1",
              bodyStatus: "complete"
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/library/novels/n2/toc") {
        return pendingN2Toc;
      }

      const readerSettingsMatch = requestUrl.pathname.match(/^\/api\/library\/novels\/(n[12])\/reader-settings$/);
      if (readerSettingsMatch) {
        const novelId = readerSettingsMatch[1] ?? "n1";
        return jsonResponse({
          novelId,
          correction: {
            quoteNormalization: false,
            hyphenDashNormalization: false,
            parenthesisNormalization: false,
            halfwidthAlnumPunctuationNormalization: false
          },
          updatedAt: null
        });
      }

      if (requestUrl.pathname === "/api/reader/state") {
        const novelId = requestUrl.searchParams.get("novelId") ?? "n1";
        return jsonResponse({
          novelId,
          lastReadEpisodeIndex: "1",
          position: 0,
          updatedAt: "2026-03-22T10:00:00Z",
          stateVersion: 1,
          updatedByClientId: "server"
        });
      }

      if (requestUrl.pathname === "/api/bookmarks") {
        return jsonResponse({ bookmarks: [] });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/episodes/1") {
        return pendingN1Episode;
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=1"
    });

    await waitFor(() => container.querySelector(".reader-loading-overlay")?.textContent?.includes("旧章 - Aだけの第1話") === true);

    await act(async () => {
      dom.window.history.pushState(null, "", "/?novelId=n2&episode=1");
      dom.window.dispatchEvent(new dom.window.PopStateEvent("popstate"));
    });

    await waitFor(() => container.querySelector(".reader-loading-overlay")?.textContent?.includes("小説B") === true);
    const loadingOverlay = container.querySelector(".reader-loading-overlay");
    expect(loadingOverlay?.textContent).toContain("#1");
    expect(loadingOverlay?.textContent).not.toContain("旧章 - Aだけの第1話");
    expect(loadingOverlay?.textContent).not.toContain("Aだけの第1話");

    await act(async () => {
      root.unmount();
    });
  });

  it("reader 復帰時に別端末の既読更新を検知して反映できる", async () => {
    let readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "1",
      position: 0,
      updatedAt: "2026-03-22T10:00:00Z",
      stateVersion: 1,
      updatedByClientId: "client-a"
    };

    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return jsonResponse({
          status: "ok",
          checkedAt: "2026-03-22T10:00:00Z",
          services: [{ id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" }]
        });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: [
            {
              novelId: "n1",
              fetcherWorkId: "work-1",
              title: "小説A",
              author: "作者A",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n1",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: "1",
              lastReadEpisodeTitle: "第1話",
              latestBookmarkEpisodeIndex: null,
              bookmarkCount: 0,
              totalEpisodes: 2
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        return jsonResponse({
          readingMode: "vertical",
          fontFamily: "mincho",
          theme: "classic",
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({ total: 0, webWorker: 0, worker: 0, running: false });
      }

      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        return jsonResponse({
          current: null,
          queued: [],
          recentCompleted: [],
          recentFailed: [],
          completedCount: 0,
          failedCount: 0,
          convertCurrent: null,
          convertQueued: []
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        return jsonResponse({
          apiBaseUrlConfigured: true,
          masterPassphraseConfigured: true,
          preferredMode: "heuristic",
          effectiveGenerationMode: "heuristic",
          settings: {
            selectedProfileId: "default",
            profiles: [
              {
                id: "default",
                label: "Default",
                hasApiKey: false,
                apiKeyMasked: null,
                modelId: null,
                providerOrder: [],
                allowFallbacks: true,
                requireParameters: false,
                updatedAt: "2026-03-22T10:00:00Z"
              }
            ]
          }
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/toc") {
        return jsonResponse({
          novelId: "n1",
          fetcherWorkId: "work-1",
          title: "小説A",
          author: "作者A",
          siteName: "小説家になろう",
          tocUrl: "https://example.com/n1",
          updatedAt: "2026-03-22T10:00:00Z",
          totalEpisodes: 2,
          story: "reader sync story",
          episodes: [
            {
              episodeIndex: "1",
              title: "第1話",
              chapter: "第一章",
              subchapter: null,
              updatedAt: "2026-03-20T10:00:00Z",
              contentEtag: "etag-1"
            },
            {
              episodeIndex: "2",
              title: "第2話",
              chapter: "第一章",
              subchapter: null,
              updatedAt: "2026-03-21T10:00:00Z",
              contentEtag: "etag-2"
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/state") {
        if (init?.method === "PUT") {
          const payload = JSON.parse(String(init.body)) as { lastReadEpisodeIndex: string | null; position: number; clientId?: string };
          readerState = {
            novelId: "n1",
            lastReadEpisodeIndex: payload.lastReadEpisodeIndex,
            position: payload.position,
            updatedAt: "2026-03-22T10:05:00Z",
            stateVersion: readerState.stateVersion + 1,
            updatedByClientId: payload.clientId ?? null
          };
        }

        return jsonResponse(readerState);
      }

      if (requestUrl.pathname === "/api/bookmarks") {
        return jsonResponse({ bookmarks: [] });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/episodes/1") {
        return jsonResponse({
          novelId: "n1",
          episodeIndex: "1",
          title: "第1話",
          chapter: "第一章",
          subchapter: null,
          html: "",
          plainTextLength: 10,
          updatedAt: "2026-03-22T10:00:00Z",
          contentEtag: "etag-1",
          readerDocument: {
            version: 1,
            blocks: [
              { type: "title", text: "第1話" },
              {
                type: "paragraph",
                section: "body",
                inlines: [{ type: "text", text: "本文1です。" }]
              }
            ]
          }
        });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/episodes/2") {
        return jsonResponse({
          novelId: "n1",
          episodeIndex: "2",
          title: "第2話",
          chapter: "第一章",
          subchapter: null,
          html: "",
          plainTextLength: 10,
          updatedAt: "2026-03-22T10:06:00Z",
          contentEtag: "etag-2",
          readerDocument: {
            version: 1,
            blocks: [
              { type: "title", text: "第2話" },
              {
                type: "paragraph",
                section: "body",
                inlines: [{ type: "text", text: "本文2です。" }]
              }
            ]
          }
        });
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=1"
    });

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("本文1です。") === true);
    await waitFor(() => getButtonByLabel(container, "次のページへ進む").disabled === false);
    await keyDown(dom.window, "ArrowLeft");
    await keyDown(dom.window, "ArrowLeft");
    await waitFor(() => Boolean(container.querySelector(".reader-next-episode-confirm")));
    expect(dom.window.document.activeElement?.textContent?.trim()).toBe("進む");

    readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "2",
      position: 0,
      updatedAt: "2026-03-22T10:06:00Z",
      stateVersion: 2,
      updatedByClientId: "other-device"
    };

    Object.defineProperty(dom.window.document, "hidden", {
      configurable: true,
      value: false
    });

    await act(async () => {
      dom.window.document.dispatchEvent(new dom.window.Event("visibilitychange"));
    });

    await waitFor(() => Boolean(container.querySelector(".reader-sync-conflict")));
    expect(container.querySelector(".reader-next-episode-confirm")).toBeNull();
    expect(container.querySelector(".reader-sync-conflict")?.getAttribute("aria-modal")).toBe("true");
    expect(dom.window.document.activeElement?.textContent?.trim()).toBe("反映して移動");
    expect(getButtonByLabel(container, "一覧へ戻る").disabled).toBe(true);
    expect(container.textContent).toContain("別端末で最終既読が更新されました。");

    await click(getButtonByText(container, "反映して移動"), dom);

    await waitFor(() => container.textContent?.includes("本文2です。") === true);
    expect(container.textContent).toContain("別端末の既読位置を反映しました。");
    expect(container.querySelector(".reader-sync-conflict")).toBeNull();

    await act(async () => {
      root.unmount();
    });
  });

  it("reader 復帰時に同一ページ writer の保存結果は同期競合にしない", async () => {
    vi.stubGlobal("crypto", { randomUUID: vi.fn(() => "reader-client") });
    const baseFetchHandler = createReaderFetchHandler();
    let readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "1",
      position: 0,
      updatedAt: "2026-03-22T10:00:00Z",
      stateVersion: 1,
      updatedByClientId: "server"
    };
    let readerStateGetCount = 0;
    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");
      if (requestUrl.pathname === "/api/reader/state" && init?.method !== "PUT") {
        readerStateGetCount += 1;
        return jsonResponse(readerState);
      }
      return baseFetchHandler(url, init);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=1"
    });

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("本文です。") === true);
    const initialReaderStateGetCount = readerStateGetCount;
    readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "1",
      position: 20,
      updatedAt: "2026-03-22T10:05:00Z",
      stateVersion: 2,
      updatedByClientId: "reader-client"
    };

    await act(async () => {
      dom.window.dispatchEvent(new dom.window.Event("focus"));
    });

    await waitFor(() => readerStateGetCount > initialReaderStateGetCount);
    expect(container.querySelector(".reader-sync-conflict")).toBeNull();

    await act(async () => {
      root.unmount();
    });
  });

  it("同期競合の上書き保存中は反映操作を実行しない", async () => {
    const baseFetchHandler = createReaderFetchHandler();
    let readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "1",
      position: 0,
      updatedAt: "2026-03-22T10:00:00Z",
      stateVersion: 1,
      updatedByClientId: "server"
    };
    let resolvePendingPut: ((response: Response) => void) | null = null;
    const pendingPut = new Promise<Response>((resolve) => {
      resolvePendingPut = resolve;
    });
    let putCount = 0;
    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");
      if (requestUrl.pathname === "/api/reader/state") {
        if (init?.method === "PUT") {
          putCount += 1;
          return pendingPut;
        }
        return jsonResponse(readerState);
      }
      return baseFetchHandler(url, init);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=1"
    });

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("本文です。") === true);

    readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "2",
      position: 0,
      updatedAt: "2026-03-22T10:06:00Z",
      stateVersion: 2,
      updatedByClientId: "other-device"
    };

    await act(async () => {
      dom.window.document.dispatchEvent(new dom.window.Event("visibilitychange"));
    });
    await waitFor(() => Boolean(container.querySelector(".reader-sync-conflict")));

    await click(getButtonByText(container, "この端末で上書き"), dom);
    await waitFor(() => getButtonByText(container, "反映して移動").disabled);
    expect(getButtonByText(container, "上書き中...").disabled).toBe(true);
    const conflictDialog = container.querySelector(".reader-sync-conflict");
    expect(conflictDialog).toBeInstanceOf(dom.window.HTMLElement);
    await waitFor(() => dom.window.document.activeElement === conflictDialog);

    await keyDownElement(conflictDialog as Element, dom, "Tab");
    expect(dom.window.document.activeElement).toBe(conflictDialog);
    await keyDownElement(conflictDialog as Element, dom, "Tab", { shiftKey: true });
    expect(dom.window.document.activeElement).toBe(conflictDialog);

    await click(getButtonByText(container, "反映して移動"), dom);
    expect(container.querySelector(".reader-sync-conflict")).not.toBeNull();
    expect(container.textContent).not.toContain("別端末の既読位置を反映しました。");
    expect(putCount).toBe(1);

    resolvePendingPut?.(
      jsonResponse({
        novelId: "n1",
        lastReadEpisodeIndex: "1",
        position: 0,
        updatedAt: "2026-03-22T10:07:00Z",
        stateVersion: 3,
        updatedByClientId: "reader-client"
      })
    );
    await waitFor(() => container.querySelector(".reader-sync-conflict") === null);
    expect(container.textContent).toContain("この端末の位置を最終既読にしました。");

    await act(async () => {
      root.unmount();
    });
  });

  it("同期競合の上書き失敗はダイアログ内に表示してフォーカスを閉じ込める", async () => {
    const baseFetchHandler = createReaderFetchHandler();
    let readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "1",
      position: 0,
      updatedAt: "2026-03-22T10:00:00Z",
      stateVersion: 1,
      updatedByClientId: "server"
    };
    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");
      if (requestUrl.pathname === "/api/reader/state") {
        if (init?.method === "PUT") {
          return jsonResponse(
            {
              novelId: "n1",
              lastReadEpisodeIndex: "1",
              position: 12,
              updatedAt: "2026-03-22T10:07:00Z",
              stateVersion: 3,
              updatedByClientId: "other-newer-device"
            },
            { status: 409 }
          );
        }
        return jsonResponse(readerState);
      }
      return baseFetchHandler(url, init);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=1"
    });

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("本文です。") === true);
    readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "2",
      position: 0,
      updatedAt: "2026-03-22T10:06:00Z",
      stateVersion: 2,
      updatedByClientId: "other-device"
    };

    await act(async () => {
      dom.window.document.dispatchEvent(new dom.window.Event("visibilitychange"));
    });
    await waitFor(() => Boolean(container.querySelector(".reader-sync-conflict")));
    await click(getButtonByText(container, "この端末で上書き"), dom);

    await waitFor(() =>
      container
        .querySelector(".reader-sync-conflict [role=\"alert\"]")
        ?.textContent?.includes("さらに新しい既読位置が保存されています。もう一度確認してください。") === true
    );
    expect(container.textContent).toContain(formatDate("2026-03-22T10:07:00Z"));
    const conflictDialog = container.querySelector(".reader-sync-conflict");
    expect(conflictDialog).toBeInstanceOf(dom.window.HTMLElement);
    expect(dom.window.document.activeElement?.textContent?.trim()).toBe("反映して移動");
    expect(container.querySelector(":scope > .message.error")).toBeNull();

    await keyDownElement(getButtonByText(container, "反映して移動"), dom, "Tab", { shiftKey: true });
    expect(dom.window.document.activeElement?.textContent?.trim()).toBe("この端末で上書き");
    await keyDownElement(getButtonByText(container, "この端末で上書き"), dom, "Tab");
    expect(dom.window.document.activeElement?.textContent?.trim()).toBe("反映して移動");
    (conflictDialog as HTMLElement).focus();
    await keyDownElement(conflictDialog as Element, dom, "Tab");
    expect(dom.window.document.activeElement?.textContent?.trim()).toBe("反映して移動");
    (conflictDialog as HTMLElement).focus();
    await keyDownElement(conflictDialog as Element, dom, "Tab", { shiftKey: true });
    expect(dom.window.document.activeElement?.textContent?.trim()).toBe("この端末で上書き");

    await act(async () => {
      root.unmount();
    });
  });

  it("同一 writer の409が送信位置と一致する場合は上書き成功として扱う", async () => {
    const baseFetchHandler = createReaderFetchHandler();
    let readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "1",
      position: 0,
      updatedAt: "2026-03-22T10:00:00Z",
      stateVersion: 1,
      updatedByClientId: "server"
    };
    const putBodies: Array<Record<string, unknown>> = [];
    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");
      if (requestUrl.pathname === "/api/reader/state") {
        if (init?.method === "PUT") {
          const payload = JSON.parse(String(init.body)) as {
            clientId?: string;
            lastReadEpisodeIndex: string | null;
            position: number;
          };
          putBodies.push(payload as unknown as Record<string, unknown>);
          return jsonResponse(
            {
              novelId: "n1",
              lastReadEpisodeIndex: payload.lastReadEpisodeIndex,
              position: payload.position,
              updatedAt: "2026-03-22T10:07:00Z",
              stateVersion: 3,
              updatedByClientId: payload.clientId ?? null
            },
            { status: 409 }
          );
        }
        return jsonResponse(readerState);
      }
      return baseFetchHandler(url, init);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=1"
    });

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("本文です。") === true);
    readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "2",
      position: 0,
      updatedAt: "2026-03-22T10:06:00Z",
      stateVersion: 2,
      updatedByClientId: "other-device"
    };

    await act(async () => {
      dom.window.document.dispatchEvent(new dom.window.Event("visibilitychange"));
    });
    await waitFor(() => Boolean(container.querySelector(".reader-sync-conflict")));
    await click(getButtonByText(container, "この端末で上書き"), dom);

    await waitFor(() => container.querySelector(".reader-sync-conflict") === null);
    expect(putBodies).toHaveLength(1);
    expect(putBodies[0]?.expectedStateVersion).toBe(2);
    expect(container.textContent).toContain("この端末の位置を最終既読にしました。");

    await act(async () => {
      root.unmount();
    });
  });

  it("同一 writer の409でも位置が違う場合は最新versionで上書きを再試行する", async () => {
    const baseFetchHandler = createReaderFetchHandler();
    let readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "1",
      position: 0,
      updatedAt: "2026-03-22T10:00:00Z",
      stateVersion: 1,
      updatedByClientId: "server"
    };
    const putBodies: Array<Record<string, unknown>> = [];
    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");
      if (requestUrl.pathname === "/api/reader/state") {
        if (init?.method === "PUT") {
          const payload = JSON.parse(String(init.body)) as {
            clientId?: string;
            lastReadEpisodeIndex: string | null;
            position: number;
          };
          putBodies.push(payload as unknown as Record<string, unknown>);
          if (putBodies.length === 1) {
            return jsonResponse(
              {
                novelId: "n1",
                lastReadEpisodeIndex: payload.lastReadEpisodeIndex,
                position: payload.position + 12,
                updatedAt: "2026-03-22T10:07:00Z",
                stateVersion: 3,
                updatedByClientId: payload.clientId ?? null
              },
              { status: 409 }
            );
          }
          return jsonResponse({
            novelId: "n1",
            lastReadEpisodeIndex: payload.lastReadEpisodeIndex,
            position: payload.position,
            updatedAt: "2026-03-22T10:08:00Z",
            stateVersion: 4,
            updatedByClientId: payload.clientId ?? null
          });
        }
        return jsonResponse(readerState);
      }
      return baseFetchHandler(url, init);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=1"
    });

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("本文です。") === true);
    readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "2",
      position: 0,
      updatedAt: "2026-03-22T10:06:00Z",
      stateVersion: 2,
      updatedByClientId: "other-device"
    };

    await act(async () => {
      dom.window.document.dispatchEvent(new dom.window.Event("visibilitychange"));
    });
    await waitFor(() => Boolean(container.querySelector(".reader-sync-conflict")));
    await click(getButtonByText(container, "この端末で上書き"), dom);

    await waitFor(() =>
      container
        .querySelector(".reader-sync-conflict [role=\"alert\"]")
        ?.textContent?.includes("さらに新しい既読位置が保存されています。もう一度確認してください。") === true
    );
    expect(putBodies[0]?.expectedStateVersion).toBe(2);

    await click(getButtonByText(container, "この端末で上書き"), dom);
    await waitFor(() => container.querySelector(".reader-sync-conflict") === null);
    expect(putBodies).toHaveLength(2);
    expect(putBodies[1]?.expectedStateVersion).toBe(3);
    expect(container.textContent).toContain("この端末の位置を最終既読にしました。");

    await act(async () => {
      root.unmount();
    });
  });

  it("古い上書き成功レスポンスで処理中に到着した新しい同期競合を閉じない", async () => {
    const baseFetchHandler = createReaderFetchHandler();
    let readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "1",
      position: 0,
      updatedAt: "2026-03-22T10:00:00Z",
      stateVersion: 1,
      updatedByClientId: "server"
    };
    let resolveOverwrite: ((response: Response) => void) | null = null;
    const putBodies: Array<Record<string, unknown>> = [];
    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");
      if (requestUrl.pathname === "/api/reader/state") {
        if (init?.method === "PUT") {
          const body = JSON.parse(String(init.body)) as Record<string, unknown>;
          putBodies.push(body);
          return new Promise<Response>((resolve) => {
            resolveOverwrite = resolve;
          });
        }
        return jsonResponse(readerState);
      }
      return baseFetchHandler(url, init);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=1"
    });

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("本文です。") === true);
    readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "2",
      position: 20,
      updatedAt: "2026-03-22T10:06:00Z",
      stateVersion: 2,
      updatedByClientId: "other-device"
    };

    await act(async () => {
      dom.window.dispatchEvent(new dom.window.Event("focus"));
    });
    await waitFor(() => Boolean(container.querySelector(".reader-sync-conflict")));
    await click(getButtonByText(container, "この端末で上書き"), dom);
    await waitFor(() => putBodies.length === 1);
    expect(putBodies[0]?.expectedStateVersion).toBe(2);

    readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "2",
      position: 80,
      updatedAt: "2026-03-22T10:08:00Z",
      stateVersion: 4,
      updatedByClientId: "other-device"
    };
    await act(async () => {
      dom.window.dispatchEvent(new dom.window.Event("focus"));
    });
    await waitFor(() => Boolean(container.querySelector(".reader-sync-conflict")));

    await act(async () => {
      resolveOverwrite?.(
        jsonResponse({
          novelId: "n1",
          lastReadEpisodeIndex: putBodies[0]?.lastReadEpisodeIndex ?? "1",
          position: putBodies[0]?.position ?? 0,
          updatedAt: "2026-03-22T10:07:00Z",
          stateVersion: 3,
          updatedByClientId: putBodies[0]?.clientId ?? "reader-client"
        })
      );
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    await waitFor(() => Boolean(container.querySelector(".reader-sync-conflict")));
    expect(container.querySelector(".reader-notice")?.textContent ?? "").not.toContain("この端末の位置を最終既読にしました。");

    await act(async () => {
      root.unmount();
    });
  });

  it("古い反映処理中に到着した新しい同期競合を閉じない", async () => {
    const episode2Response = {
      novelId: "n1",
      episodeIndex: "2",
      title: "第2話",
      chapter: "第一章",
      subchapter: null,
      sourceUrl: "https://example.com/n1/2/",
      html: "",
      plainTextLength: 8,
      updatedAt: "2026-03-21T10:00:00Z",
      contentEtag: "etag-2",
      readerDocument: {
        version: 1,
        blocks: [
          { type: "title", text: "第2話" },
          { type: "paragraph", section: "body", inlines: [{ type: "text", text: "第2話本文" }] }
        ]
      }
    };
    let resolveEpisode2Preflight: ((value: Response) => void) | null = null;
    let shouldDelayEpisode2 = true;
    const pendingEpisode2Preflight = new Promise<Response>((resolve) => {
      resolveEpisode2Preflight = resolve;
    });
    const baseFetchHandler = createReaderFetchHandler({
      episodes: [
        {
          episodeIndex: "1",
          title: "第1話",
          chapter: "第一章",
          subchapter: null,
          sourceUrl: "https://example.com/n1/1/",
          updatedAt: "2026-03-20T10:00:00Z",
          contentEtag: "etag-1",
          bodyStatus: "complete"
        },
        {
          episodeIndex: "2",
          title: "第2話",
          chapter: "第一章",
          subchapter: null,
          sourceUrl: "https://example.com/n1/2/",
          updatedAt: "2026-03-21T10:00:00Z",
          contentEtag: "etag-2",
          bodyStatus: "complete"
        }
      ],
      episodeResponses: { "2": episode2Response }
    });
    let readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "1",
      position: 0,
      updatedAt: "2026-03-22T10:00:00Z",
      stateVersion: 1,
      updatedByClientId: "server"
    };
    const readerStatePuts: Array<Record<string, unknown>> = [];
    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");
      if (requestUrl.pathname === "/api/reader/state") {
        if (init?.method === "PUT") {
          readerStatePuts.push(JSON.parse(String(init.body)) as Record<string, unknown>);
          return jsonResponse({
            novelId: "n1",
            lastReadEpisodeIndex: readerStatePuts.at(-1)?.lastReadEpisodeIndex ?? "1",
            position: readerStatePuts.at(-1)?.position ?? 0,
            updatedAt: "2026-03-22T10:02:00Z",
            stateVersion: 5,
            updatedByClientId: "reader-client"
          });
        }
        return jsonResponse(readerState);
      }
      if (requestUrl.pathname === "/api/library/novels/n1/episodes/2" && shouldDelayEpisode2) {
        shouldDelayEpisode2 = false;
        return pendingEpisode2Preflight;
      }
      return baseFetchHandler(url, init);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=1"
    });

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("本文です。") === true);
    readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "2",
      position: 20,
      updatedAt: "2026-03-22T10:06:00Z",
      stateVersion: 2,
      updatedByClientId: "other-device"
    };

    await act(async () => {
      dom.window.dispatchEvent(new dom.window.Event("focus"));
    });
    await waitFor(() => Boolean(container.querySelector(".reader-sync-conflict")));
    await click(getButtonByText(container, "反映して移動"), dom);
    await waitFor(() => getButtonByText(container, "反映中...").disabled);

    readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "2",
      position: 80,
      updatedAt: "2026-03-22T10:08:00Z",
      stateVersion: 4,
      updatedByClientId: "other-device"
    };
    await act(async () => {
      dom.window.dispatchEvent(new dom.window.Event("focus"));
    });
    await waitFor(() => Boolean(container.querySelector(".reader-sync-conflict")));

    await act(async () => {
      resolveEpisode2Preflight?.(jsonResponse(episode2Response));
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    await waitFor(() =>
      container
        .querySelector(".reader-sync-conflict [role=\"alert\"]")
        ?.textContent?.includes("さらに新しい既読位置が保存されています。もう一度確認してください。") === true
    );
    expect(container.querySelector(".reader-sync-conflict")).not.toBeNull();
    expect(container.textContent).not.toContain("別端末の既読位置を反映しました。");
    expect(readerStatePuts).toHaveLength(0);

    await act(async () => {
      root.unmount();
    });
  });

  it("未取得話への同期競合反映では競合を解除せず既読位置も保存しない", async () => {
    const baseFetchHandler = createReaderFetchHandler({
      episodes: [
        {
          episodeIndex: "1",
          title: "第1話",
          chapter: "第一章",
          subchapter: null,
          sourceUrl: "https://example.com/n1/1/",
          updatedAt: "2026-03-20T10:00:00Z",
          contentEtag: "etag-1",
          bodyStatus: "complete"
        },
        {
          episodeIndex: "2",
          title: "第2話",
          chapter: "第一章",
          subchapter: null,
          sourceUrl: "https://example.com/n1/2/",
          updatedAt: "2026-03-21T10:00:00Z",
          contentEtag: "etag-2",
          bodyStatus: "pending"
        }
      ]
    });
    let readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "1",
      position: 0,
      updatedAt: "2026-03-22T10:00:00Z",
      stateVersion: 1,
      updatedByClientId: "server"
    };
    const readerStatePuts: Array<Record<string, unknown>> = [];
    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");
      if (requestUrl.pathname === "/api/reader/state") {
        if (init?.method === "PUT") {
          readerStatePuts.push(JSON.parse(String(init.body)) as Record<string, unknown>);
          return jsonResponse({
            novelId: "n1",
            lastReadEpisodeIndex: readerStatePuts.at(-1)?.lastReadEpisodeIndex ?? "1",
            position: readerStatePuts.at(-1)?.position ?? 0,
            updatedAt: "2026-03-22T10:02:00Z",
            stateVersion: 3,
            updatedByClientId: "reader-client"
          });
        }
        return jsonResponse(readerState);
      }
      return baseFetchHandler(url, init);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=1"
    });

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("本文です。") === true);
    const putCountBeforeConflict = readerStatePuts.length;
    readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "2",
      position: 0,
      updatedAt: "2026-03-22T10:06:00Z",
      stateVersion: 2,
      updatedByClientId: "other-device"
    };

    await act(async () => {
      dom.window.dispatchEvent(new dom.window.Event("focus"));
    });
    await waitFor(() => Boolean(container.querySelector(".reader-sync-conflict")));
    const applyButton = getButtonByText(container, "反映して移動");
    expect(applyButton.disabled).toBe(true);
    expect(container.textContent).toContain("対象の話はまだ本文が取得されていません。");

    await click(applyButton, dom);
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 80));
    });

    expect(container.querySelector(".reader-sync-conflict")).not.toBeNull();
    expect(readerStatePuts).toHaveLength(putCountBeforeConflict);

    await act(async () => {
      root.unmount();
    });
  });

  it("本文取得に失敗する同期競合反映では競合を解除せず既読位置も保存しない", async () => {
    const baseFetchHandler = createReaderFetchHandler({
      episodes: [
        {
          episodeIndex: "1",
          title: "第1話",
          chapter: "第一章",
          subchapter: null,
          sourceUrl: "https://example.com/n1/1/",
          updatedAt: "2026-03-20T10:00:00Z",
          contentEtag: "etag-1",
          bodyStatus: "complete"
        },
        {
          episodeIndex: "2",
          title: "第2話",
          chapter: "第一章",
          subchapter: null,
          sourceUrl: "https://example.com/n1/2/",
          updatedAt: "2026-03-21T10:00:00Z",
          contentEtag: "etag-2",
          bodyStatus: "complete"
        }
      ]
    });
    let readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "1",
      position: 0,
      updatedAt: "2026-03-22T10:00:00Z",
      stateVersion: 1,
      updatedByClientId: "server"
    };
    const readerStatePuts: Array<Record<string, unknown>> = [];
    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");
      if (requestUrl.pathname === "/api/reader/state") {
        if (init?.method === "PUT") {
          readerStatePuts.push(JSON.parse(String(init.body)) as Record<string, unknown>);
          return jsonResponse({
            novelId: "n1",
            lastReadEpisodeIndex: readerStatePuts.at(-1)?.lastReadEpisodeIndex ?? "1",
            position: readerStatePuts.at(-1)?.position ?? 0,
            updatedAt: "2026-03-22T10:02:00Z",
            stateVersion: 3,
            updatedByClientId: "reader-client"
          });
        }
        return jsonResponse(readerState);
      }
      return baseFetchHandler(url, init);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=1"
    });

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("本文です。") === true);
    const putCountBeforeConflict = readerStatePuts.length;
    readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "2",
      position: 0,
      updatedAt: "2026-03-22T10:06:00Z",
      stateVersion: 2,
      updatedByClientId: "other-device"
    };

    await act(async () => {
      dom.window.dispatchEvent(new dom.window.Event("focus"));
    });
    await waitFor(() => Boolean(container.querySelector(".reader-sync-conflict")));
    const applyButton = getButtonByText(container, "反映して移動");
    expect(applyButton.disabled).toBe(false);

    await click(applyButton, dom);
    await waitFor(() => container.textContent?.includes("別端末の既読位置へ移動できませんでした。") === true);

    expect(container.querySelector(".reader-sync-conflict")).not.toBeNull();
    expect(readerStatePuts).toHaveLength(putCountBeforeConflict);

    await act(async () => {
      root.unmount();
    });
  });

  it("反映先の話がnullまたはTOC外の同期競合では競合を解除せず既読位置も保存しない", async () => {
    const scenarios: Array<{
      label: string;
      lastReadEpisodeIndex: string | null;
      expectedReason: string;
    }> = [
      {
        label: "null",
        lastReadEpisodeIndex: null,
        expectedReason: "反映できる既読位置がありません。"
      },
      {
        label: "missing",
        lastReadEpisodeIndex: "99",
        expectedReason: "対象の話が目次にありません。"
      }
    ];

    for (const scenario of scenarios) {
      const baseFetchHandler = createReaderFetchHandler({
        episodes: [
          {
            episodeIndex: "1",
            title: "第1話",
            chapter: "第一章",
            subchapter: null,
            sourceUrl: "https://example.com/n1/1/",
            updatedAt: "2026-03-20T10:00:00Z",
            contentEtag: "etag-1",
            bodyStatus: "complete"
          }
        ],
        totalEpisodes: 1
      });
      let readerState = {
        novelId: "n1",
        lastReadEpisodeIndex: "1" as string | null,
        position: 0,
        updatedAt: "2026-03-22T10:00:00Z",
        stateVersion: 1,
        updatedByClientId: "server"
      };
      const readerStatePuts: Array<Record<string, unknown>> = [];
      const fetchHandler: FetchHandler = async (url, init) => {
        const requestUrl = new URL(url, "http://localhost");
        if (requestUrl.pathname === "/api/reader/state") {
          if (init?.method === "PUT") {
            readerStatePuts.push(JSON.parse(String(init.body)) as Record<string, unknown>);
            return jsonResponse({
              novelId: "n1",
              lastReadEpisodeIndex: readerStatePuts.at(-1)?.lastReadEpisodeIndex ?? "1",
              position: readerStatePuts.at(-1)?.position ?? 0,
              updatedAt: "2026-03-22T10:02:00Z",
              stateVersion: 3,
              updatedByClientId: "reader-client"
            });
          }
          return jsonResponse(readerState);
        }
        return baseFetchHandler(url, init);
      };

      const { container, root, dom } = await renderApp(fetchHandler, {
        url: `http://localhost/?novelId=n1&episode=1&case=${scenario.label}`
      });

      await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("本文です。") === true);
      const putCountBeforeConflict = readerStatePuts.length;
      readerState = {
        novelId: "n1",
        lastReadEpisodeIndex: scenario.lastReadEpisodeIndex,
        position: 0,
        updatedAt: "2026-03-22T10:06:00Z",
        stateVersion: 2,
        updatedByClientId: "other-device"
      };

      await act(async () => {
        dom.window.dispatchEvent(new dom.window.Event("focus"));
      });
      await waitFor(() => Boolean(container.querySelector(".reader-sync-conflict")));
      const applyButton = getButtonByText(container, "反映して移動");
      expect(applyButton.disabled).toBe(true);
      expect(container.textContent).toContain(scenario.expectedReason);

      await click(applyButton, dom);
      await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 40));
      });

      expect(container.querySelector(".reader-sync-conflict")).not.toBeNull();
      expect(readerStatePuts).toHaveLength(putCountBeforeConflict);

      await act(async () => {
        root.unmount();
      });
    }
  });

  it("複製された storage 値があっても reader client ID をページごとに生成する", async () => {
    const storageKey = "narou-viewer.reader-client-id.v2";
    const randomUUID = vi.fn().mockReturnValueOnce("reader-client-a").mockReturnValueOnce("reader-client-b");
    vi.stubGlobal("crypto", { randomUUID });

    async function captureOverwriteClientId(): Promise<string> {
      const baseFetchHandler = createReaderFetchHandler();
      let readerState = {
        novelId: "n1",
        lastReadEpisodeIndex: "1",
        position: 0,
        updatedAt: "2026-03-22T10:00:00Z",
        stateVersion: 1,
        updatedByClientId: "server"
      };
      let capturedClientId: string | null = null;
      const fetchHandler: FetchHandler = async (url, init) => {
        const requestUrl = new URL(url, "http://localhost");
        if (requestUrl.pathname === "/api/reader/state") {
          if (init?.method === "PUT") {
            const payload = JSON.parse(String(init.body)) as {
              clientId?: string;
              lastReadEpisodeIndex: string | null;
              position: number;
            };
            capturedClientId = payload.clientId ?? null;
            return jsonResponse({
              novelId: "n1",
              lastReadEpisodeIndex: payload.lastReadEpisodeIndex,
              position: payload.position,
              updatedAt: "2026-03-22T10:07:00Z",
              stateVersion: 3,
              updatedByClientId: capturedClientId
            });
          }
          return jsonResponse(readerState);
        }
        return baseFetchHandler(url, init);
      };

      const { container, root, dom } = await renderApp(fetchHandler, {
        url: "http://localhost/?novelId=n1&episode=1",
        beforeRender: (nextDom) => {
          nextDom.window.sessionStorage.setItem(storageKey, "duplicated-client");
        }
      });

      await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("本文です。") === true);
      readerState = {
        novelId: "n1",
        lastReadEpisodeIndex: "2",
        position: 0,
        updatedAt: "2026-03-22T10:06:00Z",
        stateVersion: 2,
        updatedByClientId: "other-device"
      };

      await act(async () => {
        dom.window.document.dispatchEvent(new dom.window.Event("visibilitychange"));
      });
      await waitFor(() => Boolean(container.querySelector(".reader-sync-conflict")));
      await click(getButtonByText(container, "この端末で上書き"), dom);
      await waitFor(() => capturedClientId !== null);

      await act(async () => {
        root.unmount();
      });

      if (capturedClientId === null) {
        throw new Error("reader client ID was not captured");
      }
      return capturedClientId;
    }

    const firstClientId = await captureOverwriteClientId();
    const secondClientId = await captureOverwriteClientId();

    expect(firstClientId).toBe("reader-client-a");
    expect(secondClientId).toBe("reader-client-b");
    expect(firstClientId).not.toBe("duplicated-client");
    expect(secondClientId).not.toBe("duplicated-client");
  });

  it("話数切替中は旧話の位置を新しい話の既読位置として保存しない", async () => {
    const baseFetchHandler = createReaderFetchHandler();
    let readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "1",
      position: 0,
      updatedAt: "2026-03-22T10:00:00Z",
      stateVersion: 1,
      updatedByClientId: "server"
    };
    let resolveEpisode2: ((response: Response) => void) | null = null;
    const pendingEpisode2 = new Promise<Response>((resolve) => {
      resolveEpisode2 = resolve;
    });
    const readerStatePuts: Array<Record<string, unknown>> = [];
    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");
      if (requestUrl.pathname === "/api/reader/state") {
        if (init?.method === "PUT") {
          readerStatePuts.push(JSON.parse(String(init.body)) as Record<string, unknown>);
          return jsonResponse({
            novelId: "n1",
            lastReadEpisodeIndex: readerStatePuts.at(-1)?.lastReadEpisodeIndex ?? "1",
            position: readerStatePuts.at(-1)?.position ?? 0,
            updatedAt: "2026-03-22T10:02:00Z",
            stateVersion: 2,
            updatedByClientId: "reader-client"
          });
        }
        return jsonResponse(readerState);
      }
      if (requestUrl.pathname === "/api/library/novels/n1/episodes/2") {
        return pendingEpisode2;
      }
      return baseFetchHandler(url, init);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=1"
    });

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("本文です。") === true);
    await waitFor(() => getButtonByLabel(container, "次のページへ進む").disabled === false);
    await keyDown(dom.window, "ArrowLeft");
    await keyDown(dom.window, "ArrowLeft");
    await waitFor(() => Boolean(container.querySelector(".reader-next-episode-confirm")));
    await click(getButtonByText(container, "進む"), dom);
    await waitFor(() => Boolean(container.querySelector(".reader-loading-overlay")));

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 80));
    });
    expect(readerStatePuts.some((body) => body.lastReadEpisodeIndex === "2")).toBe(false);

    readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "2",
      position: 0,
      updatedAt: "2026-03-22T10:06:00Z",
      stateVersion: 3,
      updatedByClientId: "other-device"
    };
    await act(async () => {
      dom.window.document.dispatchEvent(new dom.window.Event("visibilitychange"));
    });
    await waitFor(() => Boolean(container.querySelector(".reader-sync-conflict")));
    expect(container.querySelector(".reader-loading-overlay")).not.toBeNull();
    expect(dom.window.document.activeElement?.textContent?.trim()).toBe("反映して移動");
    const overwriteButton = getButtonByText(container, "この端末で上書き");
    expect(overwriteButton.disabled).toBe(true);
    expect(container.textContent).toContain("本文の読み込み完了後に上書きできます。");
    await click(overwriteButton, dom);
    expect(readerStatePuts.some((body) => body.lastReadEpisodeIndex === "2")).toBe(false);

    resolveEpisode2?.(
      jsonResponse({
        novelId: "n1",
        episodeIndex: "2",
        title: "第2話",
        chapter: "第一章",
        subchapter: null,
        sourceUrl: "https://example.com/n1/2/",
        html: "",
        plainTextLength: 10,
        updatedAt: "2026-03-22T10:06:00Z",
        contentEtag: "etag-2",
        readerDocument: {
          version: 1,
          blocks: [
            { type: "title", text: "第2話" },
            { type: "paragraph", section: "body", inlines: [{ type: "text", text: "本文2です。" }] }
          ]
        }
      })
    );

    await act(async () => {
      root.unmount();
    });
  });

  it("読み上げ中に同期競合を反映しても読み上げprogressで既読位置を上書きしない", async () => {
    const speechSynthesis = createAppMockSpeechSynthesis();
    const speechDocument = {
      version: 1,
      blocks: [
        { type: "title", text: "第1話" },
        {
          type: "paragraph",
          section: "body",
          inlines: [{ type: "text", text: "読み上げ中の本文です。もう少し続きます。" }]
        }
      ]
    };
    const baseFetchHandler = createReaderFetchHandler({ readerDocument: speechDocument, plainTextLength: 24 });
    let readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "1",
      position: 0,
      updatedAt: "2026-03-22T10:00:00Z",
      stateVersion: 1,
      updatedByClientId: "server"
    };
    const readerStatePuts: Array<Record<string, unknown>> = [];
    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");
      if (requestUrl.pathname === "/api/reader/state") {
        if (init?.method === "PUT") {
          readerStatePuts.push(JSON.parse(String(init.body)) as Record<string, unknown>);
          return jsonResponse({
            novelId: "n1",
            lastReadEpisodeIndex: readerStatePuts.at(-1)?.lastReadEpisodeIndex ?? "1",
            position: readerStatePuts.at(-1)?.position ?? 0,
            updatedAt: "2026-03-22T10:08:00Z",
            stateVersion: 4,
            updatedByClientId: "reader-client"
          });
        }
        return jsonResponse(readerState);
      }
      return baseFetchHandler(url, init);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=1",
      speechSynthesis,
      speechSynthesisUtterance: MockAppSpeechSynthesisUtterance
    });

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("読み上げ中") === true);
    await click(container.querySelector('button[aria-label="読み上げ"]') as Element, dom);
    await waitFor(() => Boolean(container.querySelector(".reader-speech-panel")));
    await click(getButtonByText(container, "現在位置から再生"), dom);
    await waitFor(() => speechSynthesis.utterances.length === 1);
    const cancelCountBeforeConflict = speechSynthesis.cancel.mock.calls.length;

    readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "1",
      position: 8,
      updatedAt: "2026-03-22T10:06:00Z",
      stateVersion: 2,
      updatedByClientId: "other-device"
    };
    await act(async () => {
      dom.window.document.dispatchEvent(new dom.window.Event("visibilitychange"));
    });

    await waitFor(() => Boolean(container.querySelector(".reader-sync-conflict")));
    expect(speechSynthesis.cancel.mock.calls.length).toBeGreaterThan(cancelCountBeforeConflict);
    await click(getButtonByText(container, "反映して移動"), dom);
    await waitFor(() => container.querySelector(".reader-sync-conflict") === null);
    const putCountAfterApply = readerStatePuts.length;

    await act(async () => {
      speechSynthesis.utterances[0]?.onboundary?.({ charIndex: 10, elapsedTime: 1.5 });
      await new Promise((resolve) => setTimeout(resolve, 80));
    });

    expect(readerStatePuts).toHaveLength(putCountAfterApply);

    await act(async () => {
      root.unmount();
    });
  });

  it("画像ビューア表示中に同期競合が到着したら画像ビューアを閉じる", async () => {
    const imageDocument = {
      version: 1,
      blocks: [
        { type: "title", text: "第1話" },
        {
          type: "image",
          section: "body",
          src: "https://example.com/image.jpg",
          alt: "挿絵",
          originalUrl: "https://example.com/original.jpg",
          title: "挿絵"
        },
        {
          type: "paragraph",
          section: "body",
          inlines: [{ type: "text", text: "本文です。" }]
        }
      ]
    };
    const baseFetchHandler = createReaderFetchHandler({ readerDocument: imageDocument });
    let readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "1",
      position: 0,
      updatedAt: "2026-03-22T10:00:00Z",
      stateVersion: 1,
      updatedByClientId: "server"
    };
    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");
      if (requestUrl.pathname === "/api/reader/state" && init?.method !== "PUT") {
        return jsonResponse(readerState);
      }
      return baseFetchHandler(url, init);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=1"
    });

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("本文です。") === true);
    const image = container.querySelector(".reader-prose-paged img");
    expect(image).toBeInstanceOf(dom.window.HTMLImageElement);
    await click(image as Element, dom, { clientX: 400, clientY: 40 });
    await waitFor(() => Boolean(container.querySelector(".reader-image-viewer")));

    readerState = {
      novelId: "n1",
      lastReadEpisodeIndex: "2",
      position: 0,
      updatedAt: "2026-03-22T10:06:00Z",
      stateVersion: 2,
      updatedByClientId: "other-device"
    };
    await act(async () => {
      dom.window.document.dispatchEvent(new dom.window.Event("visibilitychange"));
    });

    await waitFor(() => Boolean(container.querySelector(".reader-sync-conflict")));
    expect(container.querySelector(".reader-image-viewer")).toBeNull();
    expect(container.querySelectorAll('[role="dialog"], [role="alertdialog"]')).toHaveLength(1);
    expect(dom.window.document.activeElement?.textContent?.trim()).toBe("反映して移動");

    await act(async () => {
      root.unmount();
    });
  });

  it("reader 画面で栞保存とキャラクター一覧生成を実行できる", async () => {
    let characterJobsRequestCount = 0;
    let characterSummaryRequestCount = 0;

    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return jsonResponse({
          status: "ok",
          checkedAt: "2026-03-22T10:00:00Z",
          services: [{ id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" }]
        });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: [
            {
              novelId: "n1",
              fetcherWorkId: "work-1",
              title: "小説A",
              author: "作者A",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n1",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: "2",
              lastReadEpisodeTitle: "第2話",
              latestBookmarkEpisodeIndex: "1",
              bookmarkCount: 1,
              totalEpisodes: 3
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        if (init?.method === "PUT") {
          return jsonResponse({
            readingMode: "vertical",
            fontFamily: "mincho",
            theme: "classic",
            updatedAt: "2026-03-22T10:00:00Z"
          });
        }

        return jsonResponse({
          readingMode: "vertical",
          fontFamily: "mincho",
          theme: "classic",
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({ total: 0, webWorker: 0, worker: 0, running: false });
      }

      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        return jsonResponse({
          current: null,
          queued: [],
          recentCompleted: [],
          recentFailed: [],
          completedCount: 0,
          failedCount: 0,
          convertCurrent: null,
          convertQueued: []
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        return jsonResponse({
          apiBaseUrlConfigured: true,
          masterPassphraseConfigured: true,
          preferredMode: "heuristic",
          effectiveGenerationMode: "heuristic",
          settings: {
            selectedProfileId: "default",
            profiles: [
              {
                id: "default",
                label: "Default",
                hasApiKey: false,
                apiKeyMasked: null,
                modelId: null,
                providerOrder: [],
                allowFallbacks: true,
                requireParameters: false,
                updatedAt: "2026-03-22T10:00:00Z"
              }
            ]
          }
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/toc") {
        return jsonResponse({
          novelId: "n1",
          fetcherWorkId: "work-1",
          title: "小説A",
          author: "作者A",
          siteName: "小説家になろう",
          tocUrl: "https://example.com/n1",
          updatedAt: "2026-03-22T10:00:00Z",
          totalEpisodes: 3,
          story: "reader test story",
          episodes: [
            {
              episodeIndex: "1",
              title: "第1話",
              chapter: "第一章",
              subchapter: null,
              updatedAt: "2026-03-20T10:00:00Z",
              contentEtag: "etag-1"
            },
            {
              episodeIndex: "2",
              title: "第2話",
              chapter: "第一章",
              subchapter: null,
              updatedAt: "2026-03-21T10:00:00Z",
              contentEtag: "etag-2"
            },
            {
              episodeIndex: "3",
              title: "第3話",
              chapter: "第一章",
              subchapter: null,
              updatedAt: "2026-03-22T10:00:00Z",
              contentEtag: "etag-3"
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/state") {
        if (init?.method === "PUT") {
          return jsonResponse({
            novelId: "n1",
            lastReadEpisodeIndex: "2",
            position: 0,
            updatedAt: "2026-03-22T10:00:00Z"
          });
        }

        return jsonResponse({
          novelId: "n1",
          lastReadEpisodeIndex: "2",
          position: 0,
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/bookmarks") {
        if (init?.method === "POST") {
          return jsonResponse({
            id: "b2",
            novelId: "n1",
            episodeIndex: "2",
            position: 0,
            label: "本文です。",
            createdAt: "2026-03-22T10:05:00Z"
          });
        }

        return jsonResponse({
          bookmarks: [
            {
              id: "b1",
              novelId: "n1",
              episodeIndex: "1",
              position: 0,
              label: "しおり",
              createdAt: "2026-03-22T10:00:00Z"
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/episodes/2") {
        return jsonResponse({
          novelId: "n1",
          episodeIndex: "2",
          title: "第2話",
          chapter: "第一章",
          subchapter: null,
          html: "",
          plainTextLength: 10,
          updatedAt: "2026-03-22T10:00:00Z",
          contentEtag: "etag-2",
          readerDocument: {
            version: 1,
            blocks: [
              { type: "title", text: "第2話" },
              {
                type: "paragraph",
                section: "body",
                inlines: [{ type: "text", text: "本文です。" }]
              }
            ]
          }
        });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/characters") {
        characterSummaryRequestCount += 1;

        return jsonResponse({
          status: "ready",
          upToEpisodeIndex: "2",
          processedUpToEpisodeIndex: "2",
          characters:
            characterSummaryRequestCount >= 2
              ? [
                  {
                    characterId: "c1",
                    canonicalName: "主人公",
                    fullName: "主人公 太郎",
                    gender: "male",
                    firstAppearanceEpisodeIndex: "1",
                    aliases: ["タロウ"],
                    appearance: "長身",
                    personality: "冷静",
                    summary: "主人公です。"
                  }
                ]
              : []
        });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/terms") {
        return jsonResponse({
          status: "ready",
          novelId: "n1",
          upToEpisodeIndex: "2",
          processedUpToEpisodeIndex: "2",
          terms: [{ term: "聖剣", reading: "せいけん", category: "item", description: "王家に伝わる剣。" }]
        });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/extraction-jobs") {
        if (init?.method === "POST") {
          return jsonResponse({
            jobId: "job-2",
            message: "人物と用語の抽出を依頼しました。"
          });
        }

        characterJobsRequestCount += 1;
        return jsonResponse({
          jobs:
            characterJobsRequestCount >= 2
              ? [
                  {
                    jobId: "job-2",
                    requestedUpToEpisodeIndex: "2",
                    status: "completed",
                    createdAt: "2026-03-22T10:02:00Z",
                    errorMessage: null
                  }
                ]
              : []
        });
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=2"
    });

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("第2話") === true);

    await click(container.querySelector('button[aria-label="栞を追加"]') as Element, dom);
    await waitFor(() => container.textContent?.includes("栞を保存しました。") === true);

    await click(container.querySelector('button[aria-label="キャラクター一覧"]') as Element, dom);
    await waitFor(() => Boolean(container.querySelector(".reader-character-panel")));
    expect(container.textContent).toContain("キャラクターは抽出されませんでした。必要なら対象話数を変えて再生成できます。");

    const generateButton = getButtonByText(container, "人物と用語を抽出");
    await click(generateButton, dom);
    await waitFor(() => container.textContent?.includes("人物と用語の抽出を依頼しました。") === true);
    await waitFor(() => container.textContent?.includes("主人公") === true);
    expect(container.textContent).toContain("過去の生成履歴");

    await click(container.querySelector('button[aria-label="キャラクター一覧を閉じる"]') as Element, dom);
    await waitFor(() => !container.textContent?.includes("キャラクター一覧"));

    await click(container.querySelector('button[aria-label="用語一覧"]') as Element, dom);
    await waitFor(() => Boolean(container.querySelector(".reader-term-panel")));
    expect(container.textContent).toContain("聖剣");
    expect(container.textContent).toContain("王家に伝わる剣。");

    await act(async () => {
      root.unmount();
    });
  });

  it("reader 画面で未生成の最新話数なら生成済みの直近一覧へフォールバックする", async () => {
    const fetchHandler: FetchHandler = async (url, _init) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return jsonResponse({
          status: "ok",
          checkedAt: "2026-03-22T10:00:00Z",
          services: [{ id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" }]
        });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: [
            {
              novelId: "n1",
              fetcherWorkId: "work-1",
              title: "小説A",
              author: "作者A",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n1",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: "3",
              lastReadEpisodeTitle: "第3話",
              latestBookmarkEpisodeIndex: "1",
              bookmarkCount: 1,
              totalEpisodes: 3
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        return jsonResponse({
          readingMode: "vertical",
          fontFamily: "mincho",
          theme: "classic",
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({ total: 0, webWorker: 0, worker: 0, running: false });
      }

      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        return jsonResponse({
          current: null,
          queued: [],
          recentCompleted: [],
          recentFailed: [],
          completedCount: 0,
          failedCount: 0,
          convertCurrent: null,
          convertQueued: []
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        return jsonResponse({
          apiBaseUrlConfigured: true,
          masterPassphraseConfigured: true,
          preferredMode: "heuristic",
          effectiveGenerationMode: "heuristic",
          settings: {
            selectedProfileId: "default",
            profiles: [
              {
                id: "default",
                label: "Default",
                hasApiKey: false,
                apiKeyMasked: null,
                modelId: null,
                providerOrder: [],
                allowFallbacks: true,
                requireParameters: false,
                updatedAt: "2026-03-22T10:00:00Z"
              }
            ]
          }
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/toc") {
        return jsonResponse({
          novelId: "n1",
          fetcherWorkId: "work-1",
          title: "小説A",
          author: "作者A",
          siteName: "小説家になろう",
          tocUrl: "https://example.com/n1",
          updatedAt: "2026-03-22T10:00:00Z",
          totalEpisodes: 3,
          story: "reader test story",
          episodes: [
            {
              episodeIndex: "1",
              title: "第1話",
              chapter: "第一章",
              subchapter: null,
              updatedAt: "2026-03-20T10:00:00Z",
              contentEtag: "etag-1"
            },
            {
              episodeIndex: "2",
              title: "第2話",
              chapter: "第一章",
              subchapter: null,
              updatedAt: "2026-03-21T10:00:00Z",
              contentEtag: "etag-2"
            },
            {
              episodeIndex: "3",
              title: "第3話",
              chapter: "第一章",
              subchapter: null,
              updatedAt: "2026-03-22T10:00:00Z",
              contentEtag: "etag-3"
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/state") {
        return jsonResponse({
          novelId: "n1",
          lastReadEpisodeIndex: "3",
          position: 0,
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/bookmarks") {
        return jsonResponse({ bookmarks: [] });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/episodes/3") {
        return jsonResponse({
          novelId: "n1",
          episodeIndex: "3",
          title: "第3話",
          chapter: "第一章",
          subchapter: null,
          html: "",
          plainTextLength: 10,
          updatedAt: "2026-03-22T10:00:00Z",
          contentEtag: "etag-3",
          readerDocument: {
            version: 1,
            blocks: [
              { type: "title", text: "第3話" },
              {
                type: "paragraph",
                section: "body",
                inlines: [{ type: "text", text: "本文です。" }]
              }
            ]
          }
        });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/characters") {
        if (requestUrl.searchParams.get("upToEpisodeIndex") === "2") {
          return jsonResponse({
            status: "not_generated",
            novelId: "n1",
            upToEpisodeIndex: "2",
            processedUpToEpisodeIndex: "1",
            characters: []
          });
        }

        if (requestUrl.searchParams.get("upToEpisodeIndex") === "1") {
          return jsonResponse({
            status: "ready",
            novelId: "n1",
            upToEpisodeIndex: "1",
            processedUpToEpisodeIndex: "1",
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
              }
            ]
          });
        }

        throw new Error(
          `Unhandled characters request upToEpisodeIndex: ${requestUrl.searchParams.get("upToEpisodeIndex")}`
        );
      }

      if (requestUrl.pathname === "/api/library/novels/n1/terms") {
        return jsonResponse({
          status: "ready",
          novelId: "n1",
          upToEpisodeIndex: "2",
          processedUpToEpisodeIndex: "2",
          terms: []
        });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/extraction-jobs") {
        return jsonResponse({ jobs: [] });
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { container, root, dom } = await renderApp(fetchHandler, {
      url: "http://localhost/?novelId=n1&episode=3"
    });

    await waitFor(() => Boolean(container.querySelector(".reader-shell")) && container.textContent?.includes("第3話") === true);

    await click(container.querySelector('button[aria-label="キャラクター一覧"]') as Element, dom);
    await waitFor(() => container.textContent?.includes("主人公") === true);
    expect(container.textContent).toContain(
      "第1話時点までの生成済み一覧を表示しています。第2話時点の一覧はまだ生成されていません。"
    );
    expect(container.textContent).toContain("第1話時点 / 1 / 1 人");

    await act(async () => {
      root.unmount();
    });
  });

  it("library 画面で作品の更新・削除を実行できる", async () => {
    let isRemoved = false;
    let updateAccepted = false;
    const confirmMock = vi.fn(() => true);

    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return jsonResponse({
          status: "ok",
          checkedAt: "2026-03-22T10:00:00Z",
          services: [
            { id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "novel-fetcher", label: "novel-fetcher", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "library", label: "library", status: "ok", summary: "ok", detail: "1 作品" }
          ]
        });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: isRemoved
            ? []
            : [
                {
                  novelId: "n1",
                  fetcherWorkId: "321",
                  title: "小説A",
                  author: "作者A",
                  siteName: "小説家になろう",
                  tocUrl: "https://example.com/n1",
                  updatedAt: "2026-03-22T10:00:00Z",
                  lastReadEpisodeIndex: "2",
                  lastReadEpisodeTitle: "第2話",
                  latestBookmarkEpisodeIndex: "2",
                  bookmarkCount: 1,
                  totalEpisodes: 12
                }
              ]
        });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        return jsonResponse({
          readingMode: "vertical",
          fontFamily: "mincho",
          theme: "classic",
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({
          total: 0,
          webWorker: 0,
          worker: 0,
          running: false
        });
      }

      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        return jsonResponse({
          current: null,
          queued: [],
          recentCompleted: updateAccepted
            ? [{ id: "update-task-1", type: "update", novelIds: ["321"], status: "completed", novelTitle: "小説A" }]
            : [],
          recentFailed: [],
          completedCount: updateAccepted ? 1 : 0,
          failedCount: 0,
          convertCurrent: null,
          convertQueued: []
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        return jsonResponse({
          apiBaseUrlConfigured: true,
          masterPassphraseConfigured: true,
          preferredMode: "heuristic",
          effectiveGenerationMode: "heuristic",
          settings: {
            selectedProfileId: "default",
            profiles: [
              {
                id: "default",
                label: "Default",
                hasApiKey: false,
                apiKeyMasked: null,
                modelId: null,
                providerOrder: [],
                allowFallbacks: true,
                requireParameters: false,
                updatedAt: "2026-03-22T10:00:00Z"
              }
            ]
          }
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/toc") {
        return jsonResponse({
          novelId: "n1",
          fetcherWorkId: "321",
          title: "小説A",
          author: "作者A",
          siteName: "小説家になろう",
          tocUrl: "https://example.com/n1",
          updatedAt: "2026-03-22T10:00:00Z",
          totalEpisodes: 12,
          story: "これはとても長いあらすじです。".repeat(4),
          episodes: [
            {
              episodeIndex: "1",
              title: "第1話",
              chapter: "第一章",
              subchapter: null,
              updatedAt: "2026-03-20T10:00:00Z",
              contentEtag: "etag-1"
            },
            {
              episodeIndex: "2",
              title: "第2話",
              chapter: "第一章",
              subchapter: null,
              updatedAt: "2026-03-21T10:00:00Z",
              contentEtag: "etag-2"
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/state") {
        return jsonResponse({
          novelId: "n1",
          lastReadEpisodeIndex: "2",
          position: 0,
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/bookmarks") {
        return jsonResponse({
          bookmarks: [
            {
              id: "b1",
              novelId: "n1",
              episodeIndex: "2",
              position: 0,
              label: "しおり",
              createdAt: "2026-03-22T10:00:00Z"
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/fetcher/works/update") {
        expect(init?.method).toBe("POST");
        expect(JSON.parse(String(init?.body))).toEqual({
          novelIds: ["n1"],
          forceRedownload: false,
          includeFrozen: false,
          convertAfterUpdate: false,
          skipUnchanged: true
        });

        updateAccepted = true;
        return jsonResponse({
          taskIds: ["update-task-1"],
          message: "更新を開始しました。"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/works/remove") {
        expect(init?.method).toBe("POST");
        expect(JSON.parse(String(init?.body))).toEqual({
          novelIds: ["n1"],
          withFiles: true
        });

        isRemoved = true;
        return jsonResponse({
          novelIds: ["n1"],
          message: "作品を削除しました。"
        });
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { container, root, dom } = await renderApp(fetchHandler);
    dom.window.confirm = confirmMock as typeof dom.window.confirm;
    vi.stubGlobal("confirm", confirmMock);

    await waitFor(() => container.textContent?.includes("Library") === true && container.textContent?.includes("小説A") === true);

    await click(getButtonByText(container, "小説A"), dom);
    await waitFor(() => container.textContent?.includes("最終更新") === true && container.textContent?.includes("更新") === true);

    await click(getButtonByText(container, "更新", "summary-action-button"), dom);
    await waitFor(() => container.textContent?.includes("更新を開始しました。taskId: update-task-1") === true);

    await click(getButtonByText(container, "削除", "summary-action-button"), dom);
    await waitFor(() => container.textContent?.includes("作品を削除しました。") === true);
    await waitFor(() => container.textContent?.includes("まだ作品がありません") === true);
    expect(confirmMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      root.unmount();
    });
  });

  it("library 画面でダウンロード開始失敗を表示する", async () => {
    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return jsonResponse({
          status: "ok",
          checkedAt: "2026-03-22T10:00:00Z",
          services: [
            { id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "novel-fetcher", label: "novel-fetcher", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "library", label: "library", status: "ok", summary: "ok", detail: "1 作品" }
          ]
        });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: [
            {
              novelId: "n1",
              fetcherWorkId: "321",
              title: "小説A",
              author: "作者A",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n1",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: "2",
              lastReadEpisodeTitle: "第2話",
              latestBookmarkEpisodeIndex: "2",
              bookmarkCount: 1,
              totalEpisodes: 12
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        return jsonResponse({
          readingMode: "vertical",
          fontFamily: "mincho",
          theme: "classic",
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({ total: 0, webWorker: 0, worker: 0, running: false });
      }

      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        return jsonResponse({
          current: null,
          queued: [],
          recentCompleted: [],
          recentFailed: [],
          completedCount: 0,
          failedCount: 0,
          convertCurrent: null,
          convertQueued: []
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        return jsonResponse({
          apiBaseUrlConfigured: true,
          masterPassphraseConfigured: true,
          preferredMode: "heuristic",
          effectiveGenerationMode: "heuristic",
          settings: {
            selectedProfileId: "default",
            profiles: [
              {
                id: "default",
                label: "Default",
                hasApiKey: false,
                apiKeyMasked: null,
                modelId: "openai/gpt-5.4-mini",
                providerOrder: [],
                allowFallbacks: true,
                requireParameters: false,
                updatedAt: "2026-03-22T10:00:00Z"
              }
            ]
          }
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }

      if (requestUrl.pathname === "/api/fetcher/works/download") {
        expect(init?.method).toBe("POST");
        expect(JSON.parse(String(init?.body))).toEqual({
          targets: ["https://example.com/new-work"],
          force: false,
          convertAfterDownload: false,
          mail: false
        });

        return jsonResponse(
          {
            error: "novel-fetcher へのダウンロード依頼に失敗しました。"
          },
          { status: 502 }
        );
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { container, root, dom } = await renderApp(fetchHandler);

    await waitFor(() => container.textContent?.includes("Library") === true && container.textContent?.includes("小説A") === true);

    await click(container.querySelector('button[aria-label="小説を追加"]') as Element, dom);
    await waitFor(() => container.textContent?.includes("ダウンロード") === true);

    const composer = container.querySelector(".library-download-composer");
    if (!(composer instanceof dom.window.HTMLDivElement)) {
      throw new Error("download composer not found");
    }
    await dropText(composer, "https://example.com/new-work", dom);

    const downloadForm = container.querySelector(".download-form");
    if (!(downloadForm instanceof dom.window.HTMLFormElement)) {
      throw new Error("download form not found");
    }
    await submitForm(downloadForm, dom);

    await waitFor(() => container.textContent?.includes("novel-fetcher へのダウンロード依頼に失敗しました。") === true);
    expect(container.textContent).toContain("novel-fetcher へのダウンロード依頼に失敗しました。");

    await act(async () => {
      root.unmount();
    });
  });

  it("AI 設定保存失敗をワークスペース内に表示する", async () => {
    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return jsonResponse({
          status: "ok",
          checkedAt: "2026-03-22T10:00:00Z",
          services: [
            { id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "novel-fetcher", label: "novel-fetcher", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "go-internal-ai", label: "Go internal AI", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "library", label: "library", status: "ok", summary: "ok", detail: "1 作品" }
          ]
        });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: [
            {
              novelId: "n1",
              fetcherWorkId: "321",
              title: "小説A",
              author: "作者A",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n1",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: "2",
              lastReadEpisodeTitle: "第2話",
              latestBookmarkEpisodeIndex: "2",
              bookmarkCount: 1,
              totalEpisodes: 12
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        return jsonResponse({
          readingMode: "vertical",
          fontFamily: "mincho",
          theme: "classic",
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({ total: 0, webWorker: 0, worker: 0, running: false });
      }

      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        return jsonResponse({
          current: null,
          queued: [],
          recentCompleted: [],
          recentFailed: [],
          completedCount: 0,
          failedCount: 0,
          convertCurrent: null,
          convertQueued: []
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        if (init?.method === "PUT") {
          return jsonResponse(
            {
              error: "保存済み APIキーを復号できませんでした。"
            },
            { status: 500 }
          );
        }

        return jsonResponse({
          apiBaseUrlConfigured: true,
          masterPassphraseConfigured: true,
          preferredMode: "llm",
          effectiveGenerationMode: "llm",
          settings: {
            selectedProfileId: "default",
            profiles: [
              {
                id: "default",
                label: "Default",
                hasApiKey: true,
                apiKeyMasked: "sk-****",
                modelId: "openai/gpt-5.4-mini",
                providerOrder: ["openai"],
                allowFallbacks: true,
                requireParameters: false,
                updatedAt: "2026-03-22T10:00:00Z"
              }
            ]
          }
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { container, root, dom } = await renderApp(fetchHandler);

    await waitFor(() => container.textContent?.includes("Library") === true && container.textContent?.includes("小説A") === true);

    await click(getButtonByText(container, "AI機能"), dom);
    await waitFor(() => Boolean(container.querySelector(".ai-generation-popover")));
    await click(getButtonByText(container, "設定", "ai-generation-nav-button"), dom);
    await waitFor(() => Boolean(container.querySelector("#ai-generation-workspace")));

    await click(getButtonByText(container, "プロファイル設定を保存"), dom);
    await waitFor(() => container.textContent?.includes("保存済み APIキーを復号できませんでした。") === true);
    expect(container.textContent).toContain("保存済み APIキーを復号できませんでした。");

    await act(async () => {
      root.unmount();
    });
  });

  it("library 画面で作品更新失敗と削除失敗を表示する", async () => {
    const confirmMock = vi.fn(() => true);

    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return jsonResponse({
          status: "ok",
          checkedAt: "2026-03-22T10:00:00Z",
          services: [
            { id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "novel-fetcher", label: "novel-fetcher", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "library", label: "library", status: "ok", summary: "ok", detail: "1 作品" }
          ]
        });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: [
            {
              novelId: "n1",
              fetcherWorkId: "321",
              title: "小説A",
              author: "作者A",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n1",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: "2",
              lastReadEpisodeTitle: "第2話",
              latestBookmarkEpisodeIndex: "2",
              bookmarkCount: 1,
              totalEpisodes: 12
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        return jsonResponse({
          readingMode: "vertical",
          fontFamily: "mincho",
          theme: "classic",
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({ total: 0, webWorker: 0, worker: 0, running: false });
      }

      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        return jsonResponse({
          current: null,
          queued: [],
          recentCompleted: [],
          recentFailed: [],
          completedCount: 0,
          failedCount: 0,
          convertCurrent: null,
          convertQueued: []
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        return jsonResponse({
          apiBaseUrlConfigured: true,
          masterPassphraseConfigured: true,
          preferredMode: "heuristic",
          effectiveGenerationMode: "heuristic",
          settings: {
            selectedProfileId: "default",
            profiles: [
              {
                id: "default",
                label: "Default",
                hasApiKey: false,
                apiKeyMasked: null,
                modelId: "openai/gpt-5.4-mini",
                providerOrder: [],
                allowFallbacks: true,
                requireParameters: false,
                updatedAt: "2026-03-22T10:00:00Z"
              }
            ]
          }
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }

      if (requestUrl.pathname === "/api/library/novels/n1/toc") {
        return jsonResponse({
          novelId: "n1",
          fetcherWorkId: "321",
          title: "小説A",
          author: "作者A",
          siteName: "小説家になろう",
          tocUrl: "https://example.com/n1",
          updatedAt: "2026-03-22T10:00:00Z",
          totalEpisodes: 12,
          story: "これはとても長いあらすじです。".repeat(4),
          episodes: [
            {
              episodeIndex: "1",
              title: "第1話",
              chapter: "第一章",
              subchapter: null,
              updatedAt: "2026-03-20T10:00:00Z",
              contentEtag: "etag-1"
            },
            {
              episodeIndex: "2",
              title: "第2話",
              chapter: "第一章",
              subchapter: null,
              updatedAt: "2026-03-21T10:00:00Z",
              contentEtag: "etag-2"
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/state") {
        return jsonResponse({
          novelId: "n1",
          lastReadEpisodeIndex: "2",
          position: 0,
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/bookmarks") {
        return jsonResponse({
          bookmarks: [
            {
              id: "b1",
              novelId: "n1",
              episodeIndex: "2",
              position: 0,
              label: "しおり",
              createdAt: "2026-03-22T10:00:00Z"
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/fetcher/works/update") {
        expect(init?.method).toBe("POST");
        return jsonResponse(
          {
            error: "novel-fetcher 側で更新ジョブを開始できませんでした。"
          },
          { status: 502 }
        );
      }

      if (requestUrl.pathname === "/api/fetcher/works/remove") {
        expect(init?.method).toBe("POST");
        return jsonResponse(
          {
            error: "作品削除ジョブの開始に失敗しました。"
          },
          { status: 500 }
        );
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { container, root, dom } = await renderApp(fetchHandler);
    dom.window.confirm = confirmMock as typeof dom.window.confirm;
    vi.stubGlobal("confirm", confirmMock);

    await waitFor(() => container.textContent?.includes("Library") === true && container.textContent?.includes("小説A") === true);

    await click(getButtonByText(container, "小説A"), dom);
    await waitFor(() => container.textContent?.includes("最終更新") === true && container.textContent?.includes("更新") === true);

    await click(getButtonByText(container, "更新", "summary-action-button"), dom);
    await waitFor(() => container.textContent?.includes("novel-fetcher 側で更新ジョブを開始できませんでした。") === true);

    await click(getButtonByText(container, "削除", "summary-action-button"), dom);
    await waitFor(() => container.textContent?.includes("作品削除ジョブの開始に失敗しました。") === true);
    expect(confirmMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      root.unmount();
    });
  });

  it("AI 連携モード切替失敗をワークスペース内に表示する", async () => {
    const fetchHandler: FetchHandler = async (url, init) => {
      const requestUrl = new URL(url, "http://localhost");

      if (requestUrl.pathname === "/api/system/status") {
        return jsonResponse({
          status: "ok",
          checkedAt: "2026-03-22T10:00:00Z",
          services: [
            { id: "viewer-api", label: "viewer-api", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "novel-fetcher", label: "novel-fetcher", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "go-internal-ai", label: "Go internal AI", status: "ok", summary: "ok", detail: "稼働中" },
            { id: "library", label: "library", status: "ok", summary: "ok", detail: "1 作品" }
          ]
        });
      }

      if (requestUrl.pathname === "/api/library/novels") {
        return jsonResponse({
          novels: [
            {
              novelId: "n1",
              fetcherWorkId: "321",
              title: "小説A",
              author: "作者A",
              siteName: "小説家になろう",
              tocUrl: "https://example.com/n1",
              updatedAt: "2026-03-22T10:00:00Z",
              lastReadEpisodeIndex: "2",
              lastReadEpisodeTitle: "第2話",
              latestBookmarkEpisodeIndex: "2",
              bookmarkCount: 1,
              totalEpisodes: 12
            }
          ]
        });
      }

      if (requestUrl.pathname === "/api/reader/preferences") {
        return jsonResponse({
          readingMode: "vertical",
          fontFamily: "mincho",
          theme: "classic",
          updatedAt: "2026-03-22T10:00:00Z"
        });
      }

      if (requestUrl.pathname === "/api/fetcher/queue") {
        return jsonResponse({ total: 0, webWorker: 0, worker: 0, running: false });
      }

      if (requestUrl.pathname === "/api/fetcher/tasks/summary") {
        return jsonResponse({
          current: null,
          queued: [],
          recentCompleted: [],
          recentFailed: [],
          completedCount: 0,
          failedCount: 0,
          convertCurrent: null,
          convertQueued: []
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/settings/preferred-mode") {
        expect(init?.method).toBe("PUT");
        return jsonResponse(
          {
            error: "AI 連携モードの切り替えに失敗しました。"
          },
          { status: 500 }
        );
      }

      if (requestUrl.pathname === "/api/ai-generation/settings") {
        return jsonResponse({
          apiBaseUrlConfigured: true,
          masterPassphraseConfigured: true,
          preferredMode: "heuristic",
          effectiveGenerationMode: "heuristic",
          settings: {
            selectedProfileId: "default",
            profiles: [
              {
                id: "default",
                label: "Default",
                hasApiKey: true,
                apiKeyMasked: "sk-****",
                modelId: "openai/gpt-5.4-mini",
                providerOrder: ["openai"],
                allowFallbacks: true,
                requireParameters: false,
                updatedAt: "2026-03-22T10:00:00Z"
              }
            ]
          }
        });
      }

      if (requestUrl.pathname === "/api/ai-generation/jobs") {
        return jsonResponse({ jobs: [] });
      }

      throw new Error(`Unhandled fetch: ${requestUrl.pathname}${requestUrl.search}`);
    };

    const { container, root, dom } = await renderApp(fetchHandler);

    await waitFor(() => container.textContent?.includes("Library") === true && container.textContent?.includes("小説A") === true);

    await click(getButtonByText(container, "AI機能"), dom);
    await waitFor(() => Boolean(container.querySelector(".ai-generation-popover")));
    await click(getButtonByText(container, "設定", "ai-generation-nav-button"), dom);
    await waitFor(() => Boolean(container.querySelector("#ai-generation-workspace")));

    await click(getButtonByText(container, "LLM連携"), dom);
    await waitFor(() => container.textContent?.includes("AI 連携モードの切り替えに失敗しました。") === true);
    expect(container.textContent).toContain("AI 連携モードの切り替えに失敗しました。");

    await act(async () => {
      root.unmount();
    });
  });
});
