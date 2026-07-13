#!/usr/bin/env bun

import { createHash } from "node:crypto";
import { mkdir, readFile, rename, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { parse as parseYaml, stringify as stringifyYaml } from "yaml";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
export const workspaceRoot = path.resolve(scriptDir, "..");
export const apiContractVersion = "1";
export const apiContractVersionHeader = "x-narou-viewer-api-contract-version";
export const apiClientBuildHeader = "x-narou-viewer-client-build";

export function createApiContractHeaders(clientBuild = "extraction-experiment") {
  return {
    [apiContractVersionHeader]: apiContractVersion,
    [apiClientBuildHeader]: clientBuild,
  };
}

export function mergeHeaders(baseHeaders = {}, additionalHeaders = {}) {
  const headers = new Headers(baseHeaders);
  new Headers(additionalHeaders).forEach((value, key) => {
    headers.set(key, value);
  });
  return Object.fromEntries(headers.entries());
}

export function parseArgs(argv, options = {}) {
  const repeatableKeys = options.repeatableKeys ?? new Set();
  const values = {};
  const positionals = [];

  for (let index = 0; index < argv.length; index += 1) {
    const token = argv[index];
    if (token === "--") {
      positionals.push(...argv.slice(index + 1));
      break;
    }

    if (!token.startsWith("--")) {
      positionals.push(token);
      continue;
    }

    const normalized = token.slice(2);
    const separatorIndex = normalized.indexOf("=");
    const key = separatorIndex >= 0 ? normalized.slice(0, separatorIndex) : normalized;
    let value = separatorIndex >= 0 ? normalized.slice(separatorIndex + 1) : true;

    if (separatorIndex < 0) {
      const next = argv[index + 1];
      if (next !== undefined && !next.startsWith("--")) {
        value = next;
        index += 1;
      }
    }

    if (repeatableKeys.has(key)) {
      const current = Array.isArray(values[key]) ? values[key] : [];
      current.push(String(value));
      values[key] = current;
      continue;
    }

    values[key] = value;
  }

  return {
    values,
    positionals,
  };
}

export function getStringOption(values, key, fallback = null) {
  const value = values[key];
  if (value === undefined || value === true || value === false) {
    return fallback;
  }

  const normalized = String(value).trim();
  return normalized.length > 0 ? normalized : fallback;
}

export function getBooleanOption(values, key, fallback = false) {
  const value = values[key];
  if (value === undefined) {
    return fallback;
  }

  if (typeof value === "boolean") {
    return value;
  }

  const normalized = String(value).trim().toLowerCase();
  if (["1", "true", "yes", "on"].includes(normalized)) {
    return true;
  }

  if (["0", "false", "no", "off"].includes(normalized)) {
    return false;
  }

  return fallback;
}

export function getRepeatableOption(values, ...keys) {
  const items = [];
  for (const key of keys) {
    const value = values[key];
    if (Array.isArray(value)) {
      items.push(...value.map((entry) => String(entry)));
    } else if (value !== undefined && value !== true && value !== false) {
      items.push(String(value));
    }
  }
  return items;
}

export function requireOption(values, key, label = key) {
  const value = getStringOption(values, key, null);
  if (!value) {
    throw new Error(`Missing required option --${label}.`);
  }
  return value;
}

export async function ensureDirectory(directoryPath) {
  await mkdir(directoryPath, { recursive: true });
}

export async function writeTextAtomic(filePath, text) {
  await ensureDirectory(path.dirname(filePath));
  const tempPath = `${filePath}.tmp-${process.pid}-${Date.now()}`;
  await writeFile(tempPath, text, "utf8");
  await rename(tempPath, filePath);
}

export async function writeJsonAtomic(filePath, value) {
  await writeTextAtomic(filePath, `${JSON.stringify(value, null, 2)}\n`);
}

export async function writeYamlAtomic(filePath, value) {
  await writeTextAtomic(filePath, stringifyYaml(value));
}

export function sha256Hex(value) {
  return createHash("sha256").update(value).digest("hex");
}

export function createRunId(prefix, novelId, upToEpisodeIndex) {
  const timestamp = new Date()
    .toISOString()
    .replace(/\.\d{3}Z$/, "Z")
    .replaceAll(":", "-");
  return `${timestamp}_${prefix}_${sanitizeFileName(novelId)}_${sanitizeFileName(String(upToEpisodeIndex))}`;
}

export function sanitizeFileName(value, fallback = "item") {
  const normalized = String(value)
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9._-]+/g, "_")
    .replace(/_+/g, "_")
    .replace(/^[_-]+|[_-]+$/g, "");

  return normalized.length > 0 ? normalized : fallback;
}

export function toAbsolutePath(targetPath) {
  return path.isAbsolute(targetPath) ? targetPath : path.join(workspaceRoot, targetPath);
}

export async function readNamedStringListFile(filePath, keys = ["profiles"]) {
  const absolutePath = toAbsolutePath(filePath);
  const text = await readFile(absolutePath, "utf8");

  if (absolutePath.endsWith(".json")) {
    const parsed = JSON.parse(text);
    if (Array.isArray(parsed)) {
      return parsed.map((value) => String(value));
    }

    if (parsed && typeof parsed === "object") {
      for (const key of keys) {
        if (Array.isArray(parsed[key])) {
          return parsed[key].map((value) => String(value));
        }
      }
    }
  }

  const parsed = parseYaml(text);
  if (Array.isArray(parsed)) {
    return parsed.map((value) => String(value));
  }

  if (parsed && typeof parsed === "object") {
    for (const key of keys) {
      if (Array.isArray(parsed[key])) {
        return parsed[key].map((value) => String(value));
      }
    }
  }

  throw new Error(`Unsupported list file format: ${absolutePath}`);
}

