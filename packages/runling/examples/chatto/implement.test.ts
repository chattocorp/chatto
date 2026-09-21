import { mkdtemp, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { execa } from "execa";
import { expect, test, vi } from "vitest";
import {
  createWorkflowContext,
  emptyTokenUsage,
  type AgentResult,
} from "runling";
import type { SpecialistUpdate } from "./agent-text.ts";
import {
  createChattoImplementation,
  prepareImplementation,
} from "./implement.ts";

const report: AgentResult = {
  outcome: "completed",
  summary: "Implemented",
  usage: emptyTokenUsage(),
};
const input = {
  directory: "/source",
  plan: "The approved plan",
  model: "test/model",
};
function fixture(failures: (string | null)[]) {
  const worker = { runOutcome: vi.fn(async () => report), dispose: vi.fn() };
  const createAgent = vi.fn(async () => worker);
  const prepare = vi.fn(async () => ({
    directory: "/isolated",
    branch: "runling/test",
  }));
  const validate = vi.fn(async () => failures.shift() ?? null);
  const hasChanges = vi.fn(async () => true);
  const onText = vi.fn(async (_message: string) => {});
  const ctx = {
    ...createWorkflowContext(),
    emit: async (update: SpecialistUpdate) => {
      await onText(update.text);
    },
  };
  return {
    worker,
    createAgent,
    prepare,
    validate,
    hasChanges,
    onText,
    ctx,
    task: createChattoImplementation({
      createAgent,
      prepare,
      validate,
      hasChanges,
    }),
  };
}

test("implements the exact plan in its new directory and repairs failed checks", async () => {
  const f = fixture(["check failed", null]);
  const ctx = f.ctx;
  expect(await f.task(ctx, input)).toEqual({
    summary: "Implemented",
    directory: "/isolated",
    branch: "runling/test",
  });
  expect(f.createAgent.mock.calls[0]).toEqual([
    expect.objectContaining({ cwd: "/isolated" }),
  ]);
  expect(f.worker.runOutcome.mock.calls[0]).toEqual([
    ctx,
    expect.stringContaining(input.plan),
    expect.objectContaining({ onText: expect.any(Function) }),
  ]);
  expect(f.worker.runOutcome.mock.calls[1]).toEqual([
    ctx,
    expect.stringContaining("check failed"),
    expect.objectContaining({ onText: expect.any(Function) }),
  ]);
  expect(f.validate).toHaveBeenCalledTimes(2);
  expect(f.onText.mock.calls.map((call) => call[0])).toEqual([
    "Preparing an isolated worktree and installing dependencies.",
    "Implementing the approved plan.\nWorktree: /isolated\nBranch: runling/test",
    "Running project checks and tests (attempt 1/3).",
    "Validation failed. Repairing the implementation before attempt 2/3.",
    "Running project checks and tests (attempt 2/3).",
  ]);
  expect(f.worker.dispose).toHaveBeenCalledOnce();
});

test("limits repairs and reports the retained worktree on failure", async () => {
  const f = fixture(["failed", "failed", "failed"]);
  await expect(f.task(createWorkflowContext(), input)).rejects.toThrow(
    "Worktree retained at /isolated (runling/test)",
  );
  expect(f.worker.runOutcome).toHaveBeenCalledTimes(3);
  expect(f.worker.dispose).toHaveBeenCalledOnce();
});

test("rejects empty changes and stops when cancelled", async () => {
  const f = fixture([]);
  f.hasChanges.mockResolvedValue(false);
  await expect(f.task(createWorkflowContext(), input)).rejects.toThrow(
    "without worktree changes",
  );
  expect(f.validate).not.toHaveBeenCalled();
  const g = fixture([]);
  const ctx = createWorkflowContext();
  g.worker.runOutcome.mockImplementationOnce(async () => ctx.abort("stop"));
  await expect(g.task(ctx, input)).rejects.toThrow("stop");
  expect(g.validate).not.toHaveBeenCalled();
  expect(g.worker.dispose).toHaveBeenCalledOnce();
});

test("creates an isolated branch without changing main and refuses a dirty source", async () => {
  const source = await mkdtemp(join(tmpdir(), "chatto-implementation-"));
  const git = (...args: string[]) =>
    execa("git", args, {
      cwd: source,
      env: {
        GIT_AUTHOR_NAME: "Test",
        GIT_AUTHOR_EMAIL: "test@example.com",
        GIT_COMMITTER_NAME: "Test",
        GIT_COMMITTER_EMAIL: "test@example.com",
      },
    });
  let workspace: { directory: string; branch: string } | undefined;
  try {
    await git("init", "--initial-branch=main");
    await writeFile(join(source, "file"), "base");
    await git("add", "file");
    await git("commit", "-m", "initial");
    workspace = await prepareImplementation(
      createWorkflowContext(),
      source,
      async () => {},
    );
    expect((await git("branch", "--show-current")).stdout).toBe("main");
    expect(
      (
        await execa("git", ["branch", "--show-current"], {
          cwd: workspace.directory,
        })
      ).stdout,
    ).toBe(workspace.branch);
    await writeFile(join(workspace.directory, "file"), "implementation");
    expect((await git("status", "--porcelain")).stdout).toBe("");
    await writeFile(join(source, "file"), "local change");
    await expect(
      prepareImplementation(createWorkflowContext(), source, async () => {}),
    ).rejects.toThrow("must be clean");
  } finally {
    if (workspace)
      await git("worktree", "remove", "--force", workspace.directory);
    await rm(source, { recursive: true, force: true });
  }
});
