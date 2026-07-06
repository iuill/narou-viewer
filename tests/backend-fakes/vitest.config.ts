import { fileURLToPath } from "node:url";
import { defineProject } from "vitest/config";

const rootDir = fileURLToPath(new URL(".", import.meta.url));

export default defineProject({
  test: {
    name: "backend-fakes",
    root: rootDir,
    environment: "node",
    include: ["**/*.test.ts"],
    testTimeout: 20000
  }
});
