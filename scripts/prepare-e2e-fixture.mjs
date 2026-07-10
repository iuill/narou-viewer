import fs from "node:fs/promises";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { fileURLToPath } from "node:url";
import { stringify } from "yaml";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, "..");
const sourceNovelFetcherDataDir = path.join(repoRoot, "tests", "fixtures", "e2e", "novel-fetcher");
const sourceNovelFetcherDatabasePath = path.join(sourceNovelFetcherDataDir, "library.sqlite");
const targetDataDir = path.join(repoRoot, "data_e2e");
const targetNovelFetcherDataDir = path.join(targetDataDir, "novel-fetcher");
const novelFetcherDatabasePath = path.join(targetNovelFetcherDataDir, "library.sqlite");
const novelFetcherWorksPath = path.join(targetNovelFetcherDataDir, "works");
const novelFetcherFixtureHashPath = path.join(targetNovelFetcherDataDir, ".fixture-sha256");
const stateDir = path.join(targetDataDir, "state");
const tmpDir = path.join(targetDataDir, "tmp");
const novelFetcherFixtureRefreshFlagPath = path.join(tmpDir, "novel-fetcher-fixture-refreshed");
const readingStatePath = path.join(stateDir, "reading_state.yaml");
const bookmarksPath = path.join(stateDir, "bookmarks.yaml");
const readerPreferencesPath = path.join(stateDir, "reader_preferences.yaml");
const aiGenerationSettingsPath = path.join(stateDir, "ai_generation_settings.yaml");
const aiUsagePath = path.join(stateDir, "ai_usage.sqlite");
const extractionJobsDir = path.join(stateDir, "extraction_jobs");
const extractionJobsIndexDir = path.join(extractionJobsDir, "index");
const legacyCharacterJobsDir = path.join(stateDir, "character_jobs");
const characterProfilesDir = path.join(stateDir, "character_profiles");
const characterEventsDir = path.join(stateDir, "character_events");
const termProfilesDir = path.join(stateDir, "term_profiles");
const command = process.argv[2] ?? "rebuild";
const novelFetcherServiceDir = path.join(repoRoot, "services", "novel-fetcher");

function createNovelId(input) {
  return Buffer.from(input, "utf8").toString("base64url");
}

function createSiteNovelId(site, siteWorkId) {
  if (typeof site !== "string" || typeof siteWorkId !== "string") {
    return null;
  }
  const normalizedSite = site.trim().toLowerCase();
  const normalizedSiteWorkId = siteWorkId.trim();
  if (normalizedSite.length === 0 || normalizedSiteWorkId.length === 0) {
    return null;
  }
  return createNovelId(`site:${normalizedSite}:${normalizedSiteWorkId}`);
}

function createEmptyReadingStateDocument() {
  return {
    schema_version: 3,
    revision: 0,
    novels: {},
  };
}

function createEmptyBookmarksDocument() {
  return {
    schema_version: 3,
    revision: 0,
    bookmarks: [],
  };
}

function createEmptyReaderPreferencesDocument() {
  return {
    schema_version: 3,
    revision: 0,
    reader: {
      reading_mode: "vertical",
      font_family: "mincho",
      theme: "classic",
      updated_at: null,
    },
  };
}

async function pathExists(targetPath) {
  try {
    await fs.access(targetPath);
    return true;
  } catch {
    return false;
  }
}

async function hashDirectory(rootDir) {
  const hash = createHash("sha256");
  const relativeFilePaths = [];

  async function collect(currentDir) {
    const entries = await fs.readdir(currentDir, {
      withFileTypes: true,
    });

    for (const entry of entries) {
      const fullPath = path.join(currentDir, entry.name);
      const relativePath = path.relative(rootDir, fullPath).split(path.sep).join("/");
      if (entry.isDirectory()) {
        await collect(fullPath);
      } else if (entry.isFile()) {
        relativeFilePaths.push(relativePath);
      }
    }
  }

  await collect(rootDir);
  relativeFilePaths.sort();

  for (const relativePath of relativeFilePaths) {
    hash.update(relativePath);
    hash.update("\0");
    hash.update(await fs.readFile(path.join(rootDir, relativePath)));
    hash.update("\0");
  }

  return hash.digest("hex");
}

