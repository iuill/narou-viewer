import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const marker = "<!-- narou-viewer-repository-size-report -->";
const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, "..");
const measuredSha = process.env.MEASURED_SHA || "";
const baseSha = process.env.BASE_SHA || "";

export const areaRules = [
  "apps/viewer-web/",
  "apps/viewer-api-go/",
  "services/novel-fetcher/",
  "tests/",
  "e2e/",
  "docs/",
  "scripts/",
  ".github/",
  ".devcontainer/",
  ".agents/",
  ".claude/",
  "deploy/",
  "ops/",
  "data_e2e/",
  "data/",
];

const sourcePrefixes = ["apps/", "services/"];

function readJson(reportDir, fileName) {
  return JSON.parse(fs.readFileSync(path.join(reportDir, fileName), "utf8"));
}

function readText(reportDir, fileName) {
  return fs.readFileSync(path.join(reportDir, fileName), "utf8").trim();
}

function emptyMetrics() {
  return { files: 0, lines: 0, code: 0, comments: 0, blank: 0, complexity: 0 };
}

function addMetrics(left, right) {
  left.files += right.files || 0;
  left.lines += right.lines || 0;
  left.code += right.code || 0;
  left.comments += right.comments || 0;
  left.blank += right.blank || 0;
  left.complexity += right.complexity || 0;
  return left;
}

function subtractMetrics(left, right) {
  return {
    files: (left.files || 0) - (right.files || 0),
    lines: (left.lines || 0) - (right.lines || 0),
    code: (left.code || 0) - (right.code || 0),
    comments: (left.comments || 0) - (right.comments || 0),
    blank: (left.blank || 0) - (right.blank || 0),
    complexity: (left.complexity || 0) - (right.complexity || 0),
  };
}

function fileMetrics(file) {
  return {
    files: 1,
    lines: file.Lines || 0,
    code: file.Code || 0,
    comments: file.Comment || 0,
    blank: file.Blank || 0,
    complexity: file.Complexity || 0,
  };
}

