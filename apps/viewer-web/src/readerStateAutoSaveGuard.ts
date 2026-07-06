export type AppliedReaderStateAutoSaveGuard = {
  novelId: string;
  episodeIndex: string;
  readingStateKey: string;
};

export type AppliedReaderStateAutoSaveGuardResult = {
  nextGuard: AppliedReaderStateAutoSaveGuard | null;
  shouldSkipCurrentSave: boolean;
};

export function consumeAppliedReaderStateAutoSaveGuard(
  guard: AppliedReaderStateAutoSaveGuard | null,
  selectedNovelId: string,
  selectedEpisodeIndex: string,
  nextReadingStateKey: string
): AppliedReaderStateAutoSaveGuardResult {
  if (!guard || guard.novelId !== selectedNovelId) {
    return { nextGuard: guard, shouldSkipCurrentSave: false };
  }

  if (guard.episodeIndex !== selectedEpisodeIndex) {
    return { nextGuard: null, shouldSkipCurrentSave: false };
  }

  if (nextReadingStateKey !== guard.readingStateKey) {
    return { nextGuard: null, shouldSkipCurrentSave: true };
  }

  return { nextGuard: null, shouldSkipCurrentSave: false };
}
