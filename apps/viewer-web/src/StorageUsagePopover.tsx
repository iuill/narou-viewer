import { type ReactNode, useCallback, useEffect, useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { fetchStorageUsage, fetchStorageUsageProgress } from "./features/storage/api";
import type { NovelStorageUsage, StorageUsageCategory, StorageUsageProgressResponse, StorageUsageResponse } from "./features/storage/types";
import { formatDate } from "./shared/date";

type Props = {
  selectedNovelId: string | null;
};

const BYTE_UNITS = ["B", "KB", "MB", "GB", "TB"];
const MAX_VISIBLE_NOVELS = 12;
const PROGRESS_POLL_INTERVAL_MS = 700;
const CATEGORY_GRADIENT_VARS: Record<StorageUsageCategory["id"], { start: string; mid: string; end: string }> = {
  novelData: {
    start: "var(--storage-usage-novel-highlight)",
    mid: "var(--storage-usage-novel-color)",
    end: "var(--storage-usage-novel-shade)"
  },
  cache: {
    start: "var(--storage-usage-cache-highlight)",
    mid: "var(--storage-usage-cache-color)",
    end: "var(--storage-usage-cache-shade)"
  },
  other: {
    start: "var(--storage-usage-other-highlight)",
    mid: "var(--storage-usage-other-color)",
    end: "var(--storage-usage-other-shade)"
  }
};

export function formatStorageBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return "0 B";
  }
  let value = bytes;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < BYTE_UNITS.length - 1) {
    value /= 1024;
    unitIndex++;
  }
  const fractionDigits = value >= 100 || unitIndex === 0 ? 0 : value >= 10 ? 1 : 2;
  return `${value.toFixed(fractionDigits)} ${BYTE_UNITS[unitIndex]}`;
}

function formatPercent(part: number, total: number): string {
  if (!Number.isFinite(part) || !Number.isFinite(total) || part <= 0 || total <= 0) {
    return "0%";
  }
  const percent = (part / total) * 100;
  if (percent < 0.1) {
    return "<0.1%";
  }
  return `${percent >= 10 ? percent.toFixed(0) : percent.toFixed(1)}%`;
}

function percentWidth(part: number, total: number): string {
  if (!Number.isFinite(part) || !Number.isFinite(total) || part <= 0 || total <= 0) {
    return "0%";
  }
  return `${Math.min(100, Math.max(0.5, (part / total) * 100))}%`;
}

function categoryTone(category: StorageUsageCategory["id"]): string {
  switch (category) {
    case "novelData":
      return "novel";
    case "cache":
      return "cache";
    default:
      return "other";
  }
}

function categoryGradient(category: StorageUsageCategory["id"]): { start: string; mid: string; end: string } {
  return CATEGORY_GRADIENT_VARS[category] ?? CATEGORY_GRADIENT_VARS.other;
}

function createStorageUsageRequestId(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `storage-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}

const DONUT_TRACK_COLOR = "var(--storage-usage-track-color)";
const DONUT_SIZE = 118;
const DONUT_STROKE = 16;
const DONUT_EDGE_PADDING = 2;
const DONUT_CENTER = DONUT_SIZE / 2;
const DONUT_RADIUS = (DONUT_SIZE - DONUT_STROKE) / 2 - DONUT_EDGE_PADDING;
const DONUT_CIRCUMFERENCE = 2 * Math.PI * DONUT_RADIUS;
const DONUT_SEGMENT_GAP_PX = 3;

type DonutArc = {
  id: string;
  color: string;
  length: number;
  offset: number;
};

function categoryDonutArcs(categories: StorageUsageCategory[], totalBytes: number): DonutArc[] {
  if (!Number.isFinite(totalBytes) || totalBytes <= 0) {
    return [];
  }
  const positive = categories.filter((category) => category.bytes > 0);
  const gap = positive.length > 1 ? DONUT_SEGMENT_GAP_PX : 0;
  let cursor = 0;
  return positive.map((category) => {
    const rawLength = (category.bytes / totalBytes) * DONUT_CIRCUMFERENCE;
    const length = Math.max(0, rawLength - gap);
    const offset = cursor;
    cursor += rawLength;
    return { id: category.id, color: `url(#storage-usage-donut-${category.id})`, length, offset };
  });
}

