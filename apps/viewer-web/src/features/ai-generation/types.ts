import type { CharacterSummaryEntry } from "../characters/types";
import type {
  ExtractionGenerationStrategy as CharacterGenerationStrategy,
  ExtractionJobSummary as CharacterJobSummary
} from "../extraction/types";
import type { EpisodeIndex } from "../reader/types";
import type { TermEntry } from "../terms/types";

export type AiGenerationSettingsResponse = {
  apiBaseUrlConfigured: boolean;
  masterPassphraseConfigured: boolean;
  preferredMode: "llm" | "heuristic";
  effectiveGenerationMode: "openrouter" | "heuristic" | "disabled";
  settings: {
    selectedProfileId: string | null;
    sharedProviders: {
      openrouter: {
        hasApiKey: boolean;
        apiKeyMasked: string | null;
        updatedAt: string | null;
      };
      googleBooks?: {
        hasApiKey: boolean;
        apiKeyMasked: string | null;
        updatedAt: string | null;
      };
    };
    profiles: Array<{
      id: string;
      label: string;
      provider: "openrouter";
      credentials: {
        source: "shared" | "custom";
        hasApiKey: boolean;
        apiKeyMasked: string | null;
        updatedAt: string | null;
      };
      modelId: string | null;
      modelInfo?: {
        contextLength: number;
        maxCompletionTokens: number;
        source: "openrouter";
      };
      providerOrder: string[];
      allowFallbacks: boolean;
      requireParameters: boolean;
      updatedAt: string | null;
    }>;
    extractionStrategyModels: {
      nameDiscoveryModelId: string | null;
    };
    extractionRuntime: {
      parallelRequestConcurrency: number;
    };
  };
};

export type AiGenerationSettingsRequest = Partial<{
  preferredMode: "llm" | "heuristic";
  selectedProfileId: string | null;
  sharedProviders: {
    openrouter: {
      apiKey?: string;
    };
    googleBooks?: {
      apiKey?: string;
    };
  };
  profiles: Array<{
    id: string;
    label: string;
    provider: "openrouter";
    credentials: {
      source: "shared" | "custom";
      apiKey?: string;
    };
    modelId: string | null;
    providerOrder: string;
    allowFallbacks: boolean;
    requireParameters: boolean;
  }>;
  extractionStrategyModels: {
    nameDiscoveryModelId: string | null;
  };
  extractionRuntime: {
    parallelRequestConcurrency: number;
  };
}>;

export type AiGenerationPreferredModeResponse = {
  preferredMode: "llm" | "heuristic";
  effectiveGenerationMode: "openrouter" | "heuristic" | "disabled";
};

export type AiGenerationJobSummary = CharacterJobSummary & {
  novelId: string;
  novelTitle: string | null;
  novelAuthor: string | null;
  profileId: string | null;
  profileLabel: string | null;
};

export type AiGenerationJobsResponse = {
  jobs: AiGenerationJobSummary[];
};

export type AiGenerationPlaygroundResponse = {
  novelId: string;
  novelTitle: string;
  upToEpisodeIndex: EpisodeIndex;
  processedUpToEpisodeIndex: EpisodeIndex;
  profileId: string | null;
  profileLabel: string | null;
  generationMode: "openrouter" | "heuristic" | "disabled";
  generationStrategy?: CharacterGenerationStrategy | null;
  modelId: string | null;
  characters: CharacterSummaryEntry[];
  terms: TermEntry[];
};

export type AiGenerationPlaygroundRequest = {
  novelId: string;
  profileId?: string;
  upToEpisodeIndex: string;
};

export type AiGenerationPlaygroundPromptPreview = {
  systemPrompt: string;
  batches: Array<{
    batchIndex: number;
    batchCount: number;
    episodeIndexes: EpisodeIndex[];
    chunkCount: number;
    chunks: Array<{
      episodeIndex: EpisodeIndex;
      title: string;
      chapter: string | null;
      subchapter: string | null;
      chunkIndex: number;
      chunkCount: number;
      text: string;
    }>;
  }>;
};

export type AiGenerationPlaygroundBatchTiming = {
  batchIndex: number;
  batchCount: number;
  episodeIndexes: EpisodeIndex[];
  chunkCount: number;
  elapsedMs: number;
  generatedCharacterCount: number;
  generatedTermCount?: number;
  activeWorkers?: Array<{
    workerIndex: number;
    batchIndex: number;
    startEpisodeIndex: EpisodeIndex;
    endEpisodeIndex: EpisodeIndex;
    phase: "discovery" | "extraction" | string;
  }>;
  mergedCharacterCount: number;
  mergedTermCount?: number;
  message: string;
};

export type AiGenerationPlaygroundProgress = {
  stage: "preparing" | "loadingEpisodes" | "generating" | "buildingResponse";
  message: string;
  progress: number;
  step: number;
  stepCount: number;
  batchIndex?: number;
  batchCount?: number;
};

export type AiGenerationPlaygroundStreamEvent =
  | ({ type: "status" } & AiGenerationPlaygroundProgress)
  | { type: "promptPreview"; preview: AiGenerationPlaygroundPromptPreview }
  | ({ type: "batchTiming" } & AiGenerationPlaygroundBatchTiming)
  | { type: "result"; result: AiGenerationPlaygroundResponse }
  | { type: "error"; error: string };

export type AiUsageSummary = {
  runCount: number;
  requestCount: number;
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
  cachedInputTokens: number;
  reasoningOutputTokens: number;
  totalCost: number;
  averageTotalTokens: number;
};

export type AiUsageRunSummary = {
  runId: string;
  feature: "reader-assistant" | "extraction" | string;
  workflowName: string;
  status: "completed" | "failed";
  startedAt: string;
  finishedAt: string;
  elapsedMs: number;
  novelId: string | null;
  novelTitle: string | null;
  currentEpisodeIndex: string | null;
  modelId: string | null;
  profileLabel: string | null;
  generationMode: "remote" | "local" | "openrouter" | "heuristic" | "disabled" | string;
  answerChars: number;
  requestCount: number;
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
  cachedInputTokens: number;
  reasoningOutputTokens: number;
  totalCost: number;
  toolCallCount: number;
  toolResultCount: number;
  hasSnapshot: boolean;
  errorMessage: string | null;
  requests: AiUsageRequestSummary[];
};

export type AiUsageRequestSummary = {
  requestIndex: number;
  kind: "tool_call" | "handoff" | "final_answer" | "sub_request" | "other";
  parentRequestIndex: number | null;
  toolNames: string[];
  toolSummaries: string[];
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
  cachedInputTokens: number;
  reasoningOutputTokens: number;
  cost: number;
};

export type AiUsageResponse = {
  summary: AiUsageSummary;
  runs: AiUsageRunSummary[];
};
