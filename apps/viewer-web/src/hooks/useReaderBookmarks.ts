import { useMemo, useState, type Dispatch, type MutableRefObject, type SetStateAction } from "react";
import {
  deriveLatestBookmark,
  formatBookmarkLocation,
  updateNovelBookmarkSummary
} from "../features/library/bookmarkSummary";
import type { NovelSummary } from "../features/library/types";
import { createBookmark, deleteBookmark } from "../features/reader/api";
import type { EpisodeDisplayReference } from "../features/reader/episodeLabels";
import type { Bookmark, EpisodeIndex } from "../features/reader/types";
import { findReaderPositionTarget, getReaderPositionFromViewport } from "../readerPosition";
import type { ReadingMode } from "../readerPreferences";

const DEFAULT_VISIBLE_BOOKMARK_COUNT = 3;

type UseReaderBookmarksOptions = {
  bookmarks: Bookmark[];
  episodeDisplayLookup: Map<EpisodeIndex, EpisodeDisplayReference>;
  isShowingAllBookmarks: boolean;
  preferFriendlyEpisodeLabels: boolean;
  readerViewportRef: MutableRefObject<HTMLDivElement | null>;
  readingMode: ReadingMode;
  selectedEpisodeIndex: EpisodeIndex | null;
  selectedNovelId: string | null;
  setBookmarks: Dispatch<SetStateAction<Bookmark[]>>;
  setError: Dispatch<SetStateAction<string | null>>;
  setNovels: Dispatch<SetStateAction<NovelSummary[]>>;
  setReaderNotice: Dispatch<SetStateAction<string | null>>;
  setSelectedPosition: Dispatch<SetStateAction<number | null>>;
};

type UseReaderBookmarksResult = {
  createCurrentBookmark: () => Promise<void>;
  deleteBookmarkById: (bookmarkId: string) => Promise<void>;
  isBookmarkSaving: boolean;
  latestBookmark: Bookmark | null;
  pendingBookmarkId: string | null;
  visibleBookmarks: Bookmark[];
};

export function useReaderBookmarks({
  bookmarks,
  episodeDisplayLookup,
  isShowingAllBookmarks,
  preferFriendlyEpisodeLabels,
  readerViewportRef,
  readingMode,
  selectedEpisodeIndex,
  selectedNovelId,
  setBookmarks,
  setError,
  setNovels,
  setReaderNotice,
  setSelectedPosition
}: UseReaderBookmarksOptions): UseReaderBookmarksResult {
  const [isBookmarkSaving, setIsBookmarkSaving] = useState(false);
  const [pendingBookmarkId, setPendingBookmarkId] = useState<string | null>(null);
  const latestBookmark = useMemo(() => deriveLatestBookmark(bookmarks), [bookmarks]);
  const visibleBookmarks = useMemo(
    () => (isShowingAllBookmarks ? bookmarks : bookmarks.slice(0, DEFAULT_VISIBLE_BOOKMARK_COUNT)),
    [bookmarks, isShowingAllBookmarks]
  );

  async function createCurrentBookmark() {
    if (!selectedNovelId || selectedEpisodeIndex === null) {
      return;
    }

    const novelId = selectedNovelId;
    const episodeIndex = selectedEpisodeIndex;
    const viewport = readerViewportRef.current;
    if (!viewport) {
      return;
    }

    const position = getReaderPositionFromViewport(viewport, readingMode);
    const labelSource = position === null ? null : findReaderPositionTarget(viewport, position);

    if (position === null) {
      setError("現在位置を特定できませんでした。");
      return;
    }

    const label = (labelSource?.innerText ?? "").replace(/\s+/g, " ").trim().slice(0, 80);

    setIsBookmarkSaving(true);
    setError(null);

    try {
      const bookmark = await createBookmark({
        novelId,
        episodeIndex,
        position,
        label: label.length > 0 ? label : null
      });
      setBookmarks((current) => {
        const nextBookmarks = [bookmark, ...current];
        setNovels((currentNovels) => updateNovelBookmarkSummary(currentNovels, novelId, nextBookmarks));
        return nextBookmarks;
      });
      setSelectedPosition(bookmark.position);
      setReaderNotice(`${formatBookmarkLocation(bookmark, episodeDisplayLookup, preferFriendlyEpisodeLabels)} に栞を保存しました。`);
    } catch (bookmarkError) {
      setError(bookmarkError instanceof Error ? bookmarkError.message : "Unknown error");
    } finally {
      setIsBookmarkSaving(false);
    }
  }

  async function deleteBookmarkById(bookmarkId: string) {
    if (!selectedNovelId) {
      return;
    }

    const novelId = selectedNovelId;
    setPendingBookmarkId(bookmarkId);
    setError(null);

    try {
      await deleteBookmark(bookmarkId);

      setBookmarks((current) => {
        const nextBookmarks = current.filter((bookmark) => bookmark.id !== bookmarkId);
        setNovels((currentNovels) => updateNovelBookmarkSummary(currentNovels, novelId, nextBookmarks));
        return nextBookmarks;
      });
    } catch (bookmarkError) {
      setError(bookmarkError instanceof Error ? bookmarkError.message : "Unknown error");
    } finally {
      setPendingBookmarkId(null);
    }
  }

  return {
    createCurrentBookmark,
    deleteBookmarkById,
    isBookmarkSaving,
    latestBookmark,
    pendingBookmarkId,
    visibleBookmarks
  };
}
