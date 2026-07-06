import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";

export const workspaceRoot = path.resolve(new URL("../../..", import.meta.url).pathname);
export const fixturesRoot = path.join(workspaceRoot, "tests", "fixtures");

export async function createTempDataDir(prefix: string): Promise<string> {
  return fs.mkdtemp(path.join(os.tmpdir(), prefix));
}

export async function copyStateFixture(name: string): Promise<string> {
  const dataDir = await createTempDataDir(`narou-viewer-backend-fakes-${name}-`);
  await fs.cp(path.join(fixturesRoot, "state", name), dataDir, {
    recursive: true,
    errorOnExist: false
  });
  return dataDir;
}

export async function readFixtureText(relativePath: string): Promise<string> {
  return fs.readFile(path.join(fixturesRoot, relativePath), "utf8");
}

export async function readFixtureJson<T>(relativePath: string): Promise<T> {
  return JSON.parse(await readFixtureText(relativePath)) as T;
}
