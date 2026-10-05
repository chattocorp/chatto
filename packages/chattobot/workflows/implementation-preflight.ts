/** Implementation preflight: host Git access, the configured origin, GitHub CLI authentication,
 * the base commit, and a retained worktree to continue. Nothing is created before it passes. */
import { realpath } from 'node:fs/promises';
import { resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { HostCommandError, type ImplementationProcess } from './implementation-process.ts';
import { loadResumableArtifact, type ImplementationMetadata } from './implementation-artifacts.ts';
import { matchesRepository } from './implementation-settings.ts';
import type { Git } from './implementation-validation.ts';

/** Run git without hooks. Another process, such as an editor that runs `git status` on the
 * worktree, can hold its index lock for a moment; retry then. */
export function createGit(execute: ImplementationProcess, signal: AbortSignal): Git {
  return async (cwd, args, commandSignal = signal) => {
    for (let attempt = 1; ; attempt++) {
      try {
        return await execute('git', ['-c', 'core.hooksPath=/dev/null', ...args], {
          cwd,
          signal: commandSignal
        });
      } catch (error) {
        if (!(error instanceof HostCommandError && error.lockHeld) || attempt === 5) throw error;
        await delay(200 * attempt, undefined, { signal: commandSignal });
      }
    }
  };
}

/** Throw unless the origin fetch and push URLs in `cwd` match `repository`. */
export function createRemoteCheck(git: Git, repository: string) {
  return async (cwd: string) => {
    const urls = await git(cwd, ['remote', 'get-url', '--all', 'origin']);
    const pushUrls = await git(cwd, ['remote', 'get-url', '--push', '--all', 'origin']);
    if (
      ![urls, pushUrls].every(
        (value) => value.trim().split('\n').length === 1 && matchesRepository(value, repository)
      )
    )
      throw new Error('Origin must match the configured GitHub repository');
  };
}

/** What preflight needs from the implementation. */
export interface PreflightContext {
  signal: AbortSignal;
  execute: ImplementationProcess;
  git: Git;
  verifyRemote: (cwd: string) => Promise<void>;
  /** The original checkout. */
  directory: string;
  artifacts: string;
  repository: string;
  baseBranch: string;
  ownerKey?: string;
  resumeArtifactId?: string;
}

/** A passed preflight: the branch and base commit to use, and the retained artifact to
 * continue, if any. A failed one names the obstacle in host-written words. */
export type Preflight =
  | {
      ok: true;
      branch: string;
      baseCommit: string;
      resumed?: { folder: string; metadata: ImplementationMetadata };
    }
  | { ok: false; branch: string; obstacle: string };

/** Check everything that must hold before an implementation creates or reuses a worktree. */
export async function preflight(context: PreflightContext, branch: string): Promise<Preflight> {
  const { signal, execute, git, verifyRemote, directory, baseBranch } = context;
  let resumed: { folder: string; metadata: ImplementationMetadata } | undefined;
  let obstacle = 'The configured implementation base branch is invalid.';
  try {
    if (context.resumeArtifactId) {
      obstacle =
        'That implementation artifact is not available for continuation in this conversation. Check its ID or start a new request.';
      resumed = await loadResumableArtifact(context.artifacts, context.resumeArtifactId, {
        ownerKey: context.ownerKey,
        repository: context.repository,
        baseBranch
      });
      branch = resumed.metadata.branch;
    }
    obstacle = 'The configured implementation base branch is invalid.';
    await git(directory, ['check-ref-format', '--branch', baseBranch]);
    obstacle = 'Could not verify that origin matches the configured GitHub repository.';
    await verifyRemote(directory);
    obstacle =
      'The GitHub CLI authentication check failed. Check gh authentication on the bot host.';
    await execute('gh', ['auth', 'status', '--hostname', 'github.com'], {
      cwd: directory,
      signal
    });
    obstacle =
      'Could not fetch the configured base branch. Set CHATTO_SOURCE_REF to a branch on origin and check Git access.';
    await git(directory, [
      'fetch',
      '--no-tags',
      'origin',
      `refs/heads/${baseBranch}:refs/remotes/origin/${baseBranch}`
    ]);
    const baseCommit =
      resumed?.metadata.baseCommit ??
      (
        await git(directory, [
          'rev-parse',
          '--verify',
          `refs/remotes/origin/${baseBranch}^{commit}`
        ])
      ).trim();
    if (resumed) {
      obstacle =
        'The saved implementation worktree or branch could not be verified. Review it locally before starting again.';
      const savedWorktree = resolve(resumed.folder, 'worktree');
      if (
        (await realpath((await git(savedWorktree, ['rev-parse', '--show-toplevel'])).trim())) !==
          (await realpath(savedWorktree)) ||
        (await git(savedWorktree, ['branch', '--show-current'])).trim() !== branch ||
        (await git(savedWorktree, ['rev-parse', 'HEAD'])).trim() !== baseCommit
      )
        throw new Error('Saved worktree mismatch');
      await verifyRemote(savedWorktree);
    }
    return { ok: true, branch, baseCommit, ...(resumed ? { resumed } : {}) };
  } catch {
    signal.throwIfAborted();
    // Return only host-owned explanations, never subprocess output or credentials.
    return { ok: false, branch, obstacle };
  }
}
