import type { ReactNode, RefObject } from "react";

export type ReaderControlAction = {
  id: string;
  label: string;
  title: string;
  className: string;
  icon: ReactNode;
  disabled?: boolean;
  ariaExpanded?: boolean;
  onClick: () => void;
};

type ReaderBottomControlsProps = {
  controlsRef: RefObject<HTMLDivElement | null>;
  isOverflowOpen: boolean;
  overflowActions: ReaderControlAction[];
  overflowRef: RefObject<HTMLDivElement | null>;
  visibleActions: ReaderControlAction[];
  onToggleOverflow: () => void;
};

export function ReaderBottomControls({
  controlsRef,
  isOverflowOpen,
  overflowActions,
  overflowRef,
  visibleActions,
  onToggleOverflow
}: ReaderBottomControlsProps) {
  return (
    <div className="reader-bottom-controls" ref={controlsRef}>
      {visibleActions.map((action) => (
        <button
          key={action.id}
          aria-expanded={action.ariaExpanded}
          aria-label={action.label}
          className={`reader-fab ${action.className}`}
          disabled={action.disabled}
          onClick={action.onClick}
          title={action.title}
          type="button"
        >
          {action.icon}
        </button>
      ))}
      {overflowActions.length > 0 ? (
        <div className="reader-overflow" ref={overflowRef}>
          <button
            aria-expanded={isOverflowOpen}
            aria-haspopup="menu"
            aria-label="その他の操作"
            className="reader-fab reader-overflow-button"
            onClick={onToggleOverflow}
            title="その他の操作"
            type="button"
          >
            <svg aria-hidden="true" className="reader-overflow-icon" viewBox="0 0 24 24">
              <path d="M5.5 10.5a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3Zm6.5 0a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3Zm6.5 0a1.5 1.5 0 1 0 0 3 1.5 1.5 0 0 0 0-3Z" />
            </svg>
          </button>
          {isOverflowOpen ? (
            <div aria-label="その他の本文操作" className="reader-overflow-panel" role="menu">
              {overflowActions.map((action) => (
                <button
                  key={action.id}
                  aria-expanded={action.ariaExpanded}
                  className="reader-overflow-item"
                  disabled={action.disabled}
                  onClick={action.onClick}
                  role="menuitem"
                  title={action.title}
                  type="button"
                >
                  <span className="reader-overflow-item-icon">{action.icon}</span>
                  <span>{action.label}</span>
                </button>
              ))}
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