export function normalizePath(filePath) {
  const normalized = String(filePath || "").replaceAll("\\", "/");
  const repoRelative = path.relative(repoRoot, normalized).replaceAll("\\", "/");
  if (repoRelative && !repoRelative.startsWith("..") && repoRelative !== normalized) {
    return repoRelative;
  }

  const withoutDot = normalized.replace(/^\.\//, "");
  for (const prefix of [...areaRules, ...sourcePrefixes]) {
    const index = withoutDot.indexOf(prefix);
    if (index >= 0) {
      return withoutDot.slice(index);
    }
  }

  return withoutDot;
}

function filePathFor(file) {
  return normalizePath(file.Location || file.Filename || "");
}

export function areaFor(filePath) {
  return areaRules.find((prefix) => filePath.startsWith(prefix)) || "root/";
}

function isTestFile(filePath) {
  const normalized = normalizePath(filePath);
  return (
    normalized.startsWith("e2e/") ||
    normalized.includes("/tests/") ||
    normalized.includes("/test/") ||
    normalized.endsWith(".test.ts") ||
    normalized.endsWith(".test.tsx") ||
    normalized.endsWith(".spec.ts") ||
    normalized.endsWith(".spec.tsx") ||
    normalized.endsWith("_test.go")
  );
}

function isApplicationSource(filePath) {
  const normalized = normalizePath(filePath);
  return sourcePrefixes.some((prefix) => normalized.startsWith(prefix)) && !isTestFile(normalized);
}

function fileRows(byFileRows) {
  return byFileRows.flatMap((language) =>
    (language.Files || []).map((file) => ({
      path: filePathFor(file),
      language: language.Name || file.Language || "",
      ...fileMetrics(file),
    })),
  );
}

function totalsByArea(files) {
  const totals = new Map();
  for (const file of files) {
    const area = areaFor(file.path);
    const current = totals.get(area) || emptyMetrics();
    addMetrics(current, file);
    totals.set(area, current);
  }
  return totals;
}

function totalsByLanguage(files) {
  const totals = new Map();
  for (const file of files) {
    const language = file.language || "Unknown";
    const current = totals.get(language) || emptyMetrics();
    addMetrics(current, file);
    totals.set(language, current);
  }
  return totals;
}

function totalFiles(files) {
  return files.reduce((sum, file) => addMetrics(sum, file), emptyMetrics());
}

function balanceTotals(files) {
  const app = emptyMetrics();
  const tests = emptyMetrics();
  for (const file of files) {
    if (isTestFile(file.path)) {
      addMetrics(tests, file);
    } else if (isApplicationSource(file.path)) {
      addMetrics(app, file);
    }
  }
  return { app, tests };
}

function diffMap(head, base) {
  const result = [];
  const keys = new Set([...head.keys(), ...base.keys()]);
  for (const key of keys) {
    result.push([key, subtractMetrics(head.get(key) || emptyMetrics(), base.get(key) || emptyMetrics())]);
  }
  return result;
}

function format(value) {
  return Number(value || 0).toLocaleString("en-US");
}

function signed(value) {
  const number = Number(value || 0);
  if (number > 0) {
    return `+${format(number)}`;
  }
  return format(number);
}

function percent(value, total) {
  if (!total) {
    return "0.0%";
  }
  return `${((Number(value || 0) / total) * 100).toFixed(1)}%`;
}

function signedPercent(value, total) {
  if (!total) {
    return "0.0%";
  }
  const result = (Number(value || 0) / total) * 100;
  return `${result > 0 ? "+" : ""}${result.toFixed(1)}%`;
}

function signedRatio(value, total) {
  if (!total) {
    return "n/a";
  }
  return signedPercent(value, Math.abs(total));
}

function markdownCell(value) {
  return String(value).replaceAll("|", "\\|");
}

function shortSha(sha) {
  return sha ? sha.slice(0, 7) : "unknown";
}

function metricsRow(label, row, formatter = format) {
  return `| ${markdownCell(label)} | ${formatter(row.files)} | ${formatter(row.lines)} | ${formatter(row.code)} | ${formatter(row.comments)} | ${formatter(row.blank)} | ${formatter(row.complexity)} |`;
}

function nonZeroDelta(row) {
  return (
    row.files !== 0 ||
    row.lines !== 0 ||
    row.code !== 0 ||
    row.comments !== 0 ||
    row.blank !== 0 ||
    row.complexity !== 0
  );
}

function sectionOrEmpty(rows, emptyText) {
  return rows.length > 0 ? rows : [`| ${emptyText} | 0 | 0 | 0 | 0 |`];
}

export function prepareRepositorySizeReport(reportDir) {
  if (!reportDir) {
    throw new Error("SCC_REPORT_DIR is required.");
  }

  const sccByFileRows = readJson(reportDir, "scc-by-file.json");
  const baseByFileRows = readJson(reportDir, "scc-base-by-file.json");
  const sccVersion = readText(reportDir, "scc.version");

  const files = fileRows(sccByFileRows);
  const baseFiles = fileRows(baseByFileRows);
  const sccTotal = totalFiles(files);
  const baseTotal = totalFiles(baseFiles);
  const totalDelta = subtractMetrics(sccTotal, baseTotal);
  const languageTotals = totalsByLanguage(files);
  const baseLanguageTotals = totalsByLanguage(baseFiles);

  const languageRows = Array.from(languageTotals.entries())
    .filter(([, row]) => row.code > 0)
    .sort(([, a], [, b]) => b.code - a.code)
    .slice(0, 8)
    .map(
      ([language, row]) =>
        `| ${markdownCell(language)} | ${format(row.files)} | ${format(row.lines)} | ${format(row.code)} | ${percent(row.code, sccTotal.code)} | ${format(row.comments)} | ${format(row.blank)} | ${format(row.complexity)} |`,
    );

  const areaRows = Array.from(totalsByArea(files).entries())
    .sort(([, a], [, b]) => b.code - a.code)
    .slice(0, 8)
    .map(
      ([area, row]) =>
        `| ${markdownCell(area)} | ${format(row.files)} | ${format(row.lines)} | ${format(row.code)} | ${percent(row.code, sccTotal.code)} | ${format(row.comments)} | ${format(row.blank)} | ${format(row.complexity)} |`,
    );

  const languageDeltaRows = diffMap(languageTotals, baseLanguageTotals)
    .filter(([, row]) => nonZeroDelta(row))
    .sort(([, a], [, b]) => Math.abs(b.code) - Math.abs(a.code) || Math.abs(b.complexity) - Math.abs(a.complexity))
    .slice(0, 8)
    .map(
      ([language, row]) =>
        `| ${markdownCell(language)} | ${signed(row.files)} | ${signed(row.code)} | ${signedPercent(row.code, baseTotal.code)} | ${signed(row.complexity)} |`,
    );

  const areaDeltaRows = diffMap(totalsByArea(files), totalsByArea(baseFiles))
    .filter(([, row]) => nonZeroDelta(row))
    .sort(([, a], [, b]) => Math.abs(b.code) - Math.abs(a.code) || Math.abs(b.complexity) - Math.abs(a.complexity))
    .slice(0, 8)
    .map(
      ([area, row]) =>
        `| ${markdownCell(area)} | ${signed(row.files)} | ${signed(row.code)} | ${signedPercent(row.code, baseTotal.code)} | ${signed(row.complexity)} |`,
    );

  const headBalance = balanceTotals(files);
  const baseBalance = balanceTotals(baseFiles);
  const appDelta = subtractMetrics(headBalance.app, baseBalance.app);
  const testDelta = subtractMetrics(headBalance.tests, baseBalance.tests);

  const complexityRows = files
    .filter((file) => file.code > 0 && file.complexity > 0)
    .sort(
      (a, b) =>
        b.complexity - a.complexity || b.complexity / b.code - a.complexity / a.code || a.path.localeCompare(b.path),
    )
    .slice(0, 8)
    .map(
      (file) =>
        `| \`${markdownCell(file.path)}\` | ${markdownCell(file.language)} | ${format(file.code)} | ${format(file.complexity)} | ${((file.complexity / file.code) * 100).toFixed(1)} |`,
    );

  const body = [
    marker,
    "## Repository Size",
    "",
    `Measured by \`${sccVersion}\` on PR head \`${shortSha(measuredSha)}\` against base \`${shortSha(baseSha)}\`.`,
    "",
    "### Summary",
    "",
    "| Scope | Files | Lines | Code | Comments | Blank | Complexity |",
    "|---|---:|---:|---:|---:|---:|---:|",
    metricsRow("PR head", sccTotal),
    metricsRow("Base", baseTotal),
    metricsRow("Delta", totalDelta, signed),
    "",
    "### Delta by Language",
    "",
    "| Language | Files Δ | Code Δ | Code Δ / Base | Complexity Δ |",
    "|---|---:|---:|---:|---:|",
    ...sectionOrEmpty(languageDeltaRows, "No language changes"),
    "",
    "### Delta by Area",
    "",
    "| Area | Files Δ | Code Δ | Code Δ / Base | Complexity Δ |",
    "|---|---:|---:|---:|---:|",
    ...sectionOrEmpty(areaDeltaRows, "No area changes"),
    "",
    "### Test Balance",
    "",
    "| Scope | App Code | Test Code | Test / App |",
    "|---|---:|---:|---:|",
    `| PR head | ${format(headBalance.app.code)} | ${format(headBalance.tests.code)} | ${percent(headBalance.tests.code, headBalance.app.code)} |`,
    `| Delta | ${signed(appDelta.code)} | ${signed(testDelta.code)} | ${signedRatio(testDelta.code, appDelta.code)} |`,
    "",
    "<details>",
    "<summary>Top languages, areas, and complexity hotspots</summary>",
    "",
    "### Top Languages by Code",
    "",
    "| Language | Files | Lines | Code | Code % | Comments | Blank | Complexity |",
    "|---|---:|---:|---:|---:|---:|---:|---:|",
    ...languageRows,
    "",
    "### Top Areas by Code",
    "",
    "| Area | Files | Lines | Code | Code % | Comments | Blank | Complexity |",
    "|---|---:|---:|---:|---:|---:|---:|---:|",
    ...areaRows,
    "",
    "### Complexity Hotspots",
    "",
    "| File | Language | Code | Complexity | Complexity / 100 Code |",
    "|---|---|---:|---:|---:|",
    ...sectionOrEmpty(complexityRows, "No complexity hotspots"),
    "",
    "</details>",
    "",
    "_Generated from the current PR checkout. `scc` honors repository ignore rules; use this as a size and trend signal, not a merge gate._",
  ].join("\n");

  fs.writeFileSync(path.join(reportDir, "repository-size-comment.md"), `${body}\n`, "utf8");
}

if (process.argv[1] && path.resolve(process.argv[1]) === __filename) {
  prepareRepositorySizeReport(process.env.SCC_REPORT_DIR);
}
