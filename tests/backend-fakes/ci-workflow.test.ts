import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";
import { parse } from "yaml";

const repositoryRoot = fileURLToPath(new URL("../../", import.meta.url));
const independentAuditCondition = [
  "$",
  "{{ !cancelled() && steps.install_dependencies_for_audit.outcome == 'success' }}",
].join("");
const applicationConcurrencyGroup = [
  "ci",
  ["$", "{{ github.workflow }}"].join(""),
  ["$", "{{ github.event_name }}"].join(""),
  ["$", "{{ github.ref }}"].join(""),
].join("-");

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

type BranchTrigger = {
  branches?: string[];
};

type Workflow = {
  concurrency?: {
    group?: string;
    "cancel-in-progress"?: boolean;
  };
  jobs: Record<string, WorkflowJob>;
  on?: {
    pull_request?: BranchTrigger;
    push?: BranchTrigger;
    schedule?: unknown;
    workflow_dispatch?: unknown;
  };
  permissions?: Record<string, string>;
};

function readWorkflow(fileName: string): Workflow {
  return parse(readFileSync(`${repositoryRoot}.github/workflows/${fileName}`, "utf8")) as Workflow;
}

describe("dependency-audit.yml", () => {
  it("runs every audit after dependency installation even if an earlier audit fails", () => {
    const auditSteps = readWorkflow("dependency-audit.yml").jobs["dependency-audit"].steps ?? [];
    const installStep = auditSteps.find((step) => step.id === "install_dependencies_for_audit");
    const commands = auditSteps.filter((step) => step.run?.startsWith("bun run audit:"));

    expect(installStep?.run).toBe("bun run install:locked");
    expect(commands).toHaveLength(4);
    for (const step of commands) {
      expect(step.if).toBe(independentAuditCondition);
    }
  });

  it("preserves pull request, main push, manual, and weekly audit entry points", () => {
    const workflow = readWorkflow("dependency-audit.yml");

    expect(Object.keys(workflow.on ?? {}).sort()).toEqual([
      "pull_request",
      "push",
      "schedule",
      "workflow_dispatch",
    ]);
    expect(workflow.on?.pull_request?.branches).toEqual(["main"]);
    expect(workflow.on?.push?.branches).toEqual(["main"]);
  });
});

describe("ci.yml application workflow", () => {
  it("handles only main pull requests, main pushes, and manual dispatch", () => {
    const workflow = readWorkflow("ci.yml");

    expect(Object.keys(workflow.on ?? {}).sort()).toEqual([
      "pull_request",
      "push",
      "workflow_dispatch",
    ]);
    expect(workflow.on?.pull_request?.branches).toEqual(["main"]);
    expect(workflow.on?.push?.branches).toEqual(["main"]);
  });

  it("cancels only older runs for the same event and ref", () => {
    const workflow = readWorkflow("ci.yml");

    expect(workflow.concurrency?.group).toBe(applicationConcurrencyGroup);
    expect(workflow.concurrency?.["cancel-in-progress"]).toBe(true);
  });

  it("keeps auxiliary checks outside application jobs", () => {
    const { jobs } = readWorkflow("ci.yml");

    expect(jobs["dependency-audit"]).toBeUndefined();
    expect(jobs["repository-size"]).toBeUndefined();

    const serviceCommands = ["viewer-api-go", "novel-fetcher"].flatMap((jobName) =>
      (jobs[jobName].steps ?? []).map((step) => step.run ?? ""),
    );
    expect(serviceCommands.some((command) => command.includes("bun run audit:"))).toBe(false);
  });

  it("starts E2E after only the viewer-web build and keeps API contract independent", () => {
    const { jobs } = readWorkflow("ci.yml");

    expect(jobs.e2e.needs).toBe("viewer-web-build");
    expect(jobs["api-contract"].needs).toBeUndefined();

    const e2eCommands = (jobs.e2e.steps ?? []).map((step) => step.run ?? "");
    expect(
      e2eCommands.filter((command) => command === "bash ./scripts/wait-and-download-artifact.sh"),
    ).toHaveLength(2);
  });

  it("runs the normal API contract suite once in its dedicated job", () => {
    const { jobs } = readWorkflow("ci.yml");
    const apiContractCommands = (jobs["api-contract"].steps ?? []).map(
      (step) => step.run ?? "",
    );

    expect(
      apiContractCommands.filter((command) => command.includes("bun run test:api-contract")),
    ).toHaveLength(1);

    for (const [jobName, job] of Object.entries(jobs)) {
      const commands = (job.steps ?? []).map((step) => step.run ?? "");

      if (jobName !== "api-contract") {
        expect(commands.some((command) => command.includes("bun run test:api-contract"))).toBe(
          false,
        );
      }
      expect(commands.some((command) => command.includes("verify:api-go:contract"))).toBe(false);
    }
  });
});

describe("repository-size.yml", () => {
  it("isolates the PR-only report and its write permission", () => {
    const workflow = readWorkflow("repository-size.yml");

    expect(Object.keys(workflow.on ?? {})).toEqual(["pull_request"]);
    expect(workflow.on?.pull_request?.branches).toEqual(["main"]);
    expect(workflow.permissions).toEqual({
      contents: "read",
      "pull-requests": "write",
    });
    expect(workflow.jobs["repository-size"]?.name).toBe("Repository size report");
  });
});

describe("security-audit.yml", () => {
  it("does not duplicate dependency auditing", () => {
    expect(readWorkflow("security-audit.yml").jobs["dependency-audit"]).toBeUndefined();
  });
});
