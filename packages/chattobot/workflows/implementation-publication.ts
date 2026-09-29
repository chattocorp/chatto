/** Host-owned publication: commit, push, create the pull request, and verify it on GitHub. */
import { writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import type { WorkflowContext } from 'runling';
import type { AgentTaskUpdate } from 'runling/agents';
import type { ImplementationProcess } from './implementation-process.ts';
import type { ImplementationMetadata } from './implementation-artifacts.ts';
import type { PullRequest } from './implementation-tools.ts';
import type { Check, Git } from './implementation-validation.ts';

/** Everything publication needs from a validated implementation. */
export interface PublicationContext {
  ctx: WorkflowContext<string, AgentTaskUpdate>;
  signal: AbortSignal;
  execute: ImplementationProcess;
  git: Git;
  /** Throws unless origin fetch and push URLs match the configured repository. */
  verifyRemote: (cwd: string) => Promise<void>;
  folder: string;
  worktree: string;
  repository: string;
  branch: string;
  baseBranch: string;
  proposal: PullRequest;
  checks: Map<string, Check>;
  metadata: ImplementationMetadata;
  save: () => Promise<void>;
}

/** Publish a validated change. Returns `unknown` when GitHub cannot confirm the expected open PR;
 * a branch or PR may exist then, so callers must not retry automatically. */
export async function publishPullRequest({
  ctx,
  signal,
  execute,
  git,
  verifyRemote,
  folder,
  worktree,
  repository,
  branch,
  baseBranch,
  proposal,
  checks,
  metadata,
  save
}: PublicationContext): Promise<'published' | 'unknown'> {
  await git(worktree, ['diff', '--cached', '--check']);
  await verifyRemote(worktree);
  const body = [
    '## Changes',
    `- ${proposal.summary.replace(/\n/g, '\n  ')}`,
    '',
    '## Verification',
    ...[...checks.values()].map(
      (check) => `- Passed locally: ${check.command.replace(/\r?\n/g, ' ')}`
    ),
    '- Tests run in CI on this pull request.',
    '',
    '## Notes',
    ...(proposal.notes.length
      ? proposal.notes.map((note) => `- ${note}`)
      : ['- No additional limitations reported by the implementation agent.']),
    '- Created by ChattoBot. Review the diff and CI results before merging.'
  ].join('\n');
  const bodyFile = resolve(folder, 'pull-request.md');
  await writeFile(bodyFile, body, { mode: 0o600 });
  await git(worktree, ['-c', 'commit.gpgsign=false', 'commit', '-m', proposal.title]);
  metadata.commit = (await git(worktree, ['rev-parse', 'HEAD'])).trim();
  metadata.stage = 'publishing';
  await ctx.emit({
    type: 'state',
    value: { phase: 'publishing', completedChecks: [...checks.keys()], pendingChecks: [] }
  });
  await save();
  await ctx.emit({
    type: 'finding',
    text: 'The implementation, typecheck, and lint are complete. Publishing the branch and pull request.'
  });
  try {
    await git(worktree, ['push', 'origin', `HEAD:refs/heads/${branch}`]);
    metadata.stage = 'pushed';
    await save();
    await execute(
      'gh',
      [
        'pr',
        'create',
        '--repo',
        repository,
        '--head',
        branch,
        '--base',
        baseBranch,
        '--title',
        proposal.title,
        '--body-file',
        bodyFile
      ],
      { cwd: worktree, signal }
    );
  } catch {
    // A request can succeed remotely but lose its response. Read back the PR
    // instead of creating another one or claiming publication did not happen.
  }
  try {
    const published = JSON.parse(
      await execute(
        'gh',
        [
          'pr',
          'view',
          branch,
          '--repo',
          repository,
          '--json',
          'url,headRefName,headRefOid,baseRefName,state'
        ],
        { cwd: worktree, signal: AbortSignal.timeout(15_000) }
      )
    );
    const prefix = `https://github.com/${repository}/pull/`;
    if (
      typeof published.url !== 'string' ||
      !published.url.toLowerCase().startsWith(prefix.toLowerCase()) ||
      !/^\d+$/.test(published.url.slice(prefix.length)) ||
      published.headRefName !== branch ||
      published.headRefOid !== metadata.commit ||
      published.baseRefName !== baseBranch ||
      published.state !== 'OPEN'
    )
      throw new Error('PR verification failed');
    metadata.prUrl = published.url;
    metadata.stage = 'published';
    await ctx.emit({ type: 'finding', text: `Opened the pull request: ${published.url}` });
    await ctx.emit({
      type: 'state',
      value: {
        phase: 'published',
        prUrl: published.url as string,
        completedChecks: [...checks.keys()],
        pendingChecks: []
      }
    });
    await save();
  } catch {
    metadata.stage = 'publication_unknown';
    await ctx.emit({ type: 'state', value: { phase: 'publication_unknown' } });
    await save();
    return 'unknown';
  }
  return 'published';
}
