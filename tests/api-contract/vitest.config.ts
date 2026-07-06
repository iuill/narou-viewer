import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    name: "api-contract",
    root: new URL(".", import.meta.url).pathname,
    environment: "node",
    include: ["cases/**/*.test.ts"],
    fileParallelism: false,
    testTimeout: 20000,
    sequence: {
      concurrent: false
    }
  }
});
