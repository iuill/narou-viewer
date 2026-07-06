#!/usr/bin/env bun

import { existsSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const rootEnvPath = path.resolve(scriptDir, "../.env.local");
const forwardedArgs = process.argv.slice(2);

if (forwardedArgs.length === 0) {
  console.error("Usage: bun scripts/run-with-root-env.mjs <command> [...args]");
  process.exit(1);
}

function parseDotEnvValue(rawValue) {
  const trimmed = rawValue.trim();
  if (!trimmed) {
    return "";
  }

  if ((trimmed.startsWith('"') && trimmed.endsWith('"')) || (trimmed.startsWith("'") && trimmed.endsWith("'"))) {
    const inner = trimmed.slice(1, -1);
    if (trimmed.startsWith('"')) {
      return inner
        .replace(/\\n/g, "\n")
        .replace(/\\r/g, "\r")
        .replace(/\\t/g, "\t")
        .replace(/\\"/g, '"')
        .replace(/\\\\/g, "\\");
    }

    return inner;
  }

  return trimmed.replace(/\s+#.*$/, "");
}

function loadRootEnvFile(filePath) {
  if (!existsSync(filePath)) {
    return {};
  }

  const fileText = readFileSync(filePath, "utf8");
  const parsed = {};
  for (const rawLine of fileText.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line || line.startsWith("#")) {
      continue;
    }

    const normalized = line.startsWith("export ") ? line.slice("export ".length) : line;
    const separatorIndex = normalized.indexOf("=");
    if (separatorIndex <= 0) {
      continue;
    }

    const key = normalized.slice(0, separatorIndex).trim();
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(key)) {
      continue;
    }

    const value = normalized.slice(separatorIndex + 1);
    parsed[key] = parseDotEnvValue(value);
  }

  return parsed;
}

const command = forwardedArgs;
const rootEnv = loadRootEnvFile(rootEnvPath);

const child = Bun.spawn(command, {
  cwd: process.cwd(),
  env: {
    ...rootEnv,
    ...process.env,
  },
  stdin: "inherit",
  stdout: "inherit",
  stderr: "inherit",
});

process.exit(await child.exited);
