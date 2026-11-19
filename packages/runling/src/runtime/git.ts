import { requireDirectory } from './directory.ts';
import { createReadStream } from 'node:fs';
import { lstat, readlink, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { resolve, join } from 'node:path';
import { createHash, type Hash } from 'node:crypto';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { usePrivateTempDirectory } from './private-temp.ts';
import type { TaskContext } from './context.ts';

const exec = promisify(execFile);

async function git(cwd: string, args: string[]) {
  const { stdout } = await exec('git', args, {
    cwd,
    encoding: 'buffer',
    maxBuffer: 128 * 1024 * 1024
  });
  return stdout;
}

async function hasHead(cwd: string) {
  try {
    await git(cwd, ['rev-parse', '--verify', 'HEAD']);
    return true;
  } catch {
    return false;
  }
}

function updateField(hasher: Hash, value: string | ArrayBuffer | Uint8Array) {
  const bytes = typeof value === 'string' ? new TextEncoder().encode(value) : value;
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
  const hasher = createHash('sha256');
  if (await hasHead(cwd)) {
    // The diff alone is empty at every clean commit. Include its base tree,
    // but not commit metadata, so committed file changes remain visible.
    updateField(hasher, await git(cwd, ['rev-parse', 'HEAD^{tree}']));
    updateField(hasher, await git(cwd, ['diff', '--binary', 'HEAD']));
  } else {
    updateField(hasher, await git(cwd, ['diff', '--binary', '--cached']));
    updateField(hasher, await git(cwd, ['diff', '--binary']));
  }

  const untrackedFiles = new TextDecoder()
    .decode(await git(cwd, ['ls-files', '--others', '--exclude-standard', '-z']))
    .split('\0')
    .filter(Boolean)
    .sort();

  for (const path of untrackedFiles) {
    await updateUntrackedFile(hasher, cwd, path);
  }

  return hasher.digest('hex');
}

export class WorkingDirectory {
  private constructor(
    readonly path: string,
    private readonly initialHash: string
  ) {}

  static async create(path: string) {
    return new WorkingDirectory(path, await workingTreeHash(path));
  }

  get hasChanges(): Promise<boolean> {
    return workingTreeHash(this.path).then((currentHash) => currentHash !== this.initialHash);
  }
}

export function getPwd(path: string) {
  return WorkingDirectory.create(path);
}

/** A private NUL-separated list of current worktree files. The caller owns its lifetime,
 * for example until containers using this file have stopped. It does not snapshot file data. */
export interface WorkingTreeManifest extends AsyncDisposable {
  readonly path: string;
}

/** Select tracked and unignored files, including local changes, without Git metadata.
 * Deleted files and submodule directories are excluded. Tracked files remain included even
 * when an ignore rule matches. Disposal removes the private manifest and its directory. */
export async function workingTreeManifest(
  ctx: Pick<TaskContext, 'signal'>,
  cwd: string
): Promise<WorkingTreeManifest> {
  requireDirectory(cwd);
  ctx.signal.throwIfAborted();
  let stdout: string;
  try {
    ({ stdout } = await exec(
      'git',
      ['ls-files', '--cached', '--others', '--exclude-standard', '-z'],
      {
        cwd,
        signal: ctx.signal,
        encoding: 'utf8',
        maxBuffer: 16 * 1024 * 1024
      }
    ));
  } catch (error) {
    ctx.signal.throwIfAborted();
    throw error;
  }
  const files: string[] = [];
  for (const file of new Set(stdout.split('\0').filter(Boolean))) {
    ctx.signal.throwIfAborted();
    try {
      if (!(await lstat(join(cwd, file))).isDirectory()) files.push(file);
    } catch (error) {
      if (!(error instanceof Error && 'code' in error && error.code === 'ENOENT')) throw error;
    }
  }
  const directory = await mkdtemp(join(await usePrivateTempDirectory(), 'git-manifest-'));
  try {
    const path = join(directory, 'files');
    await writeFile(path, files.map((file) => `${file}\0`).join(''), { mode: 0o600 });
    ctx.signal.throwIfAborted();
    return { path, [Symbol.asyncDispose]: () => rm(directory, { recursive: true, force: true }) };
  } catch (error) {
    await rm(directory, { recursive: true, force: true });
    throw error;
  }
}
