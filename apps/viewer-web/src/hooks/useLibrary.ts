import { useCallback, useEffect, useMemo, useRef, useState, type Dispatch, type SetStateAction } from "react";
import { fetchInitialData } from "../features/library/api";
import type { NovelSummary } from "../features/library/types";
import type { RuntimeStatusResponse } from "../features/runtime/types";
import { paginateItems, type PaginationResult } from "../features/library/pagination";
import { filterNovelsByQuery } from "../features/library/search";

const LIBRARY_PAGE_SIZE = 12;

type InitialLibraryData = {
  runtimeStatus: RuntimeStatusResponse;
  novels: NovelSummary[];
};

type UseLibraryOptions = {
  initialNovelId: string | null;
  isSinglePaneLibraryViewport: boolean;
  onRuntimeStatusLoaded: (status: RuntimeStatusResponse) => void;
  onError: (message: string | null) => void;
  fetchData?: () => Promise<InitialLibraryData>;
};

type UseLibraryResult = {
  novels: NovelSummary[];
  setNovels: Dispatch<SetStateAction<NovelSummary[]>>;
  filteredNovels: NovelSummary[];
  visibleLibraryNovels: NovelSummary[];
  libraryPagination: PaginationResult<NovelSummary>;
  libraryFilterQuery: string;
  setLibraryFilterQuery: Dispatch<SetStateAction<string>>;
  clearLibraryFilter: () => void;
  changeLibraryFilterQuery: (query: string) => void;
  libraryPage: number;
  setLibraryPage: Dispatch<SetStateAction<number>>;
  libraryReloadKey: number;
  requestLibraryReload: () => void;
  selectedNovelId: string | null;
  setSelectedNovelId: Dispatch<SetStateAction<string | null>>;
  currentNovel: NovelSummary | null;
  isInitialLoading: boolean;
};

function toErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Unknown error";
}

export function useLibrary({
  initialNovelId,
  isSinglePaneLibraryViewport,
  onRuntimeStatusLoaded,
  onError,
  fetchData = fetchInitialData
}: UseLibraryOptions): UseLibraryResult {
  const [novels, setNovels] = useState<NovelSummary[]>([]);
  const [libraryFilterQuery, setLibraryFilterQuery] = useState("");
  const [libraryPage, setLibraryPage] = useState(1);
  const [libraryReloadKey, setLibraryReloadKey] = useState(0);
  const [selectedNovelId, setSelectedNovelId] = useState<string | null>(initialNovelId);
  const [isInitialLoading, setIsInitialLoading] = useState(true);
  const isSinglePaneLibraryViewportRef = useRef(isSinglePaneLibraryViewport);

  const requestLibraryReload = useCallback(() => setLibraryReloadKey((current) => current + 1), []);
  const clearLibraryFilter = useCallback(() => {
    setLibraryFilterQuery("");
    setLibraryPage(1);
  }, []);
  const changeLibraryFilterQuery = useCallback((query: string) => {
    setLibraryFilterQuery(query);
    setLibraryPage(1);
  }, []);
  const filteredNovels = useMemo(() => filterNovelsByQuery(novels, libraryFilterQuery), [libraryFilterQuery, novels]);
  const libraryPagination = useMemo(
    () => paginateItems(filteredNovels, libraryPage, LIBRARY_PAGE_SIZE),
    [filteredNovels, libraryPage]
  );
  const currentNovel = useMemo(
    () => novels.find((novel) => novel.novelId === selectedNovelId) ?? null,
    [novels, selectedNovelId]
  );

  useEffect(() => {
    isSinglePaneLibraryViewportRef.current = isSinglePaneLibraryViewport;
  }, [isSinglePaneLibraryViewport]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: libraryReloadKey intentionally reloads library data.
  useEffect(() => {
    let cancelled = false;

    async function loadInitialData() {
      setIsInitialLoading(true);
      onError(null);

      try {
        const { runtimeStatus, novels: nextNovels } = await fetchData();

        if (cancelled) {
          return;
        }

        onRuntimeStatusLoaded(runtimeStatus);
        setNovels(nextNovels);
        setSelectedNovelId((current) =>
          current && nextNovels.some((novel) => novel.novelId === current)
            ? current
            : isSinglePaneLibraryViewportRef.current
              ? null
              : nextNovels[0]?.novelId ?? null
        );
      } catch (loadError) {
        if (!cancelled) {
          onError(toErrorMessage(loadError));
        }
      } finally {
        if (!cancelled) {
          setIsInitialLoading(false);
        }
      }
    }

    void loadInitialData();

    return () => {
      cancelled = true;
    };
  }, [fetchData, libraryReloadKey, onError, onRuntimeStatusLoaded]);

  useEffect(() => {
    if (libraryPage !== libraryPagination.currentPage) {
      setLibraryPage(libraryPagination.currentPage);
    }
  }, [libraryPage, libraryPagination.currentPage]);

  useEffect(() => {
    if (!selectedNovelId) {
      return;
    }

    const selectedNovelIndex = filteredNovels.findIndex((novel) => novel.novelId === selectedNovelId);
    if (selectedNovelIndex < 0) {
      return;
    }

    const nextPage = Math.floor(selectedNovelIndex / LIBRARY_PAGE_SIZE) + 1;
    setLibraryPage((currentPage) => (currentPage === nextPage ? currentPage : nextPage));
  }, [filteredNovels, selectedNovelId]);

  return {
    novels,
    setNovels,
    filteredNovels,
    visibleLibraryNovels: libraryPagination.items,
    libraryPagination,
    libraryFilterQuery,
    setLibraryFilterQuery,
    clearLibraryFilter,
    changeLibraryFilterQuery,
    libraryPage,
    setLibraryPage,
    libraryReloadKey,
    requestLibraryReload,
    selectedNovelId,
    setSelectedNovelId,
    currentNovel,
    isInitialLoading
  };
}
