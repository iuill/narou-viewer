import { useEffect, useRef, type KeyboardEvent } from "react";

import type { ReaderState } from "./features/reader/types";

type ReaderSyncConflictPanelProps = {
  canApply: boolean;
  canOverwrite: boolean;
  episodeLabel: string | null;
  applyDisabledReason: string | null;
  overwriteDisabledReason: string | null;
  resolutionError: string | null;
  resolutionState: "idle" | "applying" | "overwriting";
  serverState: ReaderState;
  formatDate: (value: string | null) => string;
  onApply: () => void;
  onOverwrite: () => void;
};

export function ReaderSyncConflictPanel({
  canApply,
  canOverwrite,
  episodeLabel,
  applyDisabledReason,
  overwriteDisabledReason,
  resolutionError,
  resolutionState,
  serverState,
  formatDate,
  onApply,
  onOverwrite
}: ReaderSyncConflictPanelProps) {
  const dialogRef = useRef<HTMLElement | null>(null);
  const primaryButtonRef = useRef<HTMLButtonElement | null>(null);
  const wasResolvingRef = useRef(false);
  const isResolving = resolutionState !== "idle";

  useEffect(() => {
    const frameId = window.requestAnimationFrame(() => {
      primaryButtonRef.current?.focus();
    });
    return () => {
      window.cancelAnimationFrame(frameId);
    };
  }, []);

  useEffect(() => {
    if (isResolving) {
      wasResolvingRef.current = true;
      dialogRef.current?.focus();
      return;
    }

    if (wasResolvingRef.current) {
      wasResolvingRef.current = false;
      primaryButtonRef.current?.focus();
    }
  }, [isResolving]);

  function handleKeyDown(event: KeyboardEvent<HTMLElement>) {
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      (primaryButtonRef.current?.disabled ? dialogRef.current : primaryButtonRef.current)?.focus();
      return;
    }

    if (event.key !== "Tab") {
      return;
    }

    const focusableElements = Array.from(
      event.currentTarget.querySelectorAll<HTMLElement>(
        'button:not(:disabled), [href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])'
      )
    );
    if (focusableElements.length === 0) {
      event.preventDefault();
      dialogRef.current?.focus();
      return;
    }

    const firstElement = focusableElements[0];
    const lastElement = focusableElements[focusableElements.length - 1];
    if (document.activeElement === event.currentTarget) {
      event.preventDefault();
      (event.shiftKey ? lastElement : firstElement).focus();
    } else if (event.shiftKey && document.activeElement === firstElement) {
      event.preventDefault();
      lastElement.focus();
    } else if (!event.shiftKey && document.activeElement === lastElement) {
      event.preventDefault();
      firstElement.focus();
    }
  }

  return (
    <div className="reader-sync-conflict-backdrop">
      <section
        aria-label="別端末の既読位置更新"
        aria-live="polite"
        aria-modal="true"
        className="reader-sync-conflict"
        onKeyDown={handleKeyDown}
        ref={dialogRef}
        role="alertdialog"
        tabIndex={-1}
      >
        <p className="reader-sync-conflict-title">別端末で最終既読が更新されました。</p>
        <p className="reader-sync-conflict-body">
          {episodeLabel
            ? `${episodeLabel} の既読位置が新しく保存されています。`
            : "新しい既読位置が保存されています。"}
          {serverState.updatedAt ? ` ${formatDate(serverState.updatedAt)} の保存を反映しますか？` : " 反映しますか？"}
        </p>
        {resolutionError ? (
          <p className="reader-sync-conflict-error" role="alert">
            {resolutionError}
          </p>
        ) : null}
        {!canApply && applyDisabledReason ? (
          <p className="reader-sync-conflict-hint">{applyDisabledReason}</p>
        ) : null}
        {!canOverwrite && overwriteDisabledReason ? (
          <p className="reader-sync-conflict-hint">{overwriteDisabledReason}</p>
        ) : null}
        <div className="reader-actions reader-sync-conflict-actions">
          <button disabled={isResolving || !canApply} ref={primaryButtonRef} onClick={onApply} type="button">
            {resolutionState === "applying" ? "反映中..." : "反映して移動"}
          </button>
          <button
            disabled={isResolving || !canOverwrite}
            onClick={() => {
              onOverwrite();
            }}
            type="button"
          >
            {resolutionState === "overwriting" ? "上書き中..." : "この端末で上書き"}
          </button>
        </div>
      </section>
    </div>
  );
}