function selectedNovelLabel(novel: NovelStorageUsage | null): string {
  if (!novel) {
    return "作品を選択すると、その作品の内訳をここに表示します。";
  }
  return `${novel.title} / ${formatStorageBytes(novel.totalBytes)}`;
}

function visibleNovelEntries(novels: NovelStorageUsage[], selectedNovel: NovelStorageUsage | null): NovelStorageUsage[] {
  const topNovels = novels.slice(0, MAX_VISIBLE_NOVELS);
  if (!selectedNovel || topNovels.some((novel) => novel.novelId === selectedNovel.novelId)) {
    return topNovels;
  }
  return [selectedNovel, ...topNovels.slice(0, MAX_VISIBLE_NOVELS - 1)];
}

export function StorageUsagePopover({ selectedNovelId }: Props): ReactNode {
  const [isOpen, setIsOpen] = useState(false);
  const [usage, setUsage] = useState<StorageUsageResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [hasLoaded, setHasLoaded] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [progress, setProgress] = useState<StorageUsageProgressResponse | null>(null);
  const [activeRequestId, setActiveRequestId] = useState<string | null>(null);

  const selectedNovel = useMemo(
    () => usage?.novels.find((novel) => novel.novelId === selectedNovelId) ?? null,
    [selectedNovelId, usage]
  );
  const visibleNovels = useMemo(
    () => visibleNovelEntries(usage?.novels ?? [], selectedNovel),
    [selectedNovel, usage]
  );

  const loadStorageUsage = useCallback(async () => {
    const requestId = createStorageUsageRequestId();
    setIsLoading(true);
    setHasLoaded(true);
    setError(null);
    setProgress(null);
    setActiveRequestId(requestId);
    try {
      setUsage(await fetchStorageUsage(requestId));
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : "ストレージ使用量の取得に失敗しました。");
    } finally {
      setIsLoading(false);
      setActiveRequestId((current) => (current === requestId ? null : current));
    }
  }, []);

  useEffect(() => {
    if (isOpen && !hasLoaded && !isLoading) {
      void loadStorageUsage();
    }
  }, [hasLoaded, isOpen, isLoading, loadStorageUsage]);

  useEffect(() => {
    if (!isOpen && !usage && error) {
      setHasLoaded(false);
    }
  }, [error, isOpen, usage]);

  useEffect(() => {
    if (!isOpen || typeof document === "undefined") {
      return;
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setIsOpen(false);
      }
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [isOpen]);

  useEffect(() => {
    if (!isOpen || !isLoading || !activeRequestId) {
      return;
    }
    let isDisposed = false;
    const refreshProgress = async () => {
      try {
        const nextProgress = await fetchStorageUsageProgress(activeRequestId);
        if (!isDisposed) {
          setProgress(nextProgress);
        }
      } catch {
        // Progress is best-effort. The main storage request still owns the user-visible error.
      }
    };
    void refreshProgress();
    const timer = window.setInterval(() => void refreshProgress(), PROGRESS_POLL_INTERVAL_MS);
    return () => {
      isDisposed = true;
      window.clearInterval(timer);
    };
  }, [activeRequestId, isLoading, isOpen]);

  const popover =
    isOpen && typeof document !== "undefined"
      ? createPortal(
          <div className="storage-usage-overlay">
            <section aria-label="ストレージ使用量" aria-modal="true" className="storage-usage-popover" role="dialog">
              <header className="storage-usage-header">
                <div>
                  <strong>ストレージ使用量</strong>
                  <span>{usage ? `${formatStorageBytes(usage.totalBytes)} / ${formatDate(usage.checkedAt)}` : "確認中"}</span>
                </div>
                <div className="storage-usage-actions">
                  <button disabled={isLoading} onClick={() => void loadStorageUsage()} type="button">
                    {isLoading ? "更新中" : "更新"}
                  </button>
                  <button aria-label="ストレージ使用量を閉じる" onClick={() => setIsOpen(false)} type="button">
                    ×
                  </button>
                </div>
              </header>
              {error ? <p className="message">{error}</p> : null}
              {isLoading ? <StorageUsageLoadingProgress progress={progress} /> : null}
              {usage ? (
                <>
                  <section className="storage-usage-overview">
                    <div aria-label={`全体 ${formatStorageBytes(usage.totalBytes)}`} className="storage-usage-donut" role="img">
                      <svg aria-hidden="true" viewBox={`0 0 ${DONUT_SIZE} ${DONUT_SIZE}`}>
                        <defs>
                          {usage.categories.map((category) => (
                            <radialGradient id={`storage-usage-donut-${category.id}`} key={category.id} cx="36%" cy="28%" fx="24%" fy="16%" r="82%">
                              <stop offset="0%" stopColor={categoryGradient(category.id).start} />
                              <stop offset="56%" stopColor={categoryGradient(category.id).mid} />
                              <stop offset="100%" stopColor={categoryGradient(category.id).end} />
                            </radialGradient>
                          ))}
                        </defs>
                        <circle cx={DONUT_CENTER} cy={DONUT_CENTER} fill="none" r={DONUT_RADIUS} stroke={DONUT_TRACK_COLOR} strokeWidth={DONUT_STROKE} />
                        <g transform={`rotate(-90 ${DONUT_CENTER} ${DONUT_CENTER})`}>
                          {categoryDonutArcs(usage.categories, usage.totalBytes).map((arc) => (
                            <circle
                              cx={DONUT_CENTER}
                              cy={DONUT_CENTER}
                              className="storage-usage-donut-arc"
                              fill="none"
                              key={arc.id}
                              r={DONUT_RADIUS}
                              stroke={arc.color}
                              strokeDasharray={`${arc.length} ${DONUT_CIRCUMFERENCE - arc.length}`}
                              strokeDashoffset={-arc.offset}
                              strokeWidth={DONUT_STROKE}
                            />
                          ))}
                        </g>
                      </svg>
                      <span>
                        <strong>全体</strong>
                        {formatStorageBytes(usage.totalBytes)}
                      </span>
                    </div>
                    <div className="storage-usage-overview-copy">
                      <strong>全体内訳</strong>
                      <span>円はカテゴリ比率、下の棒は詳細なサイズとファイル数です。</span>
                    </div>
                  </section>
                  <div className="storage-usage-category-list">
                    {usage.categories.map((category) => (
                      <article className={`storage-usage-category ${categoryTone(category.id)}`} key={category.id}>
                        <div>
                          <strong>
                            <i aria-hidden="true" className={`storage-usage-swatch ${categoryTone(category.id)}`} />
                            {category.label}
                          </strong>
                          <span>
                            {formatStorageBytes(category.bytes)} / {formatPercent(category.bytes, usage.totalBytes)}
                          </span>
                        </div>
                        <div aria-hidden="true" className="storage-usage-bar">
                          <span style={{ width: percentWidth(category.bytes, usage.totalBytes) }} />
                        </div>
                        <small>{category.fileCount} files</small>
                      </article>
                    ))}
                  </div>
                  <section className="storage-usage-selected">
                    <div className="storage-usage-section-heading">
                      <strong>選択作品</strong>
                      <span>{selectedNovelLabel(selectedNovel)}</span>
                    </div>
                    {selectedNovel ? <NovelStorageBreakdown novel={selectedNovel} totalBytes={selectedNovel.totalBytes} /> : null}
                  </section>
                  <section className="storage-usage-novel-list">
                    <div className="storage-usage-section-heading">
                      <strong>作品別</strong>
                      <span>
                        {visibleNovels.length} / {usage.novels.length} 作品
                      </span>
                    </div>
                    {visibleNovels.length > 0 ? (
                      visibleNovels.map((novel) => (
                        <NovelStorageBreakdown key={novel.novelId} novel={novel} totalBytes={usage.totalBytes} />
                      ))
                    ) : (
                      <p className="message">作品単位で表示できるデータはありません。</p>
                    )}
                  </section>
                  {usage.warnings && usage.warnings.length > 0 ? (
                    <p className="storage-usage-warning">一部のファイルは読み取り時に警告がありました。</p>
                  ) : null}
                </>
              ) : (
                isLoading ? null : <p className="message">ストレージ使用量は未取得です。</p>
              )}
            </section>
          </div>,
          document.body
        )
      : null;

  return (
    <div className="storage-usage-panel">
      <button
        aria-expanded={isOpen}
        aria-haspopup="dialog"
        className="library-export-button storage-usage-trigger"
        onClick={() => setIsOpen((current) => !current)}
        type="button"
      >
        ストレージ
      </button>
      {popover}
    </div>
  );
}

