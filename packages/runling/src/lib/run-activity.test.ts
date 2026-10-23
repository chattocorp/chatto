import { expect, test } from 'vitest';
import { emptyTokenUsage, type RunlingEvent } from 'runling';
import { isRunWaiting, summarizeRunActivity } from './run-activity.ts';
import type { RunDetail } from './runs.ts';

function run(events: RunlingEvent[]): RunDetail {
  return {
    id: 'run',
    webhook: 'test',
    workflow: 'Test',
    source: 'web',
    status: 'running',
    startedAt: 1000,
    input: 'private input',
    output: null,
    error: null,
    usage: emptyTokenUsage(),
    events
  };
}

const events: RunlingEvent[] = [
  { type: 'step.started', id: 'parent', label: 'Review change', timestamp: 0 },
  {
    type: 'agent.started',
    agentId: 'a',
    model: 'Model A',
    color: 'blue',
    activityId: 'parent',
    timestamp: 10
  },
  {
    type: 'agent.started',
    agentId: 'b',
    model: 'Model B',
    color: 'blue',
    activityId: 'parent',
    timestamp: 20
  },
  { type: 'agent.progress', agentId: 'a', text: 'Reading the tests', timestamp: 30 }
];

test('shows the latest agent update with its step and parallel activity count', () => {
  expect(summarizeRunActivity(run(events))).toEqual({
    label: 'Model A',
    step: 'Review change',
    preview: 'Reading the tests',
    startedAt: 10,
    waiting: false,
    waits: 0,
    parallel: 1
  });
  expect(
    summarizeRunActivity(
      run([
        ...events,
        { type: 'agent.progress', agentId: 'b', text: 'Checking types', timestamp: 40 }
      ])
    )?.preview
  ).toBe('Checking types');
});

test('gives waits priority over agent updates', () => {
  const activity = summarizeRunActivity(
    run([
      ...events,
      { type: 'wait.started', id: 'wait', channelId: 'c', activityId: 'parent', timestamp: 40 },
      { type: 'agent.progress', agentId: 'b', text: 'Still checking', timestamp: 50 }
    ])
  );
  expect(activity).toMatchObject({
    label: 'Waiting for a message',
    step: 'Review change',
    startedAt: 40,
    waiting: true,
    parallel: 2
  });
  expect(isRunWaiting(activity)).toBe(false);
});

test('a run waits when its only running work is a wait', () => {
  const history: RunlingEvent[] = [
    { type: 'step.started', id: 'chat', label: 'Chat', timestamp: 0 },
    {
      type: 'agent.started',
      agentId: 'bot',
      model: 'test',
      color: 'blue',
      activityId: 'chat',
      timestamp: 2
    },
    {
      type: 'agent.finished',
      agentId: 'bot',
      outcome: 'completed',
      usage: emptyTokenUsage(),
      activityId: 'chat',
      timestamp: 3
    },
    { type: 'wait.started', id: 'next', channelId: 'c', activityId: 'chat', timestamp: 4 }
  ];
  const waiting = summarizeRunActivity(run(history));
  expect(waiting).toMatchObject({
    label: 'Waiting for a message',
    step: 'Chat',
    waiting: true,
    waits: 1,
    parallel: 0
  });
  expect(isRunWaiting(waiting)).toBe(true);

  history.push({ type: 'step.started', id: 'other', label: 'Parallel work', timestamp: 5 });
  expect(isRunWaiting(summarizeRunActivity(run(history)))).toBe(false);
  history.push({
    type: 'step.finished',
    id: 'other',
    status: 'completed',
    durationMs: 1,
    timestamp: 6
  });
  expect(isRunWaiting(summarizeRunActivity(run(history)))).toBe(true);
  history.push({
    type: 'wait.finished',
    id: 'next',
    status: 'received',
    durationMs: 3,
    activityId: 'chat',
    timestamp: 7
  });
  expect(isRunWaiting(summarizeRunActivity(run(history)))).toBe(false);
});

test('handles step-only work, startup, and finished runs', () => {
  expect(summarizeRunActivity(run([]))).toBeNull();
  expect(summarizeRunActivity(run([events[0]!]))).toMatchObject({
    label: 'Review change',
    waiting: false
  });
  expect(summarizeRunActivity({ ...run(events), status: 'completed' })).toBeNull();
});

test('moves on after an activity finishes and limits long messages', () => {
  const result = summarizeRunActivity(
    run([
      ...events,
      {
        type: 'agent.finished',
        agentId: 'a',
        outcome: 'completed',
        usage: emptyTokenUsage(),
        timestamp: 40
      },
      { type: 'agent.progress', agentId: 'b', text: 'x'.repeat(1000), timestamp: 50 }
    ])
  );
  expect(result?.label).toBe('Model B');
  expect(result?.parallel).toBe(0);
  expect(result?.preview).toHaveLength(500);
});

test('counts each task that waits until its wait finishes', () => {
  const history: RunlingEvent[] = [
    ...events,
    { type: 'wait.started', id: 'first', channelId: 'a', activityId: 'parent', timestamp: 40 },
    { type: 'wait.started', id: 'second', channelId: 'b', activityId: 'parent', timestamp: 50 }
  ];
  expect(summarizeRunActivity(run(history))).toMatchObject({
    waiting: true,
    waits: 2,
    parallel: 2
  });
  history.push({
    type: 'wait.finished',
    id: 'first',
    status: 'received',
    durationMs: 20,
    timestamp: 60
  });
  expect(summarizeRunActivity(run(history))).toMatchObject({ waiting: true, waits: 1 });
  history.push({
    type: 'wait.finished',
    id: 'second',
    status: 'closed',
    durationMs: 20,
    timestamp: 70
  });
  expect(summarizeRunActivity(run(history))).toMatchObject({ waiting: false, waits: 0 });
});

test('terminal runs never show a wait as waiting', () => {
  const waiting = run([{ type: 'wait.started', id: 'wait', channelId: 'c', timestamp: 0 }]);
  expect(summarizeRunActivity(waiting)?.waiting).toBe(true);
  for (const status of ['completed', 'failed', 'interrupted'] as const) {
    expect(summarizeRunActivity({ ...waiting, status })).toBeNull();
  }
});
