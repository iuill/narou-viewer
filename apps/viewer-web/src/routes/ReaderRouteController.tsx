import { useEffect, useRef } from "react";

export type ReaderRouteControllerProps = {
  filteredNovels: Array<{ novelId: string }>;
  isInitialLoading: boolean;
  isMobileLibraryViewport: boolean;
  onClearSelection: () => void;
  onSelectNovel: (novelId: string) => void;
  onShowMobileLibraryPanel: () => void;
  selectedNovelId: string | null;
};

export function ReaderRouteController({
  filteredNovels,
  isInitialLoading,
  isMobileLibraryViewport,
  onClearSelection,
  onSelectNovel,
  onShowMobileLibraryPanel,
  selectedNovelId
}: ReaderRouteControllerProps) {
  const onClearSelectionRef = useRef(onClearSelection);
  const onSelectNovelRef = useRef(onSelectNovel);
  const onShowMobileLibraryPanelRef = useRef(onShowMobileLibraryPanel);

  useEffect(() => {
    onClearSelectionRef.current = onClearSelection;
    onSelectNovelRef.current = onSelectNovel;
    onShowMobileLibraryPanelRef.current = onShowMobileLibraryPanel;
  }, [onClearSelection, onSelectNovel, onShowMobileLibraryPanel]);

  useEffect(() => {
    if (isInitialLoading) {
      return;
    }

    if (filteredNovels.length === 0) {
      if (selectedNovelId !== null) {
        onClearSelectionRef.current();
      }
      return;
    }

    if (selectedNovelId && filteredNovels.some((novel) => novel.novelId === selectedNovelId)) {
      return;
    }

    if (isMobileLibraryViewport) {
      if (selectedNovelId !== null) {
        onClearSelectionRef.current();
      }
      onShowMobileLibraryPanelRef.current();
      return;
    }

    onSelectNovelRef.current(filteredNovels[0].novelId);
  }, [filteredNovels, isInitialLoading, isMobileLibraryViewport, selectedNovelId]);

  return null;
}
