import { describe, expect, it } from "vitest";

import {
  parseArgs,
  renderExtractionMarkdown,
  resolveExperimentRequireParameters,
  resolveReasoningEffortOption,
  resolveReportedReasoning,
} from "../../scripts/extraction-experiment-lib.mjs";

describe("extraction experiment reasoning metadata", () => {
  it.each([["--reasoning-effort"], ["--reasoning-effort="]])(
    "rejects an explicitly empty reasoning effort: %s",
    (...argv) => {
      const { values } = parseArgs(argv);
      expect(() => resolveReasoningEffortOption(values)).toThrow(
        "--reasoning-effort には none / minimal / low / medium / high / xhigh / max",
      );
    },
  );

  it("distinguishes an omitted reasoning effort from a normalized value", () => {
    expect(resolveReasoningEffortOption(parseArgs([]).values)).toBeNull();
    expect(resolveReasoningEffortOption(parseArgs(["--reasoning-effort", " XHIGH "]).values)).toBe("xhigh");
  });

  it("requires provider parameter support when reasoning effort is requested", () => {
    expect(
      resolveExperimentRequireParameters({
        explicitValue: undefined,
        reasoningEffort: "xhigh",
        hasModelOverrides: true,
      }),
    ).toBe(true);
    expect(
      resolveExperimentRequireParameters({
        explicitValue: undefined,
        reasoningEffort: null,
        hasModelOverrides: true,
      }),
    ).toBe(false);
    expect(() =>
      resolveExperimentRequireParameters({
        explicitValue: false,
        reasoningEffort: "high",
        hasModelOverrides: true,
      }),
    ).toThrow("--require-parameters false");
    expect(() =>
      resolveExperimentRequireParameters({
        explicitValue: false,
        reasoningEffort: "high",
        hasModelOverrides: false,
      }),
    ).toThrow("--require-parameters false");
  });

  it("uses server-reported reasoning metadata and rejects unverifiable runs", () => {
    expect(
      resolveReportedReasoning(
        {
          reasoning: {
            requestedEffort: "high",
            source: "environment",
            requireParameters: true,
          },
        },
        null,
      ),
    ).toEqual({ requestedEffort: "high", source: "environment", requireParameters: true });
    expect(() =>
      resolveReportedReasoning(
        {
          reasoning: {
            requestedEffort: "high",
            source: "request",
            requireParameters: false,
          },
        },
        "high",
      ),
    ).toThrow("did not require provider support");
    expect(() => resolveReportedReasoning({}, "high")).toThrow("did not report resolved reasoning");
  });

  it("does not label unverified reasoning as the provider default", () => {
    const markdown = renderExtractionMarkdown({
      profileLabel: "failed run",
      profileId: "profile-1",
      modelId: "model-1",
      reasoning: null,
      batchTimings: [],
      result: {
        processedUpToEpisodeIndex: null,
        characters: [],
        terms: [],
      },
    });

    expect(markdown).toContain("reasoningRequestedEffort: unknown");
    expect(markdown).toContain("reasoningSource: unknown");
  });
});
