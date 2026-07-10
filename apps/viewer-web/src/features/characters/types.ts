import type { EpisodeIndex } from "../reader/types";

export type CharacterSummaryEntry = {
  characterId: string;
  canonicalName: string;
  fullName: string | null;
  gender: string | null;
  firstAppearanceEpisodeIndex: EpisodeIndex;
  aliases: string[];
  appearance: string | null;
  personality: string | null;
  summary: string | null;
  importance: {
    category: "main" | "regular" | "semi-regular";
    score: number;
  } | null;
};

export type CharacterSummaryResponse = {
  status: "ready" | "partial" | "not_generated";
  novelId: string;
  upToEpisodeIndex: EpisodeIndex;
  processedUpToEpisodeIndex: EpisodeIndex | null;
  characters: CharacterSummaryEntry[];
};
