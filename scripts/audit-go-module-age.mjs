#!/usr/bin/env bun

import { spawnSync } from "node:child_process";

const minimumAgeSeconds = Number.parseInt(process.env.GO_MINIMUM_RELEASE_AGE_SECONDS ?? "1814400", 10);
const moduleRoots = (process.env.GO_MODULE_AGE_MODULE_ROOTS ?? "apps/viewer-api-go services/novel-fetcher")
  .split(/\s+/)
  .filter(Boolean);
const extraModuleQueries = (process.env.GO_MODULE_AGE_EXTRA_QUERIES ?? "golang.org/x/vuln@v1.2.0")
  .split(/\s+/)
  .filter(Boolean);
const allowedModuleKeys = new Set((process.env.GO_MODULE_AGE_ALLOWLIST ?? "").split(/\s+/).filter(Boolean));
const now = Date.now();

function runGoList(moduleRoot, args) {
  const result = spawnSync("go", ["list", "-m", "-json", ...args], {
    cwd: moduleRoot,
    encoding: "utf8",
  });

  if (result.status !== 0) {
    process.stderr.write(result.stderr);
    process.exit(result.status ?? 1);
  }

  return result.stdout;
}

function parseConcatenatedJson(input) {
  const modules = [];
  let depth = 0;
  let start = -1;
  let inString = false;
  let escaped = false;

  for (let index = 0; index < input.length; index += 1) {
    const char = input[index];

    if (inString) {
      if (escaped) {
        escaped = false;
      } else if (char === "\\") {
        escaped = true;
      } else if (char === '"') {
        inString = false;
      }
      continue;
    }

    if (char === '"') {
      inString = true;
      continue;
    }

    if (char === "{") {
      if (depth === 0) {
        start = index;
      }
      depth += 1;
      continue;
    }

    if (char === "}") {
      depth -= 1;
      if (depth === 0 && start >= 0) {
        modules.push(JSON.parse(input.slice(start, index + 1)));
        start = -1;
      }
    }
  }

  return modules;
}

function moduleKey(module) {
  return `${module.Path}@${module.Version ?? ""}`;
}

const moduleMap = new Map();
for (const moduleRoot of moduleRoots) {
  for (const module of parseConcatenatedJson(runGoList(moduleRoot, ["all"]))) {
    moduleMap.set(moduleKey(module), module);
  }

  for (const query of extraModuleQueries) {
    for (const module of parseConcatenatedJson(runGoList(moduleRoot, [query]))) {
      moduleMap.set(moduleKey(module), module);
    }
  }
}

const tooNewModules = [];
const allowlistedModules = [];
for (const module of moduleMap.values()) {
  if (!module.Version || !module.Time) {
    continue;
  }

  const ageSeconds = Math.floor((now - Date.parse(module.Time)) / 1000);
  if (ageSeconds >= minimumAgeSeconds) {
    continue;
  }

  if (allowedModuleKeys.has(moduleKey(module))) {
    allowlistedModules.push({ module, ageSeconds });
  } else {
    tooNewModules.push({ module, ageSeconds });
  }
}

if (tooNewModules.length > 0) {
  console.error(`Go modules newer than ${minimumAgeSeconds} seconds are not allowed:`);
  for (const { module, ageSeconds } of tooNewModules) {
    console.error(`- ${module.Path}@${module.Version} (${ageSeconds} seconds old, released ${module.Time})`);
  }
  process.exit(1);
}

if (allowlistedModules.length > 0) {
  console.log(`Allowlisted Go modules newer than ${minimumAgeSeconds} seconds:`);
  for (const { module, ageSeconds } of allowlistedModules) {
    console.log(`- ${module.Path}@${module.Version} (${ageSeconds} seconds old, released ${module.Time})`);
  }
}

console.log(`All Go modules are old enough or explicitly allowlisted.`);
