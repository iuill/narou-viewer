import type { ComponentProps } from "react";
import { LibraryScreen } from "../LibraryScreen";
import { AiGenerationMenu, type AiGenerationMenuProps } from "./library/AiGenerationMenu";
import { MobileStatusPanel } from "./library/MobileStatusPanel";
import { QueuePanel, type QueuePanelProps } from "./library/QueuePanel";
import { StatusPanel, type LibraryStatusPanelProps } from "./library/StatusPanel";

export type { AiGenerationMenuProps } from "./library/AiGenerationMenu";
export type { MobileStatusPanelProps } from "./library/MobileStatusPanel";
export type { FetcherTaskListEntry, QueuePanelProps } from "./library/QueuePanel";
export type { LibraryStatusPanelProps } from "./library/StatusPanel";

export type LibraryScreenShellProps = Omit<ComponentProps<typeof LibraryScreen>, "mobileStatusPanel">;

export type LibraryShellProps = {
  aiGeneration: AiGenerationMenuProps;
  isMobileLibraryViewport: boolean;
  libraryScreenProps: LibraryScreenShellProps;
  queue: QueuePanelProps;
  status: LibraryStatusPanelProps;
};

export function LibraryShell({
  aiGeneration,
  isMobileLibraryViewport,
  libraryScreenProps,
  queue,
  status
}: LibraryShellProps) {
  const {
    clientUpdateRequired,
    error,
    formatDate,
    googleBooksConfigNotice,
    viewerBuildCommitDate,
    viewerBuildSummary
  } = status;
  const { fetcherUpdateNotice } = queue;

  const handleStatusPanelToggle = () => {
    status.setIsOpen((current) => !current);
    queue.setIsOpen(false);
    aiGeneration.setIsOpen(false);
  };

  const handleQueuePanelToggle = () => {
    queue.setIsOpen((current) => !current);
    status.setIsOpen(false);
    aiGeneration.setIsOpen(false);
  };

  const handleAiGenerationMenuToggle = () => {
    aiGeneration.setIsOpen((current) => !current);
    status.setIsOpen(false);
    queue.setIsOpen(false);
  };

  const mobileStatusPanel = <MobileStatusPanel aiGeneration={aiGeneration} queue={queue} status={status} />;

  return (
    <main className={`app-shell ${isMobileLibraryViewport ? "mobile-home-shell" : ""}`}>
      <section className="hero">
        <div className="hero-brand">
          <img alt="" className="hero-logo" height="180" src="/apple-touch-icon.png" width="180" />
          <div className="hero-heading">
            <h1 className="hero-title">Web小説ビューア</h1>
            <p className="hero-version hero-version--mobile-home">{`${viewerBuildSummary} / ${viewerBuildCommitDate}`}</p>
          </div>
        </div>
        <div className={`hero-status ${status.isOpen || queue.isOpen || aiGeneration.isOpen ? "open" : ""}`}>
          <div className="hero-status-controls">
            <StatusPanel onToggle={handleStatusPanelToggle} status={status} />
            <QueuePanel formatDate={formatDate} onToggle={handleQueuePanelToggle} queue={queue} />
            <AiGenerationMenu
              aiGeneration={aiGeneration}
              formatDate={formatDate}
              onToggle={handleAiGenerationMenuToggle}
            />
          </div>
          {fetcherUpdateNotice ? (
            <p className="hero-update-notice" role="status">
              {fetcherUpdateNotice}
            </p>
          ) : null}
          {googleBooksConfigNotice ? (
            <p className="hero-update-notice hero-config-notice" role="status">
              {googleBooksConfigNotice}
            </p>
          ) : null}
          <p className="hero-version">{`${viewerBuildSummary} / ${viewerBuildCommitDate}`}</p>
        </div>
      </section>

      {error ? <p className="message error">{error}</p> : null}

      {clientUpdateRequired ? (
        <div className="client-update-required-backdrop">
          <section
            aria-label="アプリの更新が必要です"
            aria-modal="true"
            className="client-update-required"
            role="alertdialog"
          >
            <div className="client-update-required-header">
              <p className="client-update-required-title">アプリの更新が必要です</p>
            </div>
            <p className="client-update-required-body">バージョンアップしました。アプリを再読み込みしてください。</p>
            <div className="reader-actions client-update-required-actions">
              <button onClick={() => window.location.reload()} type="button">
                再読み込み
              </button>
            </div>
          </section>
        </div>
      ) : null}

      <LibraryScreen {...libraryScreenProps} mobileStatusPanel={mobileStatusPanel} />
    </main>
  );
}
