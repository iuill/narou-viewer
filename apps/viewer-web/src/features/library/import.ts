import { parseDocument } from "yaml";
import { mutateJson } from "../../api/http";
import type { LibraryExportDocument } from "./export";

export const MAX_LIBRARY_IMPORT_BYTES = 1 << 20;

export type LibraryImportResult = {
  dryRun: boolean;
  novelsMatched: number;
  novelsSkipped: number;
  readingStatesApplied: number;
  readingStatesSkipped: number;
  bookmarksApplied: number;
  bookmarksSkipped: number;
  warnings: string[];
};

export function parseLibraryImportYaml(source: string): unknown {
  if (new Blob([source]).size > MAX_LIBRARY_IMPORT_BYTES) {
    throw new Error("インポートファイルは1MB以下にしてください。");
  }
  const document = parseDocument(source, { strict: true, uniqueKeys: true });
  if (document.errors.length > 0) {
    throw new Error("YAMLを読み取れませんでした。");
  }
  return document.toJS({ maxAliasCount: 0 }) as LibraryExportDocument;
}

export function importLibraryDocument(document: unknown, dryRun: boolean): Promise<LibraryImportResult> {
  return mutateJson<LibraryImportResult, { dryRun: boolean; document: unknown }>(
    "/api/library/import",
    { dryRun, document },
    "ライブラリのインポートに失敗しました。"
  );
}

export function formatLibraryImportSummary(result: LibraryImportResult): string {
  const stateSummary = `既読 ${result.readingStatesApplied}件、栞 ${result.bookmarksApplied}件`;
  const skipped = result.readingStatesSkipped + result.bookmarksSkipped + result.novelsSkipped;
  return `${result.novelsMatched}作品を照合しました。${stateSummary}を復元します。スキップ ${skipped}件、警告 ${result.warnings.length}件。`;
}
