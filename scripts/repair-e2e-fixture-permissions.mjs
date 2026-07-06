import { execFileSync, spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, "..");
const repoDataDir = path.join(repoRoot, "data_e2e");

function resolveWorkspaceSource() {
  if (process.env.HOST_WORKSPACE_DIR) {
    return process.env.HOST_WORKSPACE_DIR;
  }

  try {
    const containerId = execFileSync("hostname", { encoding: "utf8" }).trim();
    if (!containerId) {
      return null;
    }

    const inspectOutput = execFileSync("docker", ["inspect", containerId], { encoding: "utf8" });
    const [container] = JSON.parse(inspectOutput);
    const workspaceMount = container?.Mounts?.find((mount) => mount.Destination === "/workspace");
    return typeof workspaceMount?.Source === "string" && workspaceMount.Source.length > 0
      ? workspaceMount.Source
      : null;
  } catch {
    return null;
  }
}

function resolveHostDataDir() {
  if (process.env.HOST_E2E_DATA_DIR) {
    return process.env.HOST_E2E_DATA_DIR;
  }

  const workspaceSource = resolveWorkspaceSource();
  if (workspaceSource) {
    return path.join(workspaceSource, "data_e2e");
  }

  return repoDataDir;
}

function getCurrentIds() {
  const envUid = Number.parseInt(process.env.E2E_FIXTURE_UID ?? "", 10);
  const envGid = Number.parseInt(process.env.E2E_FIXTURE_GID ?? "", 10);
  const uid = Number.isInteger(envUid) ? envUid : typeof process.getuid === "function" ? process.getuid() : 1000;
  const gid = Number.isInteger(envGid) ? envGid : typeof process.getgid === "function" ? process.getgid() : 1000;
  return { uid, gid };
}

function resolveDockerEnv() {
  const dockerEnv = { ...process.env };

  if (dockerEnv.DOCKER_API_VERSION) {
    return dockerEnv;
  }

  const versionResult = spawnSync("docker", ["version", "--format", "{{.Server.APIVersion}}"], {
    encoding: "utf8",
  });
  const combinedOutput = `${versionResult.stdout ?? ""}\n${versionResult.stderr ?? ""}`;
  const matchedVersion = combinedOutput.match(/Maximum supported API version is ([0-9.]+)/);

  if (matchedVersion?.[1]) {
    dockerEnv.DOCKER_API_VERSION = matchedVersion[1];
  }

  return dockerEnv;
}

function main() {
  if (!fs.existsSync(repoDataDir)) {
    console.log("No e2e fixture directory found. Nothing to repair.");
    return;
  }

  const hostDataDir = resolveHostDataDir();
  const { uid, gid } = getCurrentIds();
  const repairCommand = `chown -R ${uid}:${gid} /target && chmod -R a+rwX /target`;
  const repairImage = process.env.E2E_REPAIR_IMAGE || "busybox:1.37";
  const result = spawnSync(
    "docker",
    ["run", "--rm", "-u", "0:0", "-v", `${hostDataDir}:/target`, repairImage, "sh", "-c", repairCommand],
    { env: resolveDockerEnv(), stdio: "inherit" },
  );

  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }

  console.log(`Repaired e2e fixture permissions: ${path.relative(repoRoot, repoDataDir)}`);
}

main();
