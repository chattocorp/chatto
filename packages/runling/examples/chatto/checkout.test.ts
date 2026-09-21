import { mkdtemp, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { execa } from "execa";
import { afterEach, beforeEach, expect, test } from "vitest";
import { createWorkflowContext } from "runling";
import { refreshCheckout } from "./checkout.ts";

let directory: string;
let seed: string;
let copy: string;
const git = (cwd: string, ...args: string[]) => execa("git", args, { cwd, env: {
  GIT_AUTHOR_NAME: "Test", GIT_AUTHOR_EMAIL: "test@example.com", GIT_COMMITTER_NAME: "Test", GIT_COMMITTER_EMAIL: "test@example.com",
} });
beforeEach(async () => {
  directory = await mkdtemp(join(tmpdir(), "runling-checkout-"));
  seed = join(directory, "seed");
  copy = join(directory, "copy");
  await git(directory, "init", "--bare", "--initial-branch=main", "remote");
  await git(directory, "clone", join(directory, "remote"), seed);
  await writeFile(join(seed, "file"), "first");
  await git(seed, "add", "file");
  await git(seed, "commit", "-m", "initial");
  await git(seed, "push", "origin", "main");
  await git(directory, "clone", join(directory, "remote"), copy);
});
afterEach(async () => rm(directory, { recursive: true, force: true }));

test("fetches and fast-forwards a clean main checkout", async () => {
  await writeFile(join(seed, "file"), "second");
  await git(seed, "commit", "-am", "advance main");
  await git(seed, "push", "origin", "main");
  const revision = await refreshCheckout(createWorkflowContext(), copy);
  expect(revision).toBe((await git(seed, "rev-parse", "HEAD")).stdout);
  expect((await git(copy, "rev-parse", "HEAD")).stdout).toBe(revision);
});

test("refuses dirty files without discarding them", async () => {
  await writeFile(join(copy, "file"), "local work");
  await expect(refreshCheckout(createWorkflowContext(), copy)).rejects.toThrow("must be clean");
  expect((await git(copy, "diff")).stdout).toContain("local work");
});

test("refuses a different branch", async () => {
  await git(copy, "switch", "-c", "feature");
  await expect(refreshCheckout(createWorkflowContext(), copy)).rejects.toThrow("must be on main");
  expect((await git(copy, "branch", "--show-current")).stdout).toBe("feature");
});

test("refuses local commits ahead of origin without resetting", async () => {
  await writeFile(join(copy, "file"), "local commit");
  await git(copy, "commit", "-am", "local work");
  const head = (await git(copy, "rev-parse", "HEAD")).stdout;
  await expect(refreshCheckout(createWorkflowContext(), copy)).rejects.toThrow();
  expect((await git(copy, "rev-parse", "HEAD")).stdout).toBe(head);
});
