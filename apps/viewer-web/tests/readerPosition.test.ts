import { afterAll, beforeAll, describe, expect, it, vi } from "vitest";
import { getReaderVisiblePositionRange, isReaderPositionVisible, scrollReaderPositionIntoView } from "../src/readerPosition";

class HTMLElementMock {
  dataset: Record<string, string> = {};
  ownerDocument: Document | null = null;
  style: { cssText: string } = { cssText: "" };
  querySelectorAll = vi.fn(() => []);
  scrollIntoView = vi.fn();
  normalize = vi.fn();
  closest = vi.fn(() => null);
  setAttribute = vi.fn();
  remove = vi.fn();
  classList = {
    contains: vi.fn(() => false)
  };
  getBoundingClientRect = vi.fn(() => ({ top: 0, bottom: 10, left: 0, right: 10, width: 10, height: 10 }));
  getClientRects = vi.fn(() => [{ top: 0, bottom: 10, left: 0, right: 10, width: 10, height: 10 }]);
}

class TextMock {
  parentElement: HTMLElementMock | null = null;

  constructor(public data: string) {}

  get textContent(): string {
    return this.data;
  }
}

beforeAll(() => {
  vi.stubGlobal("HTMLElement", HTMLElementMock as unknown as typeof HTMLElement);
  vi.stubGlobal("Text", TextMock as unknown as typeof Text);
});

afterAll(() => {
  vi.unstubAllGlobals();
});

function createViewport() {
  const target = Object.assign(new HTMLElementMock(), {
    dataset: {
      readerPositionStart: "0",
      readerPositionEnd: "1"
    },
    scrollIntoView: vi.fn(),
    normalize: vi.fn()
  });
  const article = Object.assign(new HTMLElementMock(), {
    querySelectorAll: vi.fn(() => [target])
  });
  const viewport = {
    scrollTop: 0,
    scrollLeft: 0,
    querySelector: vi.fn(() => article)
  };

  return {
    article,
    target,
    viewport
  };
}

function createMarkerViewport() {
  const textNode = new TextMock("本文");
  const marker = new HTMLElementMock();
  const target = Object.assign(new HTMLElementMock(), {
    dataset: {
      readerPositionStart: "0",
      readerPositionEnd: "2"
    }
  });
  textNode.parentElement = target;

  const article = Object.assign(new HTMLElementMock(), {
    querySelectorAll: vi.fn(() => [target])
  });
  const viewport = {
    scrollTop: 0,
    scrollLeft: 0,
    querySelector: vi.fn(() => article)
  };

  const documentMock = {
    defaultView: {
      NodeFilter: {
        SHOW_TEXT: 4,
        FILTER_REJECT: 2,
        FILTER_ACCEPT: 1
      }
    },
    createTreeWalker: vi.fn((_root: unknown, _whatToShow: number, filter: { acceptNode(node: TextMock): number }) => {
      const accepted = filter.acceptNode(textNode) === 1;
      let consumed = false;

      return {
        nextNode() {
          if (!accepted || consumed) {
            return null;
          }

          consumed = true;
          return textNode;
        }
      };
    }),
    createRange: vi.fn(() => ({
      setStart: vi.fn(),
      setEnd: vi.fn(),
      getClientRects: vi.fn(() => []),
      getBoundingClientRect: vi.fn(() => ({ top: 0, bottom: 10, left: 0, right: 10, width: 10, height: 10 })),
      collapse: vi.fn(),
      insertNode: vi.fn()
    })),
    createElement: vi.fn(() => marker)
  } as unknown as Document;

  target.ownerDocument = documentMock;
  marker.ownerDocument = documentMock;

  return {
    marker,
    target,
    viewport
  };
}

