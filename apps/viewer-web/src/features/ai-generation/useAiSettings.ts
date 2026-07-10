import { useCallback, useMemo, useState } from "react";
import { fetchAiGenerationSettings, putAiGenerationPreferredMode, putAiGenerationSettings } from "./api";
import type { AiGenerationSettingsResponse } from "./types";
import type { RuntimeStatusResponse } from "../runtime/types";
import {
  createAiGenerationProfileDraft,
  getActiveAiGenerationSettingsProfile,
  getAiGenerationSummary,
  getAiGenerationTriggerSummary,
  toAiGenerationProfileDraft,
  toAiGenerationSharedProviderDraft,
  type ExtractionStrategyModelsDraft,
  type AiGenerationHelpKey,
  type AiGenerationProfileDraft,
  type AiGenerationSharedProviderDraft
} from "./model";

export function useAiSettings({
  refreshRuntimeStatus,
  runtimeStatus
}: {
  refreshRuntimeStatus: () => Promise<RuntimeStatusResponse | null>;
  runtimeStatus: RuntimeStatusResponse | null;
}) {
  const [settings, setSettings] = useState<AiGenerationSettingsResponse | null>(null);
  const [settingsError, setSettingsError] = useState<string | null>(null);
  const [isSettingsLoading, setIsSettingsLoading] = useState(false);
  const [isSettingsSaving, setIsSettingsSaving] = useState(false);
  const [isModeSaving, setIsModeSaving] = useState(false);
  const [preferredMode, setPreferredMode] = useState<"llm" | "heuristic">("heuristic");
  const [sharedOpenRouterDraft, setSharedOpenRouterDraft] = useState<AiGenerationSharedProviderDraft>({
    provider: "openrouter",
    apiKeyInput: "",
    apiKeyMasked: null,
    hasApiKey: false,
    updatedAt: null
  });
  const [sharedGoogleBooksDraft, setSharedGoogleBooksDraft] = useState<AiGenerationSharedProviderDraft>({
    provider: "googleBooks",
    apiKeyInput: "",
    apiKeyMasked: null,
    hasApiKey: false,
    updatedAt: null
  });
  const [profileDrafts, setProfileDrafts] = useState<AiGenerationProfileDraft[]>([]);
  const [extractionStrategyModelsDraft, setExtractionStrategyModelsDraft] = useState<ExtractionStrategyModelsDraft>({
    nameDiscoveryModelId: ""
  });
  const [selectedProfileId, setSelectedProfileId] = useState("");
  const [editingProfileId, setEditingProfileId] = useState("");
  const [openHelpKey, setOpenHelpKey] = useState<AiGenerationHelpKey | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const summaryLabel = useMemo(() => getAiGenerationSummary(settings, runtimeStatus), [settings, runtimeStatus]);
  const triggerSummaryLabel = useMemo(
    () => getAiGenerationTriggerSummary(settings, runtimeStatus),
    [settings, runtimeStatus]
  );
  const activeSettingsProfile = useMemo(() => getActiveAiGenerationSettingsProfile(settings), [settings]);

  const loadSettings = useCallback(async (options?: { background?: boolean }) => {
    const isBackground = options?.background === true;
    if (!isBackground) {
      setIsSettingsLoading(true);
    }

    try {
      const nextSettings = await fetchAiGenerationSettings();
      setSettings(nextSettings);
      setSettingsError(null);
    } catch (loadError) {
      setSettingsError(loadError instanceof Error ? loadError.message : "Unknown error");
    } finally {
      if (!isBackground) {
        setIsSettingsLoading(false);
      }
    }
  }, []);

  const applySettings = useCallback((nextSettings: AiGenerationSettingsResponse) => {
    setPreferredMode(nextSettings.preferredMode);
    setSharedOpenRouterDraft(toAiGenerationSharedProviderDraft(nextSettings.settings.sharedProviders?.openrouter));
    setSharedGoogleBooksDraft(
      toAiGenerationSharedProviderDraft(nextSettings.settings.sharedProviders?.googleBooks, "googleBooks")
    );
    const nextDrafts = nextSettings.settings.profiles.map((profile) => toAiGenerationProfileDraft(profile));
    setProfileDrafts(nextDrafts);
    setExtractionStrategyModelsDraft({
      nameDiscoveryModelId: nextSettings.settings.extractionStrategyModels?.nameDiscoveryModelId ?? ""
    });
    const nextSelectedProfileId = nextSettings.settings.selectedProfileId ?? nextSettings.settings.profiles[0]?.id ?? "";
    setSelectedProfileId(nextSelectedProfileId);
    setEditingProfileId((current) => (current && nextDrafts.some((profile) => profile.id === current) ? current : nextSelectedProfileId));
    return { nextDrafts, nextSelectedProfileId };
  }, []);

  const updateProfileDraft = useCallback(
    (profileId: string, updater: (current: AiGenerationProfileDraft) => AiGenerationProfileDraft) => {
      setProfileDrafts((current) => current.map((profile) => (profile.id === profileId ? updater(profile) : profile)));
    },
    []
  );

  const updateSharedOpenRouterDraft = useCallback(
    (updater: (current: AiGenerationSharedProviderDraft) => AiGenerationSharedProviderDraft) => {
      setSharedOpenRouterDraft((current) => updater(current));
    },
    []
  );

  const updateSharedGoogleBooksDraft = useCallback(
    (updater: (current: AiGenerationSharedProviderDraft) => AiGenerationSharedProviderDraft) => {
      setSharedGoogleBooksDraft((current) => updater(current));
    },
    []
  );

  const addProfile = useCallback(() => {
    const nextProfile = createAiGenerationProfileDraft(profileDrafts.length);

    setProfileDrafts((current) => [...current, nextProfile]);
    setEditingProfileId(nextProfile.id);
  }, [profileDrafts.length]);

  const toggleHelp = useCallback((helpKey: AiGenerationHelpKey) => {
    setOpenHelpKey((current) => (current === helpKey ? null : helpKey));
  }, []);

  const changePreferredMode = useCallback(
    async (nextMode: "llm" | "heuristic") => {
      if (nextMode === preferredMode || isModeSaving) {
        return;
      }

      const previousMode = preferredMode;
      setPreferredMode(nextMode);
      setIsModeSaving(true);
      setSettingsError(null);
      setNotice(null);

      try {
        const payload = await putAiGenerationPreferredMode(nextMode);
        setPreferredMode(payload.preferredMode);
        if (payload.preferredMode === "heuristic") {
          setSettingsError(null);
        }
        try {
          await refreshRuntimeStatus();
        } catch (runtimeStatusError) {
          console.warn("Failed to refresh runtime status after updating AI generation mode.", runtimeStatusError);
        }
        setNotice("連携モードを更新しました。");
      } catch (saveError) {
        setPreferredMode(previousMode);
        setSettingsError(saveError instanceof Error ? saveError.message : "Unknown error");
      } finally {
        setIsModeSaving(false);
      }
    },
    [isModeSaving, preferredMode, refreshRuntimeStatus]
  );

  const saveSettings = useCallback(async () => {
    setIsSettingsSaving(true);
    setSettingsError(null);
    setNotice(null);

    try {
      if (profileDrafts.length === 0) {
        throw new Error("少なくとも1つのプロファイルが必要です。");
      }

      const nextSettings = await putAiGenerationSettings({
        selectedProfileId,
        sharedProviders: {
          openrouter: {
            ...(sharedOpenRouterDraft.apiKeyInput.trim().length > 0
              ? { apiKey: sharedOpenRouterDraft.apiKeyInput.trim() }
              : {})
          },
          googleBooks: {
            ...(sharedGoogleBooksDraft.apiKeyInput.trim().length > 0
              ? { apiKey: sharedGoogleBooksDraft.apiKeyInput.trim() }
              : {})
          }
        },
        profiles: profileDrafts.map((profile) => ({
          id: profile.id,
          label: profile.label.trim(),
          provider: profile.provider,
          credentials: {
            source: profile.apiKeySource,
            ...(profile.apiKeySource === "custom" && profile.apiKeyInput.trim().length > 0
              ? { apiKey: profile.apiKeyInput.trim() }
              : {})
          },
          modelId: profile.modelId.trim() || null,
          providerOrder: profile.providerOrder,
          allowFallbacks: profile.allowFallbacks,
          requireParameters: profile.requireParameters
        })),
        extractionStrategyModels: {
          nameDiscoveryModelId: extractionStrategyModelsDraft.nameDiscoveryModelId.trim() || null
        }
      });
      setSettings(nextSettings);
      try {
        await refreshRuntimeStatus();
      } catch (runtimeStatusError) {
        console.warn("Failed to refresh runtime status after saving AI generation settings.", runtimeStatusError);
      }
      setNotice("プロファイル設定を保存しました。");
    } catch (saveError) {
      setSettingsError(saveError instanceof Error ? saveError.message : "Unknown error");
    } finally {
      setIsSettingsSaving(false);
    }
  }, [
    extractionStrategyModelsDraft.nameDiscoveryModelId,
    profileDrafts,
    refreshRuntimeStatus,
    selectedProfileId,
    sharedGoogleBooksDraft.apiKeyInput,
    sharedOpenRouterDraft.apiKeyInput
  ]);

  return {
    activeSettingsProfile,
    addProfile,
    applySettings,
    changePreferredMode,
    extractionStrategyModelsDraft,
    editingProfileId,
    isModeSaving,
    isSettingsLoading,
    isSettingsSaving,
    loadSettings,
    notice,
    openHelpKey,
    preferredMode,
    profileDrafts,
    saveSettings,
    selectedProfileId,
    setEditingProfileId,
    setNotice,
    setProfileDrafts,
    setSelectedProfileId,
    setExtractionStrategyModelsDraft,
    settings,
    settingsError,
    sharedGoogleBooksDraft,
    sharedOpenRouterDraft,
    summaryLabel,
    toggleHelp,
    triggerSummaryLabel,
    updateProfileDraft,
    updateSharedGoogleBooksDraft,
    updateSharedOpenRouterDraft
  };
}
