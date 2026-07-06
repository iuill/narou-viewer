import { afterEach, describe, expect, it, vi } from "vitest";
import { registerServiceWorker } from "../src/registerServiceWorker";

describe("registerServiceWorker", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("does nothing outside production builds", () => {
    const addEventListener = vi.fn();
    const register = vi.fn();

    registerServiceWorker({
      isProd: false,
      navigator: { serviceWorker: { register } } as never,
      window: { addEventListener } as never,
      console: { error: vi.fn() }
    });

    expect(addEventListener).not.toHaveBeenCalled();
    expect(register).not.toHaveBeenCalled();
  });

  it("does nothing when service workers are unavailable", () => {
    const addEventListener = vi.fn();

    registerServiceWorker({
      isProd: true,
      navigator: {} as never,
      window: { addEventListener } as never,
      console: { error: vi.fn() }
    });

    expect(addEventListener).not.toHaveBeenCalled();
  });

  it("registers the reader service worker after window load and logs failures", async () => {
    const addEventListener = vi.fn<(event: string, handler: () => void) => void>();
    let loadHandler: (() => void) | null = null;
    addEventListener.mockImplementation((event, handler) => {
      if (event === "load") {
        loadHandler = handler;
      }
    });

    const register = vi.fn().mockRejectedValue(new Error("registration failed"));
    const error = vi.fn();

    registerServiceWorker({
      isProd: true,
      navigator: { serviceWorker: { register } } as never,
      window: { addEventListener } as never,
      console: { error }
    });

    expect(addEventListener).toHaveBeenCalledWith("load", expect.any(Function));

    loadHandler?.();
    await Promise.resolve();
    await Promise.resolve();

    expect(register).toHaveBeenCalledWith("/reader-sw.js");
    expect(error).toHaveBeenCalledWith(
      "Failed to register reader service worker",
      expect.objectContaining({ message: "registration failed" })
    );
  });
});
