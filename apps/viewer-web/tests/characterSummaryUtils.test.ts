import { describe, expect, it } from "vitest";
import {
  isCharacterSummaryActiveJob,
  isCharacterSummaryCompletedJob,
  isCharacterSummaryProcessingJob,
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

  it("keeps paused and interrupted jobs actionable without polling them as processing", () => {
    expect(isCharacterSummaryActiveJob("queued")).toBe(true);
    expect(isCharacterSummaryActiveJob("running")).toBe(true);
    expect(isCharacterSummaryActiveJob("pausing")).toBe(true);
    expect(isCharacterSummaryActiveJob("paused")).toBe(true);
    expect(isCharacterSummaryActiveJob("interrupted")).toBe(true);
    expect(isCharacterSummaryActiveJob("completed")).toBe(false);
    expect(isCharacterSummaryActiveJob("failed")).toBe(false);
    expect(isCharacterSummaryActiveJob("canceled")).toBe(false);
    expect(isCharacterSummaryActiveJob("incompatible")).toBe(false);

    expect(isCharacterSummaryProcessingJob("queued")).toBe(true);
    expect(isCharacterSummaryProcessingJob("running")).toBe(true);
    expect(isCharacterSummaryProcessingJob("pausing")).toBe(true);
    expect(isCharacterSummaryProcessingJob("paused")).toBe(false);
    expect(isCharacterSummaryProcessingJob("interrupted")).toBe(false);
  });

  it("treats canceled, failed, and incompatible jobs as completed history", () => {
    expect(isCharacterSummaryCompletedJob("completed")).toBe(true);
    expect(isCharacterSummaryCompletedJob("failed")).toBe(true);
    expect(isCharacterSummaryCompletedJob("canceled")).toBe(true);
    expect(isCharacterSummaryCompletedJob("incompatible")).toBe(true);
    expect(isCharacterSummaryCompletedJob("queued")).toBe(false);
    expect(isCharacterSummaryCompletedJob("running")).toBe(false);
  });
});
