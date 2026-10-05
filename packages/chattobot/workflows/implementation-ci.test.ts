import { expect, test, vi } from 'vitest';
import { failedJobLog, observePullRequestChecks, rerunFailedJobs } from './implementation-ci.ts';
import {
  ImplementationCommandError,
  type ImplementationProcess
} from './implementation-process.ts';

const common = {
  repository: 'example/chatto',
  prUrl: 'https://github.com/example/chatto/pull/7',
  headCommit: 'abc123',
  cwd: '/unused',
  signal: new AbortController().signal,
  intervalMs: 1,
  timeoutMs: 50
};

test('waits for pending checks and reads JSON even when gh exits nonzero', async () => {
  let checks = 0;
  const execute = vi.fn<ImplementationProcess>(async (_command, args) => {
    if (args[1] === 'view') return JSON.stringify({ headRefOid: common.headCommit });
    if (++checks === 1) throw new ImplementationCommandError('pending', '[{"bucket":"pending"}]\n');
    return '[{"bucket":"pass"},{"bucket":"skipping"}]';
  });
  const result = await observePullRequestChecks({ ...common, execute });
  expect(result).toEqual({
    status: 'passed',
    passed: 1,
    failed: 0,
    pending: 0,
    skipped: 1,
    failures: []
  });
  expect(execute).toHaveBeenCalledWith(
    'gh',
    ['pr', 'checks', common.prUrl, '--repo', common.repository, '--json', 'bucket,name,link'],
    expect.objectContaining({ cwd: common.cwd })
  );
});

test('reports failed and cancelled checks with their names and links, not their output', async () => {
  const execute = vi.fn<ImplementationProcess>(async (_command, args) => {
    if (args[1] === 'view') return JSON.stringify({ headRefOid: common.headCommit });
    throw new ImplementationCommandError(
      'failed',
      '[{"bucket":"fail","name":"test [1/2]","link":"https://example.invalid/1"},{"bucket":"cancel"}]\nprivate output]'
    );
  });
  expect(await observePullRequestChecks({ ...common, execute })).toEqual({
    status: 'failed',
    passed: 0,
    failed: 2,
    pending: 0,
    skipped: 0,
    failures: [
      { name: 'test [1/2]', link: 'https://example.invalid/1' },
      { name: 'Unnamed check', link: '' }
    ]
  });
});

test('stopOnFailure returns the first failure while other checks are pending', async () => {
  const execute = vi.fn<ImplementationProcess>(async (_command, args) =>
    args[1] === 'view'
      ? JSON.stringify({ headRefOid: common.headCommit })
      : '[{"bucket":"pending"},{"bucket":"fail","name":"lint","link":""}]'
  );
  expect(await observePullRequestChecks({ ...common, execute, stopOnFailure: true })).toMatchObject(
    { status: 'failed', failed: 1, pending: 1 }
  );
  expect((await observePullRequestChecks({ ...common, execute, timeoutMs: 3 })).status).toBe(
    'pending'
  );
});

test('known failures do not end observation early; a new failure does', async () => {
  let polls = 0;
  const execute = vi.fn<ImplementationProcess>(async (_command, args) => {
    if (args[1] === 'view') return JSON.stringify({ headRefOid: common.headCommit });
    return ++polls < 3
      ? '[{"bucket":"pending"},{"bucket":"fail","name":"lint","link":"known"}]'
      : '[{"bucket":"pending"},{"bucket":"fail","name":"lint","link":"known"},{"bucket":"fail","name":"e2e","link":"new"}]';
  });
  const result = await observePullRequestChecks({
    ...common,
    execute,
    stopOnFailure: true,
    knownFailures: new Set(['known'])
  });
  expect(polls).toBe(3);
  expect(result).toMatchObject({ status: 'failed', failed: 2, pending: 1 });
});

