import { forwardRef } from "react";
import type { NovelSummary } from "./features/library/types";
import { ReaderFloatingPanel } from "./ReaderFloatingPanel";

type ReaderInfoPanelProps = {
  currentNovel: NovelSummary | null;
  episodeReferenceLabel: string;
  episodeTitle: string;
  pageLabel: string;
  sourceUrl: string | null;
  updatedAtLabel: string;
  onClose: () => void;
};

export const ReaderInfoPanel = forwardRef<HTMLElement, ReaderInfoPanelProps>(function ReaderInfoPanel(
  { currentNovel, episodeReferenceLabel, episodeTitle, pageLabel, sourceUrl, updatedAtLabel, onClose },
  ref
) {
  return (
    <ReaderFloatingPanel
      className="reader-info-panel reader-overlay-panel--info"
      description="作品と現在の本文に関する情報を表示します。"
      onClose={onClose}
      ref={ref}
      title="情報"
    >
      <section className="reader-panel-card reader-panel-card--hero reader-info-hero">
        <p className="reader-info-site">{currentNovel?.siteName ?? "作品情報未取得"}</p>
        <h3>{currentNovel?.title ?? "未取得"}</h3>
        <p className="reader-info-author">{currentNovel?.author || "著者未設定"}</p>
        <div className="reader-panel-chip-row">
          <span className="reader-panel-chip">全 {currentNovel?.totalEpisodes ?? "?"} 話</span>
          <span className="reader-panel-chip">栞 {currentNovel?.bookmarkCount ?? 0} 件</span>
          <span className="reader-panel-chip">最終更新 {updatedAtLabel}</span>
        </div>
      </section>
      <div className="reader-info-summary-grid">
        <section className="reader-panel-card reader-panel-card--compact reader-info-summary-card">
          <p className="reader-info-summary-label">現在の話</p>
          <strong>{episodeReferenceLabel}</strong>
          <p>{episodeTitle}</p>
        </section>
        <section className="reader-panel-card reader-panel-card--compact reader-info-summary-card">
          <p className="reader-info-summary-label">閲覧ページ</p>
          <strong>{pageLabel} ページ</strong>
        </section>
      </div>
      <section className="reader-panel-card reader-panel-card--compact reader-info-link-card">
        <p className="reader-info-link-label">元URL</p>
        {sourceUrl ? (
          <a className="reader-panel-link" href={sourceUrl} rel="noreferrer" target="_blank">
            {sourceUrl}
          </a>
        ) : (
          <p className="reader-info-link-empty">なし</p>
        )}
      </section>
    </ReaderFloatingPanel>
  );
});
