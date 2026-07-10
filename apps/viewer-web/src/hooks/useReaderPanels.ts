import { useCallback, useEffect, useState, type RefObject } from "react";
import type { ScreenMode } from "./useReaderState";

export type ReaderPanelView =
  | "toc"
  | "bookmarks"
  | "speech"
  | "settings"
  | "experimental-font"
  | "info"
  | "characters"
  | "terms"
  | "reader-ai";

type UseReaderPanelsParams = {
  closeImageViewer: () => void;
  readerControlsRef: RefObject<HTMLDivElement | null>;
  readerOverflowRef: RefObject<HTMLDivElement | null>;
  readerPanelRef: RefObject<HTMLElement | null>;
  screenMode: ScreenMode;
};

export function useReaderPanels({
  closeImageViewer,
  readerControlsRef,
  readerOverflowRef,
  readerPanelRef,
  screenMode
}: UseReaderPanelsParams) {
  const [activeReaderPanel, setActiveReaderPanel] = useState<ReaderPanelView | null>(null);
  const [isReaderOverflowOpen, setIsReaderOverflowOpen] = useState(false);

  const closeReaderPanel = useCallback(() => {
    setActiveReaderPanel(null);
    setIsReaderOverflowOpen(false);
  }, []);

  const closeActiveReaderPanel = useCallback(() => {
    setActiveReaderPanel(null);
  }, []);

  const closeCharacterSummaryPanel = useCallback(() => {
    setActiveReaderPanel((current) => (current === "characters" ? null : current));
  }, []);

  const openCharacterSummaryPanel = useCallback(() => {
    setIsReaderOverflowOpen(false);
    setActiveReaderPanel("characters");
  }, []);

  const openTermsPanel = useCallback(() => {
    setIsReaderOverflowOpen(false);
    setActiveReaderPanel("terms");
  }, []);

  const toggleReaderPanel = useCallback((panel: Exclude<ReaderPanelView, "characters">) => {
    setIsReaderOverflowOpen(false);
    setActiveReaderPanel((current) => (current === panel ? null : panel));
  }, []);

  useEffect(() => {
    if (!isReaderOverflowOpen) {
      return;
    }

    function handlePointerDown(event: PointerEvent) {
      const target = event.target as Node;
      if (readerOverflowRef.current?.contains(target)) {
        return;
      }

      setIsReaderOverflowOpen(false);
    }

    document.addEventListener("pointerdown", handlePointerDown);
    return () => {
      document.removeEventListener("pointerdown", handlePointerDown);
    };
  }, [isReaderOverflowOpen, readerOverflowRef]);

  useEffect(() => {
    if (screenMode !== "reader" || activeReaderPanel === null) {
      return;
    }

    function handlePointerDown(event: PointerEvent) {
      const target = event.target as Node;
      const isInsidePanel = readerPanelRef.current?.contains(target) ?? false;
      const isInsideControls = readerControlsRef.current?.contains(target) ?? false;
      const targetElement = target.nodeType === 1 ? (target as Element) : null;
      const isInsideReaderInteractive = (targetElement?.closest("[data-reader-panel-interactive]") ?? null) !== null;

      if (!isInsidePanel && !isInsideControls && !isInsideReaderInteractive) {
        setActiveReaderPanel(null);
      }
    }

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setActiveReaderPanel(null);
      }
    }

    document.addEventListener("pointerdown", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("pointerdown", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [activeReaderPanel, readerControlsRef, readerPanelRef, screenMode]);

  useEffect(() => {
    if (screenMode === "reader") {
      return;
    }

    setActiveReaderPanel(null);
    setIsReaderOverflowOpen(false);
    closeImageViewer();
  }, [closeImageViewer, screenMode]);

  return {
    activeReaderPanel,
    closeActiveReaderPanel,
    closeCharacterSummaryPanel,
    closeReaderPanel,
    isCharacterSummaryOpen: activeReaderPanel === "characters",
    isTermsOpen: activeReaderPanel === "terms",
    isReaderAiAssistantOpen: activeReaderPanel === "reader-ai",
    isReaderBookmarksOpen: activeReaderPanel === "bookmarks",
    isReaderExperimentalFontOpen: activeReaderPanel === "experimental-font",
    isReaderInfoOpen: activeReaderPanel === "info",
    isReaderOverflowOpen,
    isReaderSettingsOpen: activeReaderPanel === "settings",
    isReaderSpeechOpen: activeReaderPanel === "speech",
    isReaderTocOpen: activeReaderPanel === "toc",
    openCharacterSummaryPanel,
    openTermsPanel,
    setActiveReaderPanel,
    setIsReaderOverflowOpen,
    toggleReaderPanel
  };
}
