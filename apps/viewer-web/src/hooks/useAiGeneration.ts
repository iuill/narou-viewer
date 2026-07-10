import { useCallback, useEffect, useMemo, useState } from "react";
import type { NovelSummary } from "../features/library/types";
import type { RuntimeStatusResponse } from "../features/runtime/types";
import type { AiGenerationHelpKey } from "../features/ai-generation/model";
import { useAiJobs } from "../features/ai-generation/useAiJobs";
import { useAiPlayground } from "../features/ai-generation/useAiPlayground";
import { useAiSettings } from "../features/ai-generation/useAiSettings";
import { useAiUsage } from "../features/ai-generation/useAiUsage";
import {
  removeAiGenerationProfileDraft,
  resolveAiGenerationProfileDraftSelection
} from "../features/ai-generation/model";

export type AiGenerationPanelView = "settings" | "playground" | "jobs" | "usage";
export type { AiGenerationHelpKey };

type UseAiGenerationOptions = {
  isPaused: boolean;
  libraryReloadKey: number;
  novels: NovelSummary[];
  refreshRuntimeStatus: () => Promise<RuntimeStatusResponse | null>;
  runtimeStatus: RuntimeStatusResponse | null;
  selectedNovelId: string | null;
};

export function useAiGeneration({
  isPaused,
  libraryReloadKey,
  novels,
  refreshRuntimeStatus,
  runtimeStatus,
  selectedNovelId
}: UseAiGenerationOptions) {
  const [activeView, setActiveView] = useState<AiGenerationPanelView | null>(null);
  const jobs = useAiJobs();
  const usage = useAiUsage();
  const settings = useAiSettings({ refreshRuntimeStatus, runtimeStatus });
  const playground = useAiPlayground({
    loadJobs: jobs.loadJobs,
    novels,
    selectedNovelId
  });
  const activeJobCount = jobs.activeJobs.length;
  const aiGenerationSettings = settings.settings;

  const profileSelection = useMemo(
    () =>
      resolveAiGenerationProfileDraftSelection({
        profiles: settings.profileDrafts,
        selectedProfileId: settings.selectedProfileId,
        editingProfileId: settings.editingProfileId,
        playgroundProfileId: playground.playgroundProfileId
      }),
    [playground.playgroundProfileId, settings.editingProfileId, settings.profileDrafts, settings.selectedProfileId]
  );

  const openView = useCallback(
    async (view: AiGenerationPanelView) => {
      setActiveView(view);
      settings.setNotice(null);

      if (view === "settings") {
        await settings.loadSettings();
        return;
      }

      if (view === "jobs") {
        await jobs.loadJobs();
        return;
      }

      if (view === "usage") {
        await usage.loadUsage();
        return;
      }

      await Promise.all([
        settings.loadSettings({ background: true }),
        jobs.loadJobs({ background: true }),
        usage.loadUsage({ background: true })
      ]);
    },
    [jobs, settings, usage]
  );

  const removeProfile = useCallback(
    (profileId: string) => {
      const nextState = removeAiGenerationProfileDraft({
        profiles: settings.profileDrafts,
        profileId,
        selectedProfileId: settings.selectedProfileId,
        editingProfileId: settings.editingProfileId,
        playgroundProfileId: playground.playgroundProfileId
      });

      settings.setProfileDrafts(nextState.profiles);
      settings.setSelectedProfileId(nextState.selectedProfileId);
      settings.setEditingProfileId(nextState.editingProfileId);
      playground.setPlaygroundProfileId(nextState.playgroundProfileId);
    },
    [playground, settings]
  );

  // biome-ignore lint/correctness/useExhaustiveDependencies: libraryReloadKey intentionally triggers settings and job reloads.
  useEffect(() => {
    void settings.loadSettings();
    void jobs.loadJobs();
  }, [libraryReloadKey, jobs.loadJobs, settings.loadSettings]);

  useEffect(() => {
    if (!aiGenerationSettings) {
      return;
    }

    const { nextDrafts, nextSelectedProfileId } = settings.applySettings(aiGenerationSettings);
    playground.setPlaygroundProfileId((current) =>
      playground.resolveProfileId(current, nextDrafts, nextSelectedProfileId)
    );
  }, [aiGenerationSettings, playground.resolveProfileId, playground.setPlaygroundProfileId, settings.applySettings]);

  useEffect(() => {
    if (isPaused) {
      return;
    }

    if (activeJobCount === 0 && activeView !== "jobs") {
      return;
    }

    const intervalId = window.setInterval(() => {
      if (!document.hidden) {
        void jobs.loadJobs({ background: true });
      }
    }, 4000);

    return () => {
      window.clearInterval(intervalId);
    };
  }, [activeJobCount, activeView, isPaused, jobs.loadJobs]);

  return {
    activeJobs: jobs.activeJobs,
    activeSettingsProfile: settings.activeSettingsProfile,
    activeView,
    addProfile: settings.addProfile,
    changePreferredMode: settings.changePreferredMode,
    extractionStrategyModelsDraft: settings.extractionStrategyModelsDraft,
    completedJobs: jobs.completedJobs,
    defaultProfileDraft: profileSelection.defaultProfile,
    editingProfileDraft: profileSelection.editingProfile,
    editingProfileId: settings.editingProfileId,
    failedJobs: jobs.failedJobs,
    hasJobs: jobs.hasJobs,
    isJobsLoading: jobs.isJobsLoading,
    isModeSaving: settings.isModeSaving,
    isPlaygroundRunning: playground.isPlaygroundRunning,
    isSettingsLoading: settings.isSettingsLoading,
    isSettingsSaving: settings.isSettingsSaving,
    isUsageLoading: usage.isUsageLoading,
    jobFilter: jobs.jobFilter,
    jobs: jobs.jobs,
    jobsError: jobs.jobsError,
    loadJobs: jobs.loadJobs,
    loadSettings: settings.loadSettings,
    loadUsage: usage.loadUsage,
    notice: settings.notice,
    openHelpKey: settings.openHelpKey,
    openView,
    playgroundBatchTimings: playground.playgroundBatchTimings,
    playgroundError: playground.playgroundError,
    playgroundMaxEpisodeIndex: playground.playgroundMaxEpisodeIndex,
    playgroundNovelId: playground.playgroundNovelId,
    playgroundProfileDraft: profileSelection.playgroundProfile,
    playgroundProfileId: playground.playgroundProfileId,
    playgroundProgress: playground.playgroundProgress,
    playgroundPromptPreview: playground.playgroundPromptPreview,
    playgroundResponseJson: playground.playgroundResponseJson,
    playgroundResult: playground.playgroundResult,
    playgroundUpToEpisodeIndex: playground.playgroundUpToEpisodeIndex,
    preferredMode: settings.preferredMode,
    profileDrafts: settings.profileDrafts,
    removeProfile,
    runPlayground: playground.runPlayground,
    saveSettings: settings.saveSettings,
    selectedProfileId: settings.selectedProfileId,
    setActiveView,
    setExtractionStrategyModelsDraft: settings.setExtractionStrategyModelsDraft,
    setEditingProfileId: settings.setEditingProfileId,
    setJobFilter: jobs.setJobFilter,
    setPlaygroundNovelId: playground.setPlaygroundNovelId,
    setPlaygroundProfileId: playground.setPlaygroundProfileId,
    setPlaygroundUpToEpisodeIndex: playground.setPlaygroundUpToEpisodeIndex,
    setSelectedProfileId: settings.setSelectedProfileId,
    settings: settings.settings,
    settingsError: settings.settingsError,
    sharedGoogleBooksDraft: settings.sharedGoogleBooksDraft,
    sharedOpenRouterDraft: settings.sharedOpenRouterDraft,
    summaryLabel: settings.summaryLabel,
    toggleHelp: settings.toggleHelp,
    triggerSummaryLabel: settings.triggerSummaryLabel,
    updateProfileDraft: settings.updateProfileDraft,
    updateSharedGoogleBooksDraft: settings.updateSharedGoogleBooksDraft,
    updateSharedOpenRouterDraft: settings.updateSharedOpenRouterDraft,
    usage: usage.usage,
    usageError: usage.usageError,
    visibleJobs: jobs.visibleJobs
  };
}