function StorageUsageLoadingProgress({ progress }: { progress: StorageUsageProgressResponse | null }): ReactNode {
  const checkedNovels = Math.max(0, progress?.checkedNovels ?? 0);
  const totalNovels = Math.max(0, progress?.totalNovels ?? 0);
  const hasKnownTotal = totalNovels > 0;
  const width = hasKnownTotal ? `${Math.min(96, Math.max(4, (checkedNovels / totalNovels) * 100))}%` : undefined;
  const detail = hasKnownTotal
    ? `目安 ${Math.min(checkedNovels, totalNovels)} / ${totalNovels} 作品`
    : checkedNovels > 0
      ? `目安 ${checkedNovels} 作品を確認済み`
      : !progress || progress.phase === "preparing"
        ? "作品一覧を確認中"
        : "小説データを走査中";

  return (
    <aside aria-live="polite" className="storage-usage-progress">
      <div>
        <strong>確認中</strong>
        <span>{detail}</span>
      </div>
      <div aria-hidden="true" className={`storage-usage-progress-track ${hasKnownTotal ? "" : "is-indeterminate"}`}>
        <span style={width ? { width } : undefined} />
      </div>
    </aside>
  );
}

function NovelStorageBreakdown({ novel, totalBytes }: { novel: NovelStorageUsage; totalBytes: number }): ReactNode {
  const denominator = totalBytes > 0 ? totalBytes : novel.totalBytes;
  return (
    <article className="storage-usage-novel">
      <div className="storage-usage-novel-heading">
        <div>
          <strong>{novel.title}</strong>
          <span>
            {novel.siteName}
            {novel.source === "legacy" ? " / legacy" : ""}
          </span>
        </div>
        <span>
          {formatStorageBytes(novel.totalBytes)} / {formatPercent(novel.totalBytes, denominator)}
        </span>
      </div>
      <div aria-hidden="true" className="storage-usage-split-bar">
        {novel.novelDataBytes > 0 ? (
          <span className="novel" style={{ width: percentWidth(novel.novelDataBytes, denominator) }} />
        ) : null}
        {novel.cacheBytes > 0 ? (
          <span className="cache" style={{ width: percentWidth(novel.cacheBytes, denominator) }} />
        ) : null}
        {novel.otherBytes > 0 ? (
          <span className="other" style={{ width: percentWidth(novel.otherBytes, denominator) }} />
        ) : null}
      </div>
      <div className="storage-usage-novel-metrics">
        <span>
          <i aria-hidden="true" className="storage-usage-swatch novel" />
          小説 {formatStorageBytes(novel.novelDataBytes)}
        </span>
        <span>
          <i aria-hidden="true" className="storage-usage-swatch cache" />
          キャッシュ {formatStorageBytes(novel.cacheBytes)}
        </span>
        {novel.otherBytes > 0 ? (
          <span>
            <i aria-hidden="true" className="storage-usage-swatch other" />
            その他 {formatStorageBytes(novel.otherBytes)}
          </span>
        ) : null}
      </div>
    </article>
  );
}
