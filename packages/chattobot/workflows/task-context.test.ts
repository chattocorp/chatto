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

test('only requested answers and unreported terminal results wake the owner', async () => {
  const messages = [
    { type: 'task.progress', task: { id: 'implementation' } },
    { type: 'task.tool_failed', task: { id: 'implementation' } },
    { type: 'task.reply', task: { id: 'implementation' } },
    {
      type: 'task.completed',
      task: {
        name: 'Chatto implementation',
        result: JSON.stringify({ outcome: 'blocked', noticeDelivered: true })
      }
    },
    {
      type: 'task.completed',
      task: { name: 'Chatto implementation', result: JSON.stringify({ outcome: 'completed' }) }
    },
    { type: 'task.failed', task: { name: 'Chatto investigation' } }
  ];
  async function* source() {
    for (const message of messages) yield JSON.stringify(message);
  }
  const received: unknown[] = [];
  for await (const message of userFacingTaskNotifications(source()))
    received.push(JSON.parse(message));
  expect(received).toEqual(messages.slice(2, 3).concat(messages.slice(4)));
});
