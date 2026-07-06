import { useEffect } from "react";
import type { EpisodeIndex } from "../features/reader/types";
import { parseRouteSelection } from "../routing/readerRoute";
import type { ScreenMode } from "./useReaderState";

type UseReaderRouteSyncParams = {
  screenMode: ScreenMode;
  selectedEpisodeIndex: EpisodeIndex | null;
  selectedNovelId: string | null;
  selectedPosition: number | null;
  setScreenMode: (screenMode: ScreenMode) => void;
  setSelectedEpisodeIndex: (episodeIndex: EpisodeIndex | null) => void;
  setSelectedNovelId: (novelId: string | null) => void;
  setSelectedPosition: (position: number | null) => void;
};

export function useReaderRouteSync({
  screenMode,
  selectedEpisodeIndex,
  selectedNovelId,
  selectedPosition,
  setScreenMode,
  setSelectedEpisodeIndex,
  setSelectedNovelId,
  setSelectedPosition
}: UseReaderRouteSyncParams) {
  useEffect(() => {
    function handlePopState() {
      const route = parseRouteSelection();
      setSelectedNovelId(route.novelId);
      setSelectedEpisodeIndex(route.episodeIndex);
      setSelectedPosition(route.position);
      setScreenMode(route.screenMode);
    }

    window.addEventListener("popstate", handlePopState);
    return () => {
      window.removeEventListener("popstate", handlePopState);
    };
  }, [setScreenMode, setSelectedEpisodeIndex, setSelectedNovelId, setSelectedPosition]);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);

    if (selectedNovelId) {
      params.set("novelId", selectedNovelId);
    } else {
      params.delete("novelId");
    }

    if (screenMode === "reader" && selectedEpisodeIndex !== null) {
      params.set("episode", String(selectedEpisodeIndex));
    } else {
      params.delete("episode");
    }

    params.delete("line");

    if (screenMode === "reader" && selectedPosition !== null) {
      params.set("pos", String(selectedPosition));
    } else {
      params.delete("pos");
    }

    const nextSearch = params.toString();
    const nextUrl = `${window.location.pathname}${nextSearch.length > 0 ? `?${nextSearch}` : ""}${window.location.hash}`;
    window.history.replaceState(null, "", nextUrl);
  }, [screenMode, selectedEpisodeIndex, selectedNovelId, selectedPosition]);
}
