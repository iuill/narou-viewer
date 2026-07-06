import { forwardRef, type ReactNode } from "react";

type ReaderShellProps = {
  children: ReactNode;
  isFullscreen: boolean;
  isPseudoFullscreen: boolean;
  isWebKitEngine: boolean;
  theme: string;
};

export const ReaderShell = forwardRef<HTMLElement, ReaderShellProps>(function ReaderShell(
  { children, isFullscreen, isPseudoFullscreen, isWebKitEngine, theme },
  ref
) {
  return (
    <main
      className={[
        "reader-shell",
        `reader-theme-${theme}`,
        isWebKitEngine ? "reader-shell-webkit" : "",
        isFullscreen ? "reader-shell-fullscreen" : "",
        isPseudoFullscreen ? "reader-shell-fallback-fullscreen" : ""
      ]
        .filter(Boolean)
        .join(" ")}
      ref={ref}
    >
      {children}
    </main>
  );
});