async function readTextIfExists(targetPath) {
  try {
    return await fs.readFile(targetPath, "utf8");
  } catch (error) {
    if (!isNodeError(error) || error.code !== "ENOENT") {
      throw error;
    }
    return null;
  }
}

async function hasCharacterProfilesFixture() {
  if (!(await pathExists(characterProfilesDir))) {
    return false;
  }

  const entries = await fs.readdir(characterProfilesDir, {
    withFileTypes: true,
  });
  return entries.some((entry) => entry.isFile() && entry.name.endsWith(".yaml"));
}

async function hasExtractionJobsFixture() {
  if (!(await pathExists(extractionJobsIndexDir))) {
    return false;
  }

  const entries = await fs.readdir(extractionJobsIndexDir, {
    withFileTypes: true,
  });
  return entries.some((entry) => entry.isFile() && entry.name.endsWith(".yaml"));
}

async function hasTermProfilesFixture() {
  if (!(await pathExists(termProfilesDir))) {
    return false;
  }
  const entries = await fs.readdir(termProfilesDir, { withFileTypes: true });
  return entries.some((entry) => entry.isFile() && entry.name.endsWith(".yaml"));
}

async function hasStateFixture() {
  const statuses = await Promise.all([
    pathExists(readingStatePath),
    pathExists(bookmarksPath),
    pathExists(readerPreferencesPath),
    pathExists(aiGenerationSettingsPath),
    pathExists(aiUsagePath),
    hasCharacterProfilesFixture(),
    hasTermProfilesFixture(),
    hasExtractionJobsFixture(),
  ]);
  return statuses.every(Boolean);
}

function buildNovelFetcherFixture() {
  const result = spawnSync(
    "go",
    ["run", "./cmd/e2e-fixture-builder", "--output", sourceNovelFetcherDataDir, "--work-set", "e2e"],
    {
      cwd: novelFetcherServiceDir,
      encoding: "utf8",
      stdio: "inherit",
    },
  );
  if (result.error) {
    throw result.error;
  }
  if (result.status !== 0) {
    throw new Error(`novel-fetcher fixture builder failed with status ${result.status}`);
  }
}

async function copyNovelFetcherFixtureToTarget() {
  if (!(await pathExists(sourceNovelFetcherDatabasePath))) {
    throw new Error(
      `E2E novel-fetcher source fixture is missing: ${path.relative(repoRoot, sourceNovelFetcherDatabasePath)}`,
    );
  }

  await fs.rm(targetNovelFetcherDataDir, { recursive: true, force: true });
  await fs.mkdir(path.dirname(targetNovelFetcherDataDir), { recursive: true });
  await fs.cp(sourceNovelFetcherDataDir, targetNovelFetcherDataDir, {
    recursive: true,
  });
  await Promise.all([
    fs.rm(`${novelFetcherDatabasePath}-shm`, { force: true }),
    fs.rm(`${novelFetcherDatabasePath}-wal`, { force: true }),
  ]);
}

async function refreshNovelFetcherFixtureIfNeeded() {
  const sourceHash = await hashDirectory(sourceNovelFetcherDataDir);
  const currentHash = (await readTextIfExists(novelFetcherFixtureHashPath))?.trim();

  if (
    currentHash === sourceHash &&
    (await pathExists(novelFetcherDatabasePath)) &&
    (await pathExists(novelFetcherWorksPath))
  ) {
    return;
  }

  await copyNovelFetcherFixtureToTarget();
  await fs.writeFile(novelFetcherFixtureHashPath, `${sourceHash}\n`, "utf8");
  await fs.mkdir(tmpDir, { recursive: true });
  await fs.writeFile(novelFetcherFixtureRefreshFlagPath, `${new Date().toISOString()}\n`, "utf8");
}

