import type { Dispatch, RefObject, SetStateAction } from "react";
import type { AiGenerationPanelView } from "../../hooks/useAiGeneration";

export type AiGenerationMenuProps = {
  activeSettingsProfileUpdatedAt: string | null;
  activeJobsCount: number;
  failedJobsCount: number;
  isOpen: boolean;
  jobsError: string | null;
  onOpenView: (view: AiGenerationPanelView) => void | Promise<void>;
  panelRef: RefObject<HTMLDivElement | null>;
  runtimeErrorDetail: string | null;
  setIsOpen: Dispatch<SetStateAction<boolean>>;
  settingsError: string | null;
  summaryLabel: string;
  triggerStatus: string;
};

export function AiGenerationNavList({
  onOpenView
}: {
  onOpenView: (view: AiGenerationPanelView) => void | Promise<void>;
}) {
  return (
    <div className="ai-generation-nav-list">
      <button className="ai-generation-nav-button" onClick={() => void onOpenView("jobs")} type="button">
        <strong>キャラ生成履歴</strong>
        <span>キャラクター一覧生成の進行状況と失敗履歴を確認します。</span>
      </button>
      <button className="ai-generation-nav-button" onClick={() => void onOpenView("usage")} type="button">
        <strong>読書AI利用統計</strong>
        <span>読書AIのリクエスト数、トークン数、tool 呼び出しを確認します。</span>
      </button>
      <button className="ai-generation-nav-button" onClick={() => void onOpenView("settings")} type="button">
        <strong>設定</strong>
        <span>APIキー、モデル、provider order を保存します。</span>
      </button>
      <button className="ai-generation-nav-button" onClick={() => void onOpenView("playground")} type="button">
        <strong>生成テスト</strong>
        <span>作品と話数を選んでキャラクター一覧生成を試します。</span>
      </button>
    </div>
  );
}

export type AiGenerationMenuComponentProps = {
  aiGeneration: AiGenerationMenuProps;
  formatDate: (value: string | null) => string;
  onToggle: () => void;
};

export function AiGenerationMenu({ aiGeneration, formatDate, onToggle }: AiGenerationMenuComponentProps) {
  const {
    activeSettingsProfileUpdatedAt,
    activeJobsCount,
    failedJobsCount,
    isOpen,
    jobsError,
    onOpenView,
    panelRef,
    runtimeErrorDetail,
    settingsError,
    summaryLabel,
    triggerStatus
  } = aiGeneration;

  return (
    <div className="ai-generation-panel" ref={panelRef}>
      <button
        aria-expanded={isOpen}
        aria-haspopup="dialog"
        className={`status-trigger ai-generation-trigger ${triggerStatus}`}
        title={summaryLabel}
        onClick={onToggle}
        type="button"
      >
        <span aria-hidden="true" className="status-trigger-icon ai-generation-trigger-icon" />
        <span className="status-trigger-copy queue-trigger-copy">
          <strong>AI機能</strong>
        </span>
        {activeJobsCount > 0 ? (
          <span className="status-badge warn">{activeJobsCount}</span>
        ) : triggerStatus === "error" ? (
          <span className="status-badge error">要設定</span>
        ) : null}
      </button>
      {isOpen ? (
        <section aria-label="AI機能メニュー" className="queue-popover ai-generation-popover">
          <header className="queue-card-header">
            <div>
              <strong>AI機能</strong>
              <small>{summaryLabel}</small>
            </div>
            <span>{activeSettingsProfileUpdatedAt ? formatDate(activeSettingsProfileUpdatedAt) : "未設定"}</span>
          </header>
          <div className="queue-card-metrics">
            <span className={`queue-chip ${activeJobsCount > 0 ? "active" : ""}`}>進行中: {activeJobsCount} 件</span>
            <span className="queue-chip">失敗: {failedJobsCount} 件</span>
          </div>
          {settingsError ? <p className="queue-card-message">{settingsError}</p> : null}
          {runtimeErrorDetail ? <p className="queue-card-message">{runtimeErrorDetail}</p> : null}
          {jobsError ? <p className="queue-card-message">{jobsError}</p> : null}
          <AiGenerationNavList onOpenView={onOpenView} />
        </section>
      ) : null}
    </div>
  );
}
