import { readFileSync } from "node:fs";
import vm from "node:vm";
import { fileURLToPath } from "node:url";
import { describe, expect, it, vi } from "vitest";

const readerServiceWorkerSource = readFileSync(fileURLToPath(new URL("../public/reader-sw.js", import.meta.url)), "utf8");

type ServiceWorkerEventHandler = (event: { waitUntil(promise: Promise<unknown>): void }) => void;
type ServiceWorkerFetchEventHandler = (event: {
  request: Request;
  respondWith(promise: Promise<Response>): void;
}) => void;

describe("reader service worker", () => {
  it("deletes only old shell caches during activation", async () => {
    const listeners = new Map<string, ServiceWorkerEventHandler>();
    const deleteCache = vi.fn().mockResolvedValue(true);
    const claim = vi.fn().mockResolvedValue(undefined);

    vm.runInNewContext(readerServiceWorkerSource, {
      caches: {
        delete: deleteCache,
        keys: vi
          .fn()
          .mockResolvedValue(["narou-viewer-shell-v1", "narou-viewer-shell-v2", "narou-viewer-episodes-v1", "other-app-cache"])
      },
      self: {
        addEventListener: (type: string, handler: ServiceWorkerEventHandler) => {
          listeners.set(type, handler);
        },
        clients: { claim },
        skipWaiting: vi.fn()
      }
    });

    const activateHandler = listeners.get("activate");

    expect(activateHandler).toBeTypeOf("function");

    const waitUntilPromises: Promise<unknown>[] = [];
    activateHandler?.({
      waitUntil(promise) {
        waitUntilPromises.push(promise);
      }
    });

    await Promise.all(waitUntilPromises);

    expect(deleteCache).toHaveBeenCalledTimes(1);
    expect(deleteCache).toHaveBeenCalledWith("narou-viewer-shell-v1");
    expect(claim).toHaveBeenCalledTimes(1);
  });

  it("does not intercept reader state PUT requests", async () => {
    const listeners = new Map<string, ServiceWorkerFetchEventHandler>();
    const fetch = vi.fn();

    vm.runInNewContext(readerServiceWorkerSource, {
      caches: {
        open: vi.fn(),
        delete: vi.fn(),
        keys: vi.fn()
      },
      fetch,
      Response,
      self: {
        addEventListener: (type: string, handler: ServiceWorkerFetchEventHandler) => {
          listeners.set(type, handler);
        },
        skipWaiting: vi.fn()
      }
    });

    const fetchHandler = listeners.get("fetch");
    expect(fetchHandler).toBeTypeOf("function");

    const respondWith = vi.fn();
    fetchHandler?.({
      request: new Request("http://localhost/api/reader/state", {
        method: "PUT",
        body: JSON.stringify({ novelId: "n1", lastReadEpisodeIndex: "1", position: 10 }),
        headers: { "content-type": "application/json" }
      }),
      respondWith
    });

    expect(respondWith).not.toHaveBeenCalled();
    expect(fetch).not.toHaveBeenCalled();
  });
});
