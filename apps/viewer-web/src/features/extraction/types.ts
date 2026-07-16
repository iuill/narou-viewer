import type { EpisodeIndex } from "../reader/types";

export type ExtractionGenerationStrategy =
  | "serial"
  | "parallel_identity"
  | "discovery_parallel_correction";

export type ExtractionClearResponse = {
  message: string;
  characterProfileDeleted: boolean;
  characterEventsDeleted: boolean;
  termProfileDeleted: boolean;
  extractionJobsDeleted: number;
  extractionJobIndexDeleted: boolean;
  extractionCheckpointsDeleted: number;
};

export type ExtractionJobSummary = {
  jobId: string;
  requestedUpToEpisodeIndex: EpisodeIndex;
  generationMode: "openrouter" | "heuristic" | "disabled" | null;
  generationStrategy?: ExtractionGenerationStrategy | null;
  modelId: string | null;
  status: "queued" | "running" | "completed" | "failed" | "incompatible";
  progress?: number;
  progressStage?:
    | "preparing"
    | "batch"
    | "batchComplete"
    | "completed"
    | "failed"
    | "recovered"
    | string;
  currentBatchIndex?: number;
  batchCount?: number;
  completedBatchCount?: number;
  generatedCharacterCount?: number;
  generatedTermCount?: number;
  activeWorkers?: Array<{
    workerIndex: number;
    batchIndex: number;
    startEpisodeIndex: EpisodeIndex;
    endEpisodeIndex: EpisodeIndex;
    phase: "discovery" | "extraction" | string;
  }>;
  createdAt: string;
  startedAt: string | null;
  finishedAt: string | null;
  errorMessage: string | null;
};

export type ExtractionJobsResponse = { jobs: ExtractionJobSummary[] };

export type ExtractionSubmitRequest = {
  upToEpisodeIndex: EpisodeIndex;
  generationStrategy: ExtractionGenerationStrategy;
};

export type ExtractionSubmitResponse = {
  jobId: string;
  requestedUpToEpisodeIndex: EpisodeIndex;
  status: ExtractionJobSummary["status"];
  generationStrategy?: ExtractionGenerationStrategy | null;
  message: string;
};
