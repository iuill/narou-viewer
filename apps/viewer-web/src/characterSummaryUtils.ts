export type EpisodeIndex = string;
export type CharacterJobStatus = "queued" | "running" | "completed" | "failed";

function compareEpisodeIndex(left: EpisodeIndex, right: EpisodeIndex): number {
  return Number.parseInt(left, 10) - Number.parseInt(right, 10);
}

export function resolveCharacterSummaryRefreshTarget(input: {
  defaultUpToEpisodeIndex: EpisodeIndex | null;
  requestedUpToEpisodeIndex: EpisodeIndex | null;
}): EpisodeIndex | null {
  if (input.defaultUpToEpisodeIndex === null) {
    return null;
  }

  if (input.requestedUpToEpisodeIndex === null) {
    return input.defaultUpToEpisodeIndex;
  }

  return compareEpisodeIndex(input.requestedUpToEpisodeIndex, input.defaultUpToEpisodeIndex) <= 0
    ? input.requestedUpToEpisodeIndex
    : input.defaultUpToEpisodeIndex;
}

export function isCharacterSummaryRequestAllowed(input: {
  defaultUpToEpisodeIndex: EpisodeIndex | null;
  requestedUpToEpisodeIndex: EpisodeIndex | null;
}): boolean {
  return (
    input.defaultUpToEpisodeIndex !== null &&
    input.requestedUpToEpisodeIndex !== null &&
    compareEpisodeIndex(input.requestedUpToEpisodeIndex, "1") >= 0 &&
    compareEpisodeIndex(input.requestedUpToEpisodeIndex, input.defaultUpToEpisodeIndex) <= 0
  );
}

export function isCharacterSummaryActiveJob(status: CharacterJobStatus): boolean {
  return status === "queued" || status === "running";
}

export function isCharacterSummaryCompletedJob(status: CharacterJobStatus): boolean {
  return status === "completed" || status === "failed";
}
