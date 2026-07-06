import { forwardRef, type AriaRole, type ReactNode } from "react";

type Props = {
  ariaLabel?: string;
  bodyClassName?: string;
  children: ReactNode;
  className?: string;
  description?: string;
  onClose?: () => void;
  role?: AriaRole;
  title: string;
};

export const ReaderFloatingPanel = forwardRef<HTMLElement, Props>(function ReaderFloatingPanel(
  { ariaLabel, bodyClassName, children, className, description, onClose, role = "dialog", title },
  ref
) {
  return (
    <section
      aria-label={ariaLabel ?? title}
      className={["reader-overlay-panel", className].filter(Boolean).join(" ")}
      ref={ref}
      role={role}
    >
      <div className="reader-overlay-panel-header reader-panel-header">
        <div>
          <strong>{title}</strong>
          {description ? <p>{description}</p> : null}
        </div>
        {onClose ? (
          <button aria-label={`${title}を閉じる`} className="reader-overlay-panel-close reader-panel-close" onClick={onClose} type="button">
            ×
          </button>
        ) : null}
      </div>
      <div className={["reader-overlay-panel-body", bodyClassName].filter(Boolean).join(" ")}>{children}</div>
    </section>
  );
});
