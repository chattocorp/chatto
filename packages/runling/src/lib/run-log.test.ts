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
