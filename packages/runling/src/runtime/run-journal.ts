/** Run journals: one JSONL file per workflow run in `.runling/runs/`. `runling serve` and
 * `runling run` write them through this module, so every run is kept in one format and place.
 * The server reads them for its run history and console. */
import { randomUUID } from 'node:crypto';
import { createReadStream } from 'node:fs';
import { appendFile, mkdir, stat, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import type { RunlingEvent } from './events.ts';
import { randomId } from './id.ts';
import { emptyTokenUsage, type TokenUsage } from './usage.ts';

export type RunStatus = 'running' | 'completed' | 'failed' | 'interrupted' | 'cancelled';

/** How a run started: a webhook, the web console, an event source, or `runling run`. */
export type RunSource = 'webhook' | 'web' | 'source' | 'cli';

export interface RunSummary {
  id: string;
  /** Human-readable, journal-local reference. Older journals use their UUID. */
  reference?: string;
  /** Read-only compatibility with journals written by the removed resume feature. */
  recovery?: { name: string; version: number };
  attempt?: number;
  /** Webhook or source name; `cli` for command-line runs. */
  webhook: string;
  workflow: string;
  source: RunSource;
  /** Named event source; absent in older journals and HTTP-started runs. */
  sourceName?: string;
  /** Process that writes the journal. A server does not mark a live process's run interrupted.
   * Absent in older journals. */
  pid?: number;
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

/** One journal line. The first line is always `started`. Event timestamps are milliseconds
 * since the run started. */
export type RunRecord =
  | { type: 'started'; run: RunDetail }
  | { type: 'resumed'; attempt: number; resumedAt: number }
  | { type: 'event'; event: RunlingEvent }
  | {
      type: 'finished';
      status: Exclude<RunStatus, 'running'>;
      finishedAt: number;
      durationMs: number;
      usage: TokenUsage;
      output: unknown;
      error: string | null;
    };

/** Journal directory for a project. Projects that still have only the legacy `.factory/runs`
 * directory keep using it. */
export async function runJournalDirectory(cwd: string): Promise<string> {
  const current = resolve(cwd, '.runling/runs');
  const legacy = resolve(cwd, '.factory/runs');
  for (const path of [current, legacy]) {
    try {
      if ((await stat(path)).isDirectory()) return path;
      throw new Error(`Run history path is not a directory: ${path}`);
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error;
    }
  }
  return current;
}

/** A new running run owned by this process. */
export function newRun(
  fields: Pick<RunSummary, 'webhook' | 'workflow' | 'source' | 'sourceName'> & {
    input: unknown;
    reference?: string;
  }
): RunDetail {
  const { input, reference, ...rest } = fields;
  return {
    id: randomUUID(),
    reference: reference ?? randomId(),
    ...rest,
    pid: process.pid,
    input: input === undefined ? null : JSON.parse(JSON.stringify(input)),
    status: 'running',
    startedAt: Date.now(),
    output: null,
    error: null,
    events: [],
    usage: emptyTokenUsage()
  };
}

export interface RunJournal {
  readonly path: string;
  /** Append a record after all earlier records. Resolves when it is written. */
  append(record: RunRecord): Promise<void>;
}

/** An event record, with its timestamp relative to `base`, the run's `performance.now()` start. */
export const eventRecord = (event: RunlingEvent, base: number): RunRecord => ({
  type: 'event',
  event: { ...event, timestamp: Math.max(0, event.timestamp - base) }
});

/** Create the journal file with its `started` record. Fails if the file exists. */
export async function createRunJournal(directory: string, run: RunDetail): Promise<RunJournal> {
  await mkdir(directory, { recursive: true });
  const path = resolve(directory, `${run.id}.jsonl`);
  await writeFile(path, `${JSON.stringify({ type: 'started', run } satisfies RunRecord)}\n`, {
    flag: 'wx',
    mode: 0o600
  });
  let queue = Promise.resolve();
  return {
    path,
    append(record) {
      const next = queue.then(() => appendFile(path, `${JSON.stringify(record)}\n`));
      // A failed write does not stop later records; its caller receives the error.
      queue = next.catch(() => {});
      return next;
    }
  };
}

/** Read journal records with their byte lengths. An incomplete final line is skipped, and its
 * offset is kept, so that a reader can truncate it. */
export async function* readRunJournal(path: string) {
  let buffer = '';
  for await (const chunk of createReadStream(path, { encoding: 'utf8' })) {
    buffer += chunk;
    let end: number;
    while ((end = buffer.indexOf('\n')) !== -1) {
      const line = buffer.slice(0, end);
      buffer = buffer.slice(end + 1);
      yield {
        record: line ? (JSON.parse(line) as RunRecord) : undefined,
        bytes: Buffer.byteLength(line) + 1
      };
    }
  }
}

/** True when another live process still writes this run's journal. */
export function runOwnerAlive(run: Pick<RunSummary, 'pid'>): boolean {
  if (run.pid === undefined || run.pid === process.pid) return false;
  try {
    process.kill(run.pid, 0);
    return true;
  } catch (error) {
    // The process exists, but belongs to another user.
    return (error as NodeJS.ErrnoException).code === 'EPERM';
  }
}