describe("readerPosition", () => {
  it("keeps vertical restore from shifting the viewport vertically", () => {
    const { target, viewport } = createViewport();
    viewport.scrollTop = 48;
    viewport.scrollLeft = 12;

    target.scrollIntoView.mockImplementation(() => {
      viewport.scrollTop = 120;
      viewport.scrollLeft = 96;
    });

    expect(scrollReaderPositionIntoView(viewport as unknown as HTMLDivElement, 0, "vertical")).toBe(true);
    expect(target.scrollIntoView).toHaveBeenCalledWith({
      block: "start",
      inline: "start"
    });
    expect(viewport.scrollTop).toBe(48);
    expect(viewport.scrollLeft).toBe(96);
  });

  it("allows horizontal restore to adjust the vertical scroll position", () => {
    const { target, viewport } = createViewport();
    viewport.scrollTop = 32;

    target.scrollIntoView.mockImplementation(() => {
      viewport.scrollTop = 144;
    });

    expect(scrollReaderPositionIntoView(viewport as unknown as HTMLDivElement, 0, "horizontal")).toBe(true);
    expect(viewport.scrollTop).toBe(144);
  });

  it("keeps vertical restore from shifting the viewport vertically on the marker path", () => {
    const { marker, target, viewport } = createMarkerViewport();
    viewport.scrollTop = 64;
    viewport.scrollLeft = 18;

    marker.scrollIntoView.mockImplementation(() => {
      viewport.scrollTop = 156;
      viewport.scrollLeft = 104;
    });

    expect(scrollReaderPositionIntoView(viewport as unknown as HTMLDivElement, 1, "vertical")).toBe(true);
    expect(marker.scrollIntoView).toHaveBeenCalledWith({
      block: "start",
      inline: "start"
    });
    expect(viewport.scrollTop).toBe(64);
    expect(viewport.scrollLeft).toBe(104);
    expect(marker.remove).toHaveBeenCalledOnce();
    expect(target.normalize).toHaveBeenCalledOnce();
  });

  it("visible position range ignores hidden overflow targets", () => {
    const visibleTarget = Object.assign(new HTMLElementMock(), {
      dataset: {
        readerPositionStart: "10",
        readerPositionEnd: "20"
      },
      classList: {
        contains: vi.fn(() => false)
      }
    });
    const hiddenTarget = Object.assign(new HTMLElementMock(), {
      dataset: {
        readerPositionStart: "20",
        readerPositionEnd: "40"
      },
      classList: {
        contains: vi.fn((className: string) => className === "reader-page-overflow-hidden")
      }
    });
    const article = Object.assign(new HTMLElementMock(), {
      querySelectorAll: vi.fn(() => [visibleTarget, hiddenTarget])
    });
    const viewport = {
      querySelector: vi.fn(() => article),
      getBoundingClientRect: vi.fn(() => ({ top: 0, bottom: 100, left: 0, right: 100, width: 100, height: 100 }))
    };

    expect(getReaderVisiblePositionRange(viewport as unknown as HTMLDivElement)).toEqual({
      start: 10,
      end: 20
    });
  });

  it("visible position range uses visible graphemes when a text fragment crosses the page edge", () => {
    const textNode = new TextMock("abcd");
    const target = Object.assign(new HTMLElementMock(), {
      dataset: {
        readerPositionStart: "10",
        readerPositionEnd: "14"
      }
    });
    textNode.parentElement = target;

    const article = Object.assign(new HTMLElementMock(), {
      querySelectorAll: vi.fn(() => [target])
    });
    const viewport = {
      scrollTop: 0,
      scrollLeft: 0,
      clientLeft: 0,
      querySelector: vi.fn(() => article),
      getBoundingClientRect: vi.fn(() => ({ top: 0, bottom: 100, left: 0, right: 100, width: 100, height: 100 }))
    };
    const documentMock = {
      defaultView: {
        NodeFilter: {
          SHOW_TEXT: 4,
          FILTER_REJECT: 2,
          FILTER_ACCEPT: 1
        }
      },
      createTreeWalker: vi.fn((_root: unknown, _whatToShow: number, filter: { acceptNode(node: TextMock): number }) => {
        const accepted = filter.acceptNode(textNode) === 1;
        let consumed = false;

        return {
          nextNode() {
            if (!accepted || consumed) {
              return null;
            }

            consumed = true;
            return textNode;
          }
        };
      }),
      createRange: vi.fn(() => {
        let startOffset = 0;
        const getRect = () =>
          startOffset < 2
            ? { top: 0, bottom: 10, left: 0, right: 10, width: 10, height: 10 }
            : { top: 0, bottom: 10, left: 120, right: 130, width: 10, height: 10 };

        return {
          setStart: vi.fn((_node: TextMock, offset: number) => {
            startOffset = offset;
          }),
          setEnd: vi.fn(),
          getClientRects: vi.fn(() => [getRect()]),
          getBoundingClientRect: vi.fn(getRect)
        };
      })
    } as unknown as Document;

    target.ownerDocument = documentMock;

    expect(getReaderVisiblePositionRange(viewport as unknown as HTMLDivElement)).toEqual({
      start: 10,
      end: 12
    });
  });

  it("visible position range follows per-grapheme visibility fragments in vertical mode", () => {
    const fragments = [0, 1, 2, 3].map((index) =>
      Object.assign(new HTMLElementMock(), {
        classList: {
          contains: vi.fn((className: string) => className === "reader-page-overflow-hidden" && index >= 2)
        }
      })
    );
    const target = Object.assign(new HTMLElementMock(), {
      dataset: {
        readerPositionStart: "20",
        readerPositionEnd: "24"
      },
      querySelectorAll: vi.fn(() => fragments)
    });
    const article = Object.assign(new HTMLElementMock(), {
      querySelectorAll: vi.fn(() => [target])
    });
    const viewport = {
      scrollTop: 0,
      scrollLeft: 0,
      clientLeft: 0,
      querySelector: vi.fn(() => article),
      getBoundingClientRect: vi.fn(() => ({ top: 0, bottom: 100, left: 0, right: 100, width: 100, height: 100 }))
    };

    expect(
      getReaderVisiblePositionRange(viewport as unknown as HTMLDivElement, {
        mode: "vertical",
        currentVerticalPage: {
          start: 0,
          end: 100,
          shiftX: 0
        }
      })
    ).toEqual({
      start: 20,
      end: 22
    });
  });

  it("position visibility treats a visible next fragment at a range boundary as current-page content", () => {
    const previousTarget = Object.assign(new HTMLElementMock(), {
      dataset: {
        readerPositionStart: "10",
        readerPositionEnd: "20"
      }
    });
    const nextFragments = [0, 1, 2, 3].map(() => new HTMLElementMock());
    const nextTarget = Object.assign(new HTMLElementMock(), {
      dataset: {
        readerPositionStart: "20",
        readerPositionEnd: "24"
      },
      querySelectorAll: vi.fn(() => nextFragments)
    });
    const article = Object.assign(new HTMLElementMock(), {
      querySelectorAll: vi.fn(() => [previousTarget, nextTarget])
    });
    const viewport = {
      scrollTop: 0,
      scrollLeft: 0,
      clientLeft: 0,
      querySelector: vi.fn(() => article),
      getBoundingClientRect: vi.fn(() => ({ top: 0, bottom: 100, left: 0, right: 100, width: 100, height: 100 }))
    };

    expect(
      isReaderPositionVisible(viewport as unknown as HTMLDivElement, 20, {
        mode: "vertical",
        currentVerticalPage: {
          start: 0,
          end: 100,
          shiftX: 0
        }
      })
    ).toBe(true);
  });

  it("position visibility rejects hidden per-grapheme fragments in vertical mode", () => {
    const fragments = [0, 1, 2, 3].map((index) =>
      Object.assign(new HTMLElementMock(), {
        classList: {
          contains: vi.fn((className: string) => className === "reader-page-overflow-hidden" && index === 2)
        }
      })
    );
    const target = Object.assign(new HTMLElementMock(), {
      dataset: {
        readerPositionStart: "20",
        readerPositionEnd: "24"
      },
      querySelectorAll: vi.fn(() => fragments)
    });
    const article = Object.assign(new HTMLElementMock(), {
      querySelectorAll: vi.fn(() => [target])
    });
    const viewport = {
      scrollTop: 0,
      scrollLeft: 0,
      clientLeft: 0,
      querySelector: vi.fn(() => article),
      getBoundingClientRect: vi.fn(() => ({ top: 0, bottom: 100, left: 0, right: 100, width: 100, height: 100 }))
    };

    expect(
      isReaderPositionVisible(viewport as unknown as HTMLDivElement, 22, {
        mode: "vertical",
        currentVerticalPage: {
          start: 0,
          end: 100,
          shiftX: 0
        }
      })
    ).toBe(false);
  });
});
