import { AiJobsView, type AiJobsViewProps } from "./features/ai-generation/AiJobsView";
import { AiPlaygroundView, type AiPlaygroundViewProps } from "./features/ai-generation/AiPlaygroundView";
import { AiSettingsView, type AiSettingsViewProps } from "./features/ai-generation/AiSettingsView";
import { AiUsageView, type AiUsageViewProps } from "./features/ai-generation/AiUsageView";

type AiGenerationPanelView = "settings" | "playground" | "jobs" | "usage";

type Props = {
  activeView: AiGenerationPanelView;
  aiGenerationSummaryLabel: string;
  aiGenerationNotice: string | null;
  onOpenView: (view: AiGenerationPanelView) => void | Promise<void>;
  onClose: () => void;
  settingsViewProps: AiSettingsViewProps;
  playgroundViewProps: AiPlaygroundViewProps;
  jobsViewProps: AiJobsViewProps;
  usageViewProps: AiUsageViewProps;
};

export function AiGenerationWorkspace({
  activeView,
  aiGenerationSummaryLabel,
  aiGenerationNotice,
  onOpenView,
  onClose,
  settingsViewProps,
  playgroundViewProps,
  jobsViewProps,
  usageViewProps
}: Props) {
  return (
    <section className="panel ai-workspace-panel" id="ai-generation-workspace">
      <div className="panel-header">
        <div>
          <h2>AI機能</h2>
          <p>{aiGenerationSummaryLabel}</p>
        </div>
        <div className="panel-header-actions">
          <div className="mode-toggle ai-workspace-toggle">
            <button className={activeView === "jobs" ? "active" : ""} onClick={() => void onOpenView("jobs")} type="button">
              キャラ生成履歴
            </button>
            <button className={activeView === "usage" ? "active" : ""} onClick={() => void onOpenView("usage")} type="button">
              読書AI利用統計
            </button>
            <button className={activeView === "settings" ? "active" : ""} onClick={() => void onOpenView("settings")} type="button">
              設定
            </button>
            <button className={activeView === "playground" ? "active" : ""} onClick={() => void onOpenView("playground")} type="button">
              生成テスト
            </button>
          </div>
          <button className="download-cancel-button ai-workspace-close" onClick={onClose} type="button">
            閉じる
          </button>
        </div>
      </div>

      {aiGenerationNotice ? <p className="message">{aiGenerationNotice}</p> : null}

      {activeView === "settings" ? <AiSettingsView {...settingsViewProps} /> : null}

      {activeView === "playground" ? <AiPlaygroundView {...playgroundViewProps} /> : null}

      {activeView === "jobs" ? <AiJobsView {...jobsViewProps} /> : null}

      {activeView === "usage" ? <AiUsageView {...usageViewProps} /> : null}
    </section>
  );
}
