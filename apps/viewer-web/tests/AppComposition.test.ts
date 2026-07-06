import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  currentModel: {
    routeController: { marker: "route" },
    screen: {
      type: "library",
      library: { marker: "library" }
    }
  } as {
    routeController: { marker: string };
    screen:
      | { type: "library"; library: { marker: string } }
      | { type: "reader"; reader: { marker: string } };
  }
}));

vi.mock("../src/app/useViewerAppModel", () => ({
  useViewerAppModel: () => mocks.currentModel
}));

vi.mock("../src/routes/ReaderRouteController", () => ({
  ReaderRouteController: (props: { marker?: string }) =>
    createElement("div", { "data-component": "route", "data-marker": props.marker })
}));

vi.mock("../src/screens/LibraryShell", () => ({
  LibraryShell: (props: { marker?: string }) =>
    createElement("div", { "data-component": "library", "data-marker": props.marker })
}));

vi.mock("../src/screens/ReaderShell", () => ({
  ReaderShell: (props: { marker?: string }) =>
    createElement("div", { "data-component": "reader", "data-marker": props.marker })
}));

describe("App composition", () => {
  it("renders the route controller and library shell from the app model", async () => {
    mocks.currentModel = {
      routeController: { marker: "route" },
      screen: {
        type: "library",
        library: { marker: "library" }
      }
    };
    const { default: App } = await import("../src/App");

    const html = renderToStaticMarkup(createElement(App));

    expect(html).toContain('data-component="route"');
    expect(html).toContain('data-marker="route"');
    expect(html).toContain('data-component="library"');
    expect(html).toContain('data-marker="library"');
    expect(html).not.toContain('data-component="reader"');
  });

  it("renders the reader shell when the app model selects reader mode", async () => {
    mocks.currentModel = {
      routeController: { marker: "route" },
      screen: {
        type: "reader",
        reader: { marker: "reader" }
      }
    };
    const { default: App } = await import("../src/App");

    const html = renderToStaticMarkup(createElement(App));

    expect(html).toContain('data-component="route"');
    expect(html).toContain('data-component="reader"');
    expect(html).toContain('data-marker="reader"');
    expect(html).not.toContain('data-component="library"');
  });
});