async function writeStateDocuments() {
  await fs.mkdir(stateDir, { recursive: true });
  await Promise.all([
    fs.rm(extractionJobsDir, { recursive: true, force: true }),
    fs.rm(legacyCharacterJobsDir, { recursive: true, force: true }),
    fs.rm(characterProfilesDir, { recursive: true, force: true }),
    fs.rm(characterEventsDir, { recursive: true, force: true }),
    fs.rm(termProfilesDir, { recursive: true, force: true }),
  ]);
  await fs.writeFile(readingStatePath, stringify(createEmptyReadingStateDocument()), "utf8");
  await fs.writeFile(bookmarksPath, stringify(createEmptyBookmarksDocument()), "utf8");
  await fs.writeFile(readerPreferencesPath, stringify(createEmptyReaderPreferencesDocument()), "utf8");
  await fs.writeFile(aiGenerationSettingsPath, stringify(createEmptyAiGenerationSettingsDocument()), "utf8");
  await fs.chmod(readingStatePath, 0o666);
  await fs.chmod(bookmarksPath, 0o666);
  await fs.chmod(readerPreferencesPath, 0o666);
  await fs.chmod(aiGenerationSettingsPath, 0o666);
  await writeAiUsageFixture();
  await writeCharacterProfilesFixture();
  await writeTermProfilesFixture();
  await writeExtractionJobsFixture();
  console.log(`Reset e2e state: ${path.relative(repoRoot, stateDir)}`);
}

function createEmptyAiGenerationSettingsDocument() {
  return {
    schema_version: 2,
    revision: 0,
    preferred_mode: "heuristic",
    selected_profile_id: "default",
    shared_providers: {
      openrouter: {
        api_key: null,
        updated_at: null,
      },
    },
    profiles: [
      {
        id: "default",
        label: "Default",
        provider: "openrouter",
        credentials: {
          source: "shared",
          api_key: null,
          updated_at: null,
        },
        model_id: null,
        provider_order: [],
        allow_fallbacks: false,
        require_parameters: true,
        updated_at: null,
      },
    ],
  };
}

async function writeCharacterProfilesFixture() {
  const fixtureRows = await readCharacterFixtureRows();
  if (fixtureRows.length === 0) {
    return;
  }

  await fs.mkdir(characterProfilesDir, { recursive: true });
  await fs.chmod(characterProfilesDir, 0o777);

  for (const row of fixtureRows) {
    const novelId = createSiteNovelId(row.site, row.site_work_id);
    if (!novelId || typeof row.episode_index !== "string" || row.episode_index.length === 0) {
      continue;
    }

    const episodeIndex = row.episode_index;
    const characterName = "契約テスト人物";
    const document = {
      schema_version: 2,
      revision: 1,
      novel_id: novelId,
      processed_up_to_episode_index: episodeIndex,
      updated_at: "2026-05-11T03:43:24.000Z",
      characters: [
        {
          character_id: "api_contract_character",
          canonical_name: {
            text: characterName,
            episode_index: episodeIndex,
          },
          full_name: null,
          gender: null,
          first_appearance_episode_index: episodeIndex,
          aliases: [
            {
              text: characterName,
              episode_index: episodeIndex,
            },
          ],
          importance_metrics: {
            episode_mentions: [
              {
                episode_index: episodeIndex,
                count: 1,
              },
            ],
          },
          appearance_history: [
            {
              episode_index: episodeIndex,
              text: "HTTP contract fixture で検証する人物。",
            },
          ],
          personality_history: [
            {
              episode_index: episodeIndex,
              text: "契約テスト用に安定した記述を持つ。",
            },
          ],
          summary_history: [
            {
              episode_index: episodeIndex,
              text: "契約テスト人物は API contract の character summary shape を固定するための fixture。",
            },
          ],
        },
      ],
    };
    const profilePath = path.join(characterProfilesDir, `${novelId}.yaml`);
    await fs.writeFile(profilePath, stringify(document), "utf8");
    await fs.chmod(profilePath, 0o666);
  }
}

