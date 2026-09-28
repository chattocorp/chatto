import { expect, test } from 'vitest';
import {
  implementationResultMessage,
  milestoneMessage,
  ONCE_MILESTONES
} from './implementation-messages.ts';
import { workerChatText } from './implementation-safety.ts';

test('each core stage has a milestone message; other phases have none', () => {
  const prUrl = 'https://github.com/example/chatto/pull/7';
  expect(milestoneMessage({ phase: 'validating' })).toContain('running typecheck and lint');
  expect(milestoneMessage({ phase: 'published', prUrl })).toContain(
    `Opened the pull request: ${prUrl}`
  );
  expect(
    milestoneMessage({
      phase: 'ci_repairing',
      attempt: 2,
      failedChecks: ['a', 'b', 'c', 'd', 'e', 'f', 'g']
    })
  ).toBe('CI failed: `a`, `b`, `c`, `d`, `e` and 2 more. I am working on a fix (attempt 2 of 3).');
  expect(milestoneMessage({ phase: 'ci_rerun_pending', failedChecks: ['license-check'] })).toBe(
    'The CI failure in `license-check` looks unrelated to the change, for example a flaky test or an outage. I will rerun the failed jobs when the current CI run finishes.'
  );
  expect(milestoneMessage({ phase: 'ci_rerunning', failedChecks: ['license-check'] })).toBe(
    'Rerunning the failed CI jobs: `license-check`.'
  );
  expect(milestoneMessage({ phase: 'ci_fix_pushed' })).toContain('CI is running again');
  for (const phase of ['setup', 'editing', 'ci_waiting', 'publishing', 'repairing'])
    expect(milestoneMessage({ phase })).toBeUndefined();
  expect([...ONCE_MILESTONES]).toEqual(['validating']);
});

test('the final message shows the plain PR URL, the change summary, and the notes', () => {
  const message = implementationResultMessage({
    outcome: 'completed',
    summary: 'Expand /shrug when sending.\n\n\n\nDocument it at /Users/someone/secret.',
    notes: ['Browser review\nwas not done.'],
    prUrl: 'https://github.com/example/chatto/pull/7',
    worktree: '/work/tree',
    checks: [],
    workerChecks: [],
    ci: { status: 'passed', passed: 21, failed: 0, pending: 0, skipped: 2, repairs: 0 }
  });
  expect(message).toBe(
    [
      '**CI passed** for the pull request (21 checks passed): https://github.com/example/chatto/pull/7',
      '',
      '**What changed**',
      'Expand /shrug when sending.',
      '',
      'Document it at [host path]',
      '',
      '**Notes**',
      '- Browser review was not done.'
    ].join('\n')
  );
});

test('worker text without a worktree is not mangled', () => {
  expect(workerChatText('Plain text', '', 100)).toBe('Plain text');
  expect(workerChatText('x'.repeat(20), '', 10)).toBe(`${'x'.repeat(10)}…`);
});
