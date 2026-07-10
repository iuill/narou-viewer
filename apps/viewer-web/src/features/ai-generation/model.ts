import type { ExtractionGenerationStrategy as CharacterGenerationStrategy } from "../extraction/types";

export type AiGenerationMode = "openrouter" | "heuristic" | "disabled" | null;

export type AiGenerationSettingsProfileLike = {
  id: string;
  label: string;
  provider: "openrouter";
  credentials: {
    source: "shared" | "custom";
    hasApiKey: boolean;
    apiKeyMasked: string | null;
    updatedAt?: string | null;
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
  updatedAt?: string | null;
};

export type AiGenerationSettingsLike = {
  effectiveGenerationMode: AiGenerationMode;
  settings: {
    selectedProfileId: string | null;
    sharedProviders?: {
      openrouter: {
        hasApiKey: boolean;
        apiKeyMasked: string | null;
        updatedAt?: string | null;
      };
      googleBooks?: {
        hasApiKey: boolean;
        apiKeyMasked: string | null;
        updatedAt?: string | null;
      };
    };
    profiles: AiGenerationSettingsProfileLike[];
    extractionStrategyModels?: {
      nameDiscoveryModelId?: string | null;
    };
  };
};

export type RuntimeStatusLike = {
  services: Array<{
    id: string;
    summary: string;
  }>;
};

export type AiGenerationSharedProviderDraft = {
  provider: "openrouter" | "googleBooks";
  apiKeyInput: string;
  apiKeyMasked: string | null;
  hasApiKey: boolean;
  updatedAt?: string | null;
};

export type AiGenerationProfileDraft = {
  id: string;
  label: string;
  provider: "openrouter";
  apiKeySource: "shared" | "custom";
  apiKeyInput: string;
  apiKeyMasked: string | null;
  hasApiKey: boolean;
  credentialsUpdatedAt?: string | null;
  modelId: string;
  modelInfo?: {
    contextLength: number;
    maxCompletionTokens: number;
    source: "openrouter";
  };
  providerOrder: string;
  allowFallbacks: boolean;
  requireParameters: boolean;
};

export type ExtractionStrategyModelsDraft = {
  nameDiscoveryModelId: string;
};

export type AiGenerationJobStatus = "queued" | "running" | "completed" | "failed";
export type AiGenerationJobFilter = "active" | "failed" | "completed";
export type AiGenerationHelpKey =
  | "preferredMode"
  | "sharedApiKey"
  | "googleBooksApiKey"
  | "profileLabel"
  | "apiKey"
  | "apiKeySource"
  | "modelId"
  | "nameDiscoveryModelId"
  | "providerOrder"
  | "allowFallbacks"
  | "requireParameters";

export type AiGenerationJobLike = {
  status: AiGenerationJobStatus;
};

export type AiGenerationPlaygroundProgressLike = {
  stage: "preparing" | "loadingEpisodes" | "generating" | "buildingResponse";
  message: string;
  progress: number;
  step: number;
  stepCount: number;
  batchIndex?: number;
  batchCount?: number;
};

export function formatCount(value: number): string {
  return new Intl.NumberFormat("ja-JP").format(value);
}

export function getAiGenerationModeLabel(mode: AiGenerationMode): string {
  switch (mode) {
    case "openrouter":
      return "OpenRouter";
    case "heuristic":
      return "Heuristic";
    case "disabled":
      return "停止";
    default:
      return "未記録";
  }
}

export function getCharacterGenerationStrategyLabel(strategy: CharacterGenerationStrategy | null | undefined): string {
  switch (strategy) {
    case "discovery_parallel_correction":
      return "事前発見 + 並列抽出 + 補正";
    case "parallel_identity":
      return "並列抽出 + 人物・用語統合";
    case "serial":
      return "順次抽出";
    default:
      return "順次抽出";
  }
}

export function getCompactAiGenerationModelLabel(modelId: string | null): string | null {
  if (typeof modelId !== "string") {
    return null;
  }

  const normalized = modelId.trim();
  if (normalized.length === 0) {
    return null;
  }

  const segments = normalized
    .split("/")
    .map((segment) => segment.trim())
    .filter((segment) => segment.length > 0);
  if (segments.length === 0) {
    return null;
  }

  return segments.at(-1) ?? null;
}

export function getAiGenerationSummary(
  settingsResponse: AiGenerationSettingsLike | null,
  runtimeStatus: RuntimeStatusLike | null
): string {
  if (settingsResponse) {
    const activeProfile =
      settingsResponse.settings.profiles.find((profile) => profile.id === settingsResponse.settings.selectedProfileId) ??
      settingsResponse.settings.profiles[0];
    const modeLabel = getAiGenerationModeLabel(settingsResponse.effectiveGenerationMode);
    return activeProfile?.modelId ? `${modeLabel} / ${activeProfile.modelId}` : modeLabel;
  }

  const runtimeService = runtimeStatus?.services.find((service) => service.id === "go-internal-ai");
  return runtimeService?.summary ?? "確認中";
}

export function getAiGenerationTriggerSummary(
  settingsResponse: AiGenerationSettingsLike | null,
  runtimeStatus: RuntimeStatusLike | null
): string {
  if (settingsResponse) {
    const activeProfile =
      settingsResponse.settings.profiles.find((profile) => profile.id === settingsResponse.settings.selectedProfileId) ??
      settingsResponse.settings.profiles[0];
    const modeLabel = getAiGenerationModeLabel(settingsResponse.effectiveGenerationMode);
    const compactModelLabel = getCompactAiGenerationModelLabel(activeProfile?.modelId ?? null);
    return compactModelLabel ? `${modeLabel} / ${compactModelLabel}` : modeLabel;
  }

  const runtimeService = runtimeStatus?.services.find((service) => service.id === "go-internal-ai");
  return runtimeService?.summary ?? "確認中";
}

export function getActiveAiGenerationSettingsProfile(
  settingsResponse: AiGenerationSettingsLike | null
): AiGenerationSettingsProfileLike | null {
  if (!settingsResponse) {
    return null;
  }

  return (
    settingsResponse.settings.profiles.find((profile) => profile.id === settingsResponse.settings.selectedProfileId) ??
    settingsResponse.settings.profiles[0] ??
    null
  );
}

export function toAiGenerationSharedProviderDraft(
  provider:
    | {
        hasApiKey: boolean;
        apiKeyMasked: string | null;
        updatedAt?: string | null;
      }
    | undefined,
  providerId: AiGenerationSharedProviderDraft["provider"] = "openrouter"
): AiGenerationSharedProviderDraft {
  return {
    provider: providerId,
    apiKeyInput: "",
    apiKeyMasked: provider?.apiKeyMasked ?? null,
    hasApiKey: provider?.hasApiKey === true,
    updatedAt: provider?.updatedAt ?? null
  };
}

export function toAiGenerationProfileDraft(profile: AiGenerationSettingsProfileLike): AiGenerationProfileDraft {
  const legacyProfile = profile as AiGenerationSettingsProfileLike & {
    hasApiKey?: boolean;
    apiKeyMasked?: string | null;
  };
  const credentials = profile.credentials ?? {
    source: "shared" as const,
    hasApiKey: legacyProfile.hasApiKey === true,
    apiKeyMasked: legacyProfile.apiKeyMasked ?? null,
    updatedAt: null
  };

  return {
    id: profile.id,
    label: profile.label,
    provider: profile.provider ?? "openrouter",
    apiKeySource: credentials.source,
    apiKeyInput: "",
    apiKeyMasked: credentials.apiKeyMasked,
    hasApiKey: credentials.hasApiKey,
    credentialsUpdatedAt: credentials.updatedAt ?? null,
    modelId: profile.modelId ?? "",
    modelInfo: profile.modelInfo,
    providerOrder: profile.providerOrder.join(", "),
    allowFallbacks: profile.allowFallbacks,
    requireParameters: profile.requireParameters
  };
}

export function formatAiGenerationPlaygroundError(message: string): string {
  if (message.includes("profile was not found")) {
    return "選択したプロファイルが見つかりません。AI機能 > 設定 を開き直して、プロファイルを確認してください。";
  }

  if (message.includes("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE")) {
    return `${message} 保存済み AI設定の復号に必要です。サーバー側の .env.local などを確認してください。`;
  }

  if (message.includes("not configured") || message.includes("接続先")) {
    return `${message} AI機能 > 設定 でヒューリスティックに切り替えるか、LLM連携の接続先を確認してください。`;
  }

  if (
    message.includes("API key") ||
    message.includes("apiKey") ||
    message.includes("OpenRouter") ||
    message.includes("model")
  ) {
    return `${message} 選択中プロファイルの APIキー と モデル設定を確認してください。`;
  }

  return `生成テストの実行に失敗しました。${message}`;
}

export function getAiGenerationApiKeyStatusLabel(profile: AiGenerationProfileDraft): string {
  const baseLabel = profile.apiKeyMasked ?? (profile.hasApiKey ? "保存済み(復号不可)" : "未設定");
  return profile.apiKeySource === "shared" ? `共通設定を利用 (${baseLabel})` : baseLabel;
}

export function getAiGenerationSharedApiKeyStatusLabel(provider: AiGenerationSharedProviderDraft): string {
  return provider.apiKeyMasked ?? (provider.hasApiKey ? "保存済み(復号不可)" : "未設定");
}

export function resolveAiGenerationProfileDraftSelection(input: {
  profiles: AiGenerationProfileDraft[];
  selectedProfileId: string;
  editingProfileId: string;
  playgroundProfileId: string;
}): {
  defaultProfile: AiGenerationProfileDraft | null;
  editingProfile: AiGenerationProfileDraft | null;
  playgroundProfile: AiGenerationProfileDraft | null;
} {
  const defaultProfile =
    input.profiles.find((profile) => profile.id === input.selectedProfileId) ?? input.profiles[0] ?? null;
  const editingProfile =
    input.profiles.find((profile) => profile.id === input.editingProfileId) ?? defaultProfile;
  const playgroundProfile =
    input.profiles.find((profile) => profile.id === input.playgroundProfileId) ?? defaultProfile;

  return {
    defaultProfile,
    editingProfile,
    playgroundProfile
  };
}

export function partitionAiGenerationJobs<T extends AiGenerationJobLike>(jobs: T[] | null | undefined): {
  active: T[];
  failed: T[];
  completed: T[];
} {
  const nextJobs = jobs ?? [];

  return {
    active: nextJobs.filter((job) => job.status === "queued" || job.status === "running"),
    failed: nextJobs.filter((job) => job.status === "failed"),
    completed: nextJobs.filter((job) => job.status === "completed")
  };
}

export function getVisibleAiGenerationJobs<T extends AiGenerationJobLike>(input: {
  jobs: T[] | null | undefined;
  filter: AiGenerationJobFilter;
}): T[] {
  const buckets = partitionAiGenerationJobs(input.jobs);

  switch (input.filter) {
    case "failed":
      return buckets.failed;
    case "completed":
      return buckets.completed;
    default:
      return buckets.active;
  }
}

export function createAiGenerationProfileDraft(
  profileCount: number,
  createId: () => string = () => `profile-${Date.now().toString(36)}`
): AiGenerationProfileDraft {
  return {
    id: createId(),
    label: `Profile ${profileCount + 1}`,
    provider: "openrouter",
    apiKeySource: "shared",
    apiKeyInput: "",
    apiKeyMasked: null,
    hasApiKey: false,
    credentialsUpdatedAt: null,
    modelId: "",
    providerOrder: "",
    allowFallbacks: false,
    requireParameters: true
  };
}

export function removeAiGenerationProfileDraft(input: {
  profiles: AiGenerationProfileDraft[];
  profileId: string;
  selectedProfileId: string;
  editingProfileId: string;
  playgroundProfileId: string;
}): {
  profiles: AiGenerationProfileDraft[];
  selectedProfileId: string;
  editingProfileId: string;
  playgroundProfileId: string;
} {
  const profiles = input.profiles.filter((profile) => profile.id !== input.profileId);
  const fallbackProfileId = profiles[0]?.id ?? "";

  return {
    profiles,
    selectedProfileId: input.selectedProfileId === input.profileId ? fallbackProfileId : input.selectedProfileId,
    editingProfileId: input.editingProfileId === input.profileId ? fallbackProfileId : input.editingProfileId,
    playgroundProfileId: input.playgroundProfileId === input.profileId ? fallbackProfileId : input.playgroundProfileId
  };
}

export function createAiGenerationPlaygroundInitialProgress(): AiGenerationPlaygroundProgressLike {
  return {
    stage: "preparing",
    message: "進捗ストリームを開始しています。",
    progress: 5,
    step: 0,
    stepCount: 4
  };
}
