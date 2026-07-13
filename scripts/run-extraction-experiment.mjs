#!/usr/bin/env bun

import { readFile } from "node:fs/promises";
import path from "node:path";
import {
  buildDatasetSnapshot,
  createApiContractHeaders,
  createRunId,
  ensureDirectory,
  fetchJson,
  getBooleanOption,
  getRepeatableOption,
  getStringOption,
  parseArgs,
  readNdjsonStream,
  readNamedStringListFile,
  readResponseDetail,
  renderExtractionMarkdown,
  requireOption,
  resolveExperimentRequireParameters,
  resolveReportedReasoning,
  sanitizeFileName,
  sha256Hex,
  toAbsolutePath,
  writeJsonAtomic,
  writeTextAtomic,
  writeYamlAtomic,
} from "./extraction-experiment-lib.mjs";

function printHelp() {
  console.log(`Usage:
  bun run experiment:extraction:run -- \\
    --novel-id <novelId> \\
    --up-to-episode-index <episodeIndex> \\
    [--profile <profileIdOrLabel> ...] \\
    [--profiles-file <path>] \\
    [--model <modelId> ...] \\
    [--models-file <path>] \\
    [--base-profile <profileIdOrLabel>] \\
    [--provider <name> ...] \\
    [--allow-fallbacks true|false] \\
    [--require-parameters true|false] \\
    [--reasoning-effort none|minimal|low|medium|high|xhigh|max] \\
    [--system-prompt <text>] \\
    [--system-prompt-file <path>] \\
    [--all-profiles] \\
    [--api-base-url <url>] \\
    [--output-dir <path>] \\
    [--run-id <id>] \\
    [--concurrency <count>] \\
    [--fail-fast]

Examples:
  bun run experiment:extraction:run -- --novel-id n1234xx --up-to-episode-index 18 --profile default
  bun run experiment:extraction:run -- --novel-id n1234xx --up-to-episode-index 18 --profiles-file .tmp/profiles.yaml
  bun run experiment:extraction:run -- --novel-id n1234xx --up-to-episode-index 18 --model openai/gpt-4.1-mini --model anthropic/claude-3.7-sonnet
  bun run experiment:extraction:run -- --novel-id n1234xx --up-to-episode-index 18 --model openai/gpt-4.1-mini --model anthropic/claude-haiku-4.5 --concurrency 3
  bun run experiment:extraction:run -- --novel-id n1234xx --up-to-episode-index 18 --model openai/gpt-4.1-mini --system-prompt-file prompts/extraction-v2.txt`);
}

function normalizeApiBaseUrl(input) {
  return input.replace(/\/+$/, "");
}

function normalizeNovelLookupToken(value) {
  return String(value).trim().toLowerCase();
}

function extractNovelCodeFromTocUrl(tocUrl) {
  if (typeof tocUrl !== "string") {
    return null;
  }

  const match = tocUrl.toLowerCase().match(/\/([a-z]\d+[a-z]*)\/?$/);
  return match?.[1] ?? null;
}

async function resolveNovelId(apiBaseUrl, requestedNovelId) {
  const payload = await fetchJson(`${apiBaseUrl}/api/library/novels`);
  const novels = Array.isArray(payload?.novels) ? payload.novels : [];
  const token = normalizeNovelLookupToken(requestedNovelId);

  const match =
    novels.find((novel) => normalizeNovelLookupToken(novel.novelId) === token) ??
    novels.find((novel) => normalizeNovelLookupToken(novel.fetcherWorkId ?? "") === token) ??
    novels.find((novel) => normalizeNovelLookupToken(novel.tocUrl ?? "") === token) ??
    novels.find((novel) => extractNovelCodeFromTocUrl(novel.tocUrl) === token);

  if (!match) {
    throw new Error(
      `作品 ${requestedNovelId} が見つかりません。利用可能な novelId は /api/library/novels で確認してください。`,
    );
  }

  return {
    requestedNovelId,
    resolvedNovelId: match.novelId,
    title: match.title ?? null,
    tocUrl: match.tocUrl ?? null,
  };
}

