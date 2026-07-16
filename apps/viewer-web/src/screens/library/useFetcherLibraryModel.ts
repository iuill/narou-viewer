import { type DragEvent, type FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  cancelFetcherTask,
  downloadFetcherWorks,
  pauseFetcherTask,
  removeFetcherWorks,
  resumeFetcherTask,
  resumeFetcherWorks,
  updateFetcherWorks
} from "../../features/fetcher/api";
import {
  getFetcherActiveTaskEntries,
  getFetcherBusyFetcherWorkIds,
  getFetcherQueuedTasks,
  getFetcherResumableTaskEntries,
  getFetcherTaskListEntries
} from "../../features/fetcher/model";
import { extractDroppedDownloadTarget } from "../../features/library/downloadTarget";
import {
  buildLibraryExportDocument,
  createLibraryExportFileName,
  downloadTextFile,
  serializeLibraryExportToYaml
} from "../../features/library/export";
import type { NovelSummary } from "../../features/library/types";
import type { RuntimeStatusService } from "../../features/runtime/types";
import { useFetcherStatus } from "../../hooks/useFetcherStatus";

type PendingFetcherActionTask = {
  novelId: string;
  taskIds: string[];
  seenInTasks: boolean;
};

type ReaderFetcherCommands = {
  clearSelection: (options: { clearNovel: boolean }) => void;
};

type ReaderFetcherSessionCommands = {
  forgetReaderStateCache: (novelId: string) => void;
};

type UseFetcherLibraryModelInput = {
  currentNovel: NovelSummary | null;
  fetcherRuntimeService: RuntimeStatusService | null;
  libraryReloadKey: number;
  novels: NovelSummary[];
  onError: (message: string | null) => void;
  readerCommands: ReaderFetcherCommands;
  readerSessionCommands: ReaderFetcherSessionCommands;
  requestLibraryReload: () => void;
  screenMode: "library" | "reader";
  setLibraryNotice: (message: string | null) => void;
};

const SYOSETU_NCODE_PATTERN = /n\d+[a-z]+/i;
const KAKUYOMU_WORK_PATH_PATTERN = /^\/works\/(\d+)(?:\/.*)?$/;

function canResumeLibraryNovel(novel: NovelSummary): boolean {
  const hasIncompleteFetchStatus = Boolean(novel.fetchStatus && novel.fetchStatus !== "complete");
  const hasUnsavedEpisodes = typeof novel.savedEpisodes === "number" && novel.savedEpisodes < novel.totalEpisodes;

  return Boolean(novel.fetcherWorkId && (novel.resumeEpisodeId || hasIncompleteFetchStatus || hasUnsavedEpisodes));
}

function normalizeDownloadTargetForComparison(value: string | null | undefined): string {
  const trimmed = value?.trim();
  if (!trimmed) {
    return "";
  }

  try {
    const url = new URL(trimmed);
    url.hash = "";
    url.search = "";
    const syosetuNcode =
      url.hostname.toLowerCase() === "ncode.syosetu.com" ? url.pathname.match(SYOSETU_NCODE_PATTERN)?.[0] : null;
    if (syosetuNcode) {
      return `syosetu:${syosetuNcode.toLowerCase()}`;
    }

    const kakuyomuWorkId =
      url.hostname.toLowerCase() === "kakuyomu.jp" ? url.pathname.match(KAKUYOMU_WORK_PATH_PATTERN)?.[1] : null;
    if (kakuyomuWorkId) {
      return `kakuyomu:${kakuyomuWorkId}`;
    }

    url.protocol = "https:";
    return url.toString().replace(/\/+$/, "").toLowerCase();
  } catch {
    const syosetuNcode = trimmed.match(SYOSETU_NCODE_PATTERN)?.[0];
    return syosetuNcode ? `syosetu:${syosetuNcode.toLowerCase()}` : trimmed.replace(/\/+$/, "").toLowerCase();
  }
}

function findNovelIdByDownloadTarget(target: string, novels: Array<{ novelId: string; tocUrl: string | null }>): string | null {
  const targetKey = normalizeDownloadTargetForComparison(target);
  if (!targetKey) {
    return null;
  }

  return novels.find((novel) => normalizeDownloadTargetForComparison(novel.tocUrl) === targetKey)?.novelId ?? null;
}

