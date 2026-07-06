import { describe, expect, it, vi } from "vitest";
import { bootstrap } from "../src/main";

describe("main bootstrap", () => {
  it("registers the service worker and renders the app into the root element", async () => {
    const registerServiceWorker = vi.fn();
    const render = vi.fn();
    const createRoot = vi.fn().mockReturnValue({ render });
    const rootElement = { id: "root" };
    const AppComponent = () => null;

    bootstrap({
      registerServiceWorker,
      createRoot: createRoot as never,
      rootElement: rootElement as never,
      AppComponent
    });

    expect(registerServiceWorker).toHaveBeenCalledTimes(1);
    expect(createRoot).toHaveBeenCalledWith(rootElement);
    expect(render).toHaveBeenCalledTimes(1);
  });
});
