import { act, createElement, type FormEvent, useEffect } from "react";
import { createRoot, type Root } from "react-dom/client";
import { JSDOM } from "jsdom";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { NovelSummary } from "../src/features/library/types";
import { useFetcherLibraryModel } from "../src/screens/library/useFetcherLibraryModel";

const mocks = vi.hoisted(() => ({
  cancelFetcherTask: vi.fn(),
  downloadFetcherWorks: vi.fn(),
  formatLibraryImportSummary: vi.fn(),
  importLibraryDocument: vi.fn(),
  parseLibraryImportYaml: vi.fn(),
  removeFetcherWorks: vi.fn(),
  resumeFetcherWorks: vi.fn(),
  updateFetcherWorks: vi.fn(),
  fetcherStatus: {
    checkedAt: "2026-01-01T00:00:00Z",
    error: null,
    isLoading: false,
    queue: { available: true, running: false, total: 0, webWorker: 0, worker: 0 },
    tasks: null
  }
}));

vi.mock("../src/features/fetcher/api", () => ({
  cancelFetcherTask: mocks.cancelFetcherTask,
  downloadFetcherWorks: mocks.downloadFetcherWorks,
  removeFetcherWorks: mocks.removeFetcherWorks,
  resumeFetcherWorks: mocks.resumeFetcherWorks,
  updateFetcherWorks: mocks.updateFetcherWorks
}));

vi.mock("../src/hooks/useFetcherStatus", () => ({
  useFetcherStatus: () => mocks.fetcherStatus
}));

vi.mock("../src/features/library/import", () => ({
  formatLibraryImportSummary: mocks.formatLibraryImportSummary,
  importLibraryDocument: mocks.importLibraryDocument,
  MAX_LIBRARY_IMPORT_BYTES: 1 << 20,
  parseLibraryImportYaml: mocks.parseLibraryImportYaml
}));

const novels: NovelSummary[] = [
  {
    author: "作者",
    bookmarkCount: 0,
    fetcherWorkId: "work-1",
    lastReadEpisodeIndex: null,
    lastReadEpisodeTitle: null,
    latestBookmarkEpisodeIndex: null,
    novelId: "novel-1",
    siteName: "小説家になろう",
    title: "作品",
    tocUrl: "https://ncode.syosetu.com/n1234ab/",
    totalEpisodes: 1,
    updatedAt: null
  }
];

type HookSnapshot = ReturnType<typeof useFetcherLibraryModel>;
const mountedRoots = new Set<Root>();

function installDom() {
  const dom = new JSDOM('<!doctype html><html><body><div id="root"></div></body></html>', {
    url: "http://localhost/"
  });
  vi.stubGlobal("window", dom.window);
  vi.stubGlobal("document", dom.window.document);
  vi.stubGlobal("navigator", dom.window.navigator);
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  return dom;
}

function HookHost({
  onSnapshot,
  overrides = {}
}: {
  onSnapshot: (snapshot: HookSnapshot) => void;
  overrides?: Partial<Parameters<typeof useFetcherLibraryModel>[0]>;
}) {
  const snapshot = useFetcherLibraryModel({
    currentNovel: novels[0],
    fetcherRuntimeService: null,
    libraryReloadKey: 0,
    novels,
    onError: vi.fn(),
    readerCommands: { clearSelection: vi.fn() },
    readerSessionCommands: { forgetReaderStateCache: vi.fn() },
    requestLibraryReload: vi.fn(),
    screenMode: "library",
    setLibraryNotice: vi.fn(),
    ...overrides
  });

  useEffect(() => {
    onSnapshot(snapshot);
  }, [onSnapshot, snapshot]);

  return null;
}

async function renderHookHost(
  overrides: Partial<Parameters<typeof useFetcherLibraryModel>[0]> = {}
): Promise<{ dom: JSDOM; root: Root; snapshots: HookSnapshot[] }> {
  const dom = installDom();
  const rootElement = dom.window.document.getElementById("root");
  if (!rootElement) {
    throw new Error("root element not found");
  }
  const snapshots: HookSnapshot[] = [];
  const root = createRoot(rootElement);
  mountedRoots.add(root);
  await act(async () => {
    root.render(createElement(HookHost, { onSnapshot: (snapshot) => snapshots.push(snapshot), overrides }));
  });
  return { dom, root, snapshots };
}

