import { useCallback, useEffect, useMemo, useState } from "react";
import {
  deleteReaderAIProofread,
  fetchReaderAIProofread,
  generateReaderAIProofread
} from "../features/reader/api";
import type { EpisodeResponse, ReaderAIProofreadResponse } from "../features/reader/types";

export type ReaderAIProofreadState = "loading" | "not_generated" | "generating" | "ready" | "deleting" | "error";

export function useReaderAIProofread(
  episode: EpisodeResponse | null,
  onError: (message: string | null) => void
) {
  const [result, setResult] = useState<ReaderAIProofreadResponse | null>(null);
  const [state, setState] = useState<ReaderAIProofreadState>("loading");
  const [isShowingProofread, setIsShowingProofread] = useState(false);

  useEffect(() => {
    let active = true;
    setResult(null);
    setIsShowingProofread(false);
    if (!episode) {
      setState("not_generated");
      return () => {
        active = false;
      };
    }
    setState("loading");
    void fetchReaderAIProofread(episode.novelId, episode.episodeIndex)
      .then((next) => {
        if (!active) {
          return;
        }
        setResult(next);
        setState(next.status);
      })
      .catch((loadError) => {
        if (!active) {
          return;
        }
        setState("error");
        onError(loadError instanceof Error ? loadError.message : "AI校正結果の取得に失敗しました。");
      });
    return () => {
      active = false;
    };
  }, [episode, onError]);

  const generate = useCallback(async () => {
    if (!episode || state === "generating") {
      return;
    }
    setState("generating");
    try {
      const next = await generateReaderAIProofread(episode.novelId, episode.episodeIndex);
      setResult(next);
      setState("ready");
      setIsShowingProofread(true);
    } catch (generateError) {
      setState("error");
      onError(generateError instanceof Error ? generateError.message : "AI校正に失敗しました。");
    }
  }, [episode, onError, state]);

  const remove = useCallback(async () => {
    if (!episode || state === "deleting") {
      return;
    }
    setState("deleting");
    try {
      await deleteReaderAIProofread(episode.novelId, episode.episodeIndex);
      setResult(null);
      setIsShowingProofread(false);
      setState("not_generated");
    } catch (deleteError) {
      setState("error");
      onError(deleteError instanceof Error ? deleteError.message : "AI校正版の削除に失敗しました。");
    }
  }, [episode, onError, state]);

  const displayedEpisode = useMemo(() => {
    if (!episode || !isShowingProofread || result?.status !== "ready" || !result.readerDocument) {
      return episode;
    }
    return { ...episode, readerDocument: result.readerDocument };
  }, [episode, isShowingProofread, result]);

  return {
    displayedEpisode,
    generate,
    isShowingProofread,
    remove,
    result,
    setIsShowingProofread,
    state
  };
}
