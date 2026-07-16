import { describe, expect, it } from "vitest";
import {
  isCharacterSummaryActiveJob,
  isCharacterSummaryCompletedJob,
  isCharacterSummaryRequestAllowed,
  resolveCharacterSummaryRefreshTarget
} from "../src/characterSummaryUtils";

describe("characterSummaryUtils", () => {
  it("prefers the requested up-to index when it stays within the spoiler boundary", () => {
    expect(
      resolveCharacterSummaryRefreshTarget({
        defaultUpToEpisodeIndex: "17",
        requestedUpToEpisodeIndex: "5"
      })
    ).toBe("5");
  });

  it("falls back to the default up-to index when the request is missing or too large", () => {
    expect(
      resolveCharacterSummaryRefreshTarget({
        defaultUpToEpisodeIndex: "17",
        requestedUpToEpisodeIndex: null
      })
    ).toBe("17");
    expect(
      resolveCharacterSummaryRefreshTarget({
        defaultUpToEpisodeIndex: "17",
        requestedUpToEpisodeIndex: "20"
      })
    ).toBe("17");
  });

  it("returns null when the current episode is the first chapter", () => {
    expect(
      resolveCharacterSummaryRefreshTarget({
        defaultUpToEpisodeIndex: null,
        requestedUpToEpisodeIndex: "1"
      })
    ).toBeNull();
  });

  it("allows generation requests only within 1..defaultUpToEpisodeIndex", () => {
    expect(
      isCharacterSummaryRequestAllowed({
        defaultUpToEpisodeIndex: "17",
        requestedUpToEpisodeIndex: "1"
      })
    ).toBe(true);
    expect(
      isCharacterSummaryRequestAllowed({
        defaultUpToEpisodeIndex: "17",
        requestedUpToEpisodeIndex: "0"
      })
    ).toBe(false);
    expect(
      isCharacterSummaryRequestAllowed({
        defaultUpToEpisodeIndex: "17",
        requestedUpToEpisodeIndex: "20"
      })
    ).toBe(false);
    expect(
      isCharacterSummaryRequestAllowed({
        defaultUpToEpisodeIndex: null,
        requestedUpToEpisodeIndex: "1"
      })
    ).toBe(false);
  });

  it("treats failed and incompatible jobs as finished, not active", () => {
    expect(isCharacterSummaryActiveJob("queued")).toBe(true);
    expect(isCharacterSummaryActiveJob("running")).toBe(true);
    expect(isCharacterSummaryActiveJob("completed")).toBe(false);
    expect(isCharacterSummaryActiveJob("failed")).toBe(false);
    expect(isCharacterSummaryActiveJob("incompatible")).toBe(false);

    expect(isCharacterSummaryCompletedJob("completed")).toBe(true);
    expect(isCharacterSummaryCompletedJob("failed")).toBe(true);
    expect(isCharacterSummaryCompletedJob("incompatible")).toBe(true);
    expect(isCharacterSummaryCompletedJob("queued")).toBe(false);
    expect(isCharacterSummaryCompletedJob("running")).toBe(false);
  });
});
