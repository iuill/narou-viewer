import { type RefObject, useCallback, useRef } from "react";
import type { AiGenerationWorkspaceHostProps } from "../../AiGenerationWorkspaceHost";
import { getAiGenerationModeLabel } from "../../features/ai-generation/model";
import type { NovelSummary } from "../../features/library/types";
import type { RuntimeStatusResponse, RuntimeStatusService } from "../../features/runtime/types";
import { type AiGenerationPanelView, useAiGeneration } from "../../hooks/useAiGeneration";
import type { LibraryShellProps } from "../LibraryShell";

type UseAiGenerationWorkspaceModelInput = {
  isMobileLibraryViewport: boolean;
  isPanelOpen: boolean;
  isPaused: boolean;
  libraryReloadKey: number;
  novels: NovelSummary[];
  onClosePanel: () => void;
  onOpenNovelFromJob: (novelId: string) => void;
  panelRef: RefObject<HTMLDivElement | null>;
  refreshRuntimeStatus: () => Promise<RuntimeStatusResponse | null>;
  runtimeService: RuntimeStatusService | null;
  runtimeStatus: RuntimeStatusResponse | null;
  selectedNovelId: string | null;
  setIsPanelOpen: (value: boolean | ((current: boolean) => boolean)) => void;
};

export type AiGenerationWorkspaceModel = {
  menu: LibraryShellProps["aiGeneration"];
  readerAiAssistant: {
    isAvailable: boolean;
    unavailableMessage: string | null;
  };
  workspaceProps: AiGenerationWorkspaceHostProps;
  workspaceRef: RefObject<HTMLDivElement | null>;
};

