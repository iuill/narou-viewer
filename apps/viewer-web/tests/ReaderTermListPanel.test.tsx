import { afterEach, describe, expect, it, vi } from "vitest";
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { JSDOM } from "jsdom";
import { ReaderTermListPanel } from "../src/ReaderTermListPanel";

describe("ReaderTermListPanel", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("shows category and description while hiding a missing reading", async () => {
    const dom = new JSDOM(
      '<!doctype html><html><body><div id="root"></div></body></html>',
    );
    vi.stubGlobal("window", dom.window);
    vi.stubGlobal("document", dom.window.document);
    vi.stubGlobal("HTMLElement", dom.window.HTMLElement);
    vi.stubGlobal("HTMLSelectElement", dom.window.HTMLSelectElement);
    vi.stubGlobal("Node", dom.window.Node);
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    const container = dom.window.document.getElementById("root");
    if (!container) throw new Error("root container not found");
    const root = createRoot(container);
    await act(async () => {
      root.render(
        createElement(ReaderTermListPanel, {
          activeJobs: [],
          canClear: true,
          canGenerate: true,
          completedJobs: [],
          data: {
            status: "ready",
            novelId: "novel-a",
            upToEpisodeIndex: "2",
            processedUpToEpisodeIndex: "20",
            terms: [
              {
                term: "聖剣",
                reading: "せいけん",
                category: "item",
                description: "王家に伝わる剣。",
              },
              {
                term: "北の塔",
                reading: null,
                category: "place",
                description: "雪原に立つ塔。",
              },
            ],
          },
          error: null,
          formatEpisodeOrderLabel: (value: string) => value,
          isClearing: false,
          includeCurrentEpisode: false,
          isLoading: false,
          isSubmitting: false,
          notice: null,
          onClear: vi.fn(),
          onClose: vi.fn(),
          onIncludeCurrentEpisodeChange: vi.fn(),
          onRequestedGenerationStrategyChange: vi.fn(),
          onRequestedUpToEpisodeIndexChange: vi.fn(),
          onShowCharacters: vi.fn(),
          onSubmit: vi.fn(),
          requestedGenerationStrategy: "parallel_identity",
          requestedUpToEpisodeIndex: "2",
          defaultUpToEpisodeIndex: "2",
        }),
      );
    });
    expect(container.textContent).toContain("聖剣");
    expect(container.textContent).toContain("せいけん");
    expect(container.textContent).toContain("物品");
    expect(container.textContent).toContain("北の塔");
    expect(container.textContent).toContain("場所");
    expect(container.textContent).toContain("第2話時点 / 2 / 2 用語");
    expect(container.querySelectorAll(".reader-term-title span")).toHaveLength(
      1,
    );
    const categorySelect = Array.from(container.querySelectorAll("select")).find((select) =>
      select.textContent?.includes("物品")
    );
    if (!(categorySelect instanceof dom.window.HTMLSelectElement)) {
      throw new Error("category select not found");
    }
    await act(async () => {
      categorySelect.value = "item";
      categorySelect.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
    });
    expect(container.textContent).toContain("第2話時点 / 1 / 2 用語");
    expect(container.textContent).not.toContain("北の塔");
    await act(async () => root.unmount());
  });
});
