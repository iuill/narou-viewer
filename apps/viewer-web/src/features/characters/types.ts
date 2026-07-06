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

export type CharacterGenerationStrategy = "serial" | "parallel_identity" | "discovery_parallel_correction";

export type CharacterSummaryResponse = {
  status: "ready" | "not_generated";
  novelId: string;
  upToEpisodeIndex: EpisodeIndex;
  processedUpToEpisodeIndex: EpisodeIndex | null;
  characters: CharacterSummaryEntry[];
};

export type CharacterSummaryClearResponse = {
  message: string;
  profileDeleted: boolean;
  eventsDeleted: boolean;
  jobsDeleted: number;
  jobIndexDeleted: boolean;
  checkpointsDeleted: number;
};

export type CharacterJobSummary = {
  jobId: string;
  requestedUpToEpisodeIndex: EpisodeIndex;
  generationMode: "openrouter" | "heuristic" | "disabled" | null;
  generationStrategy?: CharacterGenerationStrategy | null;
  modelId: string | null;
  status: "queued" | "running" | "completed" | "failed";
  progress?: number;
  progressStage?: "preparing" | "batch" | "batchComplete" | "completed" | "failed" | "recovered" | string;
  currentBatchIndex?: number;
  batchCount?: number;
  generatedCharacterCount?: number;
  createdAt: string;
  startedAt: string | null;
  finishedAt: string | null;
  errorMessage: string | null;
};

export type CharacterJobsResponse = {
  jobs: CharacterJobSummary[];
};

export type CharacterJobSubmitRequest = {
  upToEpisodeIndex: EpisodeIndex;
  generationStrategy: CharacterGenerationStrategy;
};

export type CharacterJobSubmitResponse = {
  jobId: string;
  requestedUpToEpisodeIndex: EpisodeIndex;
  status: CharacterJobSummary["status"];
  generationStrategy?: CharacterGenerationStrategy | null;
  message: string;
};