export function useAiGenerationWorkspaceModel({
  isMobileLibraryViewport,
  isPanelOpen,
  isPaused,
  libraryReloadKey,
  novels,
  onClosePanel,
  onOpenNovelFromJob,
  refreshRuntimeStatus,
  runtimeService,
  runtimeStatus,
  selectedNovelId,
  panelRef,
  setIsPanelOpen
}: UseAiGenerationWorkspaceModelInput): AiGenerationWorkspaceModel {
  const aiGeneration = useAiGeneration({
    isPaused,
    libraryReloadKey,
    novels,
    refreshRuntimeStatus,
    runtimeStatus,
    selectedNovelId
  });
  const workspaceRef = useRef<HTMLDivElement | null>(null);
  const moveToWorkspace = useCallback(() => {
    if (!isMobileLibraryViewport) {
      return;
    }

    window.requestAnimationFrame(() => {
      const workspace = workspaceRef.current;
      workspace?.scrollIntoView({ behavior: "smooth", block: "start" });
      workspace?.focus({ preventScroll: true });
    });
  }, [isMobileLibraryViewport]);
  const handleOpenView = useCallback(
    async (view: AiGenerationPanelView) => {
      onClosePanel();
      const openPromise = aiGeneration.openView(view);
      moveToWorkspace();
      await openPromise;
    },
    [aiGeneration.openView, moveToWorkspace, onClosePanel]
  );

  const isReaderAiAssistantAvailable = aiGeneration.settings?.effectiveGenerationMode === "openrouter";
  const readerAiAssistantUnavailableMessage = isReaderAiAssistantAvailable
    ? null
    : aiGeneration.settingsError
      ? "読書AIの利用可否を確認できません。AI機能の設定を確認してください。"
      : aiGeneration.settings
        ? `読書AIはLLM連携時のみ利用できます。現在の生成方法は${getAiGenerationModeLabel(aiGeneration.settings.effectiveGenerationMode)}です。AI機能の設定でOpenRouter APIキーとモデルを設定してください。`
        : "読書AIの利用可否を確認しています。";

  return {
    menu: {
      activeSettingsProfileUpdatedAt: aiGeneration.activeSettingsProfile?.updatedAt ?? null,
      activeJobsCount: aiGeneration.activeJobs.length,
      failedJobsCount: aiGeneration.failedJobs.length,
      isOpen: isPanelOpen,
      jobsError: aiGeneration.jobsError,
      onOpenView: handleOpenView,
      panelRef,
      runtimeErrorDetail: runtimeService?.status === "error" ? runtimeService.detail : null,
      setIsOpen: setIsPanelOpen,
      settingsError: aiGeneration.settingsError,
      summaryLabel: aiGeneration.summaryLabel,
      triggerStatus: runtimeService?.status ?? "loading"
    },
    readerAiAssistant: {
      isAvailable: isReaderAiAssistantAvailable,
      unavailableMessage: readerAiAssistantUnavailableMessage
    },
    workspaceProps: {
      activeView: aiGeneration.activeView,
      aiGenerationNotice: aiGeneration.notice,
      aiGenerationSummaryLabel: aiGeneration.summaryLabel,
      jobsViewProps: {
        aiGenerationActiveJobsCount: aiGeneration.activeJobs.length,
        aiGenerationCompletedJobsCount: aiGeneration.completedJobs.length,
        aiGenerationFailedJobsCount: aiGeneration.failedJobs.length,
        aiGenerationJobFilter: aiGeneration.jobFilter,
        aiGenerationJobsError: aiGeneration.jobsError,
        hasAiGenerationJobs: aiGeneration.hasJobs,
        isAiGenerationJobsLoading: aiGeneration.isJobsLoading,
        onOpenNovelFromJob,
        onSetAiGenerationJobFilter: aiGeneration.setJobFilter,
        visibleAiGenerationJobs: aiGeneration.visibleJobs
      },
      onClose: () => aiGeneration.setActiveView(null),
      onOpenView: handleOpenView,
      playgroundViewProps: {
        aiGenerationPlaygroundBatchTimings: aiGeneration.playgroundBatchTimings,
        aiGenerationPlaygroundError: aiGeneration.playgroundError,
        aiGenerationPlaygroundMaxEpisodeIndex: aiGeneration.playgroundMaxEpisodeIndex,
        aiGenerationPlaygroundNovelId: aiGeneration.playgroundNovelId,
        aiGenerationPlaygroundProfileId: aiGeneration.playgroundProfileId,
        aiGenerationPlaygroundProgress: aiGeneration.playgroundProgress,
        aiGenerationPlaygroundPromptPreview: aiGeneration.playgroundPromptPreview,
        aiGenerationPlaygroundResponseJson: aiGeneration.playgroundResponseJson,
        aiGenerationPlaygroundResult: aiGeneration.playgroundResult,
        aiGenerationPlaygroundUpToEpisodeIndex: aiGeneration.playgroundUpToEpisodeIndex,
        aiGenerationProfileDrafts: aiGeneration.profileDrafts,
        isAiGenerationPlaygroundRunning: aiGeneration.isPlaygroundRunning,
        novels,
        onRunAiGenerationPlayground: aiGeneration.runPlayground,
        onSetAiGenerationPlaygroundNovelId: aiGeneration.setPlaygroundNovelId,
        onSetAiGenerationPlaygroundProfileId: aiGeneration.setPlaygroundProfileId,
        onSetAiGenerationPlaygroundUpToEpisodeIndex: aiGeneration.setPlaygroundUpToEpisodeIndex
      },
      settingsViewProps: {
        aiGenerationPreferredMode: aiGeneration.preferredMode,
        aiGenerationProfileDrafts: aiGeneration.profileDrafts,
        aiGenerationSettings: aiGeneration.settings,
        aiGenerationSettingsError: aiGeneration.settingsError,
        aiGenerationSharedGoogleBooksDraft: aiGeneration.sharedGoogleBooksDraft,
        aiGenerationSharedOpenRouterDraft: aiGeneration.sharedOpenRouterDraft,
        characterSummaryStrategyModelsDraft: aiGeneration.characterSummaryStrategyModelsDraft,
        defaultAiGenerationProfileDraft: aiGeneration.defaultProfileDraft,
        editingAiGenerationProfileDraft: aiGeneration.editingProfileDraft,
        editingAiGenerationProfileId: aiGeneration.editingProfileId,
        isAiGenerationModeSaving: aiGeneration.isModeSaving,
        isAiGenerationSettingsLoading: aiGeneration.isSettingsLoading,
        isAiGenerationSettingsSaving: aiGeneration.isSettingsSaving,
        onAddAiGenerationProfile: aiGeneration.addProfile,
        onAiGenerationPreferredModeChange: aiGeneration.changePreferredMode,
        onRemoveAiGenerationProfile: aiGeneration.removeProfile,
        onSaveAiGenerationSettings: aiGeneration.saveSettings,
        onSelectEditingAiGenerationProfile: aiGeneration.setEditingProfileId,
        onSetCharacterSummaryStrategyModelsDraft: aiGeneration.setCharacterSummaryStrategyModelsDraft,
        onSetSelectedAiGenerationProfileId: aiGeneration.setSelectedProfileId,
        onToggleAiGenerationHelp: aiGeneration.toggleHelp,
        onUpdateAiGenerationProfileDraft: aiGeneration.updateProfileDraft,
        onUpdateAiGenerationSharedGoogleBooksDraft: aiGeneration.updateSharedGoogleBooksDraft,
        onUpdateAiGenerationSharedOpenRouterDraft: aiGeneration.updateSharedOpenRouterDraft,
        openAiGenerationHelpKey: aiGeneration.openHelpKey,
        selectedAiGenerationProfileId: aiGeneration.selectedProfileId
      },
      usageViewProps: {
        aiUsage: aiGeneration.usage,
        aiUsageError: aiGeneration.usageError,
        isAiUsageLoading: aiGeneration.isUsageLoading
      }
    },
    workspaceRef
  };
}
