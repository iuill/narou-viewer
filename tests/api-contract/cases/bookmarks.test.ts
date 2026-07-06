import { describe, expect, it } from "vitest";
import {
  MUTATING_CONTRACT_TESTS_ENABLED,
  REQUIRE_CONTRACT_FIXTURE,
  expectErrorShape,
  expectIsoStringOrNull,
  expectJsonResponse,
  expectNonEmptyFixtureArray,
  requestJson,
} from "../harness/apiClient";
import { findFixtureEpisode } from "../harness/fixtures";

type BookmarkContract = {
  id: string;
  novelId: string;
  episodeIndex: string;
  position: number;
  label: string | null;
  createdAt: string;
};

function expectBookmarkShape(
  value: unknown,
): asserts value is BookmarkContract {
  expect(value).toEqual(
    expect.objectContaining({
      id: expect.any(String),
      novelId: expect.any(String),
      episodeIndex: expect.any(String),
      position: expect.any(Number),
      createdAt: expect.any(String),
    }),
  );
  const record = value as Record<string, unknown>;
  expect(record).toHaveProperty("label");
  expectIsoStringOrNull(record.createdAt);
}

describe("bookmarks contract", () => {
  it("returns bookmark collection shape", async () => {
    const response = await requestJson<{ bookmarks: unknown[] }>(
      "/api/bookmarks",
    );

    expectJsonResponse(response);
    expect(response.json).toEqual({
      bookmarks: expect.any(Array),
    });

    if (response.json.bookmarks.length > 0) {
      expectBookmarkShape(response.json.bookmarks[0]);
    }

    if (!REQUIRE_CONTRACT_FIXTURE) {
      return;
    }

    if (!MUTATING_CONTRACT_TESTS_ENABLED) {
      expectNonEmptyFixtureArray(response.json.bookmarks, "bookmarks");
      return;
    }

    const fixtureEpisode = await findFixtureEpisode("bookmarks");
    if (!fixtureEpisode) {
      return;
    }

    let bookmarkId: string | null = null;
    try {
      const created = await requestJson<BookmarkContract>("/api/bookmarks", {
        method: "POST",
        body: {
          novelId: fixtureEpisode.novelId,
          episodeIndex: fixtureEpisode.episodeIndex,
          position: 1,
          label: "api-contract-fixture",
        },
      });
      expectJsonResponse(created, 201);
      expectBookmarkShape(created.json);
      bookmarkId = created.json.id;

      const updatedList = await requestJson<{ bookmarks: unknown[] }>(
        `/api/bookmarks?novelId=${encodeURIComponent(fixtureEpisode.novelId)}`,
      );
      expectJsonResponse(updatedList);
      expectNonEmptyFixtureArray(updatedList.json.bookmarks, "bookmarks");
      expect(
        updatedList.json.bookmarks.some(
          (bookmark) => (bookmark as { id?: unknown }).id === bookmarkId,
        ),
      ).toBe(true);
      expectBookmarkShape(updatedList.json.bookmarks[0]);
    } finally {
      if (bookmarkId) {
        const deleted = await requestJson(
          `/api/bookmarks/${encodeURIComponent(bookmarkId)}`,
          {
            method: "DELETE",
          },
        );
        expectJsonResponse(deleted);
        expect(deleted.json).toEqual({ deleted: true });
      }
    }
  });

  it("keeps bookmark validation and missing-delete errors stable", async () => {
    const missingRequiredFields = await requestJson("/api/bookmarks", {
      method: "POST",
      body: {},
    });
    expectJsonResponse(missingRequiredFields, 400);
    expectErrorShape(missingRequiredFields.json);

    const invalidPosition = await requestJson("/api/bookmarks", {
      method: "POST",
      body: {
        novelId: "contract-validation-only",
        episodeIndex: "1",
        position: -1,
      },
    });
    expectJsonResponse(invalidPosition, 400);
    expectErrorShape(invalidPosition.json);

    const missingDelete = await requestJson(
      "/api/bookmarks/__api_contract_missing__",
      {
        method: "DELETE",
      },
    );
    expectJsonResponse(missingDelete, 404);
    expectErrorShape(missingDelete.json);
  });
});