function resolveProfiles({ availableProfiles, selectedProfileId, requestedProfiles, allProfiles }) {
  if (allProfiles) {
    return availableProfiles;
  }

  if (requestedProfiles.length === 0) {
    if (!selectedProfileId) {
      throw new Error("プロファイルが指定されておらず、AI 生成設定にも選択中プロファイルがありません。");
    }

    const selectedProfile = availableProfiles.find((profile) => profile.id === selectedProfileId);
    if (!selectedProfile) {
      throw new Error(`選択中プロファイル ${selectedProfileId} が AI 生成設定に見つかりません。`);
    }

    return [selectedProfile];
  }

  const resolved = [];
  for (const token of requestedProfiles) {
    const match =
      availableProfiles.find((profile) => profile.id === token) ??
      availableProfiles.find((profile) => profile.label === token);

    if (!match) {
      const availableLabels = availableProfiles.map((profile) => `${profile.id} (${profile.label})`).join(", ");
      throw new Error(`プロファイル ${token} が見つかりません。利用可能なプロファイル: ${availableLabels}`);
    }

    if (!resolved.some((profile) => profile.id === match.id)) {
      resolved.push(match);
    }
  }

  return resolved;
}

function resolveBaseProfile({ availableProfiles, selectedProfileId, baseProfileToken }) {
  if (baseProfileToken) {
    const explicitMatch =
      availableProfiles.find((profile) => profile.id === baseProfileToken) ??
      availableProfiles.find((profile) => profile.label === baseProfileToken);

    if (!explicitMatch) {
      const availableLabels = availableProfiles.map((profile) => `${profile.id} (${profile.label})`).join(", ");
      throw new Error(
        `ベースプロファイル ${baseProfileToken} が見つかりません。利用可能なプロファイル: ${availableLabels}`,
      );
    }

    return explicitMatch;
  }

  if (selectedProfileId) {
    const selected = availableProfiles.find((profile) => profile.id === selectedProfileId);
    if (selected) {
      return selected;
    }
  }

  return availableProfiles[0] ?? null;
}

function hasUsableApiKey(settings, baseProfile) {
  const sharedHasApiKey = settings?.settings?.sharedProviders?.openrouter?.hasApiKey === true;
  const baseProfileHasApiKey = baseProfile?.credentials?.hasApiKey === true;
  return sharedHasApiKey || baseProfileHasApiKey;
}

function buildExecutionBaseName(index, profile) {
  const ordinal = String(index + 1).padStart(2, "0");
  const label = sanitizeFileName(profile.modelId ?? profile.id, profile.id);
  return `${ordinal}_${label}`;
}

function buildModelExecutionBaseName(index, modelId) {
  const ordinal = String(index + 1).padStart(2, "0");
  return `${ordinal}_${sanitizeFileName(modelId, `model_${ordinal}`)}`;
}

function parsePositiveIntegerOption(values, key, fallback) {
  const rawValue = getStringOption(values, key, null);
  if (rawValue === null) {
    return fallback;
  }

  if (!/^\d+$/.test(rawValue)) {
    throw new Error(`--${key} には 1 以上の整数を指定してください。`);
  }

  const parsed = Number.parseInt(rawValue, 10);
  if (!Number.isSafeInteger(parsed) || parsed < 1) {
    throw new Error(`--${key} には 1 以上の整数を指定してください。`);
  }

  return parsed;
}

async function loadSystemPromptOverride(values) {
  const inlinePrompt = getStringOption(values, "system-prompt", null);
  const systemPromptFile = getStringOption(values, "system-prompt-file", null);

  if (inlinePrompt && systemPromptFile) {
    throw new Error("--system-prompt と --system-prompt-file は同時に指定できません。");
  }

  if (inlinePrompt) {
    return {
      text: inlinePrompt,
      source: "inline",
      filePath: null,
    };
  }

  if (!systemPromptFile) {
    return null;
  }

  const absolutePath = toAbsolutePath(systemPromptFile);
  const text = (await readFile(absolutePath, "utf8")).trim();
  if (text.length === 0) {
    throw new Error(`system prompt ファイルが空です: ${absolutePath}`);
  }

  return {
    text,
    source: "file",
    filePath: absolutePath,
  };
}

