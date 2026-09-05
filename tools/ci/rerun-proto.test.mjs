import assert from "node:assert/strict";
import test from "node:test";
import { rerunProto } from "./rerun-proto.mjs";

function fixture(label = "api-breaking-change") {
  const calls = [];
  const statuses = [];
  let attempt = 1;
  const context = {
    repo: { owner: "org", repo: "repo" },
    runId: 10,
    payload: {
      action: "unlabeled",
      label: { name: label },
      pull_request: { number: 42 },
    },
  };
  const github = {
    rest: {
      repos: { createCommitStatus: async (args) => statuses.push(args) },
      pulls: {
        get: async () => ({
          data: { state: "open", head: { sha: "current" } },
        }),
      },
      actions: {
        listWorkflowRuns: async (args) => {
          calls.push(args);
          return {
            data: {
              workflow_runs: [
                {
                  id: 5,
                  run_attempt: 1,
                  status: "completed",
                  pull_requests: [{ number: 42 }],
                },
              ],
            },
          };
        },
        listJobsForWorkflowRun: () => {},
        reRunJobForWorkflowRun: async (args) => {
          calls.push(args);
          attempt = 2;
        },
      },
    },
    paginate: async () => [
      {
        id: 7,
        name: "codegen-proto-drift",
        status: "completed",
        conclusion: "success",
        run_attempt: attempt,
        steps: [{ name: "Read current public API waiver" }],
      },
    ],
  };
  return {
    github,
    context,
    core: { info() {} },
    sleep: async () => {},
    calls,
    statuses,
  };
}
test("unrelated labels do not query, restart, or change CI status", async () => {
  const f = fixture("documentation");
  await rerunProto(f);
  assert.deepEqual(f.calls, []);
  assert.deepEqual(f.statuses, []);
});
test("removing a waiver blocks merging until only the protobuf job is revalidated", async () => {
  const f = fixture();
  await rerunProto(f);
  assert.equal(f.calls[0].head_sha, "current");
  assert.equal(f.calls[1].job_id, 7);
  assert.deepEqual(
    f.statuses.map(({ state }) => state),
    ["pending", "success"],
  );
  assert.ok(
    f.statuses.every(({ context }) => context === "codegen-proto-drift"),
  );
});
test("an active run is allowed to finish before revalidation", async () => {
  const f = fixture();
  const original = f.github.rest.actions.listWorkflowRuns;
  let pending = true;
  f.github.rest.actions.listWorkflowRuns = async (args) =>
    pending
      ? {
          data: {
            workflow_runs: [
              { id: 5, status: "in_progress", pull_requests: [{ number: 42 }] },
            ],
          },
        }
      : original(args);
  f.sleep = async () => {
    assert.equal(f.statuses[0].state, "pending");
    pending = false;
  };
  await rerunProto(f);
  assert.equal(f.statuses.at(-1).state, "success");
});
test("old workflow runs fail closed until the branch is updated", async () => {
  const f = fixture();
  f.github.paginate = async () => [
    { id: 7, name: "codegen-proto-drift", steps: [] },
  ];
  await assert.rejects(rerunProto(f), /predates live waiver checks/);
  assert.equal(f.statuses.at(-1).state, "failure");
});
test("a failed rerun cannot clear the required status", async () => {
  const f = fixture();
  const original = f.github.paginate;
  f.github.paginate = async (...args) =>
    (await original(...args)).map((job) => ({ ...job, conclusion: "failure" }));
  await assert.rejects(rerunProto(f), /Protobuf revalidation failed/);
  assert.equal(f.statuses.at(-1).state, "failure");
});
