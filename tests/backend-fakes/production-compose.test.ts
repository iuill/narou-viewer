import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";
import { parse } from "yaml";

const repositoryRoot = fileURLToPath(new URL("../../", import.meta.url));
const environmentVariable = (contents: string): string => ["$", "{", contents, "}"].join("");
const publicHTTPPort = [
  environmentVariable("NAROU_VIEWER_HTTP_BIND:-127.0.0.1"),
  environmentVariable("NAROU_VIEWER_HTTP_PORT:-8080"),
  "80",
].join(":");

type ComposeService = {
  depends_on?: string[] | Record<string, unknown>;
  expose?: string[];
  ports?: string[];
  restart?: string;
};

type ProductionCompose = {
  services: Record<string, ComposeService>;
};

function readProductionCompose(): ProductionCompose {
  return parse(readFileSync(`${repositoryRoot}docker-compose.prod.yml`, "utf8")) as ProductionCompose;
}

function readViewerWebNginxConfig(): string {
  return readFileSync(`${repositoryRoot}deploy/viewer-web/default.conf`, "utf8").replaceAll(
    "\r\n",
    "\n",
  );
}

function locationBody(config: string, selector: string): string {
  const marker = `  location ${selector} {\n`;
  const start = config.indexOf(marker);
  const end = config.indexOf("\n  }", start + marker.length);

  expect(start, `missing location ${selector}`).toBeGreaterThanOrEqual(0);
  expect(end, `unterminated location ${selector}`).toBeGreaterThan(start);
  return config.slice(start + marker.length, end);
}

describe("production compose", () => {
  it("runs three long-lived application containers and one data initializer", () => {
    const { services } = readProductionCompose();
    const longLivedServices = Object.entries(services)
      .filter(([, service]) => service.restart === "unless-stopped")
      .map(([name]) => name)
      .sort();

    expect(Object.keys(services).sort()).toEqual([
      "novel-fetcher",
      "shared-data-init",
      "viewer-api",
      "viewer-web",
    ]);
    expect(longLivedServices).toEqual(["novel-fetcher", "viewer-api", "viewer-web"]);
  });

  it("publishes only the compatible viewer-web HTTP endpoint", () => {
    const { services } = readProductionCompose();

    expect(services["viewer-web"].depends_on).toEqual(["viewer-api"]);
    expect(services["viewer-web"].ports).toEqual([publicHTTPPort]);
    expect(services["viewer-api"].ports).toBeUndefined();
    expect(services["viewer-api"].expose).toEqual(["8080"]);
    expect(services["novel-fetcher"].ports).toBeUndefined();
    expect(services["novel-fetcher"].expose).toEqual(["33006"]);
  });
});

describe("viewer-web production Nginx", () => {
  it("proxies API traffic with the existing streaming and forwarding settings", () => {
    const config = readViewerWebNginxConfig();
    const apiLocation = locationBody(config, "/api/");

    expect(apiLocation).toContain("set $viewer_api_upstream viewer-api:8080;");
    expect(apiLocation).toContain("proxy_pass http://$viewer_api_upstream;");
    expect(apiLocation).toContain("proxy_http_version 1.1;");
    expect(apiLocation).toContain("proxy_buffering off;");
    expect(apiLocation).toContain("proxy_read_timeout 180s;");
    expect(apiLocation).toContain("proxy_set_header Host $host;");
    expect(apiLocation).toContain("proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;");
    expect(apiLocation).toContain("proxy_set_header X-Forwarded-Proto $scheme;");
    expect(locationBody(config, "= /api")).toContain("return 404;");
  });

  it("preserves no-store shell responses, security headers, and SPA fallback", () => {
    const config = readViewerWebNginxConfig();

    expect(config).toContain('add_header X-Content-Type-Options "nosniff" always;');
    expect(config).toContain('add_header Referrer-Policy "no-referrer" always;');

    for (const selector of ["= /build-info.json", "= /", "= /index.html"]) {
      const body = locationBody(config, selector);
      expect(body).toContain('add_header Cache-Control "no-store" always;');
      expect(body).toContain('add_header X-Content-Type-Options "nosniff" always;');
      expect(body).toContain('add_header Referrer-Policy "no-referrer" always;');
    }

    expect(locationBody(config, "/")).toContain("try_files $uri $uri/ /index.html;");
  });
});
