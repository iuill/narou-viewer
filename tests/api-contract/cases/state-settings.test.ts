import { describe, expect, it } from "vitest";
import {
  MUTATING_CONTRACT_TESTS_ENABLED,
  expectErrorShape,
  expectJsonResponse,
  requestJson,
} from "../harness/apiClient";
import { findFixtureEpisode } from "../harness/fixtures";
import {
  expectNovelReaderSettingsShape,
  expectReaderPreferencesShape,
  expectReaderStateShape,
} from "../harness/shapes";

type ReaderStateContract = {
  novelId: string;
  lastReadEpisodeIndex: string | null;
  position: number;
  scroll: { type: "ratio"; value: number } | null;
  stateVersion: number;
  updatedByClientId: string | null;
};

type NovelReaderSettingsContract = {
  novelId: string;
  correction: {
    quoteNormalization: boolean;
    hyphenDashNormalization: boolean;
    parenthesisNormalization: boolean;
    halfwidthAlnumPunctuationNormalization: boolean;
  };
  updatedAt: string | null;
};

describe("reader state and settings contract", () => {
  it("returns reader preferences with revision metadata", async () => {
    const response = await requestJson("/api/reader/preferences");

    expectJsonResponse(response);
    expectReaderPreferencesShape(response.json);
  });

  it("returns reader state for a requested novel id", async () => {
    const response = await requestJson(
      "/api/reader/state?novelId=contract-validation-only",
    );

    expectJsonResponse(response);
    expectReaderStateShape(response.json);
  });

  it("returns per-novel reader settings for a requested novel id", async () => {
    const fixtureEpisode = await findFixtureEpisode("reader settings read");
    const novelId = fixtureEpisode?.novelId ?? "contract-validation-only";
    const response = await requestJson(
      `/api/library/novels/${encodeURIComponent(novelId)}/reader-settings`,
    );

    expectJsonResponse(response);
    expectNovelReaderSettingsShape(response.json);
  });

  it("keeps reader state read validation errors stable", async () => {
    const response = await requestJson("/api/reader/state");

    expectJsonResponse(response, 400);
    expectErrorShape(response.json);
  });

  it("keeps reader state update validation errors stable", async () => {
    const oldClient = await requestJson("/api/reader/state", {
      method: "PUT",
      headers: {
        "x-narou-viewer-api-contract-version": "",
        "x-narou-viewer-client-build": "legacy-client",
      },
      body: {
        novelId: "contract-validation-only",
        lastReadEpisodeIndex: "1",
        position: 1,
        expectedStateVersion: 0,
      },
    });
    expectJsonResponse(oldClient, 426);
    expectErrorShape(oldClient.json);
    expect(oldClient.json).toEqual(
      expect.objectContaining({
        code: "CLIENT_UPDATE_REQUIRED",
      }),
    );

    const missingExpectedVersion = await requestJson("/api/reader/state", {
      method: "PUT",
      body: {
        novelId: "contract-validation-only",
        lastReadEpisodeIndex: "1",
        position: 1,
      },
    });
    expectJsonResponse(missingExpectedVersion, 400);
    expectErrorShape(missingExpectedVersion.json);

    const nullExpectedVersion = await requestJson("/api/reader/state", {
      method: "PUT",
      body: {
        novelId: "contract-validation-only",
        lastReadEpisodeIndex: "1",
        position: 1,
        expectedStateVersion: null,
      },
    });
    expectJsonResponse(nullExpectedVersion, 400);
    expectErrorShape(nullExpectedVersion.json);

    const response = await requestJson("/api/reader/state", {
      method: "PUT",
      body: {
        novelId: "contract-validation-only",
        lastReadEpisodeIndex: "1",
        position: -1,
      },
    });

    expectJsonResponse(response, 400);
    expectErrorShape(response.json);
  });

  it("keeps reader preferences validation errors stable", async () => {
    const emptyPatch = await requestJson("/api/reader/preferences", {
      method: "PUT",
      body: {},
    });
    expectJsonResponse(emptyPatch, 400);
    expectErrorShape(emptyPatch.json);

    const invalidTheme = await requestJson("/api/reader/preferences", {
      method: "PUT",
      body: {
        theme: "invalid",
      },
    });
    expectJsonResponse(invalidTheme, 400);
    expectErrorShape(invalidTheme.json);
  });

  it("keeps per-novel reader settings validation errors stable", async () => {
    const fixtureEpisode = await findFixtureEpisode("reader settings validation");
    if (!fixtureEpisode) {
      return;
    }

    const emptyPatch = await requestJson(
      `/api/library/novels/${encodeURIComponent(fixtureEpisode.novelId)}/reader-settings`,
      {
        method: "PUT",
        body: {},
      },
    );
    expectJsonResponse(emptyPatch, 400);
    expectErrorShape(emptyPatch.json);

    const invalidCorrection = await requestJson(
      `/api/library/novels/${encodeURIComponent(fixtureEpisode.novelId)}/reader-settings`,
      {
        method: "PUT",
        body: {
          correction: {
            quoteNormalization: "invalid",
          },
        },
      },
    );
    expectJsonResponse(invalidCorrection, 400);
    expectErrorShape(invalidCorrection.json);

    const invalidHalfwidthCorrection = await requestJson(
      `/api/library/novels/${encodeURIComponent(fixtureEpisode.novelId)}/reader-settings`,
      {
        method: "PUT",
        body: {
          correction: {
            halfwidthAlnumPunctuationNormalization: "invalid",
          },
        },
      },
    );
    expectJsonResponse(invalidHalfwidthCorrection, 400);
    expectErrorShape(invalidHalfwidthCorrection.json);

    const emptyCorrection = await requestJson(
      `/api/library/novels/${encodeURIComponent(fixtureEpisode.novelId)}/reader-settings`,
      {
        method: "PUT",
        body: {
          correction: {},
        },
      },
    );
    expectJsonResponse(emptyCorrection, 400);
    expectErrorShape(emptyCorrection.json);
  });

  it.runIf(MUTATING_CONTRACT_TESTS_ENABLED)(
    "persists reader state updates",
    async () => {
      const fixtureEpisode = await findFixtureEpisode("reader state mutation");
      if (!fixtureEpisode) {
        return;
      }

      const original = await requestJson<ReaderStateContract>(
        `/api/reader/state?novelId=${encodeURIComponent(fixtureEpisode.novelId)}`,
      );
      expectJsonResponse(original);
      expectReaderStateShape(original.json);

      const nextPayload = {
        novelId: fixtureEpisode.novelId,
        lastReadEpisodeIndex: fixtureEpisode.episodeIndex,
        position: original.json.position === 17 ? 18 : 17,
        scroll: { type: "ratio" as const, value: 0.42 },
        clientId: "api-contract",
        expectedStateVersion: original.json.stateVersion,
      };

      try {
        const updated = await requestJson("/api/reader/state", {
          method: "PUT",
          body: nextPayload,
        });
        expectJsonResponse(updated);
        expectReaderStateShape(updated.json);
        expect(updated.json).toEqual(
          expect.objectContaining({
            novelId: fixtureEpisode.novelId,
            lastReadEpisodeIndex: fixtureEpisode.episodeIndex,
            position: nextPayload.position,
            scroll: nextPayload.scroll,
            updatedByClientId: nextPayload.clientId,
          }),
        );
        const conflict = await requestJson("/api/reader/state", {
          method: "PUT",
          body: {
            ...nextPayload,
            position: nextPayload.position + 1,
            expectedStateVersion: original.json.stateVersion,
          },
        });
        expectJsonResponse(conflict, 409);
        expectReaderStateShape(conflict.json);
        expect(conflict.json).toEqual(
          expect.objectContaining({
            novelId: fixtureEpisode.novelId,
            position: nextPayload.position,
            stateVersion: updated.json.stateVersion,
          }),
        );
      } finally {
        const current = await requestJson<ReaderStateContract>(
          `/api/reader/state?novelId=${encodeURIComponent(fixtureEpisode.novelId)}`,
        );
        expectJsonResponse(current);
        const restored = await requestJson("/api/reader/state", {
          method: "PUT",
          body: {
            novelId: fixtureEpisode.novelId,
            lastReadEpisodeIndex: original.json.lastReadEpisodeIndex,
            position: original.json.position,
            scroll: original.json.scroll,
            clientId: original.json.updatedByClientId,
            expectedStateVersion: current.json.stateVersion,
          },
        });
        expectJsonResponse(restored);
        expectReaderStateShape(restored.json);
        expect(restored.json).toEqual(
          expect.objectContaining({
            novelId: fixtureEpisode.novelId,
            lastReadEpisodeIndex: original.json.lastReadEpisodeIndex,
            position: original.json.position,
            scroll: original.json.scroll,
            updatedByClientId: original.json.updatedByClientId,
          }),
        );
      }
    },
  );

  it.runIf(MUTATING_CONTRACT_TESTS_ENABLED)(
    "persists reader preferences updates",
    async () => {
      const original = await requestJson<{
        readingMode: string;
        theme: string;
        fontFamily: string;
      }>("/api/reader/preferences");
      expectJsonResponse(original);

      const nextPayload = {
        readingMode:
          original.json.readingMode === "horizontal"
            ? "vertical"
            : "horizontal",
        theme: original.json.theme,
        fontFamily: original.json.fontFamily,
      };
      const originalPayload = {
        readingMode: original.json.readingMode,
        theme: original.json.theme,
        fontFamily: original.json.fontFamily,
      };

      try {
        const updated = await requestJson("/api/reader/preferences", {
          method: "PUT",
          body: nextPayload,
        });

        expectJsonResponse(updated);
        expect(updated.json).toEqual(expect.objectContaining(nextPayload));
        expectReaderPreferencesShape(updated.json);
      } finally {
        const restored = await requestJson("/api/reader/preferences", {
          method: "PUT",
          body: originalPayload,
        });
        expectJsonResponse(restored);
        expect(restored.json).toEqual(expect.objectContaining(originalPayload));
        expectReaderPreferencesShape(restored.json);
      }
    },
  );

  it.runIf(MUTATING_CONTRACT_TESTS_ENABLED)(
    "persists per-novel reader correction partial updates",
    async () => {
      const fixtureEpisode = await findFixtureEpisode(
        "reader settings mutation",
      );
      if (!fixtureEpisode) {
        return;
      }

      const settingsUrl = `/api/library/novels/${encodeURIComponent(fixtureEpisode.novelId)}/reader-settings`;
      const original = await requestJson<NovelReaderSettingsContract>(
        settingsUrl,
      );
      expectJsonResponse(original);
      expectNovelReaderSettingsShape(original.json);

      const nextHalfwidth =
        !original.json.correction.halfwidthAlnumPunctuationNormalization;

      try {
        const updated = await requestJson<NovelReaderSettingsContract>(
          settingsUrl,
          {
            method: "PUT",
            body: {
              correction: {
                halfwidthAlnumPunctuationNormalization: nextHalfwidth,
              },
            },
          },
        );
        expectJsonResponse(updated);
        expectNovelReaderSettingsShape(updated.json);
        expect(updated.json.correction).toEqual({
          quoteNormalization: original.json.correction.quoteNormalization,
          hyphenDashNormalization:
            original.json.correction.hyphenDashNormalization,
          parenthesisNormalization:
            original.json.correction.parenthesisNormalization,
          halfwidthAlnumPunctuationNormalization: nextHalfwidth,
        });

        const reloaded = await requestJson<NovelReaderSettingsContract>(
          settingsUrl,
        );
        expectJsonResponse(reloaded);
        expectNovelReaderSettingsShape(reloaded.json);
        expect(
          reloaded.json.correction.halfwidthAlnumPunctuationNormalization,
        ).toBe(nextHalfwidth);
      } finally {
        const restored = await requestJson(settingsUrl, {
          method: "PUT",
          body: {
            correction: original.json.correction,
          },
        });
        expectJsonResponse(restored);
        expectNovelReaderSettingsShape(restored.json);
      }
    },
  );
});
