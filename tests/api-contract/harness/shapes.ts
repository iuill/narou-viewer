import { expect } from "vitest";
import { expectIsoStringOrNull } from "./apiClient";

export type NovelSummary = {
  novelId: string;
  title: string;
  author: string;
  totalEpisodes: number;
  lastActivityAt: string | null;
};

export function expectNovelSummaryShape(value: unknown): asserts value is NovelSummary {
  expect(value).toEqual(
    expect.objectContaining({
      novelId: expect.any(String),
      title: expect.any(String),
      author: expect.any(String),
      siteName: expect.any(String),
      story: expect.any(String),
      bookmarkCount: expect.any(Number),
      totalEpisodes: expect.any(Number)
    })
  );
  const record = value as Record<string, unknown>;
  expect(record).toHaveProperty("fetcherWorkId");
  expect(record).toHaveProperty("tocUrl");
  expect(record).toHaveProperty("updatedAt");
  expect(record).toHaveProperty("lastReadEpisodeIndex");
  expect(record).toHaveProperty("lastReadEpisodeTitle");
  expect(record).toHaveProperty("latestBookmarkEpisodeIndex");
  expect(record).toHaveProperty("lastActivityAt");
  expectIsoStringOrNull(record.updatedAt);
  expectIsoStringOrNull(record.lastActivityAt);
}

export function expectReaderStateShape(value: unknown): void {
  expect(value).toEqual(
    expect.objectContaining({
      novelId: expect.any(String),
      position: expect.any(Number),
      stateVersion: expect.any(Number)
    })
  );
  const record = value as Record<string, unknown>;
  expect(record).toHaveProperty("lastReadEpisodeIndex");
  expect(record).toHaveProperty("scroll");
  expect(record).toHaveProperty("updatedAt");
  expect(record).toHaveProperty("updatedByClientId");
  expectIsoStringOrNull(record.updatedAt);
}

export function expectReaderPreferencesShape(value: unknown): void {
  expect(value).toEqual(
    expect.objectContaining({
      readingMode: expect.any(String),
      theme: expect.any(String),
      fontFamily: expect.any(String)
    })
  );
  const record = value as Record<string, unknown>;
  expect(record).toHaveProperty("updatedAt");
  expectIsoStringOrNull(record.updatedAt);
}

export function expectNovelReaderSettingsShape(value: unknown): void {
  expect(value).toEqual(
    expect.objectContaining({
      novelId: expect.any(String),
      correction: expect.objectContaining({
        quoteNormalization: expect.any(Boolean),
        hyphenDashNormalization: expect.any(Boolean),
        parenthesisNormalization: expect.any(Boolean),
        halfwidthAlnumPunctuationNormalization: expect.any(Boolean)
      })
    })
  );
  const record = value as Record<string, unknown>;
  expect(record).toHaveProperty("updatedAt");
  expectIsoStringOrNull(record.updatedAt);
}
