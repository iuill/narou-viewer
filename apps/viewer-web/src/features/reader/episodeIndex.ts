export type EpisodeIndex = string;

export function getPreviousEpisodeIndex(episodeIndex: EpisodeIndex | null): EpisodeIndex | null {
  if (episodeIndex === null || !/^\d+$/.test(episodeIndex)) {
    return null;
  }

  const previous = Number.parseInt(episodeIndex, 10) - 1;
  return previous > 0 ? String(previous) : null;
}

export function compareEpisodeIndex(left: EpisodeIndex, right: EpisodeIndex): number {
  return Number.parseInt(left, 10) - Number.parseInt(right, 10);
}

export function normalizeEpisodeIndex(value: unknown): EpisodeIndex | null {
  if (typeof value === "string" && /^\d+$/.test(value)) {
    return value;
  }

  if (typeof value === "number" && Number.isInteger(value) && value >= 0) {
    return String(value);
  }

  return null;
}
