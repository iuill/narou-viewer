import type { EpisodeIndex } from "./episodeIndex";

export function createReadingStateKey(
  novelId: string | null,
  episodeIndex: EpisodeIndex | null,
  position: number | null
): string | null {
  if (!novelId || episodeIndex === null || position === null) {
    return null;
  }

  return `${novelId}:${episodeIndex}:${position}`;
}
