import { expect, test } from 'vitest';
import type { RunlingEvent } from 'runling';
import { logTime, logTreeRows, nextLogFollow, runLogRows } from './run-log.ts';

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
      type: 'task.activity',
      channelId: 'channel',
      message: 'Validation failed',
      level: 'error',
      timestamp: 50
    },
    { type: 'step.finished', id: 'task', status: 'failed', durationMs: 1234, timestamp: 60 }
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
        level: 'error',
        message: 'task-1 · Validation failed',
        depth: 1,
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
        message: 'task-1 failed · 1 s',
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
    { type: 'task.linked', taskId: 'child', channelId: 'channel', timestamp: 2 },
    { type: 'task.activity', channelId: 'channel', message: 'Working', timestamp: 3 },
    { type: 'step.finished', id: 'child', status: 'completed', durationMs: 3, timestamp: 4 },
    { type: 'step.finished', id: 'root', status: 'completed', durationMs: 5, timestamp: 5 }
  ];
  const rows = runLogRows(events);
  expect(rows.map((row) => row.event.depth)).toEqual([0, 1, 2, 1, 0]);
  expect(
    logTreeRows(rows).map(({ depth, joinsAbove, continuesBelow }) => ({
      depth,
      joinsAbove,
      continuesBelow
    }))
  ).toEqual([
    { depth: 0, joinsAbove: false, continuesBelow: true },
    { depth: 1, joinsAbove: false, continuesBelow: true },
    { depth: 2, joinsAbove: false, continuesBelow: false },
    { depth: 1, joinsAbove: true, continuesBelow: false },
    { depth: 0, joinsAbove: true, continuesBelow: false }
  ]);
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
