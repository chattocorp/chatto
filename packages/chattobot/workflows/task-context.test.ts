import { expect, test } from 'vitest';
import {
  notificationUrls,
  taskContext,
  taskNotification,
  userFacingTaskNotifications
} from './task-context.ts';

test('new host phase supersedes an old setup announcement without changing retained history', () => {
  const task = {
    id: 'worker',
    name: 'Implementation',
    status: 'running' as const,
    state: { phase: 'editing' },
    stateAt: 200,
    progress: 'Installing dependencies',
    progressAt: 100,
    output: [],
    droppedOutput: 0
  };
  expect(taskContext([task])[0]).not.toHaveProperty('progress');
  expect(taskContext([task])[0]?.state).toEqual({ phase: 'editing' });
  expect(task.progress).toBe('Installing dependencies');
  expect(taskContext([{ ...task, progressAt: 300 }])[0]?.progress).toBe(task.progress);
});

test('completed blocked results supersede stale active state and notification snapshots', () => {
  const result = JSON.stringify({
    outcome: 'blocked',
    checks: [{ passed: false, diagnostic: 'Expected 1, received 0' }]
  });
  const [current] = taskContext([
    {
      id: 'worker',
      name: 'Implementation',
      status: 'completed',
      state: { phase: 'validating' },
      result,
      output: [],
      droppedOutput: 0
    }
  ]);
  expect(current?.state).toBeUndefined();
  expect(current?.result).toEqual(JSON.parse(result));
  expect(
    JSON.parse(
      taskNotification(
        JSON.stringify({
          type: 'task.progress',
          task: { id: 'worker', state: { phase: 'preparing' } }
        })
      )
    )
  ).toEqual({ type: 'task.progress', taskId: 'worker' });
});

test.each(['Plain task failure', '{"outcome":"blocked"\n[Task result truncated]', ''])(
  'preserves non-JSON results: %s',
  (result) => {
    const task = {
      id: 'worker',
      name: 'Worker',
      status: 'completed' as const,
      result,
      output: [],
      droppedOutput: 0
    };
    expect(taskContext([task])[0]?.result).toBe(result);
    expect(task.result).toBe(result);
  }
);

test('decoded results are detached from the original retained snapshot', () => {
  const task = {
    id: 'worker',
    name: 'Worker',
    status: 'completed' as const,
    result: '{"outcome":"completed","plan":{"goal":"Original"}}',
    output: [],
    droppedOutput: 0
  };
  const result = taskContext([task])[0]!.result as { plan: { goal: string } };
  result.plan.goal = 'Changed';
  expect(JSON.parse(task.result).plan.goal).toBe('Original');
});

test('answers, notices, and terminal results wake the owner; routine updates do not', async () => {
  const messages = [
    { type: 'task.progress', task: { id: 'implementation' } },
    { type: 'task.tool_failed', task: { id: 'implementation' } },
    { type: 'task.reply', task: { id: 'implementation' } },
    { type: 'task.notice', text: 'Tests pass.', task: { id: 'implementation' } },
    { type: 'task.completed', task: { name: 'Chatto implementation', result: '{}' } },
    { type: 'task.failed', task: { name: 'Chatto investigation' } }
  ];
  async function* source() {
    for (const message of messages) yield JSON.stringify(message);
  }
  const received: unknown[] = [];
  for await (const message of userFacingTaskNotifications(source()))
    received.push(JSON.parse(message));
  expect(received).toEqual(messages.slice(2));
});

test('only a new pull request and a final result carry URLs that the user must receive', () => {
  const url = 'https://github.com/example/chatto/pull/7';
  const notice = (data: object) => JSON.stringify({ type: 'task.notice', text: 'x', data });
  expect(notificationUrls(notice({ milestone: 'published', prUrl: url }))).toEqual([url]);
  expect(notificationUrls(notice({ milestone: 'ci_failed', prUrl: url }))).toEqual([]);
  expect(
    notificationUrls(
      JSON.stringify({
        type: 'task.completed',
        task: { result: JSON.stringify({ outcome: 'completed', prUrl: url }) }
      })
    )
  ).toEqual([url]);
  expect(
    notificationUrls(notice({ milestone: 'published', prUrl: 'javascript:alert(1)' }))
  ).toEqual([]);
  expect(notificationUrls('not json')).toEqual([]);
});

test('a notice notification carries its text for the owner', () => {
  expect(
    JSON.parse(
      taskNotification(
        JSON.stringify({ type: 'task.notice', text: 'Tests pass.', task: { id: 'worker' } })
      )
    )
  ).toEqual({
    type: 'task.notice',
    taskId: 'worker',
    text: 'Tests pass.',
    // Progress without a milestone: pass on only what is new.
    report: expect.stringContaining('worker’s progress')
  });
  expect(
    JSON.parse(
      taskNotification(
        JSON.stringify({
          type: 'task.notice',
          text: 'The pull request is open.',
          data: { milestone: 'published', prUrl: 'https://github.com/example/chatto/pull/7' },
          task: { id: 'worker' }
        })
      )
    )
  ).toMatchObject({
    data: { milestone: 'published' },
    report: expect.stringContaining('Include data.prUrl once, exactly as given.')
  });
});

test('a finished implementation wakes the owner with its result and a request for a full report', () => {
  const result = {
    outcome: 'completed',
    summary: 'Expand /shrug when sending.',
    notes: ['Browser review was not done.'],
    prUrl: 'https://github.com/example/chatto/pull/7',
    ci: { status: 'passed', passed: 21 },
    checks: [
      { command: 'check', passed: true },
      { command: 'lint', passed: false }
    ],
    workerChecks: [],
    worktree: '/private/worktree'
  };
  const notification = JSON.parse(
    taskNotification(
      JSON.stringify({
        type: 'task.completed',
        task: {
          id: 'implementation',
          name: 'Chatto implementation',
          result: JSON.stringify(result)
        }
      })
    )
  );
  expect(notification).toMatchObject({
    type: 'task.completed',
    taskId: 'implementation',
    result: {
      outcome: 'completed',
      summary: 'Expand /shrug when sending.',
      notes: ['Browser review was not done.'],
      prUrl: 'https://github.com/example/chatto/pull/7',
      ci: { status: 'passed', passed: 21 },
      failedChecks: ['lint']
    },
    report: expect.stringContaining('Write it in full, not briefly')
  });
  // Host paths stay out of the prompt.
  expect(JSON.stringify(notification)).not.toContain('/private/worktree');
  expect(
    JSON.parse(
      taskNotification(
        JSON.stringify({
          type: 'task.completed',
          task: { id: 'x', name: 'Chatto source investigation' }
        })
      )
    )
  ).toEqual({ type: 'task.completed', taskId: 'x' });
});
