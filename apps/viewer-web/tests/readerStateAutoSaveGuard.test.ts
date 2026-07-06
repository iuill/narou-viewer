import { describe, expect, it } from "vitest";
import { consumeAppliedReaderStateAutoSaveGuard } from "../src/readerStateAutoSaveGuard";

describe("consumeAppliedReaderStateAutoSaveGuard", () => {
  const guard = {
    novelId: "n1",
    episodeIndex: "1",
    readingStateKey: "n1:1:8"
  };

  it("skips only the current mismatched save and clears the guard", () => {
    const result = consumeAppliedReaderStateAutoSaveGuard(guard, "n1", "1", "n1:1:0");

    expect(result).toEqual({
      nextGuard: null,
      shouldSkipCurrentSave: true
    });
  });

  it("clears the guard without skipping when the applied position is observed", () => {
    const result = consumeAppliedReaderStateAutoSaveGuard(guard, "n1", "1", "n1:1:8");

    expect(result).toEqual({
      nextGuard: null,
      shouldSkipCurrentSave: false
    });
  });

  it("clears the guard without skipping after moving to another episode", () => {
    const result = consumeAppliedReaderStateAutoSaveGuard(guard, "n1", "2", "n1:2:0");

    expect(result).toEqual({
      nextGuard: null,
      shouldSkipCurrentSave: false
    });
  });

  it("keeps unrelated novel guards untouched", () => {
    const result = consumeAppliedReaderStateAutoSaveGuard(guard, "n2", "1", "n2:1:0");

    expect(result).toEqual({
      nextGuard: guard,
      shouldSkipCurrentSave: false
    });
  });
});
