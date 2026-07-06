import { useMemo, useState } from "react";
import { useLibrary } from "../hooks/useLibrary";
import { useMobileLibraryViewport } from "../hooks/useMobileLibraryViewport";
import { useRuntimeStatus } from "../hooks/useRuntimeStatus";
import { parseRouteSelection } from "../routing/readerRoute";
import type { ReaderRouteControllerProps } from "../routes/ReaderRouteController";
import { useLibraryWorkspaceModel } from "../screens/library/useLibraryWorkspaceModel";
import { useReaderWorkspaceModel } from "../screens/reader/useReaderWorkspaceModel";
import type { LibraryShellProps } from "../screens/LibraryShell";
import type { ReaderScreenModel } from "../screens/ReaderShell";
import { useClientUpdateRequirement } from "./useClientUpdateRequirement";

export type ViewerAppModel = {
  routeController: ReaderRouteControllerProps;
  screen:
    | {
        type: "reader";
        reader: ReaderScreenModel;
      }
    | {
        type: "library";
        library: LibraryShellProps;
      };
};

export function useViewerAppModel(): ViewerAppModel {
  const initialSelection = useMemo(() => parseRouteSelection(), []);
  const isMobileLibraryViewport = useMobileLibraryViewport();
  const [error, setError] = useState<string | null>(null);
  const { clientUpdateRequired } = useClientUpdateRequirement();
  const {
    status: runtimeStatus,
    statusLabel: runtimeStatusLabel,
    aiGenerationService: aiGenerationRuntimeService,
    fetcherService: fetcherRuntimeService,
    setStatus: setRuntimeStatus,
    refreshStatus: refreshRuntimeStatus
  } = useRuntimeStatus();
  const {
    novels,
    setNovels,
    filteredNovels,
    visibleLibraryNovels,
    libraryPagination,
    libraryFilterQuery,
    clearLibraryFilter,
    changeLibraryFilterQuery,
    setLibraryPage,
    libraryReloadKey,
    requestLibraryReload,
    selectedNovelId,
    setSelectedNovelId,
    currentNovel,
    isInitialLoading
  } = useLibrary({
    initialNovelId: initialSelection.novelId,
    isSinglePaneLibraryViewport: isMobileLibraryViewport,
    onRuntimeStatusLoaded: setRuntimeStatus,
    onError: setError
  });
  const readerWorkspace = useReaderWorkspaceModel({
    clientUpdateRequired,
    currentNovel,
    error,
    initialSelection,
    libraryReloadKey,
    selectedNovelId,
    setError,
    setNovels,
    setSelectedNovelId
  });
  const libraryWorkspace = useLibraryWorkspaceModel({
    aiGenerationRuntimeService,
    clientUpdateRequired,
    currentNovel,
    error,
    fetcherRuntimeService,
    filteredNovels,
    initialNovelId: initialSelection.novelId,
    isInitialLoading,
    isMobileLibraryViewport,
    libraryFilterQuery,
    libraryPagination,
    onClearLibraryFilter: clearLibraryFilter,
    onLibraryFilterQueryChange: changeLibraryFilterQuery,
    onLibraryPageChange: setLibraryPage,
    libraryReloadKey,
    novels,
    readerWorkspace,
    refreshRuntimeStatus,
    requestLibraryReload,
    runtimeStatus,
    runtimeStatusLabel,
    selectedNovelId,
    setError,
    visibleLibraryNovels
  });

  if (readerWorkspace.screenMode === "reader") {
    return {
      routeController: libraryWorkspace.routeController,
      screen: {
        type: "reader",
        reader: libraryWorkspace.reader
      }
    };
  }

  return {
    routeController: libraryWorkspace.routeController,
    screen: {
      type: "library",
      library: libraryWorkspace.shell
    }
  };
}