function mergePendingFetcherActionTask(
  current: PendingFetcherActionTask[],
  novelId: string,
  taskIds: string[]
): PendingFetcherActionTask[] {
  const normalizedTaskIds = [...new Set(taskIds.map((taskId) => taskId.trim()).filter(Boolean))];
  if (normalizedTaskIds.length === 0) {
    return current;
  }

  const existingIndex = current.findIndex((entry) => entry.novelId === novelId);
  if (existingIndex === -1) {
    return [...current, { novelId, taskIds: normalizedTaskIds, seenInTasks: false }];
  }

  const next = [...current];
  const existing = current[existingIndex];
  next[existingIndex] = {
    ...existing,
    taskIds: [...new Set([...existing.taskIds, ...normalizedTaskIds])],
    seenInTasks: false
  };
  return next;
}

export function useFetcherLibraryModel({
  currentNovel,
  fetcherRuntimeService,
  libraryReloadKey,
  novels,
  onError,
  readerCommands,
  readerSessionCommands,
  requestLibraryReload,
  screenMode,
  setLibraryNotice
}: UseFetcherLibraryModelInput) {
  const [cancelingFetcherTaskIds, setCancelingFetcherTaskIds] = useState<Set<string>>(() => new Set());
  const [controllingFetcherTaskIds, setControllingFetcherTaskIds] = useState<Set<string>>(() => new Set());
  const [downloadingNovelIds, setDownloadingNovelIds] = useState<Set<string>>(() => new Set());
  const [lastReliableFetcherBusyNovelIds, setLastReliableFetcherBusyNovelIds] = useState<Set<string>>(() => new Set());
  const [pendingFetcherActionTasks, setPendingFetcherActionTasks] = useState<PendingFetcherActionTask[]>([]);
  const [resumingNovelIds, setResumingNovelIds] = useState<Set<string>>(() => new Set());
  const [updatingNovelIds, setUpdatingNovelIds] = useState<Set<string>>(() => new Set());
  const [downloadTarget, setDownloadTarget] = useState("");
  const [downloadForce, setDownloadForce] = useState(false);
  const [isDownloadSubmitting, setIsDownloadSubmitting] = useState(false);
  const [isLibraryExporting, setIsLibraryExporting] = useState(false);
  const [isDownloadComposerOpen, setIsDownloadComposerOpen] = useState(false);
  const [isDownloadDropActive, setIsDownloadDropActive] = useState(false);
  const [isNovelActionSubmitting, setIsNovelActionSubmitting] = useState(false);
  const notifiedFetcherWarningKeysRef = useRef(new Set<string>());

  const handleFetcherQueueSettled = useCallback(() => {
    requestLibraryReload();
  }, [requestLibraryReload]);
  const {
    queue: fetcherQueue,
    tasks: fetcherTasks,
    checkedAt: fetcherStatusCheckedAt,
    isLoading: isFetcherStatusLoading,
    error: fetcherStatusError
  } = useFetcherStatus({
    isPaused: screenMode === "reader",
    refreshKey: libraryReloadKey,
    onQueueSettled: handleFetcherQueueSettled
  });

  async function handleDownloadSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const trimmedTarget = downloadTarget.trim();
    if (trimmedTarget.length === 0) {
      onError("Nコードまたは作品URLを入力してください。");
      return;
    }

    const matchedDownloadNovelId = findNovelIdByDownloadTarget(trimmedTarget, novels);
    if (matchedDownloadNovelId) {
      setDownloadingNovelIds((current) => new Set(current).add(matchedDownloadNovelId));
    }
    setIsDownloadSubmitting(true);
    onError(null);
    setLibraryNotice(null);

    try {
      const result = await downloadFetcherWorks({
        targets: [trimmedTarget],
        force: downloadForce,
        convertAfterDownload: false,
        mail: false
      });

      const firstTaskId = result.taskIds[0];
      if (matchedDownloadNovelId) {
        setPendingFetcherActionTasks((current) =>
          mergePendingFetcherActionTask(current, matchedDownloadNovelId, result.taskIds)
        );
      }
      setDownloadTarget("");
      setIsDownloadComposerOpen(false);
      setIsDownloadDropActive(false);
      setLibraryNotice(firstTaskId ? `ダウンロードを開始しました。taskId: ${firstTaskId}` : result.message);
      requestLibraryReload();
    } catch (downloadError) {
      onError(downloadError instanceof Error ? downloadError.message : "Unknown error");
    } finally {
      if (matchedDownloadNovelId) {
        setDownloadingNovelIds((current) => {
          const next = new Set(current);
          next.delete(matchedDownloadNovelId);
          return next;
        });
      }
      setIsDownloadSubmitting(false);
    }
  }

  function handleDownloadDrop(event: DragEvent<HTMLDivElement>) {
    event.preventDefault();
    setIsDownloadDropActive(false);
    const droppedTarget = extractDroppedDownloadTarget(event.dataTransfer);
    if (!droppedTarget) {
      onError("ドロップされたデータから URL を読み取れませんでした。");
      return;
    }

    onError(null);
    setIsDownloadComposerOpen(true);
    setDownloadTarget(droppedTarget);
  }

  async function handleUpdateNovel(novelId: string) {
    if (fetcherActionBusyNovelIds.has(novelId)) {
      return;
    }

    setUpdatingNovelIds((current) => new Set(current).add(novelId));
    setIsNovelActionSubmitting(true);
    onError(null);
    setLibraryNotice(null);

    try {
      const result = await updateFetcherWorks({
        novelIds: [novelId],
        forceRedownload: false,
        includeFrozen: false,
        convertAfterUpdate: false,
        skipUnchanged: true
      });

      const firstTaskId = result.taskIds[0];
      setPendingFetcherActionTasks((current) => mergePendingFetcherActionTask(current, novelId, result.taskIds));
      setLibraryNotice(firstTaskId ? `更新を開始しました。taskId: ${firstTaskId}` : result.message);
      requestLibraryReload();
    } catch (updateError) {
      onError(updateError instanceof Error ? updateError.message : "Unknown error");
    } finally {
      setUpdatingNovelIds((current) => {
        const next = new Set(current);
        next.delete(novelId);
        return next;
      });
      setIsNovelActionSubmitting(false);
    }
  }

  async function handleUpdateCurrentNovel() {
    if (!currentNovel) {
      return;
    }

    await handleUpdateNovel(currentNovel.novelId);
  }

  async function handleResumeNovel(novelId: string) {
    if (fetcherActionBusyNovelIds.has(novelId)) {
      return;
    }

    setResumingNovelIds((current) => new Set(current).add(novelId));
    onError(null);
    setLibraryNotice(null);

    try {
      const result = await resumeFetcherWorks({
        novelIds: [novelId]
      });

      const firstTaskId = result.taskIds[0];
      setPendingFetcherActionTasks((current) => mergePendingFetcherActionTask(current, novelId, result.taskIds));
      setLibraryNotice(firstTaskId ? `ダウンロード再開を開始しました。taskId: ${firstTaskId}` : result.message);
      requestLibraryReload();
    } catch (resumeError) {
      onError(resumeError instanceof Error ? resumeError.message : "Unknown error");
    } finally {
      setResumingNovelIds((current) => {
        const next = new Set(current);
        next.delete(novelId);
        return next;
      });
    }
  }

  async function handleExportLibrary() {
    if (isLibraryExporting || novels.length === 0) {
      return;
    }

    setIsLibraryExporting(true);
    onError(null);
    setLibraryNotice(null);

    try {
      const exportedAt = new Date().toISOString();
      const exportDocument = await buildLibraryExportDocument(novels, exportedAt);
      const fileName = createLibraryExportFileName(exportedAt);
      downloadTextFile(serializeLibraryExportToYaml(exportDocument), fileName, "application/x-yaml;charset=utf-8");
      setLibraryNotice(`${fileName} を保存しました。`);
    } catch (exportError) {
      onError(exportError instanceof Error ? exportError.message : "Unknown error");
    } finally {
      setIsLibraryExporting(false);
    }
  }

  async function handleRemoveCurrentNovel() {
    if (!currentNovel) {
      return;
    }

    const confirmed = window.confirm(`「${currentNovel.title}」を削除します。既存データも削除されます。`);
    if (!confirmed) {
      return;
    }

    setIsNovelActionSubmitting(true);
    onError(null);
    setLibraryNotice(null);

    try {
      const result = await removeFetcherWorks({
        novelIds: [currentNovel.novelId],
        withFiles: true
      });

      readerSessionCommands.forgetReaderStateCache(currentNovel.novelId);
      readerCommands.clearSelection({ clearNovel: true });
      setLibraryNotice(result.message);
      requestLibraryReload();
    } catch (removeError) {
      onError(removeError instanceof Error ? removeError.message : "Unknown error");
    } finally {
      setIsNovelActionSubmitting(false);
    }
  }

  async function handleCancelFetcherTask(taskId: string) {
    if (controllingFetcherTaskIds.has(taskId)) {
      return;
    }

    setCancelingFetcherTaskIds((current) => new Set(current).add(taskId));
    setControllingFetcherTaskIds((current) => new Set(current).add(taskId));
    onError(null);

    try {
      const payload = await cancelFetcherTask(taskId);

      setLibraryNotice(payload.message ?? "タスクを中止しました。");
      requestLibraryReload();
    } catch (cancelError) {
      onError(cancelError instanceof Error ? cancelError.message : "Unknown error");
    } finally {
      setCancelingFetcherTaskIds((current) => {
        const next = new Set(current);
        next.delete(taskId);
        return next;
      });
      setControllingFetcherTaskIds((current) => {
        const next = new Set(current);
        next.delete(taskId);
        return next;
      });
    }
  }

  async function handlePauseFetcherTask(taskId: string) {
    if (controllingFetcherTaskIds.has(taskId)) {
      return;
    }

    setControllingFetcherTaskIds((current) => new Set(current).add(taskId));
    onError(null);

    try {
      const payload = await pauseFetcherTask(taskId);
      setLibraryNotice(payload.message ?? "タスクの一時停止を受け付けました。");
      requestLibraryReload();
    } catch (pauseError) {
      onError(pauseError instanceof Error ? pauseError.message : "Unknown error");
    } finally {
      setControllingFetcherTaskIds((current) => {
        const next = new Set(current);
        next.delete(taskId);
        return next;
      });
    }
  }

  async function handleResumeFetcherTask(taskId: string) {
    if (controllingFetcherTaskIds.has(taskId)) {
      return;
    }

    setControllingFetcherTaskIds((current) => new Set(current).add(taskId));
    onError(null);

    try {
      const payload = await resumeFetcherTask(taskId);
      setLibraryNotice(payload.message ?? "タスクを再開しました。");
      requestLibraryReload();
    } catch (resumeError) {
      onError(resumeError instanceof Error ? resumeError.message : "Unknown error");
    } finally {
      setControllingFetcherTaskIds((current) => {
        const next = new Set(current);
        next.delete(taskId);
        return next;
      });
    }
  }

  const hasFetcherStatus = fetcherQueue !== null || fetcherTasks !== null;
  const queueStatusLabel = isFetcherStatusLoading
    ? "確認中"
    : fetcherQueue?.degraded || fetcherTasks?.degraded
      ? "未接続"
      : !hasFetcherStatus && fetcherStatusError
        ? "取得失敗"
        : fetcherQueue?.running
          ? "稼働中"
          : "待機中";
  const activeFetcherTaskEntries = useMemo(() => getFetcherActiveTaskEntries(fetcherTasks), [fetcherTasks]);
  const resumableFetcherTaskEntries = useMemo(() => getFetcherResumableTaskEntries(fetcherTasks), [fetcherTasks]);
  const fetcherTaskEntries = useMemo(
    () => [...activeFetcherTaskEntries, ...resumableFetcherTaskEntries],
    [activeFetcherTaskEntries, resumableFetcherTaskEntries]
  );
  const activeFetcherTasks = useMemo(() => activeFetcherTaskEntries.map((entry) => entry.task), [activeFetcherTaskEntries]);
  const activeFetcherTaskIds = useMemo(
    () => new Set(activeFetcherTaskEntries.map((entry) => entry.task.id).filter(Boolean)),
    [activeFetcherTaskEntries]
  );
  const knownFetcherTaskIds = useMemo(() => {
    const taskIds = new Set<string>();
    for (const entry of activeFetcherTaskEntries) {
      if (entry.task.id) {
        taskIds.add(entry.task.id);
      }
    }
    for (const task of fetcherTasks?.recentCompleted ?? []) {
      if (task.id) {
        taskIds.add(task.id);
      }
    }
    for (const task of fetcherTasks?.recentFailed ?? []) {
      if (task.id) {
        taskIds.add(task.id);
      }
    }
    for (const task of fetcherTasks?.paused ?? []) {
      if (task.id) {
        taskIds.add(task.id);
      }
    }
    for (const task of fetcherTasks?.interrupted ?? []) {
      if (task.id) {
        taskIds.add(task.id);
      }
    }
    return taskIds;
  }, [activeFetcherTaskEntries, fetcherTasks]);
  const fetcherWorkIdToNovelId = useMemo(
    () => new Map(novels.map((novel) => [novel.fetcherWorkId, novel.novelId] as const)),
    [novels]
  );
  const fetcherBusyFetcherWorkIds = useMemo(() => getFetcherBusyFetcherWorkIds(fetcherTasks), [fetcherTasks]);
  const fetcherBusyNovelIds = useMemo(() => {
    const busyNovelIds = new Set<string>();

    for (const fetcherWorkId of fetcherBusyFetcherWorkIds) {
      const novelId = fetcherWorkIdToNovelId.get(fetcherWorkId);
      if (novelId) {
        busyNovelIds.add(novelId);
      }
    }

    return busyNovelIds;
  }, [fetcherBusyFetcherWorkIds, fetcherWorkIdToNovelId]);
  const isFetcherTasksSnapshotReliable =
    fetcherTasks !== null && fetcherTasks.available !== false && fetcherTasks.degraded !== true;

  useEffect(() => {
    if (!isFetcherTasksSnapshotReliable) {
      return;
    }

    const warningEntries = [
      ...activeFetcherTasks,
      ...(fetcherTasks?.recentCompleted ?? []),
      ...(fetcherTasks?.recentFailed ?? [])
    ].flatMap((task) =>
      task.warnings.map((warning) => ({
        key: `${task.id}:${warning}`,
        warning
      }))
    );
    const nextWarning = warningEntries.find((entry) => !notifiedFetcherWarningKeysRef.current.has(entry.key));
    if (!nextWarning) {
      return;
    }

    notifiedFetcherWarningKeysRef.current.add(nextWarning.key);
    setLibraryNotice(nextWarning.warning);
  }, [activeFetcherTasks, fetcherTasks, isFetcherTasksSnapshotReliable, setLibraryNotice]);

  useEffect(() => {
    if (isFetcherTasksSnapshotReliable) {
      setLastReliableFetcherBusyNovelIds(fetcherBusyNovelIds);
    }
  }, [fetcherBusyNovelIds, isFetcherTasksSnapshotReliable]);

  const effectiveFetcherBusyNovelIds = isFetcherTasksSnapshotReliable
    ? fetcherBusyNovelIds
    : lastReliableFetcherBusyNovelIds;
  const pendingFetcherActionNovelIds = useMemo(
    () => new Set(pendingFetcherActionTasks.map((entry) => entry.novelId)),
    [pendingFetcherActionTasks]
  );
  const fetcherActionBusyNovelIds = useMemo(
    () =>
      new Set([
        ...effectiveFetcherBusyNovelIds,
        ...downloadingNovelIds,
        ...pendingFetcherActionNovelIds,
        ...updatingNovelIds,
        ...resumingNovelIds
      ]),
    [downloadingNovelIds, effectiveFetcherBusyNovelIds, pendingFetcherActionNovelIds, resumingNovelIds, updatingNovelIds]
  );
  const hasActiveFetcherTasks = activeFetcherTasks.length > 0;
  const hasFetcherTaskActivity = hasActiveFetcherTasks || resumableFetcherTaskEntries.length > 0;
  const currentFetcherTask = fetcherTasks?.current ?? fetcherTasks?.convertCurrent ?? null;
  const queuedTaskPreviewEntries = useMemo(
    () => getFetcherTaskListEntries(getFetcherQueuedTasks(fetcherTasks).slice(0, 3)),
    [fetcherTasks]
  );
  const recentFailedFetcherTaskPreviewEntries = useMemo(
    () => getFetcherTaskListEntries(fetcherTasks?.recentFailed.slice(0, 2) ?? []),
    [fetcherTasks]
  );
  const pausedFetcherTaskPreviewEntries = useMemo(
    () => getFetcherTaskListEntries(fetcherTasks?.paused.slice(0, 3) ?? []),
    [fetcherTasks]
  );
  const interruptedFetcherTaskPreviewEntries = useMemo(
    () => getFetcherTaskListEntries(fetcherTasks?.interrupted.slice(0, 3) ?? []),
    [fetcherTasks]
  );
  const resumableNovels = useMemo(() => novels.filter((novel) => canResumeLibraryNovel(novel)), [novels]);
  const updatableNovels = useMemo(() => novels.filter((novel) => Boolean(novel.fetcherWorkId)), [novels]);
  const fetcherUpdateNotice =
    fetcherRuntimeService?.versionInfo?.updateAvailable &&
    fetcherRuntimeService.versionInfo.current &&
    fetcherRuntimeService.versionInfo.latest
      ? `novel-fetcher ${fetcherRuntimeService.versionInfo.latest} が利用できます。現在は ${fetcherRuntimeService.versionInfo.current} を使用中です。`
      : null;

  useEffect(() => {
    if (!isFetcherTasksSnapshotReliable) {
      return;
    }

    setPendingFetcherActionTasks((current) => {
      let changed = false;
      const next: PendingFetcherActionTask[] = [];

      for (const entry of current) {
        const isActive = entry.taskIds.some((taskId) => activeFetcherTaskIds.has(taskId));
        if (isActive) {
          if (entry.seenInTasks) {
            next.push(entry);
          } else {
            next.push({ ...entry, seenInTasks: true });
            changed = true;
          }
          continue;
        }

        if (!entry.seenInTasks) {
          if (entry.taskIds.some((taskId) => knownFetcherTaskIds.has(taskId))) {
            changed = true;
            continue;
          }
          next.push(entry);
          continue;
        }

        changed = true;
      }

      return changed ? next : current;
    });
  }, [activeFetcherTaskIds, isFetcherTasksSnapshotReliable, knownFetcherTaskIds]);

  return {
    activeFetcherTaskEntries,
    activeFetcherTasks,
    cancelingFetcherTaskIds,
    controllingFetcherTaskIds,
    currentFetcherTask,
    downloadForce,
    downloadTarget,
    fetcherActionBusyNovelIds,
    fetcherQueue,
    fetcherStatusCheckedAt,
    fetcherStatusError,
    fetcherTaskEntries,
    fetcherTasks,
    fetcherUpdateNotice,
    handleCancelFetcherTask,
    handlePauseFetcherTask,
    handleResumeFetcherTask,
    handleDownloadDrop,
    handleDownloadSubmit,
    handleExportLibrary,
    handleRemoveCurrentNovel,
    handleResumeNovel,
    handleUpdateCurrentNovel,
    handleUpdateNovel,
    hasActiveFetcherTasks,
    hasFetcherTaskActivity,
    hasFetcherStatus,
    interruptedFetcherTaskPreviewEntries,
    isDownloadComposerOpen,
    isDownloadDropActive,
    isDownloadSubmitting,
    isLibraryExporting,
    isNovelActionSubmitting,
    queuedTaskPreviewEntries,
    queueStatusLabel,
    pausedFetcherTaskPreviewEntries,
    recentFailedFetcherTaskPreviewEntries,
    resumableNovels,
    setDownloadForce,
    setDownloadTarget,
    setIsDownloadComposerOpen,
    setIsDownloadDropActive,
    updatableNovels
  };
}
