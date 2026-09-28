/** Journal of one `runling run` execution, for later review. It uses the server's run-journal
 * record format, but a separate directory: the server owns `.runling/runs/` and marks journals
 * that are still running there as interrupted when it starts. */
import { randomUUID } from 'node:crypto';
import { appendFile, mkdir, writeFile } from 'node:fs/promises';
import { relative, resolve } from 'node:path';
import type { RunlingEvent } from './events.ts';
import { randomId } from './id.ts';
import type { TokenUsage } from './usage.ts';

/** Directory for command-line run journals, relative to the working directory. */
export const CLI_JOURNAL_DIRECTORY = '.runling/cli-runs';

export interface RunJournal {
  id: string;
  /** Human-readable name for the run, printed in the log. */
  reference: string;
  /** Journal path relative to the working directory. */
  path: string;
  /** Queue one event. Write failures surface from `finish`. */
  record(event: RunlingEvent): void;
  finish(result: {
    status: 'completed' | 'failed' | 'cancelled';
    durationMs: number;
    usage: TokenUsage;
    output: unknown;
    error: string | null;
  }): Promise<void>;
}

/** Create the journal file and its `started` record. */
export async function createRunJournal(
  workflow: string,
  input: unknown,
  cwd = process.cwd()
): Promise<RunJournal> {
  const directory = resolve(cwd, CLI_JOURNAL_DIRECTORY);
  await mkdir(directory, { recursive: true, mode: 0o700 });
  const id = randomUUID();
  const reference = randomId();
  const file = resolve(directory, `${id}.jsonl`);
  const base = performance.now();
  const started = {
    type: 'started',
    run: {
      id,
      reference,
      webhook: 'cli',
      workflow,
      source: 'cli',
      input: input === undefined ? null : JSON.parse(JSON.stringify(input)),
      status: 'running',
      startedAt: Date.now(),
      output: null,
      error: null,
      events: [],
      usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 }
    }
  };
  await writeFile(file, `${JSON.stringify(started)}\n`, { flag: 'wx', mode: 0o600 });
  // Keep records in order without blocking the workflow on each write.
  let pending = Promise.resolve();
  let failure: unknown;
  const append = (record: unknown) => {
    pending = pending
      .then(() => appendFile(file, `${JSON.stringify(record)}\n`))
      .catch((cause) => {
        failure ??= cause;
      });
  };
  return {
    id,
    reference,
    path: relative(cwd, file),
    record(event) {
      append({
        type: 'event',
        event: { ...event, timestamp: Math.max(0, event.timestamp - base) }
      });
    },
    async finish(result) {
      append({ type: 'finished', finishedAt: Date.now(), ...result });
      await pending;
      if (failure) throw failure;
    }
  };
}