function createInitialManifest({
  runId,
  apiBaseUrl,
  requestedNovelId,
  novelId,
  novelTitle,
  tocUrl,
  upToEpisodeIndex,
  concurrency,
  systemPromptOverride,
  reasoningEffort,
  profiles,
  models,
  baseProfileId,
}) {
  return {
    runId,
    experimentType: "extraction",
    createdAt: new Date().toISOString(),
    updatedAt: null,
    finishedAt: null,
    status: "running",
    apiBaseUrl,
    requestedNovelId,
    novelId,
    novelTitle,
    tocUrl,
    upToEpisodeIndex,
    concurrency,
    requestedReasoningEffort: reasoningEffort,
    systemPromptOverrideHash: systemPromptOverride ? sha256Hex(systemPromptOverride.text) : null,
    systemPromptOverrideSource: systemPromptOverride?.source ?? null,
    systemPromptOverrideFile:
      systemPromptOverride?.source === "file" && systemPromptOverride.filePath
        ? path.relative(process.cwd(), systemPromptOverride.filePath)
        : null,
    systemPromptOverrideSavedFile: systemPromptOverride ? "system-prompt-override.txt" : null,
    datasetHash: null,
    promptHash: null,
    promptPreviewVariesByProfile: false,
    profiles: profiles.map((profile) => ({
      profileId: profile.id,
      profileLabel: profile.label,
      modelId: profile.modelId,
      providerOrder: Array.isArray(profile.providerOrder) ? profile.providerOrder : [],
      allowFallbacks: Boolean(profile.allowFallbacks),
      requireParameters: profile.requireParameters !== false,
      requestedReasoningEffort: reasoningEffort,
    })),
    modelOverrides: models.map((modelId) => ({
      modelId,
      baseProfileId,
      requestedReasoningEffort: reasoningEffort,
    })),
    executions: [],
  };
}

function updateManifestStatus(manifest) {
  const statuses = manifest.executions.map((execution) => execution.status);
  if (statuses.length === 0) {
    manifest.status = "failed";
    return;
  }

  if (statuses.some((status) => status === "pending" || status === "running")) {
    manifest.status = "running";
    return;
  }

  if (statuses.every((status) => status === "completed")) {
    manifest.status = "completed";
    return;
  }

  if (statuses.every((status) => status === "failed")) {
    manifest.status = "failed";
    return;
  }

  manifest.status = "partial_failure";
}

