import assert from "node:assert/strict";
import test from "node:test";
import { rerunProto } from "./rerun-proto.mjs";

function fixture(label = "api-breaking-change") {
  const calls = [];
  const context = {
    repo: { owner: "org", repo: "repo" },
    payload: {
      action: "unlabeled",
      label: { name: label },
      pull_request: { number: 42 },
    },
  };
  const github = {
    rest: {
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
                { id: 5, status: "completed", pull_requests: [{ number: 42 }] },
              ],
            },
          };
        },
        listJobsForWorkflowRun: () => {},
        reRunJobForWorkflowRun: async (args) => calls.push(args),
      },
    },
    paginate: async () => [
      {
        id: 7,
        name: "codegen-proto-drift",
        steps: [{ name: "Read current public API waiver" }],
      },
      { id: 8, name: "test-cli" },
    ],
  };
  return { github, context, core: { info() {} }, sleep: async () => {}, calls };
}
test("unrelated labels do not query or restart CI", async () => {
  const f = fixture("documentation");
  await rerunProto(f);
  assert.deepEqual(f.calls, []);
});
test("removing a waiver reruns only the current revision protobuf job", async () => {
  const f = fixture();
  await rerunProto(f);
  assert.equal(f.calls[0].head_sha, "current");
  assert.equal(f.calls[1].job_id, 7);
  assert.equal(f.calls.length, 2);
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
    pending = false;
  };
  await rerunProto(f);
  assert.equal(f.calls.at(-1).job_id, 7);
});

test("old workflow runs must be updated before label revalidation", async () => {
  const f = fixture();
  f.github.paginate = async () => [
    { id: 7, name: "codegen-proto-drift", steps: [] },
  ];
  await assert.rejects(rerunProto(f), /predates live waiver checks/);
  assert.equal(f.calls.length, 1);
});
