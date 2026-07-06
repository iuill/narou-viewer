import { describe, expect, it } from "vitest";

import { areaFor, normalizePath } from "../../scripts/prepare-repository-size-report.mjs";

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
});
