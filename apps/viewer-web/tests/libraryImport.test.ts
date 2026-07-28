import { afterEach, describe, expect, it, vi } from "vitest";
import {
  formatLibraryImportSummary,
  importLibraryDocument,
  parseLibraryImportYaml
} from "../src/features/library/import";

describe("library import", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("parses a library export YAML document", () => {
    const document = parseLibraryImportYaml(`
formatVersion: 1
exportedAt: 2026-07-28T12:00:00Z
novelsCount: 0
exportWarnings: []
novels: []
`);
    expect(document).toMatchObject({ formatVersion: 1, novelsCount: 0, novels: [] });
  });

  it("rejects malformed, aliased, and oversized YAML", () => {
    expect(() => parseLibraryImportYaml("novels: [")).toThrow("YAMLを読み取れませんでした。");
    expect(() => parseLibraryImportYaml("formatVersion: &version 1\ncopy: *version\n")).toThrow();
    expect(() => parseLibraryImportYaml("x".repeat((1 << 20) + 1))).toThrow("インポートファイルは1MB以下にしてください。");
  });

  it("sends dry-run and formats its summary", async () => {
    const result = {
      dryRun: true,
      novelsMatched: 1,
      novelsSkipped: 1,
      readingStatesApplied: 1,
      readingStatesSkipped: 0,
      bookmarksApplied: 2,
      bookmarksSkipped: 1,
      warnings: ["missing"]
    };
    const fetchMock = vi.fn(async () => new Response(JSON.stringify(result), {
      status: 200,
      headers: { "content-type": "application/json" }
    }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(importLibraryDocument({ formatVersion: 1 }, true)).resolves.toEqual(result);
    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toEqual({
      dryRun: true,
      document: { formatVersion: 1 }
    });
    expect(formatLibraryImportSummary(result)).toBe(
      "1作品を照合しました。既読 1件、栞 2件を復元します。スキップ 2件、警告 1件。"
    );
  });
});
