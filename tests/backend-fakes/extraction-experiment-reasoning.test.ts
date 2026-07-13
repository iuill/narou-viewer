import { describe, expect, it } from "vitest";

import {
  resolveExperimentRequireParameters,
  resolveReportedReasoning,
} from "../../scripts/extraction-experiment-lib.mjs";

describe("extraction experiment reasoning metadata", () => {
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
});
