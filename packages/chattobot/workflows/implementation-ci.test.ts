import { expect, test, vi } from 'vitest';
import { observePullRequestChecks } from './implementation-ci.ts';
import {
  ImplementationCommandError,
  type ImplementationProcess
} from './implementation-process.ts';

const common = {
  repository: 'example/chatto',
  prUrl: 'https://github.com/example/chatto/pull/7',
  cwd: '/unused',
  signal: new AbortController().signal,
  intervalMs: 1,
  timeoutMs: 50
};

test('waits for pending checks and reads JSON even when gh exits nonzero', async () => {
  const execute = vi
    .fn<ImplementationProcess>()
    .mockRejectedValueOnce(new ImplementationCommandError('pending', '[{"bucket":"pending"}]\n'))
    .mockResolvedValueOnce('[{"bucket":"pass"},{"bucket":"skipping"}]');
  const result = await observePullRequestChecks({ ...common, execute });
  expect(result).toEqual({ status: 'passed', passed: 1, failed: 0, pending: 0, skipped: 1 });
  expect(execute).toHaveBeenCalledWith(
    'gh',
    ['pr', 'checks', common.prUrl, '--repo', common.repository, '--json', 'bucket'],
    expect.objectContaining({ cwd: common.cwd })
  );
});

test('reports failed and cancelled checks without exposing check output', async () => {
  const execute = vi
    .fn<ImplementationProcess>()
    .mockRejectedValue(
      new ImplementationCommandError(
        'failed',
        '[{"bucket":"fail"},{"bucket":"cancel"}]\nprivate output'
      )
    );
  expect(await observePullRequestChecks({ ...common, execute })).toEqual({
    status: 'failed',
    passed: 0,
    failed: 2,
    pending: 0,
    skipped: 0
  });
});

test('does not claim CI passed when every reported check was skipped', async () => {
  const execute = vi.fn<ImplementationProcess>().mockResolvedValue('[{"bucket":"skipping"}]');
  expect(await observePullRequestChecks({ ...common, execute })).toEqual({
    status: 'skipped',
    passed: 0,
    failed: 0,
    pending: 0,
    skipped: 1
  });
});

test('bounds pending and unavailable check observation', async () => {
  const pending = vi.fn<ImplementationProcess>().mockResolvedValue('[{"bucket":"pending"}]');
  expect(
    (await observePullRequestChecks({ ...common, execute: pending, timeoutMs: 3 })).status
  ).toBe('pending');
  const unavailable = vi.fn<ImplementationProcess>().mockResolvedValue('private invalid response');
  expect((await observePullRequestChecks({ ...common, execute: unavailable })).status).toBe(
    'unavailable'
  );
  expect(unavailable).toHaveBeenCalledTimes(3);
});
