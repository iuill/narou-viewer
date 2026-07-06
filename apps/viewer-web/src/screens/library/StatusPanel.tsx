import type { Dispatch, RefObject, SetStateAction } from "react";
import type { ApiClientUpdateRequiredEventDetail } from "../../api/contract";
import type { RuntimeStatusResponse } from "../../features/runtime/types";

export type LibraryStatusPanelProps = {
  clientUpdateRequired: ApiClientUpdateRequiredEventDetail | null;
  error: string | null;
  formatDate: (value: string | null) => string;
  googleBooksConfigNotice: string | null;
  isOpen: boolean;
  panelRef: RefObject<HTMLDivElement | null>;
  runtimeStatus: RuntimeStatusResponse | null;
  runtimeStatusLabel: string;
  setIsOpen: Dispatch<SetStateAction<boolean>>;
  viewerBuildCommitDate: string;
  viewerBuildSummary: string;
};

export type StatusPanelProps = {
  onToggle: () => void;
  status: LibraryStatusPanelProps;
};

export function StatusPanel({ onToggle, status }: StatusPanelProps) {
  const { formatDate, isOpen, panelRef, runtimeStatus, runtimeStatusLabel } = status;

  return (
    <div className="hero-status-group" ref={panelRef}>
      <button
        aria-expanded={isOpen}
        aria-haspopup="dialog"
        className={`status-trigger ${runtimeStatus ? runtimeStatus.status : "loading"}`}
        onClick={onToggle}
        type="button"
      >
        <span aria-hidden="true" className="status-trigger-icon" />
        <span className="status-trigger-copy">
          <strong>動作状況</strong>
          <small>{runtimeStatusLabel}</small>
        </span>
      </button>
      {isOpen ? (
        <section aria-label="サービスの動作状況" className="status-popover">
          <header>
            <strong>サービスの状態</strong>
            <span>{runtimeStatus ? formatDate(runtimeStatus.checkedAt) : "確認中"}</span>
          </header>
          <div className="status-popover-list">
            {(runtimeStatus?.services ?? []).map((service) => (
              <article className="status-item" key={service.id}>
                <div className="status-item-heading">
                  <strong>{service.label}</strong>
                  <span className={`status-badge ${service.status}`}>{service.summary}</span>
                </div>
                <p>{service.detail}</p>
              </article>
            ))}
            {!runtimeStatus ? <p className="message">動作状況を確認中...</p> : null}
          </div>
        </section>
      ) : null}
    </div>
  );
}
