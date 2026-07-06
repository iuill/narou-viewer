export type ViewerBuildInfo = {
  version: string;
  gitHash: string | null;
  gitShortHash: string | null;
  gitCommitDate: string | null;
};

export const viewerBuildInfo: ViewerBuildInfo =
  typeof __APP_BUILD_INFO__ === "undefined"
    ? {
        version: "0.0.0",
        gitHash: null,
        gitShortHash: null,
        gitCommitDate: null
      }
    : __APP_BUILD_INFO__;

export function formatViewerBuildSummary(buildInfo: ViewerBuildInfo): string {
  const version = buildInfo.version.trim();
  const parts = [version.length > 0 ? `v${version}` : "version未取得"];

  if (buildInfo.gitShortHash) {
    parts.push(buildInfo.gitShortHash);
  }

  return parts.join(" / ");
}

export function formatViewerBuildHash(buildInfo: ViewerBuildInfo): string {
  return buildInfo.gitHash ?? "未取得";
}

export function formatViewerBuildCommitDate(
  buildInfo: ViewerBuildInfo,
  formatDate: (value: string | null) => string
): string {
  return buildInfo.gitCommitDate ? formatDate(buildInfo.gitCommitDate) : "未取得";
}

function normalizeBuildValue(value: string | null | undefined): string | null {
  const trimmed = value?.trim() ?? "";
  return trimmed.length > 0 ? trimmed : null;
}

function buildIdentity(buildInfo: ViewerBuildInfo): string {
  return [
    normalizeBuildValue(buildInfo.version) ?? "0.0.0",
    normalizeBuildValue(buildInfo.gitHash) ?? normalizeBuildValue(buildInfo.gitShortHash) ?? "unknown"
  ].join("+");
}

export function isViewerBuildOutdated(current: ViewerBuildInfo, latest: ViewerBuildInfo): boolean {
  return buildIdentity(current) !== buildIdentity(latest);
}

export async function fetchLatestViewerBuildInfo(): Promise<ViewerBuildInfo> {
  const response = await fetch(`/build-info.json?ts=${Date.now()}`, {
    cache: "no-store",
    headers: {
      accept: "application/json"
    }
  });

  if (!response.ok) {
    throw new Error("Failed to fetch viewer build info.");
  }

  return (await response.json()) as ViewerBuildInfo;
}
