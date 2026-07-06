import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createPublicationEntry, fetchNovelPublications, putPublicationDisplayCover, putPublicationEntry } from "../features/publications/api";
import type {
  NovelPublicationsResponse,
  PublicationDisplayCoverRequest,
  PublicationEntry,
  PublicationEntryRequest
} from "../features/publications/types";

type UseNovelPublicationsOptions = {
  novelId: string | null;
  onError: (message: string | null) => void;
  onSaved?: () => void;
};

type UseNovelPublicationsResult = {
  data: NovelPublicationsResponse | null;
  displayCoverEntryId: string;
  entries: PublicationEntry[];
  isLoading: boolean;
  savingEntryId: string | null;
  createEntry: (body: PublicationEntryRequest) => Promise<void>;
  saveEntry: (entryId: string, body: PublicationEntryRequest) => Promise<void>;
  setDisplayCover: (body: PublicationDisplayCoverRequest) => Promise<void>;
};

function toErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "書籍情報の処理に失敗しました。";
}

export function useNovelPublications({ novelId, onError, onSaved }: UseNovelPublicationsOptions): UseNovelPublicationsResult {
  const [data, setData] = useState<NovelPublicationsResponse | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [savingEntryId, setSavingEntryId] = useState<string | null>(null);
  const activeNovelIdRef = useRef<string | null>(novelId);

  useEffect(() => {
    activeNovelIdRef.current = novelId;
    if (!novelId) {
      setData(null);
      setIsLoading(false);
      setSavingEntryId(null);
      return;
    }
    let cancelled = false;
    setSavingEntryId(null);
    setIsLoading(true);
    void fetchNovelPublications(novelId)
      .then((nextData) => {
        if (!cancelled) {
          setData(nextData);
        }
      })
      .catch((error) => {
        if (!cancelled) {
          onError(toErrorMessage(error));
        }
      })
      .finally(() => {
        if (!cancelled) {
          setIsLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [novelId, onError]);

  const runMutation = useCallback(
    async (savingId: string, mutate: (activeNovelId: string) => Promise<NovelPublicationsResponse>) => {
      if (!novelId) {
        return;
      }
      const activeNovelId = novelId;
      setSavingEntryId(savingId);
      onError(null);
      try {
        const nextData = await mutate(activeNovelId);
        if (activeNovelIdRef.current === activeNovelId && nextData.novelId === activeNovelId) {
          setData(nextData);
          onSaved?.();
        }
      } catch (error) {
        if (activeNovelIdRef.current === activeNovelId) {
          onError(toErrorMessage(error));
        }
      } finally {
        if (activeNovelIdRef.current === activeNovelId) {
          setSavingEntryId(null);
        }
      }
    },
    [novelId, onError, onSaved]
  );

  const createEntry = useCallback(
    async (body: PublicationEntryRequest) => {
      await runMutation(`create-${body.kind ?? "entry"}`, (activeNovelId) => createPublicationEntry(activeNovelId, body));
    },
    [runMutation]
  );

  const saveEntry = useCallback(
    async (entryId: string, body: PublicationEntryRequest) => {
      await runMutation(entryId, (activeNovelId) => putPublicationEntry(activeNovelId, entryId, body));
    },
    [runMutation]
  );

  const setDisplayCover = useCallback(
    async (body: PublicationDisplayCoverRequest) => {
      await runMutation("display-cover", (activeNovelId) => putPublicationDisplayCover(activeNovelId, body));
    },
    [runMutation]
  );

  const entries = useMemo(() => data?.entries ?? [], [data]);

  return {
    data,
    displayCoverEntryId: data?.displayCoverEntryId ?? "",
    entries,
    isLoading,
    savingEntryId,
    createEntry,
    saveEntry,
    setDisplayCover
  };
}
