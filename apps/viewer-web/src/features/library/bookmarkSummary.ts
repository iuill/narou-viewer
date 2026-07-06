import { shouldUseFriendlyEpisodeLabels, type EpisodeDisplayReference, type EpisodeLike, formatEpisodeReferenceLabel } from "../reader/episodeLabels";

export function formatNovelLastReadLabel(novel: {
  siteName: string;
  tocUrl: string | null;
  lastReadEpisodeIndex: string | null;
  lastReadEpisodeTitle: string | null;
}): string {
  if (novel.lastReadEpisodeIndex === null) {
    return "未読";
  }

  if (shouldUseFriendlyEpisodeLabels(novel.siteName, novel.tocUrl) && novel.lastReadEpisodeTitle) {
    return novel.lastReadEpisodeTitle;
  }

  return novel.lastReadEpisodeIndex;
}

export function deriveLatestBookmark<TBookmark extends { createdAt: string }>(bookmarks: TBookmark[]): TBookmark | null {
  if (bookmarks.length === 0) {
    return null;
  }

  let latestBookmark = bookmarks[0];
  let latestCreatedAt = Date.parse(bookmarks[0].createdAt);

  for (const bookmark of bookmarks.slice(1)) {
    const currentCreatedAt = Date.parse(bookmark.createdAt);
    if (!Number.isNaN(currentCreatedAt) && (Number.isNaN(latestCreatedAt) || currentCreatedAt > latestCreatedAt)) {
      latestCreatedAt = currentCreatedAt;
      latestBookmark = bookmark;
    }
  }

  return latestBookmark;
}

export function formatBookmarkLocation<TEpisode extends EpisodeLike>(
  bookmark: { episodeIndex: string },
  episodeLookup: Map<string, EpisodeDisplayReference<TEpisode>>,
  preferFriendlyEpisodeLabels: boolean
): string {
  return formatEpisodeReferenceLabel(bookmark.episodeIndex, episodeLookup, preferFriendlyEpisodeLabels);
}

export function updateNovelBookmarkSummary<
  TNovel extends { novelId: string; bookmarkCount: number; latestBookmarkEpisodeIndex: string | null },
  TBookmark extends { episodeIndex: string; createdAt: string }
>(novels: TNovel[], novelId: string, nextBookmarks: TBookmark[]): TNovel[] {
  const latestBookmark = deriveLatestBookmark(nextBookmarks);

  return novels.map((novel) =>
    novel.novelId === novelId
      ? {
          ...novel,
          bookmarkCount: nextBookmarks.length,
          latestBookmarkEpisodeIndex: latestBookmark?.episodeIndex ?? null
        }
      : novel
  );
}
