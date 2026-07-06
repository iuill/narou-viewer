import { useCallback, useEffect, useMemo, useState } from "react";
import { postAiGenerationPlaygroundStream, readAiGenerationPlaygroundStream } from "./api";
import type {
  AiGenerationPlaygroundBatchTiming,
  AiGenerationPlaygroundProgress,
  AiGenerationPlaygroundPromptPreview,
  AiGenerationPlaygroundRequest,
  AiGenerationPlaygroundResponse
} from "./types";
import type { NovelSummary } from "../library/types";
import { createAiGenerationPlaygroundInitialProgress, type AiGenerationProfileDraft } from "./model";

const AI_GENERATION_PLAYGROUND_DEFAULT_EPISODE_INDEX = 3;

export function useAiPlayground({
  loadJobs,
  novels,
  selectedNovelId
}: {
  loadJobs: (options?: { background?: boolean }) => Promise<void>;
  novels: NovelSummary[];
  selectedNovelId: string | null;
}) {
  const [playgroundNovelId, setPlaygroundNovelId] = useState("");
  const [playgroundProfileId, setPlaygroundProfileId] = useState("");
  const [playgroundUpToEpisodeIndex, setPlaygroundUpToEpisodeIndex] = useState(
    String(AI_GENERATION_PLAYGROUND_DEFAULT_EPISODE_INDEX)
  );
  const [playgroundPromptPreview, setPlaygroundPromptPreview] =
    useState<AiGenerationPlaygroundPromptPreview | null>(null);
  const [playgroundBatchTimings, setPlaygroundBatchTimings] = useState<AiGenerationPlaygroundBatchTiming[]>([]);
  const [playgroundResult, setPlaygroundResult] = useState<AiGenerationPlaygroundResponse | null>(null);
  const [playgroundProgress, setPlaygroundProgress] = useState<AiGenerationPlaygroundProgress | null>(null);
  const [playgroundError, setPlaygroundError] = useState<string | null>(null);
  const [isPlaygroundRunning, setIsPlaygroundRunning] = useState(false);

  const playgroundNovel = useMemo(
    () => novels.find((novel) => novel.novelId === playgroundNovelId) ?? null,
    [playgroundNovelId, novels]
  );
  const playgroundResponseJson = useMemo(() => (playgroundResult ? JSON.stringify(playgroundResult, null, 2) : ""), [playgroundResult]);
  const playgroundMaxEpisodeIndex = playgroundNovel ? String(playgroundNovel.totalEpisodes) : "";

  const runPlayground = useCallback(async () => {
    if (!playgroundNovelId) {
      setPlaygroundError("作品を選択してください。");
      return;
    }

    if (!/^\d+$/.test(playgroundUpToEpisodeIndex)) {
      setPlaygroundError("対象話数を入力してください。");
      return;
    }

    setIsPlaygroundRunning(true);
    setPlaygroundError(null);

    try {
      const requestPayload = {
        novelId: playgroundNovelId,
        profileId: playgroundProfileId || undefined,
        upToEpisodeIndex: playgroundUpToEpisodeIndex
      } satisfies AiGenerationPlaygroundRequest;

      setPlaygroundPromptPreview(null);
      setPlaygroundBatchTimings([]);
      setPlaygroundResult(null);
      setPlaygroundProgress(createAiGenerationPlaygroundInitialProgress());

      const response = await postAiGenerationPlaygroundStream(requestPayload);
      const nextResult = await readAiGenerationPlaygroundStream(response, {
        onProgress: setPlaygroundProgress,
        onPromptPreview: setPlaygroundPromptPreview,
        onBatchTimings: setPlaygroundBatchTimings
      });
      setPlaygroundResult(nextResult);
      await loadJobs({ background: true });
    } catch (runError) {
      setPlaygroundError(runError instanceof Error ? runError.message : "Unknown error");
      setPlaygroundResult(null);
    } finally {
      setPlaygroundProgress(null);
      setIsPlaygroundRunning(false);
    }
  }, [loadJobs, playgroundNovelId, playgroundProfileId, playgroundUpToEpisodeIndex]);

  const resolveProfileId = useCallback((current: string, profiles: AiGenerationProfileDraft[], selectedProfileId: string) => {
    return current && profiles.some((profile) => profile.id === current) ? current : selectedProfileId;
  }, []);

  useEffect(() => {
    if (playgroundNovelId) {
      return;
    }

    const nextNovelId = selectedNovelId ?? novels[0]?.novelId ?? "";
    if (nextNovelId) {
      setPlaygroundNovelId(nextNovelId);
    }
  }, [novels, playgroundNovelId, selectedNovelId]);

  useEffect(() => {
    if (!playgroundNovel) {
      setPlaygroundUpToEpisodeIndex("");
      return;
    }

    setPlaygroundUpToEpisodeIndex((current) => {
      if (/^\d+$/.test(current) && Number.parseInt(current, 10) <= playgroundNovel.totalEpisodes) {
        return current;
      }

      return String(
        Math.max(1, Math.min(AI_GENERATION_PLAYGROUND_DEFAULT_EPISODE_INDEX, playgroundNovel.totalEpisodes))
      );
    });
  }, [playgroundNovel]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: playground target changes should reset transient run output.
  useEffect(() => {
    setPlaygroundResult(null);
    setPlaygroundError(null);
    setPlaygroundPromptPreview(null);
    setPlaygroundBatchTimings([]);
    setPlaygroundProgress(null);
  }, [playgroundNovelId, playgroundUpToEpisodeIndex]);

  return {
    isPlaygroundRunning,
    playgroundBatchTimings,
    playgroundError,
    playgroundMaxEpisodeIndex,
    playgroundNovelId,
    playgroundProfileId,
    playgroundProgress,
    playgroundPromptPreview,
    playgroundResponseJson,
    playgroundResult,
    playgroundUpToEpisodeIndex,
    resolveProfileId,
    runPlayground,
    setPlaygroundNovelId,
    setPlaygroundProfileId,
    setPlaygroundUpToEpisodeIndex
  };
}
