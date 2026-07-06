#!/usr/bin/env bun

import { readFile } from "node:fs/promises";

const expectedGoVersion = (await readFile(".go-version", "utf8")).trim();

if (!/^\d+\.\d+\.\d+$/.test(expectedGoVersion)) {
  console.error(`.go-version must contain a full Go patch version, got: ${expectedGoVersion}`);
  process.exit(1);
}

const expectedGoImage = `golang:${expectedGoVersion}-bookworm`;
const checks = [
  {
    file: "apps/viewer-api-go/go.mod",
    pattern: new RegExp(`^go ${escapeRegExp(expectedGoVersion)}$`, "m"),
    description: `go directive must be ${expectedGoVersion}`,
  },
  {
    file: "services/novel-fetcher/go.mod",
    pattern: new RegExp(`^go ${escapeRegExp(expectedGoVersion)}$`, "m"),
    description: `go directive must be ${expectedGoVersion}`,
  },
  {
    file: ".devcontainer/viewer-dev/Dockerfile",
    pattern: new RegExp(`^ARG GO_VERSION=${escapeRegExp(expectedGoVersion)}$`, "m"),
    description: `GO_VERSION arg must be ${expectedGoVersion}`,
  },
  {
    file: ".devcontainer/docker-compose.yml",
    pattern: new RegExp(escapeRegExp(`image: ${expectedGoImage}`)),
    description: `novel-fetcher dev image must be ${expectedGoImage}`,
  },
  {
    file: ".devcontainer/docker-compose.yml",
    pattern: new RegExp(escapeRegExp(`image: ${"${NOVEL_FETCHER_E2E_IMAGE:-"}${expectedGoImage}}`)),
    description: `novel-fetcher E2E default image must be ${expectedGoImage}`,
  },
  {
    file: ".devcontainer/docker-compose.yml",
    pattern: new RegExp(escapeRegExp(`image: ${"${VIEWER_API_GO_E2E_IMAGE:-"}${expectedGoImage}}`)),
    description: `viewer-api-go E2E default image must be ${expectedGoImage}`,
  },
  {
    file: "deploy/viewer-api-go/Dockerfile",
    pattern: new RegExp(`^FROM ${escapeRegExp(expectedGoImage)} AS build$`, "m"),
    description: `production viewer-api-go build image must be ${expectedGoImage}`,
  },
  {
    file: "deploy/novel-fetcher/Dockerfile",
    pattern: new RegExp(`^FROM ${escapeRegExp(expectedGoImage)} AS build$`, "m"),
    description: `production novel-fetcher build image must be ${expectedGoImage}`,
  },
  {
    file: "README.md",
    pattern: new RegExp(`Go ${escapeRegExp(expectedGoVersion)} \\(\\\`GOTOOLCHAIN=local\\\`\\)`),
    description: `README Go version must mention ${expectedGoVersion}`,
  },
  {
    file: "docs/testing/e2e-setup.md",
    pattern: new RegExp(escapeRegExp(expectedGoImage)),
    description: `E2E setup docs must mention ${expectedGoImage}`,
  },
];

const failures = [];

for (const check of checks) {
  const content = await readFile(check.file, "utf8");
  if (!check.pattern.test(content)) {
    failures.push(`- ${check.file}: ${check.description}`);
  }
}

for (const file of [".devcontainer/devcontainer.json", ".devcontainer/devcontainer-lock.json"]) {
  const content = await readFile(file, "utf8");
  const parsed = JSON.parse(content);
  const featureNames = Object.keys(parsed.features ?? {});
  const goFeatures = featureNames.filter((featureName) => /(^|\/)go:\d+$/.test(featureName));

  for (const featureName of goFeatures) {
    failures.push(`- ${file}: Go must be installed by .devcontainer/viewer-dev/Dockerfile, not ${featureName}`);
  }
}

for (const file of [".github/workflows/ci.yml", ".github/workflows/security-audit.yml"]) {
  const content = await readFile(file, "utf8");
  const goVersionFiles = [...content.matchAll(/go-version-file:\s*([^\s]+)/g)].map((match) => match[1]);
  const driftedVersionFiles = goVersionFiles.filter((value) => value !== ".go-version");

  if (goVersionFiles.length === 0) {
    failures.push(`- ${file}: setup-go must declare go-version-file: .go-version`);
  }

  for (const value of driftedVersionFiles) {
    failures.push(`- ${file}: setup-go must use .go-version, got ${value}`);
  }
}

const versionReferenceChecks = [
  {
    file: "README.md",
    pattern: /Go (\d+\.\d+\.\d+) \(/g,
    description: "README Go version references",
  },
  {
    file: "docs/testing/e2e-setup.md",
    pattern: /golang:(\d+\.\d+\.\d+)-bookworm/g,
    description: "E2E docs Go image references",
  },
];

for (const check of versionReferenceChecks) {
  const content = await readFile(check.file, "utf8");
  const versions = [...content.matchAll(check.pattern)].map((match) => match[1]);
  if (versions.length === 0) {
    failures.push(`- ${check.file}: ${check.description} must mention ${expectedGoVersion}`);
    continue;
  }

  for (const version of versions) {
    if (version !== expectedGoVersion) {
      failures.push(`- ${check.file}: ${check.description} must be ${expectedGoVersion}, got ${version}`);
    }
  }
}

if (failures.length > 0) {
  console.error(`Go toolchain version drift detected. .go-version is ${expectedGoVersion}.`);
  console.error(failures.join("\n"));
  process.exit(1);
}

console.log(`Go toolchain version is consistent at ${expectedGoVersion}.`);

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
