import { expect, test } from 'vitest';
import type { RunlingEvent } from 'runling';
import { activityLogRows, logTime, logTreeRows, nextLogFollow, runLogRows } from './run-log.ts';

test('compact activities retain one stable row through completion and keep fields in details', () => {
  const events: RunlingEvent[] = [
    { type: 'step.started', id: 'read', label: 'Read context', timestamp: 10 },
    {
      type: 'log',
      source: 'step',
      level: 'info',
      message: 'Read context',
      fields: { threadId: 'thread', messages: 2 },
      depth: 0,
      color: 'blue',
      timestamp: 11
    }
  ];
  const before = structuredClone(events);
  const live = activityLogRows(events, 'running');
  expect(live).toHaveLength(1);
  expect(live[0]).toMatchObject({
    id: 0,
    event: { message: 'Read context' },
    summary: { node: { status: 'running' }, fields: { threadId: 'thread' } }
  });
  const finished = activityLogRows(
    [
      ...events,
      { type: 'step.finished', id: 'read', status: 'completed', durationMs: 125, timestamp: 135 }
    ],
    'completed'
  );
  expect(finished).toHaveLength(1);
  expect(finished[0]).toMatchObject({
    id: live[0]!.id,
    event: { level: 'success' },
    summary: { node: { durationMs: 125, status: 'completed' } }
  });
  expect(events).toEqual(before);
  expect(runLogRows(events).map((row) => row.event.message)).toEqual([
    'Started task-1',
    'Read context threadId="thread" messages=2'
  ]);
});

test('compact projection preserves interleaved diagnostics, messages, and failed activity details', () => {
  const events: RunlingEvent[] = [
    { type: 'step.started', id: 'a', label: 'First', timestamp: 0 },
    { type: 'step.started', id: 'b', label: 'Second', activityId: 'a', timestamp: 1 },
    {
      type: 'log',
      source: 'step',
      level: 'info',
      message: 'First',
      depth: 0,
      color: 'blue',
      timestamp: 2
    },
    {
      type: 'log',
      source: 'step',
      level: 'info',
      message: 'Second',
      activityId: 'a',
      depth: 1,
      color: 'blue',
      timestamp: 3
    },
    {
      type: 'log',
      level: 'warn',
      message: 'Retrying',
      fields: { attempt: 2 },
      activityId: 'b',
      depth: 2,
      color: 'gold',
      timestamp: 4
    },
    {
      type: 'log',
      level: 'info',
      message: 'Result is ready',
      depth: 0,
      color: 'blue',
      timestamp: 5
    },
    {
      type: 'step.finished',
      id: 'b',
      status: 'failed',
      error: 'Connection refused',
      durationMs: 5,
      timestamp: 6
    },
    {
      type: 'message.sent',
      id: 'm',
      channelId: 'host',
      direction: 'input',
      payload: 'Continue',
      timestamp: 7
    }
  ];
  const rows = activityLogRows(events, 'running');
  expect(rows.map((row) => row.event.message)).toEqual([
    'First',
    'Second',
    'Retrying',
    'Result is ready',
    'Connection refused',
    'Message to workflow · Continue'
  ]);
  expect(rows[1]!.summary!.logs).toContain('Connection refused');
  expect(rows[1]!.event.depth).toBe(1);
  expect(rows[2]!.event).toMatchObject({ level: 'warn', fields: { attempt: 2 } });
  expect(activityLogRows(events, 'interrupted')[0]!.summary!.node.status).toBe('interrupted');
});

test('agent metadata replaces lifecycle noise but retains replies and errors, including orphan logs', () => {
  const usage = { input: 3, output: 2, cacheRead: 0, cacheWrite: 0, cost: 0.01 };
  const events: RunlingEvent[] = [
    {
      type: 'agent.started',
      agentId: 'agent',
      model: 'provider/model',
      color: 'blue',
      timestamp: 0
    },
    ...[
      'Agent started (model: provider/model)',
      'Here is the answer',
      'Token usage: the explanation you requested'
    ].map((message, index) => ({
      type: 'log' as const,
      source: 'agent' as const,
      sourceId: 'agent',
      level: 'info' as const,
      message: `[agent] ${message}`,
      depth: 1,
      color: 'blue',
      timestamp: index + 1
    })),
    {
      type: 'log',
      source: 'agent',
      sourceId: 'agent',
      level: 'error',
      message: '[agent] Tool failed',
      depth: 1,
      color: 'red',
      timestamp: 4
    },
    {
      type: 'log',
      source: 'agent',
      sourceId: 'agent',
      level: 'info',
      message: '[agent] Token usage: in 3, out 2',
      depth: 1,
      color: 'blue',
      timestamp: 4.5
    },
    { type: 'agent.finished', agentId: 'agent', outcome: 'completed', usage, timestamp: 5 },
    { type: 'execution.finished', timestamp: 5 },
    {
      type: 'log',
      source: 'agent',
      sourceId: 'missing',
      level: 'info',
      message: 'Token usage: legacy',
      depth: 0,
      color: 'blue',
      timestamp: 6
    }
  ];
  const rows = activityLogRows(events, 'completed');
  expect(rows.map((row) => row.event.message)).toEqual([
    'Agent',
    'Here is the answer',
    'Token usage: the explanation you requested',
    'Tool failed',
    'Token usage: legacy'
  ]);
  expect(rows[0]!.summary!.node).toMatchObject({ usage, label: 'provider/model' });
  expect(rows[0]!.summary!.logs).toContain('Token usage: in 3, out 2');
  expect(rows[3]!.event.level).toBe('error');
});

