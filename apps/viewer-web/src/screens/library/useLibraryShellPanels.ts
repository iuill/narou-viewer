import { useEffect, useRef, useState } from "react";

export function useLibraryShellPanels() {
  const statusPanelRef = useRef<HTMLDivElement | null>(null);
  const queuePanelRef = useRef<HTMLDivElement | null>(null);
  const aiGenerationPanelRef = useRef<HTMLDivElement | null>(null);
  const [isStatusPanelOpen, setIsStatusPanelOpen] = useState(false);
  const [isQueuePanelOpen, setIsQueuePanelOpen] = useState(false);
  const [isAiGenerationPanelOpen, setIsAiGenerationPanelOpen] = useState(false);

  useEffect(() => {
    if (!isStatusPanelOpen && !isQueuePanelOpen && !isAiGenerationPanelOpen) {
      return;
    }

    function handlePointerDown(event: PointerEvent) {
      const target = event.target as Node;
      const isInsideStatus = statusPanelRef.current?.contains(target) ?? false;
      const isInsideQueue = queuePanelRef.current?.contains(target) ?? false;
      const isInsideAiGeneration = aiGenerationPanelRef.current?.contains(target) ?? false;

      if (!isInsideStatus) {
        setIsStatusPanelOpen(false);
      }

      if (!isInsideQueue) {
        setIsQueuePanelOpen(false);
      }

      if (!isInsideAiGeneration) {
        setIsAiGenerationPanelOpen(false);
      }
    }

    document.addEventListener("pointerdown", handlePointerDown);
    return () => {
      document.removeEventListener("pointerdown", handlePointerDown);
    };
  }, [isAiGenerationPanelOpen, isQueuePanelOpen, isStatusPanelOpen]);

  return {
    aiGeneration: {
      isOpen: isAiGenerationPanelOpen,
      panelRef: aiGenerationPanelRef,
      setIsOpen: setIsAiGenerationPanelOpen
    },
    queue: {
      isOpen: isQueuePanelOpen,
      panelRef: queuePanelRef,
      setIsOpen: setIsQueuePanelOpen
    },
    status: {
      isOpen: isStatusPanelOpen,
      panelRef: statusPanelRef,
      setIsOpen: setIsStatusPanelOpen
    }
  };
}
