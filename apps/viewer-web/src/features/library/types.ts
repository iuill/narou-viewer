import type { EpisodeIndex } from "../reader/types";
import type { RuntimeStatusResponse } from "../runtime/types";

export type NovelSummary = {
  novelId: string;
  fetcherWorkId: string;
  title: string;
  author: string;
  siteName: string;
  tocUrl: string | null;
  story?: string | null;
  updatedAt: string | null;
  lastActivityAt?: string | null;
  lastReadEpisodeIndex: EpisodeIndex | null;
  lastReadEpisodeTitle: string | null;
  latestBookmarkEpisodeIndex: EpisodeIndex | null;
  bookmarkCount: number;
  totalEpisodes: number;
  savedEpisodes?: number;
  fetchStatus?: string;
  publicationCoverImageUrl?: string;
  publicationCoverKind?: "novel" | "comic";
  publicationCoverSource?: string;
  publicationCoverSourceUrl?: string;
  lastFetchError?: string | null;
  failedEpisodeId?: string | null;
  resumeEpisodeId?: string | null;
};

export type NovelsResponse = {
  novels: NovelSummary[];
};

export type InitialData = {
  runtimeStatus: RuntimeStatusResponse;
  novels: NovelsResponse["novels"];
};
