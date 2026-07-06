import fs from "node:fs";
import path from "node:path";
import { defineConfig } from "@playwright/test";
import { playwrightProjects } from "./playwright.targets";

const apiContractHeaders = {
  "x-narou-viewer-api-contract-version": "1",
  "x-narou-viewer-client-build": "playwright-e2e"
};

function parseDevContainerEnvValue(rawValue: string): string {
  const value = rawValue.trim();
  const quote = value[0];

  if (quote === "\"" || quote === "'") {
    const end = value.indexOf(quote, 1);
    return end >= 0 ? value.slice(1, end) : value.slice(1);
  }

  return value.replace(/\s+#.*$/u, "").trim();
}

function readDevContainerEnvValue(key: string): string | undefined {
  const envPath = path.resolve(process.cwd(), ".devcontainer", ".env");
  if (!fs.existsSync(envPath)) {
    return undefined;
  }

  const lines = fs.readFileSync(envPath, "utf8").split(/\r?\n/u);
  for (const rawLine of lines) {
    const line = rawLine.trim();
    if (!line.startsWith(`${key}=`)) {
      continue;
    }

    return parseDevContainerEnvValue(line.slice(key.length + 1));
  }

  return undefined;
}

const e2eHostPort = process.env.VIEWER_WEB_E2E_HOST_PORT ?? readDevContainerEnvValue("VIEWER_WEB_E2E_HOST_PORT") ?? "15173";
const defaultBaseURL =
  fs.existsSync("/.dockerenv") || process.env.CODESPACES === "true"
    ? "http://viewer-web-e2e:15173"
    : `http://127.0.0.1:${e2eHostPort}`;
const baseURL = process.env.PLAYWRIGHT_BASE_URL ?? defaultBaseURL;
const htmlOutputDir = path.resolve(process.cwd(), process.env.PLAYWRIGHT_HTML_OUTPUT_DIR ?? "playwright-report");
const isGitHubActions = process.env.GITHUB_ACTIONS === "true";
const progressReporter = process.stdout.isTTY ? "list" : "line";

function resolveArtifactMode<const T extends readonly string[]>(
  envName: string,
  envValue: string | undefined,
  allowedValues: T,
  fallback: T[number]
): T[number] {
  if (!envValue) {
    return fallback;
  }

  if (allowedValues.includes(envValue)) {
    return envValue;
  }

  throw new Error(`${envName} must be one of: ${allowedValues.join(", ")}`);
}

const traceMode = resolveArtifactMode(
  "PLAYWRIGHT_TRACE_MODE",
  process.env.PLAYWRIGHT_TRACE_MODE,
  ["off", "on", "retain-on-failure", "on-first-retry", "on-all-retries", "retain-on-first-failure"] as const,
  isGitHubActions ? "retain-on-failure" : "on"
);
const screenshotMode = resolveArtifactMode(
  "PLAYWRIGHT_SCREENSHOT_MODE",
  process.env.PLAYWRIGHT_SCREENSHOT_MODE,
  ["off", "on", "only-on-failure", "on-first-failure"] as const,
  isGitHubActions ? "only-on-failure" : "on"
);

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  workers: process.env.CI ? 2 : undefined,
  forbidOnly: !!process.env.CI,
  failOnFlakyTests: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  reporter: [[progressReporter], ["html", { open: "never", outputFolder: htmlOutputDir }]],
  use: {
    baseURL,
    extraHTTPHeaders: apiContractHeaders,
    trace: traceMode,
    screenshot: screenshotMode
  },
  projects: playwrightProjects,
  expect: {
    timeout: 10_000
  },
  timeout: 30_000
});