export async function fetchJson(url, init = {}) {
  const response = await fetch(url, {
    ...init,
    headers: mergeHeaders(createApiContractHeaders(), init.headers),
  });
  if (!response.ok) {
    const detail = await readResponseDetail(response);
    throw new Error(`${url} responded with ${response.status}${detail ? `: ${detail}` : "."}`);
  }

  return response.json();
}

export async function readResponseDetail(response) {
  try {
    const contentType = response.headers.get("content-type") ?? "";
    if (contentType.includes("application/json")) {
      const payload = await response.json();
      if (payload && typeof payload === "object" && typeof payload.error === "string") {
        return payload.error;
      }
      return JSON.stringify(payload);
    }

    return (await response.text()).trim();
  } catch {
    return "";
  }
}

export async function readNdjsonStream(response, onEvent) {
  if (!response.body) {
    throw new Error("Response body was empty.");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  const emitLine = async (line) => {
    const trimmed = line.trim();
    if (!trimmed) {
      return;
    }

    await onEvent(JSON.parse(trimmed));
  };

  while (true) {
    const { value, done } = await reader.read();
    buffer += decoder.decode(value ?? new Uint8Array(), { stream: !done });

    const lines = buffer.split("\n");
    buffer = lines.pop() ?? "";
    for (const line of lines) {
      await emitLine(line);
    }

    if (done) {
      break;
    }
  }

  if (buffer.trim().length > 0) {
    await emitLine(buffer);
  }
}

export function buildDatasetSnapshot({ novelId, novelTitle, upToEpisodeIndex, preview }) {
  return {
    novelId,
    novelTitle,
    upToEpisodeIndex,
    systemPrompt: preview.systemPrompt,
    batchCount: Array.isArray(preview.batches) ? preview.batches.length : 0,
    chunkCount: Array.isArray(preview.batches)
      ? preview.batches.reduce((total, batch) => total + (Array.isArray(batch.chunks) ? batch.chunks.length : 0), 0)
      : 0,
    batches: Array.isArray(preview.batches)
      ? preview.batches.map((batch) => ({
          batchIndex: batch.batchIndex,
          batchCount: batch.batchCount,
          episodeIndexes: batch.episodeIndexes,
          chunkCount: batch.chunkCount,
          chunks: Array.isArray(batch.chunks)
            ? batch.chunks.map((chunk) => ({
                episodeIndex: chunk.episodeIndex,
                title: chunk.title,
                chapter: chunk.chapter,
                subchapter: chunk.subchapter,
                chunkIndex: chunk.chunkIndex,
                chunkCount: chunk.chunkCount,
                text: chunk.text,
              }))
            : [],
        }))
      : [],
  };
}

function joinNonEmpty(values, separator = ", ") {
  return values
    .map((value) => (value === null || value === undefined ? "" : String(value).trim()))
    .filter((value) => value.length > 0)
    .join(separator);
}

export function renderExtractionMarkdown(execution) {
  const result = execution.result;
  const lines = [
    `# ${execution.profileLabel ?? execution.profileId}`,
    "",
    `- modelId: ${execution.modelId ?? "unknown"}`,
    `- profileId: ${execution.profileId}`,
    `- reasoningEffort: ${execution.reasoningEffort ?? "default"}`,
    `- processedUpToEpisodeIndex: ${result.processedUpToEpisodeIndex}`,
    `- characterCount: ${Array.isArray(result.characters) ? result.characters.length : 0}`,
    `- termCount: ${Array.isArray(result.terms) ? result.terms.length : 0}`,
  ];

  if (execution.batchTimings.length > 0) {
    const totalElapsedMs = execution.batchTimings.reduce((total, timing) => total + (timing.elapsedMs ?? 0), 0);
    lines.push(`- totalElapsedMs: ${totalElapsedMs}`);
  }

  lines.push("");
  lines.push("## Characters");
  lines.push("");

  for (const character of result.characters) {
    lines.push(`### ${character.canonicalName}`);
    lines.push("");
    lines.push(`- firstAppearanceEpisodeIndex: ${character.firstAppearanceEpisodeIndex}`);
    if (character.fullName) {
      lines.push(`- fullName: ${character.fullName}`);
    }
    if (character.gender) {
      lines.push(`- gender: ${character.gender}`);
    }

    const aliases = Array.isArray(character.aliases) ? character.aliases : [];
    if (aliases.length > 0) {
      lines.push(`- aliases: ${joinNonEmpty(aliases)}`);
    }
    if (character.appearance) {
      lines.push(`- appearance: ${character.appearance}`);
    }
    if (character.personality) {
      lines.push(`- personality: ${character.personality}`);
    }
    if (character.summary) {
      lines.push(`- summary: ${character.summary}`);
    }
    lines.push("");
  }

  lines.push("## Terms", "");
  for (const term of Array.isArray(result.terms) ? result.terms : []) {
    lines.push(`### ${term.term}`, "");
    if (term.reading) lines.push(`- reading: ${term.reading}`);
    lines.push(`- category: ${term.category}`, `- description: ${term.description}`, "");
  }

  return `${lines.join("\n").trimEnd()}\n`;
}
