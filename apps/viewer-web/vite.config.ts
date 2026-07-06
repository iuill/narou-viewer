import { defineConfig, type Plugin, type ViteDevServer } from "vite";
import react from "@vitejs/plugin-react";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { buildPwaManifest, normalizePublicAssetBaseUrl, resolvePublicAssetUrl } from "./src/pwaAssets";

type ViewerBuildInfo = {
  version: string;
  gitHash: string | null;
  gitShortHash: string | null;
  gitCommitDate: string | null;
};

function readWorkspacePackageVersion(): string {
  const packageJsonPath = new URL("./package.json", import.meta.url);
  const packageJson = JSON.parse(readFileSync(packageJsonPath, "utf8")) as { version?: string };

  return packageJson.version?.trim() || "0.0.0";
}

function readGitValue(args: string[]): string | null {
  try {
    const output = execFileSync("git", args, {
      cwd: fileURLToPath(new URL("../../", import.meta.url)),
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"]
    }).trim();

    return output.length > 0 ? output : null;
  } catch {
    return null;
  }
}

function resolveViewerBuildInfo(): ViewerBuildInfo {
  const version = process.env.VITE_APP_VERSION?.trim() || readWorkspacePackageVersion();
  const gitHash = process.env.VITE_APP_GIT_SHA?.trim() || readGitValue(["rev-parse", "HEAD"]);
  const gitCommitDate = process.env.VITE_APP_GIT_COMMIT_DATE?.trim() || readGitValue(["log", "-1", "--format=%cI"]);

  return {
    version,
    gitHash,
    gitShortHash: gitHash ? gitHash.slice(0, 7) : null,
    gitCommitDate
  };
}

function resolveApiProxyTarget(): string {
  return process.env.VITE_API_PROXY_TARGET ?? `http://localhost:${process.env.VIEWER_API_PORT ?? 8080}`;
}

const viewerBuildInfo = resolveViewerBuildInfo();
const publicAssetBaseUrl = normalizePublicAssetBaseUrl(process.env.VITE_PUBLIC_ASSET_BASE_URL);

function createPwaManifestSource(): string {
  return `${JSON.stringify(buildPwaManifest(publicAssetBaseUrl), null, 2)}\n`;
}

function createBuildInfoSource(): string {
  return `${JSON.stringify(viewerBuildInfo, null, 2)}\n`;
}

function createPwaAssetPlugin(): Plugin {
  const replacements = new Map<string, string>([
    ["__PWA_FAVICON_ICO_URL__", resolvePublicAssetUrl("favicon.ico", publicAssetBaseUrl)],
    ["__PWA_FAVICON_PNG_URL__", resolvePublicAssetUrl("favicon-32x32.png", publicAssetBaseUrl)],
    ["__PWA_APPLE_TOUCH_ICON_URL__", resolvePublicAssetUrl("apple-touch-icon.png", publicAssetBaseUrl)]
  ]);

  return {
    name: "narou-viewer-pwa-assets",
    configureServer(server: ViteDevServer) {
      server.middlewares.use((req, res, next) => {
        const requestPath = req.url?.split("?")[0] ?? "";

        if (requestPath === "/manifest.webmanifest") {
          res.setHeader("Content-Type", "application/manifest+json; charset=utf-8");
          res.end(createPwaManifestSource());
          return;
        }

        if (requestPath === "/build-info.json") {
          res.setHeader("Cache-Control", "no-store");
          res.setHeader("Content-Type", "application/json; charset=utf-8");
          res.end(createBuildInfoSource());
          return;
        }

        next();
      });
    },
    transformIndexHtml(html: string) {
      let transformed = html;

      for (const [placeholder, value] of replacements) {
        transformed = transformed.replaceAll(placeholder, value);
      }

      return transformed;
    },
    generateBundle() {
      this.emitFile({
        type: "asset",
        fileName: "manifest.webmanifest",
        source: createPwaManifestSource()
      });
      this.emitFile({
        type: "asset",
        fileName: "build-info.json",
        source: createBuildInfoSource()
      });
    }
  };
}

export default defineConfig({
  // node_modules 配下で root 所有のキャッシュが残ると Vite が再生成できないため、
  // 共有ワークスペースの一時ディレクトリへキャッシュを逃がす。
  cacheDir: fileURLToPath(new URL("../../.tmp/viewer-web/vite", import.meta.url)),
  define: {
    __APP_BUILD_INFO__: JSON.stringify(viewerBuildInfo)
  },
  plugins: [react(), createPwaAssetPlugin()],
  server: {
    allowedHosts: true,
    watch: {
      // Dev Container + Windows host volume でも変更検知できるようにする
      usePolling: true,
      interval: 200
    },
    proxy: {
      "/api": {
        target: resolveApiProxyTarget(),
        changeOrigin: true
      }
    }
  },
  preview: {
    allowedHosts: true,
    proxy: {
      "/api": {
        target: resolveApiProxyTarget(),
        changeOrigin: true
      }
    }
  }
});
