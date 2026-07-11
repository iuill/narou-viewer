import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { afterEach, describe, expect, it } from "vitest";

import {
  areaFor,
  normalizePath,
  prepareRepositorySizeReport,
} from "../../scripts/prepare-repository-size-report.mjs";

const temporaryDirectories: string[] = [];

afterEach(() => {
  for (const directory of temporaryDirectories.splice(0)) {
    rmSync(directory, { recursive: true, force: true });
  }
});

describe("repository size report area classification", () => {
  it("classifies first-class repository areas without folding them into root", () => {
    expect(areaFor("apps/viewer-api-go/internal/httpapi/server.go")).toBe("apps/viewer-api-go/");
    expect(areaFor("apps/viewer-web/src/App.tsx")).toBe("apps/viewer-web/");
    expect(areaFor("services/novel-fetcher/internal/storage/store.go")).toBe("services/novel-fetcher/");
    expect(areaFor("tests/api-contract/cases/health-system.test.ts")).toBe("tests/");
    expect(areaFor("data_e2e/novel-fetcher/library.sqlite")).toBe("data_e2e/");
    expect(areaFor(".claude/settings.local.json")).toBe(".claude/");
  });

  it("normalizes scc file paths before area classification", () => {
    const normalized = normalizePath("./apps/viewer-api-go/internal/store/store_test.go");
    expect(normalized).toBe("apps/viewer-api-go/internal/store/store_test.go");
    expect(areaFor(normalized)).toBe("apps/viewer-api-go/");
  });

  it("renders language, area, and complexity details without a collapsed section", () => {
    const reportDirectory = mkdtempSync(path.join(tmpdir(), "repository-size-report-"));
    temporaryDirectories.push(reportDirectory);

    const report = [
      {
        Name: "TypeScript",
        Files: [
          {
            Location: "apps/viewer-web/src/App.tsx",
            Lines: 20,
            Code: 16,
            Comment: 2,
            Blank: 2,
            Complexity: 4,
          },
        ],
      },
    ];
    writeFileSync(path.join(reportDirectory, "scc-by-file.json"), JSON.stringify(report));
    writeFileSync(path.join(reportDirectory, "scc-base-by-file.json"), JSON.stringify(report));
    writeFileSync(path.join(reportDirectory, "scc.version"), "scc version 3.7.0\n");

    prepareRepositorySizeReport(reportDirectory);

    const comment = readFileSync(path.join(reportDirectory, "repository-size-comment.md"), "utf8");
    expect(comment).toContain("### Top languages, areas, and complexity hotspots");
    expect(comment).toContain("#### Top Languages by Code");
    expect(comment).toContain("#### Top Areas by Code");
    expect(comment).toContain("#### Complexity Hotspots");
    expect(comment).not.toContain("<details>");
    expect(comment).not.toContain("<summary>");
  });
});
