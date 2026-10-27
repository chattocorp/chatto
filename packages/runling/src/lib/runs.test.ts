import { expect, test } from 'vitest';
import { newRun } from '../runtime/run-journal.ts';
import { applyRecord, continueRun, projectRecord } from './runs.ts';
import { buildTimeline } from './timeline.ts';

test('history aligns execution clocks and closes interrupted activities at the prior boundary', () => {
  const first = newRun({ webhook: 'test', workflow: 'test', source: 'web', input: null });
  first.startedAt = 1000;
  first.durationMs = 200;
  first.events = [{ type: 'step.started', id: 'old', label: 'Old', timestamp: 20 }];
  const next = { ...first, startedAt: 5000, durationMs: undefined, events: [] };
  const combined = continueRun(first, next);
  const live = applyRecord(
    combined,
    projectRecord(combined, {
      type: 'event',
      event: { type: 'step.started', id: 'new', label: 'New', timestamp: 30 }
    })
  );
  expect(buildTimeline(live.events, 'running')).toMatchObject([
    { id: 'old', startedAt: 20, durationMs: 180, status: 'interrupted' },
    { id: 'new', startedAt: 4030, status: 'running' }
  ]);
  expect(
    projectRecord(combined, {
      type: 'finished',
      status: 'completed',
      finishedAt: 5100,
      durationMs: 100,
      usage: next.usage,
      output: null,
      error: null
    })
  ).toMatchObject({ durationMs: 4100 });
});
