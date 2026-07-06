type PwaManifestIcon = {
  src: string;
  sizes: string;
  type: string;
  purpose: "any";
};

type PwaManifest = {
  id: "/";
  name: string;
  short_name: string;
  description: string;
  lang: string;
  start_url: "/";
  scope: "/";
  display: "standalone";
  background_color: string;
  theme_color: string;
  categories: string[];
  icons: PwaManifestIcon[];
};

export const PUBLIC_ICON_FILENAMES = [
  "apple-touch-icon.png",
  "pwa-192x192.png",
  "pwa-512x512.png",
  "favicon.ico",
  "favicon-32x32.png"
] as const;

type PublicIconFilename = (typeof PUBLIC_ICON_FILENAMES)[number];

export function normalizePublicAssetBaseUrl(value: string | undefined | null): string | null {
  const trimmed = value?.trim();

  if (!trimmed) {
    return null;
  }

  let parsed: URL;

  try {
    parsed = new URL(trimmed);
  } catch {
    throw new Error("VITE_PUBLIC_ASSET_BASE_URL must be an absolute http(s) URL.");
  }

  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    throw new Error("VITE_PUBLIC_ASSET_BASE_URL must use http or https.");
  }

  if (parsed.username || parsed.password || parsed.search || parsed.hash) {
    throw new Error("VITE_PUBLIC_ASSET_BASE_URL must not include credentials, query parameters, or a hash.");
  }

  if (parsed.pathname !== "/") {
    throw new Error("VITE_PUBLIC_ASSET_BASE_URL must not include a path.");
  }

  return parsed.origin;
}

export function resolvePublicAssetUrl(filename: PublicIconFilename, publicAssetBaseUrl: string | null): string {
  return publicAssetBaseUrl ? `${publicAssetBaseUrl}/${filename}` : `/${filename}`;
}

export function buildPwaManifest(publicAssetBaseUrl: string | null): PwaManifest {
  return {
    id: "/",
    name: "Web小説ビューア",
    short_name: "小説ビューア",
    description: "Web小説を一覧・目次・本文から快適に読むための個人利用向けビューアです。",
    lang: "ja-JP",
    start_url: "/",
    scope: "/",
    display: "standalone",
    background_color: "#f4efe6",
    theme_color: "#9d4b2f",
    categories: ["books", "entertainment"],
    icons: [
      {
        src: resolvePublicAssetUrl("pwa-192x192.png", publicAssetBaseUrl),
        sizes: "192x192",
        type: "image/png",
        purpose: "any"
      },
      {
        src: resolvePublicAssetUrl("pwa-512x512.png", publicAssetBaseUrl),
        sizes: "512x512",
        type: "image/png",
        purpose: "any"
      }
    ]
  };
}
