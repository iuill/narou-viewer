import { existsSync, readFileSync, readdirSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const repoRoot = resolve(import.meta.dirname, "../../..");
const frontendRoot = resolve(repoRoot, "apps/viewer-web");
const removedFrontendFacades = [
  "api/client",
  "api/types",
  "appHelpers",
  "viewerUtils"
];

function readSource(relativePath: string): string {
  return readFileSync(resolve(repoRoot, relativePath), "utf8");
}

function moduleSpecifiers(source: string): string[] {
  const staticSpecifiers = Array.from(
    source.matchAll(/^\s*(?:import|export)(?:\s+type)?[\s\S]*?\sfrom\s+["']([^"']+)["'];/gm),
    (match) => match[1]
  );
  const dynamicSpecifiers = Array.from(source.matchAll(/import\(\s*["']([^"']+)["']\s*\)/g), (match) => match[1]);
  return [...staticSpecifiers, ...dynamicSpecifiers];
}

function listSourceFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = resolve(directory, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "dist" ? [] : listSourceFiles(path);
    }
    return entry.isFile() && /\.(?:ts|tsx)$/.test(entry.name) ? [path] : [];
  });
}

describe("frontend App composition architecture", () => {
  it("keeps App.tsx free from feature-specific orchestration", () => {
    const source = readSource("apps/viewer-web/src/App.tsx");
    const imports = moduleSpecifiers(source);

    expect(imports).toEqual(expect.arrayContaining([
      "./app/useViewerAppModel",
      "./routes/ReaderRouteController",
      "./screens/LibraryShell",
      "./screens/ReaderShell"
    ]));
    expect(imports).not.toEqual(expect.arrayContaining(["react"]));
    expect(source).not.toMatch(/features\/(reader|fetcher|library)/);
    expect(source).not.toMatch(/use(?:Reader|AiGeneration|CharacterSummary|Fetcher|Library)\w*\(/);
    expect(source).not.toMatch(/use(?:State|Effect|Memo|Callback|Ref)\(/);
  });

  it("keeps screen-specific props out of the app model boundary", () => {
    const source = readSource("apps/viewer-web/src/app/useViewerAppModel.tsx");

    expect(source).not.toMatch(/libraryScreenProps:\s*\{/);
    expect(source).not.toMatch(/state:\s*\{/);
    expect(source).not.toMatch(/commands:\s*\{/);
    expect(source).not.toMatch(/refs:\s*\{/);
    expect(source).not.toMatch(/features\/(?:reader|fetcher)\//);
  });

  it("keeps workspace details out of shell components", () => {
    const libraryWorkspace = readSource("apps/viewer-web/src/screens/library/useLibraryWorkspaceModel.ts");
    const libraryShell = readSource("apps/viewer-web/src/screens/LibraryShell.tsx");

    expect(libraryWorkspace).not.toMatch(/return\s+props\s*;/);
    expect(libraryShell).not.toMatch(/use(?:State|Effect|Memo|Callback)\(/);
    expect(libraryShell).not.toMatch(/(?:download|resume|cancel|remove|update)FetcherWorks/);
    expect(libraryShell).not.toMatch(/use(?:FetcherStatus|AiGeneration|Library)\(/);
  });

  it("passes Reader AI availability without a state mirror", () => {
    const libraryWorkspace = readSource("apps/viewer-web/src/screens/library/useLibraryWorkspaceModel.ts");
    const readerWorkspace = readSource("apps/viewer-web/src/screens/reader/useReaderWorkspaceModel.tsx");

    expect(libraryWorkspace).not.toContain("setAiReaderAssistant");
    expect(readerWorkspace).not.toContain("setAiReaderAssistant");
    expect(readerWorkspace).not.toMatch(/useState<[^>]*ReaderAiAssistantAvailability/);
  });

  it("keeps deprecated frontend facade modules removed", () => {
    const removedFacadePaths = removedFrontendFacades.map((path) => `src/${path}.ts`);

    expect(removedFacadePaths.filter((path) => existsSync(resolve(frontendRoot, path)))).toEqual([]);

    const forbiddenImportPattern = new RegExp(
      `(?:^|/)(${removedFrontendFacades.map((path) => path.replace(/[\\^$.*+?()[\]{}|]/g, "\\$&")).join("|")})(?:\\.ts)?$`
    );
    const sourceFiles = [
      ...listSourceFiles(resolve(frontendRoot, "src")),
      ...listSourceFiles(resolve(frontendRoot, "tests"))
    ];
    const offenders = sourceFiles.flatMap((filePath) => {
      const source = readFileSync(filePath, "utf8");
      return moduleSpecifiers(source)
        .filter((path) => forbiddenImportPattern.test(path.replace(/^(?:\.\.?\/)+/, "")))
        .map((path) => `${filePath.replace(`${frontendRoot}/`, "")}: ${path}`);
    });

    expect(offenders).toEqual([]);
  });
});
