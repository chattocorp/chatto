import { writeFile } from "node:fs/promises";
import { spawnProcess } from "../../test/process.ts";
import { afterEach, beforeEach, describe, expect, test } from "vitest";
import { chmod, mkdtemp, rm, symlink, unlink } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { getPwd, workingTreeHash } from "runling/git";

async function git(cwd: string, ...args: string[]) {
  const process = spawnProcess(["git", ...args], {
    cwd,
    stdout: "pipe",
    stderr: "pipe",
  });
  const [exitCode, stderr] = await Promise.all([
    process.exited,
    new Response(process.stderr).text(),
  ]);

  if (exitCode !== 0) {
    throw new Error(stderr);
  }
}

describe("workingTreeHash", () => {
  let cwd: string;

  beforeEach(async () => {
    cwd = await mkdtemp(join(tmpdir(), "runling-git-test-"));
    await git(cwd, "init", "--quiet");
    await writeFile(join(cwd, "tracked.txt"), "initial\n");
    await git(cwd, "add", "tracked.txt");
    await git(
      cwd,
      "-c",
      "user.name=Runling Test",
      "-c",
      "user.email=runling@example.com",
      "commit",
      "--quiet",
      "-m",
      "initial",
    );
  });

  afterEach(async () => {
    await rm(cwd, { recursive: true, force: true });
  });

  test("is stable while the working tree is unchanged", async () => {
    expect(await workingTreeHash(cwd)).toBe(await workingTreeHash(cwd));
  });

  test("changes when a tracked file changes", async () => {
    const before = await workingTreeHash(cwd);
    await writeFile(join(cwd, "tracked.txt"), "changed\n");
    expect(await workingTreeHash(cwd)).not.toBe(before);
  });

  test("includes untracked file contents", async () => {
    const clean = await workingTreeHash(cwd);
    const path = join(cwd, "untracked.txt");

    await writeFile(path, "first\n");
    const first = await workingTreeHash(cwd);

    await writeFile(path, "second\n");
    const second = await workingTreeHash(cwd);

    expect(first).not.toBe(clean);
    expect(second).not.toBe(first);

    await unlink(path);
    expect(await workingTreeHash(cwd)).toBe(clean);
  });

  test("detects file changes that were committed after a snapshot", async () => {
    const snapshot = await getPwd(cwd);
    await writeFile(join(cwd, "tracked.txt"), "committed change\n");
    await git(cwd, "-c", "user.name=Runling Test", "-c", "user.email=runling@example.com",
      "commit", "--quiet", "-am", "Change tracked file");
    expect(await snapshot.hasChanges).toBe(true);
  });

  test("ignores commits that do not change the tree", async () => {
    const snapshot = await getPwd(cwd);
    await git(cwd, "-c", "user.name=Runling Test", "-c", "user.email=runling@example.com",
      "commit", "--quiet", "--allow-empty", "-m", "Metadata only");
    expect(await snapshot.hasChanges).toBe(false);
  });

  test("includes untracked file modes", async () => {
    const path = join(cwd, "script.sh");
    await writeFile(path, "#!/bin/sh\n");
    const before = await workingTreeHash(cwd);

    await chmod(path, 0o755);

    expect(await workingTreeHash(cwd)).not.toBe(before);
  });

  test("includes untracked symlink targets", async () => {
    await writeFile(join(cwd, "first.txt"), "same contents\n");
    await writeFile(join(cwd, "second.txt"), "same contents\n");
    const path = join(cwd, "link.txt");
    await symlink("first.txt", path);
    const before = await workingTreeHash(cwd);

    await unlink(path);
    await symlink("second.txt", path);

    expect(await workingTreeHash(cwd)).not.toBe(before);
  });

  test("supports repositories without a commit", async () => {
    const freshCwd = await mkdtemp(join(tmpdir(), "runling-unborn-test-"));

    try {
      await git(freshCwd, "init", "--quiet");
      await writeFile(join(freshCwd, "new.txt"), "staged\n");
      await git(freshCwd, "add", "new.txt");
      const pwd = await getPwd(freshCwd);

      await writeFile(join(freshCwd, "new.txt"), "changed\n");

      expect(await pwd.hasChanges).toBe(true);
    } finally {
      await rm(freshCwd, { recursive: true, force: true });
    }
  });

  test("working directory snapshots report unchanged state", async () => {
    const pwd = await getPwd(cwd);
    expect(await pwd.hasChanges).toBe(false);
  });

  test("working directory snapshots report later changes", async () => {
    const pwd = await getPwd(cwd);
    await writeFile(join(cwd, "tracked.txt"), "changed\n");
    expect(await pwd.hasChanges).toBe(true);
  });
});

test("requires an explicit directory instead of inspecting the process directory", async () => {
  // @ts-expect-error A directory is required.
  await expect(workingTreeHash()).rejects.toThrow("An explicit directory is required");
  // @ts-expect-error A directory is required.
  await expect(getPwd()).rejects.toThrow("An explicit directory is required");
});
