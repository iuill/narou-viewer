import { useCallback, useState } from "react";
import { fetchAiUsage } from "./api";
import type { AiUsageResponse } from "./types";

export function useAiUsage() {
  const [usage, setUsage] = useState<AiUsageResponse | null>(null);
  const [usageError, setUsageError] = useState<string | null>(null);
  const [isUsageLoading, setIsUsageLoading] = useState(false);

  const loadUsage = useCallback(async (options?: { background?: boolean }) => {
    const isBackground = options?.background === true;
    if (!isBackground) {
      setIsUsageLoading(true);
    }

    try {
      const nextUsage = await fetchAiUsage();
      setUsage(nextUsage);
      setUsageError(null);
    } catch (loadError) {
      setUsage(null);
      setUsageError(loadError instanceof Error ? loadError.message : "Unknown error");
    } finally {
      if (!isBackground) {
        setIsUsageLoading(false);
      }
    }
  }, []);

  return {
    isUsageLoading,
    loadUsage,
    usage,
    usageError
  };
}