async function writeTermProfilesFixture() {
  const fixtureRows = await readCharacterFixtureRows();
  await fs.mkdir(termProfilesDir, { recursive: true });
  await fs.chmod(termProfilesDir, 0o777);
  for (const row of fixtureRows) {
    const novelId = createSiteNovelId(row.site, row.site_work_id);
    if (!novelId || typeof row.episode_index !== "string" || row.episode_index.length === 0) {
      continue;
    }
    const isTermListFixture = row.site_work_id === "n3234ab";
    const isPartialWriteFixture = row.site_work_id === "n5234ab";
    const processedEpisodeIndex = isPartialWriteFixture ? "2" : row.episode_index;
    const generatedTerms = isTermListFixture
      ? [
          {
            term: "星見の塔",
            reading_history: [{ text: "ほしみのとう", episode_index: row.episode_index }],
            category_history: [{ category: "place", episode_index: row.episode_index }],
            description_history: [
              { text: "夜空を観測するための合成 fixture の塔。", episode_index: row.episode_index },
            ],
          },
        ]
      : isPartialWriteFixture
        ? [
            {
              term: "未確定の未来語",
              reading_history: [],
              category_history: [{ category: "other", episode_index: "2" }],
              description_history: [{ text: "character frontier より先行した履歴。", episode_index: "2" }],
            },
          ]
        : [];
    const profilePath = path.join(termProfilesDir, `${novelId}.yaml`);
    await fs.writeFile(
      profilePath,
      stringify({
        schema_version: 1,
        novel_id: novelId,
        processed_up_to_episode_index: processedEpisodeIndex,
        terms: generatedTerms,
      }),
      "utf8",
    );
    await fs.chmod(profilePath, 0o666);
  }
}

async function writeExtractionJobsFixture() {
  const fixtureRows = await readCharacterFixtureRows();
  const firstFixture = fixtureRows
    .map((row) => ({
      novelId: createSiteNovelId(row.site, row.site_work_id),
      episodeIndex: typeof row.episode_index === "string" ? row.episode_index : null,
    }))
    .find((row) => row.novelId && row.episodeIndex);
  if (!firstFixture) {
    return;
  }

  const novelId = firstFixture.novelId;
  const jobId = "extraction_api_contract_fixture";
  const timestamp = "2026-05-11T03:43:24.000Z";

  await fs.mkdir(extractionJobsIndexDir, { recursive: true });
  await fs.chmod(extractionJobsDir, 0o777);
  await fs.chmod(extractionJobsIndexDir, 0o777);

  await fs.writeFile(
    path.join(extractionJobsDir, `${jobId}.yaml`),
    stringify({
      schema_version: 2,
      revision: 1,
      job_id: jobId,
      novel_id: novelId,
      requested_up_to_episode_index: firstFixture.episodeIndex,
      profile_id: "default",
      profile_label: "Default",
      generation_mode: "heuristic",
      generation_strategy: "serial",
      model_id: null,
      status: "completed",
      progress: 100,
      progress_stage: "completed",
      generated_character_count: 1,
      generated_term_count: 0,
      created_at: timestamp,
      started_at: timestamp,
      finished_at: timestamp,
      error_message: null,
    }),
    "utf8",
  );
  await fs.writeFile(
    path.join(extractionJobsIndexDir, `${novelId}.yaml`),
    stringify({
      schema_version: 2,
      revision: 1,
      novel_id: novelId,
      active_job_id: null,
      job_ids: [jobId],
    }),
    "utf8",
  );
  await fs.chmod(path.join(extractionJobsDir, `${jobId}.yaml`), 0o666);
  await fs.chmod(path.join(extractionJobsIndexDir, `${novelId}.yaml`), 0o666);
}

