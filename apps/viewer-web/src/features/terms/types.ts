import type { EpisodeIndex } from "../reader/types";

export type TermCategory =
  | "organization"
  | "place"
  | "item"
  | "skill"
  | "race"
  | "event"
  | "other";

export type TermEntry = {
  term: string;
  reading: string | null;
  category: TermCategory;
  description: string;
};

export type TermsResponse = {
  status: "ready" | "not_generated";
  novelId: string;
  upToEpisodeIndex: EpisodeIndex;
  processedUpToEpisodeIndex: EpisodeIndex | null;
  terms: TermEntry[];
};
