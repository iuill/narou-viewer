export type RuntimeStatusLevel = "ok" | "warn" | "error";

export type RuntimeStatusService = {
  id: string;
  label: string;
  status: RuntimeStatusLevel;
  summary: string;
  detail: string;
  versionInfo?: {
    current: string | null;
    latest: string | null;
    updateAvailable: boolean;
  };
};

export type RuntimeStatusResponse = {
  status: RuntimeStatusLevel;
  checkedAt: string;
  services: RuntimeStatusService[];
};
