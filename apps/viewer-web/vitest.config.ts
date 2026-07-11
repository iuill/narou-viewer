import { fileURLToPath } from "node:url";
import { defineProject } from "vitest/config";

const rootDir = fileURLToPath(new URL(".", import.meta.url));

export default defineProject({
  test: {
    name: "viewer-web",
    root: rootDir,
    environment: "node",
    include: ["tests/**/*.test.{ts,tsx}"]
  }
});
