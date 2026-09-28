import { expect, test } from 'vitest';
import { taskContext, taskNotification, userFacingTaskNotifications } from './task-context.ts';

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

test('answers, notices, and unposted terminal results wake the owner; posted results do not', async () => {
  const result = (outcome: string) =>
    JSON.stringify({ outcome, summary: 'Done.', checks: [], workerChecks: [] });
  const messages = [
    { type: 'task.progress', task: { id: 'implementation' } },
    { type: 'task.tool_failed', task: { id: 'implementation' } },
    { type: 'task.reply', task: { id: 'implementation' } },
    { type: 'task.notice', text: 'Tests pass.', task: { id: 'implementation' } },
    { type: 'task.completed', task: { name: 'Chatto implementation', result: result('blocked') } },
    { type: 'task.completed', task: { name: 'Chatto implementation', result: '{"odd":true}' } },
    { type: 'task.failed', task: { name: 'Chatto investigation' } }
  ];
  async function* source() {
    for (const message of messages) yield JSON.stringify(message);
  }
  const posted: string[] = [];
  const received: unknown[] = [];
  for await (const message of userFacingTaskNotifications(source(), {
    postResult: async (text) => {
      posted.push(text);
    }
  }))
    received.push(JSON.parse(message));
  expect(posted).toEqual([
    'The implementation stopped: Done. Please tell me how you want to proceed.'
  ]);
  // The owner does not see the posted result; an unreadable result still reaches it.
  expect(received).toEqual([messages[2], messages[3], messages[5], messages[6]]);
});

test('an implementation result that cannot be posted wakes the owner instead', async () => {
  const message = {
    type: 'task.completed',
    task: {
      name: 'Chatto implementation',
      result: JSON.stringify({ outcome: 'blocked', summary: 'x', checks: [], workerChecks: [] })
    }
  };
  async function* source() {
    yield JSON.stringify(message);
  }
  const received: unknown[] = [];
  for await (const notice of userFacingTaskNotifications(source(), {
    postResult: async () => {
      throw new Error('Chat post failed');
    }
  }))
    received.push(JSON.parse(notice));
  expect(received).toEqual([message]);
});

test('a notice notification carries its text for the owner', () => {
  expect(
    JSON.parse(
      taskNotification(
        JSON.stringify({ type: 'task.notice', text: 'Tests pass.', task: { id: 'worker' } })
      )
    )
  ).toEqual({ type: 'task.notice', taskId: 'worker', text: 'Tests pass.' });
});
