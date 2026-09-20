import { requireDirectory } from "./directory.ts";
import { createReadStream } from "node:fs";
import { lstat, readlink } from "node:fs/promises";
import { resolve } from "node:path";
import { createHash, type Hash } from "node:crypto";
import { execFile } from "node:child_process";
import { promisify } from "node:util";

const exec = promisify(execFile);

async function git(cwd: string, args: string[]) {
  const { stdout } = await exec("git", args, {
    cwd,
    encoding: "buffer",
    maxBuffer: 128 * 1024 * 1024,
  });
  return stdout;
}

async function hasHead(cwd: string) {
  try {
    await git(cwd, ["rev-parse", "--verify", "HEAD"]);
    return true;
  } catch {
    return false;
  }
}

function updateField(hasher: Hash, value: string | ArrayBuffer | Uint8Array) {
  const bytes =
    typeof value === "string" ? new TextEncoder().encode(value) : value;
  hasher.update(`${bytes.byteLength}:`);
  hasher.update(bytes instanceof ArrayBuffer ? new Uint8Array(bytes) : bytes);
}

async function updateUntrackedFile(hasher: Hash, cwd: string, path: string) {
  const absolutePath = resolve(cwd, path);
  const stat = await lstat(absolutePath);

  updateField(hasher, path);
  updateField(hasher, String(stat.mode));

  if (stat.isSymbolicLink()) {
    updateField(hasher, await readlink(absolutePath));
    return;
  }

  if (!stat.isFile()) {
    return;
  }

  updateField(hasher, String(stat.size));
  for await (const chunk of createReadStream(absolutePath)) {
    hasher.update(chunk);
  }
}

export async function workingTreeHash(cwd: string) {
  requireDirectory(cwd);
  const hasher = createHash("sha256");
  if (await hasHead(cwd)) {
    // The diff alone is empty at every clean commit. Include its base tree,
    // but not commit metadata, so committed file changes remain visible.
    updateField(hasher, await git(cwd, ["rev-parse", "HEAD^{tree}"]));
    updateField(hasher, await git(cwd, ["diff", "--binary", "HEAD"]));
  } else {
    updateField(hasher, await git(cwd, ["diff", "--binary", "--cached"]));
    updateField(hasher, await git(cwd, ["diff", "--binary"]));
  }

  const untrackedFiles = new TextDecoder()
    .decode(
      await git(cwd, ["ls-files", "--others", "--exclude-standard", "-z"]),
    )
    .split("\0")
    .filter(Boolean)
    .sort();

  for (const path of untrackedFiles) {
    await updateUntrackedFile(hasher, cwd, path);
  }

  return hasher.digest("hex");
}

export class WorkingDirectory {
  private constructor(
    readonly path: string,
    private readonly initialHash: string,
  ) {}

  static async create(path: string) {
    return new WorkingDirectory(path, await workingTreeHash(path));
  }

  get hasChanges(): Promise<boolean> {
    return workingTreeHash(this.path).then(
      (currentHash) => currentHash !== this.initialHash,
    );
  }
}

export function getPwd(path: string) {
  return WorkingDirectory.create(path);
}
