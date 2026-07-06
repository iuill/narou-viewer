import path from "node:path";
import { devices, type PlaywrightTestOptions, type PlaywrightTestProject } from "@playwright/test";

type TargetEnvironment = {
  name: string;
  use: PlaywrightTestOptions;
};

const { defaultBrowserType: _unusedBrowserType, ...iPhone16eFallbackDevice } = devices["iPhone 14"];
const playwrightResultsRootDir = process.env.PLAYWRIGHT_RESULTS_ROOT_DIR ?? "test-results";

// Keep target definitions centralized here so new environments can be added with one small edit.
export const targetEnvironments: TargetEnvironment[] = [
  {
    name: "pc-xga",
    use: {
      browserName: "chromium",
      viewport: {
        width: 1024,
        height: 768
      }
    }
  },
  {
    name: "iphone-16e",
    use: {
      // Playwright does not provide a built-in iPhone 16e preset yet.
      // Reuse the closest built-in iPhone profile so mobile emulation stays enabled.
      ...iPhone16eFallbackDevice,
      browserName: "webkit"
    }
  }
];

export const playwrightProjects: PlaywrightTestProject[] = targetEnvironments.map((target) => ({
  name: target.name,
  outputDir: path.join(playwrightResultsRootDir, target.name),
  use: target.use
}));
