import type {
  RunDetail as JournalRunDetail,
  RunSummary as JournalRunSummary
} from '../runtime/run-journal.ts';

// Journal records are defined with the journal writer that `runling run` and `runling serve` share.
export type { RunRecord, RunSource, RunStatus } from '../runtime/run-journal.ts';
import type { RunRecord } from '../runtime/run-journal.ts';

export interface RunActivity {
  label: string;
  step?: string;
  preview?: string;
  waiting: boolean;
  pendingInputs: number;
  parallel: number;
}
/** A journal summary with the server's live activity preview. */
export interface RunSummary extends JournalRunSummary {
  activity?: RunActivity | null;
}
export type RunDetail = JournalRunDetail & { activity?: RunActivity | null };

export function applyRecord(run: RunDetail, record: RunRecord): RunDetail {
  if (record.type === 'started') return record.run;
  if (record.type === 'resumed')
    return {
      ...run,
      status: 'running',
      attempt: record.attempt,
      finishedAt: undefined,
      error: null,
      output: null,
      events: [
        ...run.events,
        { type: 'workflow.resumed', attempt: record.attempt, timestamp: run.durationMs ?? 0 }
      ]
    };
  if (record.type === 'event') {
    return {
      ...run,
      events: [...run.events, record.event],
      usage: record.event.type === 'usage.updated' ? record.event.usage : run.usage
    };
  }
  const { type: _, ...result } = record;
  return { ...run, ...result };
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