test('failed job logs keep the failure and drop timestamps, runner paths, and cleanup', async () => {
  const log = [
    '2026-07-13T23:39:40.1Z Run pnpm test',
    '2026-07-13T23:39:47.0Z FAIL /home/runner/work/chatto/chatto/apps/frontend/src/a.spec.ts',
    '2026-07-13T23:39:47.2Z ##[error]Process completed with exit code 1.',
    '2026-07-13T23:39:48.5Z Cleaning up orphan processes'
  ].join('\n');
  const execute = vi.fn<ImplementationProcess>(async () => log);
  const access = { execute, repository: 'example/chatto', cwd: '/unused', signal: common.signal };
  const text = await failedJobLog(
    access,
    { name: 'test', link: 'https://github.com/example/chatto/actions/runs/12/job/34' },
    '/unused'
  );
  expect(text).toBe(
    'Run pnpm test\nFAIL apps/frontend/src/a.spec.ts\n##[error]Process completed with exit code 1.'
  );
  expect(execute).toHaveBeenCalledWith(
    'gh',
    ['api', '--allow-escape-sequences', 'repos/example/chatto/actions/jobs/34/logs'],
    expect.objectContaining({ keepTail: true })
  );
  // Links outside the configured repository's Actions are never fetched.
  expect(
    await failedJobLog(
      access,
      { name: 'x', link: 'https://github.com/other/repo/actions/runs/1/job/2' },
      '/unused'
    )
  ).toBe('No GitHub Actions log is available for this check.');
  expect(execute).toHaveBeenCalledOnce();
});

test('reruns each failed workflow run once', async () => {
  const execute = vi.fn<ImplementationProcess>(async () => '');
  const access = { execute, repository: 'example/chatto', cwd: '/unused', signal: common.signal };
  await rerunFailedJobs(access, [
    { name: 'a', link: 'https://github.com/example/chatto/actions/runs/12/job/1' },
    { name: 'b', link: 'https://github.com/example/chatto/actions/runs/12/job/2' },
    { name: 'c', link: '' }
  ]);
  expect(execute).toHaveBeenCalledExactlyOnceWith(
    'gh',
    ['run', 'rerun', '12', '--failed', '--repo', 'example/chatto'],
    expect.anything()
  );
  await expect(rerunFailedJobs(access, [{ name: 'c', link: '' }])).rejects.toThrow();
});

test('does not claim CI passed when every reported check was skipped', async () => {
  const execute = vi.fn<ImplementationProcess>(async (_command, args) =>
    args[1] === 'view'
      ? JSON.stringify({ headRefOid: common.headCommit })
      : '[{"bucket":"skipping"}]'
  );
  expect(await observePullRequestChecks({ ...common, execute })).toEqual({
    status: 'skipped',
    passed: 0,
    failed: 0,
    pending: 0,
    skipped: 1,
    failures: []
  });
});

test('bounds pending and unavailable check observation', async () => {
  const pending = vi.fn<ImplementationProcess>(async (_command, args) =>
    args[1] === 'view'
      ? JSON.stringify({ headRefOid: common.headCommit })
      : '[{"bucket":"pending"}]'
  );
  expect(
    (await observePullRequestChecks({ ...common, execute: pending, timeoutMs: 3 })).status
  ).toBe('pending');
  const unavailable = vi.fn<ImplementationProcess>(async (_command, args) =>
    args[1] === 'view'
      ? JSON.stringify({ headRefOid: common.headCommit })
      : 'private invalid response'
  );
  expect((await observePullRequestChecks({ ...common, execute: unavailable })).status).toBe(
    'unavailable'
  );
  expect(unavailable).toHaveBeenCalledTimes(6);
});

test('waits when GitHub has not attached checks yet', async () => {
  let checks = 0;
  const execute = vi.fn<ImplementationProcess>(async (_command, args) => {
    if (args[1] === 'view') return JSON.stringify({ headRefOid: common.headCommit });
    if (++checks === 1)
      throw new ImplementationCommandError('No checks', 'no checks reported on the pull request');
    return '[{"bucket":"pass"}]';
  });
  expect((await observePullRequestChecks({ ...common, execute })).status).toBe('passed');
  expect(checks).toBe(2);
});

test('stops reporting CI when the PR head changes', async () => {
  const execute = vi
    .fn<ImplementationProcess>()
    .mockResolvedValue(JSON.stringify({ headRefOid: 'different-commit' }));
  expect((await observePullRequestChecks({ ...common, execute })).status).toBe('head_changed');
  expect(execute).toHaveBeenCalledTimes(1);
});