describe("useFetcherLibraryModel", () => {
  afterEach(async () => {
    await act(async () => {
      for (const root of mountedRoots) {
        root.unmount();
      }
      mountedRoots.clear();
      await Promise.resolve();
    });
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
    mocks.cancelFetcherTask.mockReset();
    mocks.downloadFetcherWorks.mockReset();
    mocks.formatLibraryImportSummary.mockReset();
    mocks.importLibraryDocument.mockReset();
    mocks.parseLibraryImportYaml.mockReset();
    mocks.removeFetcherWorks.mockReset();
    mocks.resumeFetcherWorks.mockReset();
    mocks.updateFetcherWorks.mockReset();
    mocks.fetcherStatus.tasks = null;
  });

  it("closes the download composer and marks the matched novel busy after starting a download", async () => {
    const requestLibraryReload = vi.fn();
    const setLibraryNotice = vi.fn();
    const onError = vi.fn();
    mocks.downloadFetcherWorks.mockResolvedValue({ message: "ok", taskIds: ["task-1"] });
    const { snapshots } = await renderHookHost({ onError, requestLibraryReload, setLibraryNotice });
    const latest = () => snapshots[snapshots.length - 1];

    await act(async () => {
      latest().setIsDownloadComposerOpen(true);
      latest().setIsDownloadDropActive(true);
      latest().setDownloadTarget("https://ncode.syosetu.com/n1234ab/");
    });

    await act(async () => {
      await latest().handleDownloadSubmit({ preventDefault: vi.fn() } as unknown as FormEvent<HTMLFormElement>);
    });

    expect(mocks.downloadFetcherWorks).toHaveBeenCalledWith({
      convertAfterDownload: false,
      force: false,
      mail: false,
      targets: ["https://ncode.syosetu.com/n1234ab/"]
    });
    expect(latest().isDownloadComposerOpen).toBe(false);
    expect(latest().isDownloadDropActive).toBe(false);
    expect(latest().downloadTarget).toBe("");
    expect(latest().fetcherActionBusyNovelIds.has("novel-1")).toBe(true);
    expect(setLibraryNotice).toHaveBeenCalledWith("ダウンロードを開始しました。taskId: task-1");
    expect(requestLibraryReload).toHaveBeenCalled();
    expect(onError).toHaveBeenCalledWith(null);

  });

  it("removes the current novel through fetcher and clears reader selection/cache", async () => {
    const requestLibraryReload = vi.fn();
    const setLibraryNotice = vi.fn();
    const clearSelection = vi.fn();
    const forgetReaderStateCache = vi.fn();
    mocks.removeFetcherWorks.mockResolvedValue({ message: "removed", novelIds: ["novel-1"] });
    const { snapshots } = await renderHookHost({
      readerCommands: { clearSelection },
      readerSessionCommands: { forgetReaderStateCache },
      requestLibraryReload,
      setLibraryNotice
    });
    Object.defineProperty(window, "confirm", {
      configurable: true,
      value: vi.fn(() => true)
    });

    await act(async () => {
      await snapshots[snapshots.length - 1].handleRemoveCurrentNovel();
    });

    expect(mocks.removeFetcherWorks).toHaveBeenCalledWith({ novelIds: ["novel-1"], withFiles: true });
    expect(forgetReaderStateCache).toHaveBeenCalledWith("novel-1");
    expect(clearSelection).toHaveBeenCalledWith({ clearNovel: true });
    expect(setLibraryNotice).toHaveBeenCalledWith("removed");
    expect(requestLibraryReload).toHaveBeenCalled();

  });

  it("previews and applies an imported library after confirmation", async () => {
    const requestLibraryReload = vi.fn();
    const setLibraryNotice = vi.fn();
    const clearSelection = vi.fn();
    const document = { formatVersion: 1 };
    const preview = { warnings: ["警告"], dryRun: true };
    const result = { warnings: [], dryRun: false };
    mocks.parseLibraryImportYaml.mockReturnValue(document);
    mocks.importLibraryDocument.mockResolvedValueOnce(preview).mockResolvedValueOnce(result);
    mocks.formatLibraryImportSummary.mockReturnValueOnce("確認結果").mockReturnValueOnce("適用結果");
    const { snapshots } = await renderHookHost({
      readerCommands: { clearSelection },
      requestLibraryReload,
      setLibraryNotice
    });
    Object.defineProperty(window, "confirm", {
      configurable: true,
      value: vi.fn(() => true)
    });

    await act(async () => {
      await snapshots[snapshots.length - 1].handleImportLibrary(new File(["formatVersion: 1"], "library.yaml"));
    });

    expect(mocks.parseLibraryImportYaml).toHaveBeenCalledWith("formatVersion: 1");
    expect(mocks.importLibraryDocument).toHaveBeenNthCalledWith(1, document, true);
    expect(mocks.importLibraryDocument).toHaveBeenNthCalledWith(2, document, false);
    expect(window.confirm).toHaveBeenCalledWith(expect.stringContaining("確認結果"));
    expect(clearSelection).toHaveBeenCalledWith({ clearNovel: false });
    expect(setLibraryNotice).toHaveBeenCalledWith("読み込みました。適用結果");
    expect(requestLibraryReload).toHaveBeenCalled();
    expect(snapshots[snapshots.length - 1].isLibraryImporting).toBe(false);
  });

  it("does not apply an imported library when confirmation is declined", async () => {
    const setLibraryNotice = vi.fn();
    mocks.parseLibraryImportYaml.mockReturnValue({ formatVersion: 1 });
    mocks.importLibraryDocument.mockResolvedValue({ warnings: [], dryRun: true });
    mocks.formatLibraryImportSummary.mockReturnValue("確認結果");
    const { snapshots } = await renderHookHost({ setLibraryNotice });
    Object.defineProperty(window, "confirm", {
      configurable: true,
      value: vi.fn(() => false)
    });

    await act(async () => {
      await snapshots[snapshots.length - 1].handleImportLibrary(new File(["formatVersion: 1"], "library.yaml"));
    });

    expect(mocks.importLibraryDocument).toHaveBeenCalledTimes(1);
    expect(setLibraryNotice).toHaveBeenCalledWith("読み込みを取り消しました。確認結果");
  });

  it("rejects an oversized import before reading the file", async () => {
    const onError = vi.fn();
    const { snapshots } = await renderHookHost({ onError });
    const file = new File(["small"], "library.yaml");
    Object.defineProperty(file, "size", { value: (1 << 20) + 1 });
    const text = vi.spyOn(file, "text");

    await act(async () => {
      await snapshots[snapshots.length - 1].handleImportLibrary(file);
    });

    expect(text).not.toHaveBeenCalled();
    expect(onError).toHaveBeenCalledWith("読み込みファイルは1MB以下にしてください。");
    expect(mocks.importLibraryDocument).not.toHaveBeenCalled();
  });

  it("reports failed resume, update, remove, cancel, and drop actions", async () => {
    const onError = vi.fn();
    const setLibraryNotice = vi.fn();
    mocks.resumeFetcherWorks.mockRejectedValue(new Error("resume failed"));
    mocks.updateFetcherWorks.mockRejectedValue(new Error("update failed"));
    mocks.removeFetcherWorks.mockRejectedValue(new Error("remove failed"));
    mocks.cancelFetcherTask.mockRejectedValue(new Error("cancel failed"));
    const { snapshots } = await renderHookHost({ onError, setLibraryNotice });
    Object.defineProperty(window, "confirm", {
      configurable: true,
      value: vi.fn(() => true)
    });
    const latest = () => snapshots[snapshots.length - 1];

    await act(async () => {
      await latest().handleResumeNovel("novel-1");
      await latest().handleUpdateNovel("novel-1");
      await latest().handleRemoveCurrentNovel();
      await latest().handleCancelFetcherTask("task-1");
    });

    expect(onError).toHaveBeenCalledWith("resume failed");
    expect(onError).toHaveBeenCalledWith("update failed");
    expect(onError).toHaveBeenCalledWith("remove failed");
    expect(onError).toHaveBeenCalledWith("cancel failed");
    expect(setLibraryNotice).toHaveBeenCalledWith(null);

    await act(async () => {
      latest().handleDownloadDrop({
        preventDefault: vi.fn(),
        dataTransfer: {
          getData: vi.fn(() => "")
        }
      } as never);
    });
    expect(onError).toHaveBeenCalledWith("ドロップされたデータから URL を読み取れませんでした。");

  });
});
