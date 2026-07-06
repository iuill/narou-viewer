import { describe, expect, it } from "vitest";
import { buildPwaManifest, normalizePublicAssetBaseUrl, resolvePublicAssetUrl } from "../src/pwaAssets";

describe("pwa assets", () => {
  it("falls back to same-origin asset paths when no public asset base URL is configured", () => {
    expect(normalizePublicAssetBaseUrl(undefined)).toBeNull();
    expect(resolvePublicAssetUrl("apple-touch-icon.png", null)).toBe("/apple-touch-icon.png");

    expect(buildPwaManifest(null)).toMatchObject({
      name: "Web小説ビューア",
      short_name: "小説ビューア",
      description: "Web小説を一覧・目次・本文から快適に読むための個人利用向けビューアです。"
    });
    expect(buildPwaManifest(null).icons).toEqual([
      {
        src: "/pwa-192x192.png",
        sizes: "192x192",
        type: "image/png",
        purpose: "any"
      },
      {
        src: "/pwa-512x512.png",
        sizes: "512x512",
        type: "image/png",
        purpose: "any"
      }
    ]);
  });

  it("normalizes a configured public asset base URL and builds absolute icon URLs", () => {
    const publicAssetBaseUrl = normalizePublicAssetBaseUrl(" https://203.0.113.10:4443/ ");

    expect(publicAssetBaseUrl).toBe("https://203.0.113.10:4443");
    expect(resolvePublicAssetUrl("favicon.ico", publicAssetBaseUrl)).toBe("https://203.0.113.10:4443/favicon.ico");

    expect(buildPwaManifest(publicAssetBaseUrl).icons).toEqual([
      {
        src: "https://203.0.113.10:4443/pwa-192x192.png",
        sizes: "192x192",
        type: "image/png",
        purpose: "any"
      },
      {
        src: "https://203.0.113.10:4443/pwa-512x512.png",
        sizes: "512x512",
        type: "image/png",
        purpose: "any"
      }
    ]);
  });

  it("rejects invalid public asset base URLs early", () => {
    expect(() => normalizePublicAssetBaseUrl("203.0.113.10:4443")).toThrowError(
      "VITE_PUBLIC_ASSET_BASE_URL must be an absolute http(s) URL."
    );
    expect(() => normalizePublicAssetBaseUrl("https://203.0.113.10:4443/assets")).toThrowError(
      "VITE_PUBLIC_ASSET_BASE_URL must not include a path."
    );
    expect(() => normalizePublicAssetBaseUrl("https://203.0.113.10:4443?icon=1")).toThrowError(
      "VITE_PUBLIC_ASSET_BASE_URL must not include credentials, query parameters, or a hash."
    );
  });
});
