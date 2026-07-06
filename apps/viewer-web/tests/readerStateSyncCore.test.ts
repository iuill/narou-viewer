import { describe, expect, it } from "vitest";

import type { ReaderState } from "../src/features/reader/types";
import {
  classifyReaderStateAdoption,
  isNewerReaderSyncConflict,
  isSelfWriterConflictResolution,
  shouldKeepConflictBlockedForAcceptedState,
  shouldRegisterReaderSyncConflict,
  type PutReaderStateInput
} from "../src/hooks/readerStateSyncCore";

function createReaderState(overrides: Partial<ReaderState> = {}): ReaderState {
  return {
    novelId: "novel-a",
    lastReadEpisodeIndex: "2",
    position: 12,
    updatedAt: "2026-06-15T00:00:00.000Z",
    stateVersion: 1,
    updatedByClientId: "other-client",
    ...overrides
  };
}

function createPutInput(overrides: Partial<PutReaderStateInput> = {}): PutReaderStateInput {
  return {
    novelId: "novel-a",
    episodeIndex: "2",
    position: 12,
    ...overrides
  };
}

describe("readerStateSyncCore", () => {
  it("classifies stale, equal, and advanced server states by version", () => {
    const current = createReaderState({ stateVersion: 3 });

    expect(classifyReaderStateAdoption(current, createReaderState({ stateVersion: 2 }))).toBe("stale");
    expect(classifyReaderStateAdoption(current, createReaderState({ stateVersion: 3 }))).toBe("equal");
    expect(classifyReaderStateAdoption(current, createReaderState({ stateVersion: 4 }))).toBe("advanced");
    expect(classifyReaderStateAdoption(null, createReaderState({ stateVersion: 0 }))).toBe("advanced");
  });

  it("registers external sync conflicts only when the visible reading position differs", () => {
    const externalState = createReaderState({
      lastReadEpisodeIndex: "2",
      position: 30,
      updatedByClientId: "other-client"
    });

    expect(
      shouldRegisterReaderSyncConflict({
        nextReaderState: externalState,
        readerClientId: "client-a",
        screenMode: "reader",
        selectedNovelId: "novel-a",
        visiblePosition: { episodeIndex: "2", position: 12 }
      })
    ).toBe(true);

    expect(
      shouldRegisterReaderSyncConflict({
        nextReaderState: externalState,
        readerClientId: "client-a",
        screenMode: "reader",
        selectedNovelId: "novel-a",
        visiblePosition: { episodeIndex: "2", position: 30 }
      })
    ).toBe(false);
  });

  it("does not register conflicts outside the active selected reader context", () => {
    const externalState = createReaderState({ updatedByClientId: "other-client" });

    expect(
      shouldRegisterReaderSyncConflict({
        nextReaderState: externalState,
        readerClientId: "client-a",
        screenMode: "library",
        selectedNovelId: "novel-a",
        visiblePosition: { episodeIndex: "2", position: 12 }
      })
    ).toBe(false);
    expect(
      shouldRegisterReaderSyncConflict({
        nextReaderState: externalState,
        readerClientId: "client-a",
        screenMode: "reader",
        selectedNovelId: "novel-b",
        visiblePosition: { episodeIndex: "2", position: 12 }
      })
    ).toBe(false);
    expect(
      shouldRegisterReaderSyncConflict({
        nextReaderState: createReaderState({ updatedByClientId: "client-a" }),
        readerClientId: "client-a",
        screenMode: "reader",
        selectedNovelId: "novel-a",
        visiblePosition: { episodeIndex: "2", position: 20 }
      })
    ).toBe(false);
    expect(
      shouldRegisterReaderSyncConflict({
        nextReaderState: externalState,
        readerClientId: "client-a",
        screenMode: "reader",
        selectedNovelId: "novel-a",
        visiblePosition: { episodeIndex: null, position: 12 }
      })
    ).toBe(false);
  });

  it("keeps conflict blocking while an accepted state is older than the visible conflict", () => {
    const conflict = { serverState: createReaderState({ stateVersion: 4 }) };

    expect(shouldKeepConflictBlockedForAcceptedState(conflict, createReaderState({ stateVersion: 3 }))).toBe(true);
    expect(shouldKeepConflictBlockedForAcceptedState(conflict, createReaderState({ stateVersion: 4 }))).toBe(false);
    expect(
      shouldKeepConflictBlockedForAcceptedState(conflict, createReaderState({ novelId: "novel-b", stateVersion: 1 }))
    ).toBe(false);
  });

  it("treats same-writer conflicts as successful only when the reading position matches the request", () => {
    expect(
      isSelfWriterConflictResolution(createPutInput({ position: 30 }), createReaderState({ position: 30, updatedByClientId: "client-a" }), "client-a")
    ).toBe(true);
    expect(
      isSelfWriterConflictResolution(createPutInput({ position: 30 }), createReaderState({ position: 31, updatedByClientId: "client-a" }), "client-a")
    ).toBe(false);
    expect(
      isSelfWriterConflictResolution(
        createPutInput({ position: 30 }),
        createReaderState({ position: 30, updatedByClientId: "other-client" }),
        "client-a"
      )
    ).toBe(false);
  });

  it("ignores older conflict payloads for the currently visible conflict", () => {
    const currentConflict = { serverState: createReaderState({ stateVersion: 5 }) };

    expect(isNewerReaderSyncConflict(currentConflict, createReaderState({ stateVersion: 4 }))).toBe(false);
    expect(isNewerReaderSyncConflict(currentConflict, createReaderState({ stateVersion: 5 }))).toBe(false);
    expect(isNewerReaderSyncConflict(currentConflict, createReaderState({ stateVersion: 6 }))).toBe(true);
    expect(isNewerReaderSyncConflict(null, createReaderState({ stateVersion: 1 }))).toBe(true);
  });
});