async function readCharacterFixtureRows() {
  if (!(await pathExists(novelFetcherDatabasePath))) {
    return [];
  }

  const { Database } = await import("bun:sqlite");
  const db = new Database(novelFetcherDatabasePath, {
    readonly: true,
    strict: true,
  });

  let fixtureRows;
  try {
    fixtureRows = db
      .query(
        `
        SELECT
          w.site,
          w.site_work_id,
          COALESCE(NULLIF(e.display_index, ''), e.episode_id) AS episode_index
        FROM works w
        JOIN episodes e ON e.work_id = w.id
        WHERE w.site <> ''
          AND w.site_work_id <> ''
          AND e.sort_order = (
            SELECT MIN(first_episode.sort_order)
            FROM episodes first_episode
            WHERE first_episode.work_id = w.id
          )
        ORDER BY w.id ASC
      `,
      )
      .all();
  } finally {
    db.close();
  }

  return fixtureRows;
}

async function writeAiUsageFixture() {
  await Promise.all([
    fs.rm(aiUsagePath, { force: true }),
    fs.rm(`${aiUsagePath}-shm`, { force: true }),
    fs.rm(`${aiUsagePath}-wal`, { force: true }),
  ]);

  const { Database } = await import("bun:sqlite");
  const db = new Database(aiUsagePath, {
    create: true,
    readwrite: true,
    strict: true,
  });

  try {
    db.exec(`
      PRAGMA journal_mode = WAL;
      PRAGMA foreign_keys = ON;

      CREATE TABLE IF NOT EXISTS ai_usage_runs (
        run_id TEXT PRIMARY KEY,
        feature TEXT NOT NULL,
        workflow_name TEXT NOT NULL,
        status TEXT NOT NULL,
        started_at TEXT NOT NULL,
        finished_at TEXT NOT NULL,
        elapsed_ms INTEGER NOT NULL,
        novel_id TEXT,
        novel_title TEXT,
        current_episode_index TEXT,
        model_id TEXT,
        profile_id TEXT,
        profile_label TEXT,
        generation_mode TEXT NOT NULL,
        answer_chars INTEGER NOT NULL,
        request_count INTEGER NOT NULL,
        input_tokens INTEGER NOT NULL,
        output_tokens INTEGER NOT NULL,
        total_tokens INTEGER NOT NULL,
        cached_input_tokens INTEGER NOT NULL,
        reasoning_output_tokens INTEGER NOT NULL,
        total_cost REAL NOT NULL DEFAULT 0,
        tool_call_count INTEGER NOT NULL,
        tool_result_count INTEGER NOT NULL,
        error_message TEXT
      );

      CREATE TABLE IF NOT EXISTS ai_usage_requests (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        run_id TEXT NOT NULL REFERENCES ai_usage_runs(run_id) ON DELETE CASCADE,
        request_index INTEGER NOT NULL,
        kind TEXT NOT NULL DEFAULT 'other',
        parent_request_index INTEGER,
        tool_names TEXT NOT NULL DEFAULT '[]',
        tool_summaries TEXT NOT NULL DEFAULT '[]',
        input_tokens INTEGER NOT NULL,
        output_tokens INTEGER NOT NULL,
        total_tokens INTEGER NOT NULL,
        cached_input_tokens INTEGER NOT NULL,
        reasoning_output_tokens INTEGER NOT NULL,
        cost REAL NOT NULL DEFAULT 0
      );

      CREATE TABLE IF NOT EXISTS ai_usage_events (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        run_id TEXT NOT NULL REFERENCES ai_usage_runs(run_id) ON DELETE CASCADE,
        event_index INTEGER NOT NULL,
        type TEXT NOT NULL,
        tool_name TEXT
      );

      CREATE TABLE IF NOT EXISTS ai_usage_run_snapshots (
        run_id TEXT PRIMARY KEY REFERENCES ai_usage_runs(run_id) ON DELETE CASCADE,
        snapshot_json TEXT NOT NULL
      );

      CREATE INDEX IF NOT EXISTS ai_usage_runs_started_at_idx ON ai_usage_runs(started_at DESC);
      CREATE INDEX IF NOT EXISTS ai_usage_requests_run_id_idx ON ai_usage_requests(run_id);
      CREATE INDEX IF NOT EXISTS ai_usage_events_run_id_idx ON ai_usage_events(run_id);
    `);

    const runId = "e2e-ai-usage-contract-run";
    db.prepare(
      `
      INSERT INTO ai_usage_runs (
        run_id, feature, workflow_name, status, started_at, finished_at, elapsed_ms,
        novel_id, novel_title, current_episode_index, model_id, profile_id, profile_label,
        generation_mode, answer_chars, request_count, input_tokens, output_tokens, total_tokens,
        cached_input_tokens, reasoning_output_tokens, total_cost, tool_call_count, tool_result_count,
        error_message
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `,
    ).run(
      runId,
      "reader-assistant",
      "reader-ai-assistant",
      "completed",
      "2026-05-11T03:43:24.000Z",
      "2026-05-11T03:43:40.000Z",
      16000,
      "demo-novel",
      "Phase 1 contract fixture novel",
      "1",
      "openai/gpt-5-mini",
      "default",
      "Default",
      "remote",
      1200,
      1,
      1200,
      300,
      1500,
      100,
      0,
      0.0015,
      1,
      1,
      null,
    );
    db.prepare(
      `
      INSERT INTO ai_usage_requests (
        run_id, request_index, kind, parent_request_index, tool_names, tool_summaries,
        input_tokens, output_tokens, total_tokens, cached_input_tokens, reasoning_output_tokens, cost
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `,
    ).run(
      runId,
      1,
      "tool_call",
      null,
      JSON.stringify(["load_episode_range"]),
      JSON.stringify(["load_episode_range 1-1"]),
      1200,
      300,
      1500,
      100,
      0,
      0.0015,
    );
    db.prepare(
      `
      INSERT INTO ai_usage_run_snapshots (run_id, snapshot_json)
      VALUES (?, ?)
    `,
    ).run(runId, JSON.stringify({ runId, fixture: "phase1-api-contract" }));
  } finally {
    db.close();
  }

  await Promise.all(
    [aiUsagePath, `${aiUsagePath}-shm`, `${aiUsagePath}-wal`].map((targetPath) => chmodIfExists(targetPath, 0o666)),
  );
}

