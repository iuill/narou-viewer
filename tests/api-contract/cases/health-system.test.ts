import { describe, expect, it } from "vitest";
import { expectErrorShape, expectJsonResponse, requestJson } from "../harness/apiClient";

describe("health and system status contract", () => {
  it("returns viewer-api health metadata", async () => {
    const response = await requestJson("/api/health");

    expectJsonResponse(response);
    expect(response.json).toEqual(
      expect.objectContaining({
        status: "ok",
        service: "viewer-api",
        runtime: expect.objectContaining({
          viewerDataDirConfigured: expect.any(Boolean),
          stateDirReady: expect.any(Boolean)
        })
      })
    );
  });

  it("returns aggregated runtime status services", async () => {
    const response = await requestJson("/api/system/status");

    expectJsonResponse(response);
    expect(response.json).toEqual(
      expect.objectContaining({
        status: expect.stringMatching(/^(ok|warn|error)$/),
        services: expect.any(Array),
        checkedAt: expect.any(String)
      })
    );
    expect(Number.isNaN(Date.parse((response.json as { checkedAt: string }).checkedAt))).toBe(false);
    const services = (response.json as { services: Array<{ id?: unknown; label?: unknown }> })
      .services;
    expect(services.length).toBeGreaterThan(0);
    expect(services.map((service) => service.id)).toEqual(
      expect.arrayContaining(["viewer-api", "novel-fetcher", "go-internal-ai", "library"])
    );
    expect(services.find((service) => service.id === "go-internal-ai")).toEqual(
      expect.objectContaining({ label: "Go internal AI" })
    );
    expect(services[0]).toEqual(
      expect.objectContaining({
        id: expect.any(String),
        label: expect.any(String),
        status: expect.stringMatching(/^(ok|warn|error)$/),
        detail: expect.any(String)
      })
    );
  });

  it("returns storage usage breakdown", async () => {
    const response = await requestJson("/api/system/storage");

    expectJsonResponse(response);
    expect(response.json).toEqual(
      expect.objectContaining({
        checkedAt: expect.any(String),
        totalBytes: expect.any(Number),
        categories: expect.arrayContaining([
          expect.objectContaining({
            id: expect.stringMatching(/^(novelData|cache|other)$/),
            label: expect.any(String),
            bytes: expect.any(Number),
            fileCount: expect.any(Number),
          }),
        ]),
        novels: expect.any(Array),
      }),
    );
    expect(Number.isNaN(Date.parse((response.json as { checkedAt: string }).checkedAt))).toBe(false);
    const warnings = (response.json as { warnings?: unknown }).warnings;
    if (warnings !== undefined) {
      expect(warnings).toEqual(expect.arrayContaining([expect.any(String)]));
    }
  });

  it("returns storage usage progress", async () => {
    const response = await requestJson("/api/system/storage/progress?requestId=contract-scan");

    expectJsonResponse(response);
    expect(response.json).toEqual(
      expect.objectContaining({
        requestId: "contract-scan",
        state: expect.stringMatching(/^(idle|running|completed|error)$/),
        phase: expect.stringMatching(/^(preparing|scanning|completed)$/),
        checkedNovels: expect.any(Number),
        totalNovels: expect.any(Number),
      }),
    );
    const updatedAt = (response.json as { updatedAt?: unknown }).updatedAt;
    if (updatedAt !== undefined) {
      expect(typeof updatedAt).toBe("string");
      expect(Number.isNaN(Date.parse(updatedAt))).toBe(false);
    }
  });

  it("returns structured JSON for missing API routes", async () => {
    const response = await requestJson("/api/__missing__");

    expectJsonResponse(response, 404);
    expectErrorShape(response.json);
    expect(response.json).toEqual(
      expect.objectContaining({
        code: "NOT_FOUND",
        message: "Not found.",
      }),
    );
  });
});