test('shows structured warning fields without changing the journal message', () => {
  const event: RunlingEvent = {
    type: 'log',
    level: 'warn',
    message: 'Retrying',
    fields: { attempts: 2, detail: 'one\ntwo' },
    depth: 0,
    color: 'gold',
    timestamp: 1
  };
  const [row] = runLogRows([event]);
  expect(row?.event.message).toBe('Retrying attempts=2 detail="one\\ntwo"');
  expect(row?.event.level).toBe('warn');
  expect(event.message).toBe('Retrying');
});

test('shows recorded logs and task milestones in order', () => {
  const events: RunlingEvent[] = [
    { type: 'step.started', id: 'task', label: 'private task label', timestamp: 0 },
    { type: 'log', level: 'info', message: 'Starting', depth: 1, color: 'blue', timestamp: 20 },
    { type: 'agent.action', agentId: 'agent', action: 'Working', timestamp: 30 },
    {
      type: 'log',
      level: 'debug',
      message: 'Detail',
      depth: 2,
      color: 'gray',
      source: 'agent',
      sourceId: 'agent',
      timestamp: 40
    },
    { type: 'task.linked', taskId: 'task', channelId: 'channel', timestamp: 45 },
    {
      type: 'message.sent',
      id: 'message',
      channelId: 'channel',
      direction: 'input',
      payload: 'Check again',
      timestamp: 50
    },
    {
      type: 'step.finished',
      id: 'task',
      status: 'failed',
      durationMs: 1234,
      error: 'Error: Command failed (exit 128)',
      timestamp: 60
    }
  ];
  expect(runLogRows(events)).toEqual([
    {
      id: 0,
      activity: { id: 'task', reference: 'task-1', embedded: true },
      event: {
        type: 'log',
        level: 'info',
        message: 'Started task-1',
        depth: 0,
        color: 'dodgerblue',
        source: 'step',
        timestamp: 0
      }
    },
    { id: 1, event: events[1] },
    { id: 3, event: events[3] },
    {
      id: 5,
      activity: { id: 'task', reference: 'task-1', embedded: true },
      event: {
        type: 'log',
        level: 'info',
        message: 'Message to task-1 · Check again',
        depth: 0,
        color: 'dodgerblue',
        source: 'step',
        timestamp: 50
      }
    },
    {
      id: 6,
      activity: { id: 'task', reference: 'task-1', embedded: true },
      event: {
        type: 'log',
        level: 'error',
        message: 'task-1 failed · 1 s · Error: Command failed (exit 128)',
        depth: 0,
        color: 'dodgerblue',
        source: 'step',
        timestamp: 60
      }
    }
  ]);
  expect(JSON.stringify(runLogRows(events))).not.toContain('private task label');
  expect(logTime(3_723_999)).toBe('01:02:03');
});

test('keeps nested task milestones at the depth of their parent task', () => {
  const events: RunlingEvent[] = [
    { type: 'step.started', id: 'root', label: 'Root', timestamp: 0 },
    { type: 'step.started', id: 'child', label: 'Child', activityId: 'root', timestamp: 1 },
    { type: 'step.finished', id: 'child', status: 'completed', durationMs: 3, timestamp: 4 },
    { type: 'step.finished', id: 'root', status: 'completed', durationMs: 5, timestamp: 5 }
  ];
  const rows = runLogRows(events);
  expect(rows.map((row) => row.event.depth)).toEqual([0, 1, 1, 0]);
  expect(
    logTreeRows(rows).map(({ depth, joinsAbove, continuesBelow }) => ({
      depth,
      joinsAbove,
      continuesBelow
    }))
  ).toEqual([
    { depth: 0, joinsAbove: false, continuesBelow: true },
    { depth: 1, joinsAbove: false, continuesBelow: true },
    { depth: 1, joinsAbove: true, continuesBelow: false },
    { depth: 0, joinsAbove: true, continuesBelow: false }
  ]);
});

