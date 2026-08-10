import { useCallback, useEffect, useMemo, useRef, useState } from "react";
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
  const episodeKey = episode ? `${episode.novelId}\n${episode.episodeIndex}\n${episode.contentEtag}` : null;
  const episodeIdentity = episode ? `${episode.novelId}\n${episode.episodeIndex}` : null;
  const currentEpisodeKeyRef = useRef(episodeKey);
  const previousEpisodeIdentityRef = useRef<string | null>(null);
  const mountedRef = useRef(true);
  currentEpisodeKeyRef.current = episodeKey;

  useEffect(
    () => () => {
      mountedRef.current = false;
    },
    []
  );

  useEffect(() => {
    let active = true;
    const isSameEpisode = previousEpisodeIdentityRef.current === episodeIdentity;
    previousEpisodeIdentityRef.current = episodeIdentity;
    setResult(null);
    if (!isSameEpisode) {
      setIsShowingProofread(false);
    }
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
        if (next.status !== "ready") {
          setIsShowingProofread(false);
        }
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
  }, [episode, episodeIdentity, onError]);

  const generate = useCallback(async () => {
    if (!episode || state === "generating" || state === "deleting") {
      return;
    }
    const requestEpisodeKey = episodeKey;
    onError(null);
    setState("generating");
    try {
      const next = await generateReaderAIProofread(episode.novelId, episode.episodeIndex);
      if (!mountedRef.current || currentEpisodeKeyRef.current !== requestEpisodeKey) {
        return;
      }
      setResult(next);
      setState("ready");
      setIsShowingProofread(true);
    } catch (generateError) {
      if (!mountedRef.current || currentEpisodeKeyRef.current !== requestEpisodeKey) {
        return;
      }
      setState("error");
      onError(generateError instanceof Error ? generateError.message : "AI校正に失敗しました。");
    }
  }, [episode, episodeKey, onError, state]);

  const remove = useCallback(async () => {
    if (!episode || state === "generating" || state === "deleting") {
      return;
    }
    const requestEpisodeKey = episodeKey;
    onError(null);
    setState("deleting");
    try {
      await deleteReaderAIProofread(episode.novelId, episode.episodeIndex);
      if (!mountedRef.current || currentEpisodeKeyRef.current !== requestEpisodeKey) {
        return;
      }
      setResult(null);
      setIsShowingProofread(false);
      setState("not_generated");
    } catch (deleteError) {
      if (!mountedRef.current || currentEpisodeKeyRef.current !== requestEpisodeKey) {
        return;
      }
      setState("error");
      onError(deleteError instanceof Error ? deleteError.message : "AI校正版の削除に失敗しました。");
    }
  }, [episode, episodeKey, onError, state]);

  const displayedEpisode = useMemo(() => {
    if (
      !episode ||
      !isShowingProofread ||
      result?.status !== "ready" ||
      !result.readerDocument ||
      result.novelId !== episode.novelId ||
      result.episodeIndex !== episode.episodeIndex
    ) {
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
