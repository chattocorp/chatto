import type { RunlingEvent, TokenUsage } from "runling";

export type RunStatus = "running" | "completed" | "failed" | "interrupted" | "cancelled";
export interface RunActivity {
  label: string;
  step?: string;
  preview?: string;
  waiting: boolean;
  pendingInputs: number;
  parallel: number;
}
export interface RunSummary {
  activity?: RunActivity | null;
  id: string;
  /** Human-readable, journal-local reference. Older journals use their UUID. */
  reference?: string;
  /** Read-only compatibility with journals written by the removed resume feature. */
  recovery?: { name: string; version: number };
  attempt?: number;
  webhook: string;
  workflow: string;
  source: "webhook" | "web" | "source";
  /** Named event source; absent in older journals and HTTP-started runs. */
  sourceName?: string;
  status: RunStatus;
  startedAt: number;
  finishedAt?: number;
  durationMs?: number;
  usage: TokenUsage;
}
export interface RunDetail extends RunSummary {
  input: unknown;
  output: unknown;
  error: string | null;
  events: RunlingEvent[];
}
export type RunRecord =
  | { type: "started"; run: RunDetail }
  | { type: "resumed"; attempt: number; resumedAt: number }
  | { type: "event"; event: RunlingEvent }
  | {
      type: "finished";
      status: Exclude<RunStatus, "running">;
      finishedAt: number;
      durationMs: number;
      usage: TokenUsage;
      output: unknown;
      error: string | null;
    };

export function applyRecord(run: RunDetail, record: RunRecord): RunDetail {
  if (record.type === "started") return record.run;
  if (record.type === "resumed") return { ...run, status: "running", attempt: record.attempt, finishedAt: undefined, error: null, output: null,
    events: [...run.events, { type: "workflow.resumed", attempt: record.attempt, timestamp: run.durationMs ?? 0 }] };
  if (record.type === "event") {
    return {
      ...run,
      events: [...run.events, record.event],
      usage:
        record.event.type === "usage.updated" ? record.event.usage : run.usage,
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
