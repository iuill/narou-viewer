import { defineConfig } from "vitest/config";

const defaultCoverageReporters = ["text", "json-summary", "json", "html"];

const parsedCoverageReporters = (process.env.VITEST_COVERAGE_REPORTERS ?? defaultCoverageReporters.join(","))
  .split(",")
  .map((value) => value.trim())
  .filter(Boolean);

const coverageReporters =
  parsedCoverageReporters.length > 0 ? parsedCoverageReporters : defaultCoverageReporters;

const coverageReportsDirectory = process.env.VITEST_COVERAGE_REPORTS_DIR?.trim() || "./coverage";

export default defineConfig({
  test: {
    projects: [
      "./apps/viewer-web/vitest.config.ts",
      "./tests/backend-fakes/vitest.config.ts"
    ],
    coverage: {
      provider: "istanbul",
      reporter: coverageReporters,
      reportsDirectory: coverageReportsDirectory,
      thresholds: {
        statements: 85,
        branches: 78,
        functions: 85,
        lines: 85
      },
      include: ["apps/viewer-web/src/**/*.{ts,tsx}"],
      exclude: [
        "**/*.d.ts",
        "**/dist/**",
        "**/tests/**",
        "**/coverage/**",
        "**/playwright-report/**",
        "**/test-results/**",
        "e2e/**",
        "scripts/**"
      ]
    }
  }
});