test('shows messages between the host and the top-level task, not between tasks', () => {
  const events: RunlingEvent[] = [
    { type: 'step.started', id: 'root', label: 'Root', timestamp: 0 },
    { type: 'task.linked', taskId: 'root', channelId: 'host', timestamp: 0 },
    {
      type: 'message.sent',
      id: 'in',
      channelId: 'host',
      direction: 'input',
      payload: '{"text":"Hello"}',
      timestamp: 1
    },
    { type: 'step.started', id: 'child', label: 'Child', activityId: 'root', timestamp: 2 },
    { type: 'task.linked', taskId: 'child', channelId: 'child', activityId: 'root', timestamp: 2 },
    {
      type: 'message.sent',
      id: 'between',
      channelId: 'child',
      direction: 'input',
      payload: 'Private',
      activityId: 'root',
      timestamp: 3
    },
    {
      type: 'message.sent',
      id: 'out',
      channelId: 'host',
      direction: 'update',
      payload: 'x'.repeat(2100),
      timestamp: 4
    }
  ];
  const rows = runLogRows(events).filter((row) => row.event.message.includes(' · '));
  expect(rows.map((row) => row.event.message)).toEqual([
    'Message to task-1 · {"text":"Hello"}',
    `Update from task-1 · ${'x'.repeat(2000)}…`
  ]);
  expect(rows[0]!.activity).toEqual({ id: 'root', reference: 'task-1', embedded: true });
});

test('links log lines to the task that owns their activity', () => {
  const events: RunlingEvent[] = [
    { type: 'step.started', id: 'root', label: 'Root', timestamp: 0 },
    { type: 'step.started', id: 'child', label: 'Child', activityId: 'root', timestamp: 1 },
    {
      type: 'log',
      level: 'info',
      message: 'From child',
      depth: 1,
      color: 'blue',
      activityId: 'child',
      timestamp: 2
    },
    { type: 'command.started', id: 'command', command: 'test', activityId: 'child', timestamp: 3 },
    {
      type: 'log',
      level: 'info',
      message: 'From command',
      depth: 2,
      color: 'blue',
      activityId: 'command',
      timestamp: 4
    },
    { type: 'log', level: 'info', message: 'Outside tasks', depth: 0, color: 'blue', timestamp: 5 }
  ];
  const rows = runLogRows(events);
  expect(
    rows
      .filter((row) => row.event.type === 'log' && row.event.message.startsWith('From'))
      .map((row) => row.activity)
  ).toEqual([
    { id: 'child', reference: 'task-2', embedded: false },
    { id: 'child', reference: 'task-2', embedded: false }
  ]);
  expect(rows.at(-1)?.activity).toBeUndefined();
});

test('links bracketed agent names to their activity details', () => {
  const events: RunlingEvent[] = [
    { type: 'step.started', id: 'root', label: 'Root', timestamp: 0 },
    {
      type: 'agent.started',
      agentId: 'bright-cats-1234',
      model: 'example',
      color: 'blue',
      activityId: 'root',
      timestamp: 1
    },
    {
      type: 'log',
      level: 'info',
      message: '[bright-cats-1234] Working',
      depth: 1,
      color: 'blue',
      source: 'agent',
      sourceId: 'bright-cats-1234',
      activityId: 'root',
      timestamp: 2
    }
  ];
  expect(runLogRows(events).at(-1)?.activity).toEqual({
    id: 'bright-cats-1234:1',
    reference: 'bright-cats-1234',
    embedded: true
  });
});

test('links a task label log line to the step that wrote it', () => {
  const events: RunlingEvent[] = [
    { type: 'step.started', id: 'root', label: 'Root', timestamp: 0 },
    {
      type: 'log',
      level: 'info',
      message: 'Root',
      depth: 0,
      color: 'blue',
      source: 'step',
      timestamp: 1
    },
    { type: 'step.started', id: 'child', label: 'accumulate', activityId: 'root', timestamp: 2 },
    {
      type: 'task.linked',
      taskId: 'child',
      channelId: 'channel',
      activityId: 'root',
      timestamp: 2.5
    },
    {
      type: 'log',
      level: 'info',
      message: 'accumulate',
      depth: 1,
      color: 'blue',
      source: 'step',
      activityId: 'root',
      timestamp: 3
    }
  ];
  expect(
    runLogRows(events)
      .filter((row) => row.event.message === 'accumulate')
      .at(-1)?.activity
  ).toEqual({
    id: 'child',
    reference: 'accumulate',
    embedded: true
  });
});

test('follows at the bottom, pauses on upward scroll, and resumes there', () => {
  expect(nextLogFollow(true, 100, { scrollTop: 90, scrollHeight: 500, clientHeight: 100 })).toBe(
    false
  );
  expect(nextLogFollow(false, 90, { scrollTop: 120, scrollHeight: 500, clientHeight: 100 })).toBe(
    false
  );
  expect(nextLogFollow(false, 120, { scrollTop: 394, scrollHeight: 500, clientHeight: 100 })).toBe(
    true
  );
  expect(nextLogFollow(true, 394, { scrollTop: 394, scrollHeight: 600, clientHeight: 100 })).toBe(
    true
  );
});
