import { expect, test, vi } from 'vitest';
import { observePullRequestChecks } from './implementation-ci.ts';
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
  expect(result).toEqual({ status: 'passed', passed: 1, failed: 0, pending: 0, skipped: 1 });
  expect(execute).toHaveBeenCalledWith(
    'gh',
    ['pr', 'checks', common.prUrl, '--repo', common.repository, '--json', 'bucket'],
    expect.objectContaining({ cwd: common.cwd })
  );
});

test('reports failed and cancelled checks without exposing check output', async () => {
  const execute = vi.fn<ImplementationProcess>(async (_command, args) => {
    if (args[1] === 'view') return JSON.stringify({ headRefOid: common.headCommit });
    throw new ImplementationCommandError(
      'failed',
      '[{"bucket":"fail"},{"bucket":"cancel"}]\nprivate output'
    );
  });
  expect(await observePullRequestChecks({ ...common, execute })).toEqual({
    status: 'failed',
    passed: 0,
    failed: 2,
    pending: 0,
    skipped: 0
  });
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
    skipped: 1
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
