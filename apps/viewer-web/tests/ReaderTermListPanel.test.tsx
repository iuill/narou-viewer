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
    vi.stubGlobal("Node", dom.window.Node);
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    const container = dom.window.document.getElementById("root");
    if (!container) throw new Error("root container not found");
    const root = createRoot(container);
    await act(async () => {
      root.render(
        createElement(ReaderTermListPanel, {
          data: {
            status: "ready",
            novelId: "novel-a",
            upToEpisodeIndex: "2",
            processedUpToEpisodeIndex: "2",
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
          isLoading: false,
          notice: null,
          onClose: vi.fn(),
        }),
      );
    });
    expect(container.textContent).toContain("聖剣");
    expect(container.textContent).toContain("せいけん");
    expect(container.textContent).toContain("物品");
    expect(container.textContent).toContain("北の塔");
    expect(container.textContent).toContain("場所");
    expect(container.querySelectorAll(".reader-term-title span")).toHaveLength(
      1,
    );
    await act(async () => root.unmount());
  });
});
