import type {
  RunDetail as JournalRunDetail,
  RunSummary as JournalRunSummary
} from '../runtime/run-journal.ts';

// Journal records are defined with the journal writer that `runling run` and `runling serve` share.
export type { RunRecord, RunSource, RunStatus } from '../runtime/run-journal.ts';
import type { RunRecord } from '../runtime/run-journal.ts';
import { accumulateTokenUsage, emptyTokenUsage, type TokenUsage } from '../runtime/usage.ts';

/** Combine prior attempts with a replacement total for the current attempt. */
export function runUsage(run: RunSummary, usage: TokenUsage): TokenUsage {
  return accumulateTokenUsage({ ...(run.previousUsage ?? emptyTokenUsage()) }, usage);
}

export interface RunActivity {
  label: string;
  step?: string;
  preview?: string;
  /** Run time when the selected activity started, in milliseconds. */
  startedAt: number;
  /** The selected activity is a task that waits for a message. */
  waiting: boolean;
  /** Tasks that wait for a message. */
  waits: number;
  parallel: number;
}
/** A journal summary with the server's live activity preview. */
export interface RunSummary extends JournalRunSummary {
  activity?: RunActivity | null;
}
export type RunDetail = JournalRunDetail & { activity?: RunActivity | null };

/** Extend a server-owned history in place; journal records contain only the new execution. */
export function continueRun(previous: RunDetail, next: RunDetail): RunDetail {
  const events = previous.events;
  events.push(
    {
      type: 'execution.finished',
      timestamp: previous.durationMs ?? events.at(-1)?.timestamp ?? 0
    },
    ...next.events
  );
  return {
    ...next,
    startedAt: previous.startedAt,
    executionStartedAt: next.startedAt,
    events
  };
}

/** Journal times are execution-relative; console times share the first execution's origin. */
export function projectRecord(run: RunDetail, record: RunRecord): RunRecord {
  const offset = (run.executionStartedAt ?? run.startedAt) - run.startedAt;
  if (record.type === 'event')
    return { ...record, event: { ...record.event, timestamp: record.event.timestamp + offset } };
  if (record.type === 'finished') return { ...record, durationMs: record.durationMs + offset };
  return record;
}

export function applyRecord(run: RunDetail, record: RunRecord): RunDetail {
  if (record.type === 'started') return record.run;
  if (record.type === 'event') {
    return {
      ...run,
      ...(record.event.type === 'run.title' ? { title: record.event.title } : {}),
      events: [...run.events, record.event],
      usage: record.event.type === 'usage.updated' ? runUsage(run, record.event.usage) : run.usage
    };
  }
  const { type: _, ...result } = record;
  return { ...run, ...result, usage: runUsage(run, result.usage) };
}

export function duration(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)} ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`;
  return `${Math.floor(ms / 60_000)}m ${Math.floor((ms % 60_000) / 1000)}s`;
}

export interface WebhookInfo {
  name: string;
  workflow: string;
  path: string;
  input: Record<string, unknown>;
  output: Record<string, unknown>;
}
