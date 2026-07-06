import { useEffect, useState, type Dispatch, type MutableRefObject, type SetStateAction } from "react";
import {
  exitDocumentFullscreen,
  getFullscreenElement,
  requestElementFullscreen,
  resolveReaderFullscreenToggleAction,
  shouldExitReaderFullscreenOnReturn,
  supportsElementFullscreen
} from "../features/reader/fullscreen";

type UseReaderFullscreenOptions = {
  onReturnToLibrary: () => void;
  readerShellRef: MutableRefObject<HTMLElement | null>;
  screenMode: "library" | "reader";
  setReaderNotice: Dispatch<SetStateAction<string | null>>;
};

type UseReaderFullscreenResult = {
  handleReturnToLibrary: () => Promise<void>;
  handleToggleReaderFullscreen: () => Promise<void>;
  isReaderFullscreen: boolean;
  isReaderPseudoFullscreen: boolean;
};

export function useReaderFullscreen({
  onReturnToLibrary,
  readerShellRef,
  screenMode,
  setReaderNotice
}: UseReaderFullscreenOptions): UseReaderFullscreenResult {
  const [isNativeReaderFullscreen, setIsNativeReaderFullscreen] = useState(false);
  const [isReaderPseudoFullscreen, setIsReaderPseudoFullscreen] = useState(false);
  const isReaderFullscreen = isNativeReaderFullscreen || isReaderPseudoFullscreen;

  useEffect(() => {
    const syncReaderFullscreenState = () => {
      const fullscreenElement = getFullscreenElement();
      const isCurrentReaderShellFullscreen = Boolean(readerShellRef.current && fullscreenElement === readerShellRef.current);
      setIsNativeReaderFullscreen(isCurrentReaderShellFullscreen);
      if (isCurrentReaderShellFullscreen) {
        setIsReaderPseudoFullscreen(false);
      }
    };

    syncReaderFullscreenState();
    document.addEventListener("fullscreenchange", syncReaderFullscreenState);
    document.addEventListener("webkitfullscreenchange", syncReaderFullscreenState);

    return () => {
      document.removeEventListener("fullscreenchange", syncReaderFullscreenState);
      document.removeEventListener("webkitfullscreenchange", syncReaderFullscreenState);
    };
  }, [readerShellRef]);

  useEffect(() => {
    if (screenMode === "reader") {
      return;
    }

    setIsNativeReaderFullscreen(false);
    setIsReaderPseudoFullscreen(false);
  }, [screenMode]);

  useEffect(() => {
    if (screenMode !== "reader") {
      return;
    }

    const rootElement = document.getElementById("root");
    document.documentElement.classList.add("reader-active");
    document.body.classList.add("reader-active");
    rootElement?.classList.add("reader-active");

    return () => {
      document.documentElement.classList.remove("reader-active");
      document.body.classList.remove("reader-active");
      rootElement?.classList.remove("reader-active");
    };
  }, [screenMode]);

  async function handleToggleReaderFullscreen() {
    const readerShell = readerShellRef.current;
    const action = resolveReaderFullscreenToggleAction({
      hasReaderShell: readerShell !== null,
      isNativeFullscreen: readerShell !== null && getFullscreenElement() === readerShell,
      isPseudoFullscreen: isReaderPseudoFullscreen,
      supportsNativeFullscreen: readerShell !== null && supportsElementFullscreen(readerShell)
    });

    if (action === "noop") {
      return;
    }

    if (action === "exit-native") {
      try {
        await exitDocumentFullscreen();
      } catch {
        setReaderNotice("フルスクリーン表示を解除できませんでした。");
      }
      return;
    }

    if (action === "disable-pseudo") {
      setIsReaderPseudoFullscreen(false);
      return;
    }

    if (action === "enable-pseudo") {
      setIsReaderPseudoFullscreen(true);
      return;
    }

    try {
      if (!readerShell) {
        return;
      }
      await requestElementFullscreen(readerShell);
      if (getFullscreenElement() !== readerShell) {
        setIsReaderPseudoFullscreen(true);
      }
    } catch {
      setIsReaderPseudoFullscreen(true);
    }
  }

  async function handleReturnToLibrary() {
    setIsReaderPseudoFullscreen(false);

    if (
      shouldExitReaderFullscreenOnReturn({
        hasReaderShell: readerShellRef.current !== null,
        isNativeFullscreen: readerShellRef.current !== null && getFullscreenElement() === readerShellRef.current
      })
    ) {
      try {
        await exitDocumentFullscreen();
      } catch {
        setReaderNotice("フルスクリーン表示を解除できませんでした。");
      }
    }

    onReturnToLibrary();
  }

  return {
    handleReturnToLibrary,
    handleToggleReaderFullscreen,
    isReaderFullscreen,
    isReaderPseudoFullscreen
  };
}