async function run() {
  const { values } = parseArgs(process.argv.slice(2), {
    repeatableKeys: new Set(["profile", "profile-id", "model", "provider"]),
  });

  if (getBooleanOption(values, "help", false)) {
    printHelp();
    return;
  }

  const requestedNovelId = requireOption(values, "novel-id");
  const upToEpisodeIndex = requireOption(values, "up-to-episode-index");
  const apiBaseUrl = normalizeApiBaseUrl(
    getStringOption(values, "api-base-url", process.env.VIEWER_API_BASE_URL ?? "http://127.0.0.1:8080"),
  );
  const failFast = getBooleanOption(values, "fail-fast", false);
  const concurrency = parsePositiveIntegerOption(values, "concurrency", 3);
  const allProfiles = getBooleanOption(values, "all-profiles", false);
  const allowFallbacksOverride =
    values["allow-fallbacks"] === undefined ? undefined : getBooleanOption(values, "allow-fallbacks", false);
  const outputDir = toAbsolutePath(getStringOption(values, "output-dir", "data/ai-experiments/runs"));
  const runId = getStringOption(values, "run-id", createRunId("extraction", requestedNovelId, upToEpisodeIndex));

  const requestedProfiles = getRepeatableOption(values, "profile", "profile-id");
  const profilesFile = getStringOption(values, "profiles-file", null);
  if (profilesFile) {
    requestedProfiles.push(...(await readNamedStringListFile(profilesFile, ["profiles"])));
  }

  const requestedModels = [...new Set(getRepeatableOption(values, "model"))];
  const modelsFile = getStringOption(values, "models-file", null);
  if (modelsFile) {
    requestedModels.push(...(await readNamedStringListFile(modelsFile, ["models", "profiles"])));
  }
  const normalizedRequestedModels = [
    ...new Set(requestedModels.map((value) => value.trim()).filter((value) => value.length > 0)),
  ];
  const rawReasoningEffort = getStringOption(values, "reasoning-effort", null);
  const reasoningEffort = rawReasoningEffort === null ? null : rawReasoningEffort.trim().toLowerCase();
  if (
    reasoningEffort !== null &&
    !new Set(["none", "minimal", "low", "medium", "high", "xhigh", "max"]).has(reasoningEffort)
  ) {
    throw new Error("--reasoning-effort には none / minimal / low / medium / high / xhigh / max を指定してください。");
  }
  const explicitRequireParameters =
    values["require-parameters"] === undefined ? undefined : getBooleanOption(values, "require-parameters", true);
  const requireParametersOverride = resolveExperimentRequireParameters({
    explicitValue: explicitRequireParameters,
    reasoningEffort,
    hasModelOverrides: normalizedRequestedModels.length > 0,
  });
  const providerOrderOverride = [
    ...new Set(
      getRepeatableOption(values, "provider")
        .map((value) => value.trim())
        .filter((value) => value.length > 0),
    ),
  ];
  const baseProfileToken = getStringOption(values, "base-profile", null);
  const systemPromptOverride = await loadSystemPromptOverride(values);

  const settings = await fetchJson(`${apiBaseUrl}/api/ai-generation/settings`);
  const systemStatus = normalizedRequestedModels.length > 0 ? await fetchJson(`${apiBaseUrl}/api/system/status`) : null;
  const resolvedNovel = await resolveNovelId(apiBaseUrl, requestedNovelId);
  const novelId = resolvedNovel.resolvedNovelId;
  if (requestedNovelId !== novelId) {
    console.log(`[resolve] ${requestedNovelId} -> ${novelId}${resolvedNovel.title ? ` (${resolvedNovel.title})` : ""}`);
  }
  const availableProfiles = Array.isArray(settings?.settings?.profiles) ? settings.settings.profiles : [];
  const selectedProfileId =
    typeof settings?.settings?.selectedProfileId === "string" ? settings.settings.selectedProfileId : null;
  const shouldRunSavedProfiles = allProfiles || requestedProfiles.length > 0 || normalizedRequestedModels.length === 0;
  const profiles = shouldRunSavedProfiles
    ? resolveProfiles({
        availableProfiles,
        selectedProfileId,
        requestedProfiles,
        allProfiles,
      })
    : [];
  const baseProfile =
    normalizedRequestedModels.length > 0
      ? resolveBaseProfile({
          availableProfiles,
          selectedProfileId,
          baseProfileToken,
        })
      : null;

  if (normalizedRequestedModels.length > 0 && !hasUsableApiKey(settings, baseProfile)) {
    const aiService = Array.isArray(systemStatus?.services)
      ? systemStatus.services.find((service) => service?.id === "go-internal-ai")
      : null;
    const mockHint =
      typeof aiService?.detail === "string" && aiService.detail.includes("mock")
        ? " Go internal AI は現在 mock モードで動作しています。"
        : "";
    throw new Error(
      `モデル直接指定には利用可能な OpenRouter API キーが必要ですが、共有設定にもベースプロファイルにも設定されていません。${mockHint}AI 生成設定で共有 OpenRouter API キーを保存するか、--base-profile <custom-key を持つプロファイル> を指定してください。`,
    );
  }

  const runDir = path.join(outputDir, runId);
  const rawDir = path.join(runDir, "raw");
  const outputsDir = path.join(runDir, "outputs");
  const evaluationsDir = path.join(runDir, "evaluations");
  await ensureDirectory(rawDir);
  await ensureDirectory(outputsDir);
  await ensureDirectory(evaluationsDir);
  if (systemPromptOverride) {
    await writeTextAtomic(path.join(runDir, "system-prompt-override.txt"), `${systemPromptOverride.text}\n`);
  }

  const manifestPath = path.join(runDir, "manifest.yaml");
  const manifest = createInitialManifest({
    runId,
    apiBaseUrl,
    requestedNovelId,
    novelId,
    novelTitle: resolvedNovel.title,
    tocUrl: resolvedNovel.tocUrl,
    upToEpisodeIndex,
    concurrency,
    systemPromptOverride,
    reasoningEffort,
    profiles,
    models: normalizedRequestedModels,
    baseProfileId: baseProfile?.id ?? null,
  });

  const executionTargets = [
    ...profiles.map((profile) => ({
      type: "profile",
      profile,
      baseName: buildExecutionBaseName(0, profile),
      reasoningEffort,
      displayLabel: `${profile.label} (${profile.modelId ?? profile.id})`,
    })),
    ...normalizedRequestedModels.map((modelId) => ({
      type: "model",
      modelId,
      baseProfile,
      providerOrderOverride,
      allowFallbacksOverride,
      requireParametersOverride,
      reasoningEffort,
      baseName: "",
      displayLabel: modelId,
    })),
  ].map((target, index) =>
    target.type === "profile"
      ? {
          ...target,
          baseName: buildExecutionBaseName(index, target.profile),
        }
      : {
          ...target,
          baseName: buildModelExecutionBaseName(index, target.modelId),
        },
  );

  const promptPreviewFile = path.join(runDir, "prompt-preview.json");
  const datasetFile = path.join(runDir, "dataset.json");
  const executionRecords = executionTargets.map((target) => {
    const baseName = target.baseName;
    const rawFile = path.join(rawDir, `${baseName}.json`);
    const markdownFile = path.join(outputsDir, `${baseName}.md`);
    return {
      targetType: target.type,
      profileId: target.type === "profile" ? target.profile.id : (target.baseProfile?.id ?? null),
      profileLabel: target.type === "profile" ? target.profile.label : (target.baseProfile?.label ?? null),
      requestedModelId: target.type === "model" ? target.modelId : null,
      modelId: target.type === "profile" ? target.profile.modelId : target.modelId,
      reasoning: null,
      status: "pending",
      startedAt: null,
      finishedAt: null,
      rawFile: path.relative(runDir, rawFile),
      markdownFile: path.relative(runDir, markdownFile),
      promptPreviewFile: "prompt-preview.json",
      batchTimingCount: 0,
      errorMessage: null,
    };
  });
  manifest.executions.push(...executionRecords);
  await writeYamlAtomic(manifestPath, manifest);

  let sharedPromptPreviewHash = null;
  let manifestWriteQueue = Promise.resolve();
  let sharedStateQueue = Promise.resolve();
  let nextExecutionIndex = 0;
  let stopScheduling = false;

  function queueManifestWrite() {
    const task = manifestWriteQueue.then(() => writeYamlAtomic(manifestPath, manifest));
    manifestWriteQueue = task.catch(() => {});
    return task;
  }

  function queueSharedStateUpdate(callback) {
    const task = sharedStateQueue.then(callback);
    sharedStateQueue = task.catch(() => {});
    return task;
  }

  async function executeTarget(target, executionRecord) {
    const rawFile = path.join(runDir, executionRecord.rawFile);
    const markdownFile = path.join(runDir, executionRecord.markdownFile);
    executionRecord.status = "running";
    executionRecord.startedAt = new Date().toISOString();
    await queueManifestWrite();

    console.log(`[run] ${target.displayLabel}`);
    try {
      const response = await fetch(`${apiBaseUrl}/api/ai-generation/playground/extraction/stream`, {
        method: "POST",
        headers: {
          ...createApiContractHeaders(),
          "content-type": "application/json",
        },
        body: JSON.stringify({
          novelId,
          upToEpisodeIndex,
          ...(target.type === "profile"
            ? {
                profileId: target.profile.id,
                ...(target.reasoningEffort ? { reasoningEffort: target.reasoningEffort } : {}),
                ...(systemPromptOverride ? { systemPromptOverride: systemPromptOverride.text } : {}),
              }
            : {
                profileId: target.baseProfile?.id ?? null,
                modelId: target.modelId,
                ...(target.reasoningEffort ? { reasoningEffort: target.reasoningEffort } : {}),
                ...(target.providerOrderOverride.length > 0 ? { providerOrder: target.providerOrderOverride } : {}),
                ...(target.allowFallbacksOverride !== undefined
                  ? { allowFallbacks: target.allowFallbacksOverride }
                  : {}),
                ...(target.requireParametersOverride !== undefined
                  ? { requireParameters: target.requireParametersOverride }
                  : {}),
                ...(systemPromptOverride ? { systemPromptOverride: systemPromptOverride.text } : {}),
              }),
        }),
      });

      if (!response.ok) {
        const detail = await readResponseDetail(response);
        throw new Error(`viewer-api responded with ${response.status}${detail ? `: ${detail}` : "."}`);
      }

      const streamState = {
        promptPreview: null,
        batchTimings: [],
        result: null,
      };

      await readNdjsonStream(response, async (event) => {
        if (event.type === "status") {
          console.log(`  [status][${target.displayLabel}] ${event.message}`);
          return;
        }

        if (event.type === "promptPreview") {
          streamState.promptPreview = event.preview;
          return;
        }

        if (event.type === "batchTiming") {
          streamState.batchTimings.push(event);
          console.log(`  [batch][${target.displayLabel}] ${event.message} (${event.elapsedMs}ms)`);
          return;
        }

        if (event.type === "result") {
          streamState.result = event.result;
          return;
        }

        if (event.type === "error") {
          throw new Error(event.error);
        }
      });

      if (!streamState.result) {
        throw new Error("Playground stream did not return a result.");
      }

      const reasoning = resolveReportedReasoning(streamState.result, target.reasoningEffort);

      const promptPreview = streamState.promptPreview;
      if (!promptPreview) {
        throw new Error("Playground stream did not return a prompt preview.");
      }

      const promptPreviewHash = sha256Hex(JSON.stringify(promptPreview));
      await queueSharedStateUpdate(async () => {
        if (!sharedPromptPreviewHash) {
          sharedPromptPreviewHash = promptPreviewHash;
          const datasetSnapshot = buildDatasetSnapshot({
            novelId: streamState.result.novelId,
            novelTitle: streamState.result.novelTitle,
            upToEpisodeIndex: streamState.result.upToEpisodeIndex,
            preview: promptPreview,
          });
          manifest.datasetHash = sha256Hex(JSON.stringify(datasetSnapshot));
          manifest.promptHash = sha256Hex(promptPreview.systemPrompt);
          manifest.novelTitle = streamState.result.novelTitle;
          await writeJsonAtomic(promptPreviewFile, promptPreview);
          await writeJsonAtomic(datasetFile, datasetSnapshot);
          return;
        }

        if (sharedPromptPreviewHash !== promptPreviewHash) {
          manifest.promptPreviewVariesByProfile = true;
        }
      });

      const rawPayload = {
        runId,
        executedAt: new Date().toISOString(),
        targetType: target.type,
        profileId: target.type === "profile" ? target.profile.id : (target.baseProfile?.id ?? null),
        profileLabel: target.type === "profile" ? target.profile.label : (target.baseProfile?.label ?? null),
        requestedModelId: target.type === "model" ? target.modelId : null,
        modelId: target.type === "profile" ? target.profile.modelId : target.modelId,
        reasoning,
        providerOrder: target.type === "profile" ? target.profile.providerOrder : target.providerOrderOverride,
        allowFallbacks:
          target.type === "profile" ? target.profile.allowFallbacks : (target.allowFallbacksOverride ?? null),
        requireParameters: reasoning.requireParameters,
        systemPromptOverrideHash: systemPromptOverride ? sha256Hex(systemPromptOverride.text) : null,
        promptPreview,
        batchTimings: streamState.batchTimings,
        result: streamState.result,
      };

      await writeJsonAtomic(rawFile, rawPayload);
      await writeTextAtomic(markdownFile, renderExtractionMarkdown(rawPayload));

      executionRecord.status = "completed";
      executionRecord.finishedAt = new Date().toISOString();
      executionRecord.batchTimingCount = streamState.batchTimings.length;
      executionRecord.promptPreviewHash = promptPreviewHash;
      executionRecord.characterCount = Array.isArray(streamState.result.characters)
        ? streamState.result.characters.length
        : 0;
      executionRecord.termCount = Array.isArray(streamState.result.terms) ? streamState.result.terms.length : 0;
      executionRecord.processedUpToEpisodeIndex = streamState.result.processedUpToEpisodeIndex;
      executionRecord.reasoning = reasoning;
    } catch (error) {
      executionRecord.status = "failed";
      executionRecord.finishedAt = new Date().toISOString();
      executionRecord.errorMessage = error instanceof Error ? error.message : "Experiment execution failed.";
      console.error(`  [error][${target.displayLabel}] ${executionRecord.errorMessage}`);

      if (failFast) {
        stopScheduling = true;
      }
    } finally {
      updateManifestStatus(manifest);
      const now = new Date().toISOString();
      manifest.updatedAt = now;
      if (manifest.status === "completed" || manifest.status === "failed" || manifest.status === "partial_failure") {
        manifest.finishedAt = now;
      } else {
        manifest.finishedAt = null;
      }
      await queueManifestWrite();
    }
  }

  async function worker() {
    while (true) {
      if (stopScheduling) {
        return;
      }

      const currentIndex = nextExecutionIndex;
      if (currentIndex >= executionTargets.length) {
        return;
      }

      nextExecutionIndex += 1;
      await executeTarget(executionTargets[currentIndex], executionRecords[currentIndex]);
    }
  }

  const workerCount = Math.max(1, Math.min(concurrency, executionTargets.length || 1));
  console.log(`[plan] ${executionTargets.length} 件を並列 ${workerCount} で実行します。`);
  await Promise.all(Array.from({ length: workerCount }, () => worker()));
  await sharedStateQueue;
  await manifestWriteQueue;

  console.log(`[done] ${runDir}`);

  if (manifest.status !== "completed") {
    process.exitCode = 1;
  }
}

run().catch((error) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
});
