import { useCallback, useMemo, useState } from "react";
import { fetchRuntimeStatus } from "../features/runtime/api";
import type { RuntimeStatusResponse, RuntimeStatusService } from "../features/runtime/types";

const AI_GENERATION_SERVICE_ID = "go-internal-ai";
const FETCHER_SERVICE_ID = "novel-fetcher";

export function getRuntimeStatusLabel(status: RuntimeStatusResponse | null): string {
  if (status?.status === "error") {
    return "要確認";
  }

  if (status?.status === "warn") {
    return "一部制限あり";
  }

  if (status?.status === "ok") {
    return "正常";
  }

  return "確認中";
}

export function findRuntimeService(
  status: RuntimeStatusResponse | null,
  serviceId: string
): RuntimeStatusService | null {
  return status?.services.find((service) => service.id === serviceId) ?? null;
}

type UseRuntimeStatusOptions = {
  fetchStatus?: () => Promise<RuntimeStatusResponse>;
};

type UseRuntimeStatusResult = {
  status: RuntimeStatusResponse | null;
  statusLabel: string;
  aiGenerationService: RuntimeStatusService | null;
  fetcherService: RuntimeStatusService | null;
  setStatus: (status: RuntimeStatusResponse) => void;
  refreshStatus: () => Promise<RuntimeStatusResponse>;
};

export function useRuntimeStatus(options: UseRuntimeStatusOptions = {}): UseRuntimeStatusResult {
  const fetchStatus = options.fetchStatus ?? fetchRuntimeStatus;
  const [status, setStatus] = useState<RuntimeStatusResponse | null>(null);

  const refreshStatus = useCallback(async () => {
    const nextStatus = await fetchStatus();
    setStatus(nextStatus);
    return nextStatus;
  }, [fetchStatus]);

  const statusLabel = useMemo(() => getRuntimeStatusLabel(status), [status]);
  const aiGenerationService = useMemo(() => findRuntimeService(status, AI_GENERATION_SERVICE_ID), [status]);
  const fetcherService = useMemo(() => findRuntimeService(status, FETCHER_SERVICE_ID), [status]);

  return {
    status,
    statusLabel,
    aiGenerationService,
    fetcherService,
    setStatus,
    refreshStatus
  };
}
