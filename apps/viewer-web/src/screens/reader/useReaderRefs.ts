import { useRef } from "react";

export function useReaderRefs() {
  return {
    imageViewerStageRef: useRef<HTMLDivElement | null>(null),
    nextEpisodeConfirmPrimaryButtonRef: useRef<HTMLButtonElement | null>(null),
    nextEpisodeConfirmReturnFocusRef: useRef<HTMLElement | null>(null),
    readerControlsRef: useRef<HTMLDivElement | null>(null),
    readerOverflowRef: useRef<HTMLDivElement | null>(null),
    readerPageIndicatorRef: useRef<HTMLParagraphElement | null>(null),
    readerPanelRef: useRef<HTMLElement | null>(null),
    readerShellRef: useRef<HTMLElement | null>(null),
    readerViewportRef: useRef<HTMLDivElement | null>(null)
  };
}
