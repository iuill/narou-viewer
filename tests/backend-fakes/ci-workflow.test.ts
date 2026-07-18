import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";
import { parse } from "yaml";

const repositoryRoot = fileURLToPath(new URL("../../", import.meta.url));
const independentAuditCondition = [
  "$",
  "{{ !cancelled() && steps.install_dependencies_for_audit.outcome == 'success' }}",
].join("");

type WorkflowStep = {
  id?: string;
  if?: string;
  name?: string;
  run?: string;
};

type WorkflowJob = {
  name?: string;
  needs?: string | string[];
  steps?: WorkflowStep[];
};

type Workflow = {
  on?: Record<string, unknown>;
  jobs: Record<string, WorkflowJob>;
};

function readWorkflow(fileName: string): Workflow {
  return parse(readFileSync(`${repositoryRoot}.github/workflows/${fileName}`, "utf8")) as Workflow;
}

describe.each(["ci.yml", "security-audit.yml"])("%s dependency audit", (fileName) => {
  it("runs every audit after dependency installation even if an earlier audit fails", () => {
    const auditSteps = readWorkflow(fileName).jobs["dependency-audit"].steps ?? [];
    const installStep = auditSteps.find((step) => step.id === "install_dependencies_for_audit");
    const commands = auditSteps.filter((step) => step.run?.startsWith("bun run audit:"));

    expect(installStep?.run).toBe("bun run install:locked");
    expect(commands).toHaveLength(4);
    for (const step of commands) {
      expect(step.if).toBe(independentAuditCondition);
    }
  });
});

describe("ci.yml application workflow", () => {
  it("handles pull requests, main pushes, and manual dispatch in one workflow", () => {
    const workflow = readWorkflow("ci.yml");

    expect(Object.keys(workflow.on ?? {}).sort()).toEqual([
      "pull_request",
      "push",
      "workflow_dispatch",
    ]);
  });

  it("keeps dependency audit parallel and outside service build jobs", () => {
    const { jobs } = readWorkflow("ci.yml");

    expect(jobs["dependency-audit"].needs).toBeUndefined();
    expect(jobs["dependency-audit"].name).toContain("Dependency and toolchain audit");

    const serviceCommands = ["viewer-api-go", "novel-fetcher"].flatMap((jobName) =>
      (jobs[jobName].steps ?? []).map((step) => step.run ?? ""),
    );
    expect(serviceCommands.some((command) => command.includes("bun run audit:"))).toBe(false);
  });

  it("starts downstream service tests after only the viewer-web build", () => {
    const { jobs } = readWorkflow("ci.yml");

    expect(jobs.e2e.needs).toBe("viewer-web-build");
    expect(jobs["api-contract"].needs).toBe("viewer-web-build");

    const e2eCommands = (jobs.e2e.steps ?? []).map((step) => step.run ?? "");
    expect(
      e2eCommands.filter((command) => command === "bash ./scripts/wait-and-download-artifact.sh"),
    ).toHaveLength(2);
  });
});
