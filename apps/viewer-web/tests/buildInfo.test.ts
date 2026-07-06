import { afterEach, describe, expect, it, vi } from "vitest";
import {
  fetchLatestViewerBuildInfo,
  formatViewerBuildCommitDate,
  formatViewerBuildHash,
  formatViewerBuildSummary,
  isViewerBuildOutdated,
  type ViewerBuildInfo
} from "../src/buildInfo";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("buildInfo", () => {
  const formatDate = (value: string | null) => `date:${value}`;

  it("formats version, short hash, and commit date for the summary label", () => {
    const buildInfo: ViewerBuildInfo = {
      version: "0.1.0",
      gitHash: "abcdef1234567890",
      gitShortHash: "abcdef1",
      gitCommitDate: "2026-03-12T10:15:00.000Z"
    };

    expect(formatViewerBuildSummary(buildInfo)).toBe("v0.1.0 / abcdef1");
    expect(formatViewerBuildHash(buildInfo)).toBe("abcdef1234567890");
    expect(formatViewerBuildCommitDate(buildInfo, formatDate)).toBe("date:2026-03-12T10:15:00.000Z");
  });

  it("falls back cleanly when git metadata is unavailable", () => {
    const buildInfo: ViewerBuildInfo = {
      version: "0.1.0",
      gitHash: null,
      gitShortHash: null,
      gitCommitDate: null
    };

    expect(formatViewerBuildSummary(buildInfo)).toBe("v0.1.0");
    expect(formatViewerBuildHash(buildInfo)).toBe("未取得");
    expect(formatViewerBuildCommitDate(buildInfo, formatDate)).toBe("未取得");
  });

  it("detects when the served viewer build differs from the running bundle", () => {
    const current: ViewerBuildInfo = {
      version: "0.1.0",
      gitHash: "current",
      gitShortHash: "current",
      gitCommitDate: null
    };

    expect(isViewerBuildOutdated(current, { ...current })).toBe(false);
    expect(isViewerBuildOutdated(current, { ...current, gitHash: "latest", gitShortHash: "latest" })).toBe(true);
  });

  it("fetches the latest viewer build info without using cache", async () => {
    const latest: ViewerBuildInfo = {
      version: "0.1.0",
      gitHash: "abcdef1234567890",
      gitShortHash: "abcdef1",
      gitCommitDate: "2026-03-12T10:15:00.000Z"
    };
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(latest), {
        status: 200,
        headers: { "content-type": "application/json" }
      })
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(fetchLatestViewerBuildInfo()).resolves.toEqual(latest);
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringMatching(/^\/build-info\.json\?ts=\d+$/),
      expect.objectContaining({
        cache: "no-store",
        headers: { accept: "application/json" }
      })
    );
  });
});