async function chmodIfExists(targetPath, mode) {
  try {
    await fs.chmod(targetPath, mode);
  } catch (error) {
    if (!isNodeError(error) || error.code !== "ENOENT") {
      throw error;
    }
  }
}

function isNodeError(error) {
  return typeof error === "object" && error !== null && "code" in error;
}

async function ensureNovelFetcherFixture() {
  await refreshNovelFetcherFixtureIfNeeded();

  await fs.chmod(targetNovelFetcherDataDir, 0o777);
  await fs.chmod(novelFetcherDatabasePath, 0o666);
  console.log(`E2E novel-fetcher fixture ready: ${path.relative(repoRoot, novelFetcherDatabasePath)}`);
}

async function initFixture() {
  await ensureNovelFetcherFixture();
  if (!(await hasStateFixture())) {
    console.log("E2E state fixture is incomplete. Recreating state.");
    await writeStateDocuments();
  }
}

async function rebuildFixture() {
  await fs.rm(sourceNovelFetcherDataDir, { recursive: true, force: true });
  await fs.mkdir(sourceNovelFetcherDataDir, { recursive: true });
  buildNovelFetcherFixture();
  await fs.rm(targetNovelFetcherDataDir, { recursive: true, force: true });
  await ensureNovelFetcherFixture();
  await writeStateDocuments();
}

async function resetState() {
  await writeStateDocuments();
}

if (command === "init") {
  await initFixture();
} else if (command === "rebuild") {
  await rebuildFixture();
} else if (command === "reset-state") {
  await resetState();
} else {
  throw new Error(`Unknown command: ${command}`);
}
