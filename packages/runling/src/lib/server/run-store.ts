/** Persist web runs and bridge their lifecycle to live console subscribers. */
import type { WebhookTask } from 'runling/web';
import { serverLog } from '../../runtime/server-log.ts';
import { createServerActivityLog } from '../../runtime/server-activity.ts';
import { mkdir, readdir, appendFile, truncate } from 'node:fs/promises';
import { resolve, dirname } from 'node:path';
import { randomId } from '../../runtime/id.ts';
import {
  createRunJournal,
  eventRecord,
  newRun,
  readRunJournal,
  runJournalDirectory,
  runOwnerAlive,
  type RunJournal
} from '../../runtime/run-journal.ts';
import { runWorkflow, type WorkflowExecution } from 'runling';
import { type RunDetail, type RunRecord, type RunSummary } from '../runs.ts';

import { summarizeRunActivity } from '../run-activity.ts';

const validId = /^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/;
type Listener = (id: string, record: RunRecord) => void;

function summary(run: RunDetail): RunSummary {
  const { input: _, output: _o, error: _e, events: _v, ...value } = run;
  return value;
}

// Server-owned event arrays can grow in place. Browser state uses applyRecord.
function applyStoredRecord(run: RunDetail, record: RunRecord, includeDetails = true) {
  if (record.type === 'resumed') {
    if (includeDetails)
      run.events.push({
        type: 'workflow.resumed',
        attempt: record.attempt,
        timestamp: run.durationMs ?? 0
      });
    Object.assign(run, {
      status: 'running',
      attempt: record.attempt,
      finishedAt: undefined,
      error: null,
      output: null
    });
  } else if (record.type === 'event') {
    if (includeDetails) run.events.push(record.event);
    if (record.event.type === 'usage.updated') run.usage = record.event.usage;
  } else if (record.type === 'finished') {
    const { type: _, output, error, ...result } = record;
    Object.assign(run, result);
    if (includeDetails) Object.assign(run, { output, error });
  }
}

/** Keeps the run history of one journal directory. The server writes its own runs; runs that
 * `runling run` writes are read at startup. */
export class RunStore {
  private runs = new Map<string, RunSummary>();
  private details = new Map<string, RunDetail>();
  private journals = new Map<string, RunJournal>();
  private controllers = new Map<string, AbortController>();
  private executions = new Set<Promise<WorkflowExecution>>();
  private listeners = new Set<Listener>();
  private references = new Set<string>();
  private readonly shutdownReason = new Error('Runling server stopped.');
  private closing = false;

  constructor(
    readonly directory: string,
    private readonly createReference = randomId
  ) {}

  async init(): Promise<void> {
    await mkdir(this.directory, { recursive: true });
    for (const file of await readdir(this.directory)) {
      const id = file.replace(/\.jsonl$/, '');
      if (!file.endsWith('.jsonl') || !validId.test(id)) continue;
      const path = resolve(this.directory, file);
      try {
        const { run, end, lastTimestamp } = await this.read(id, false);
        if (!run) continue;
        if (run.reference) this.references.add(run.reference);
        // Another process, such as `runling run`, can still write this journal.
        if (run.status === 'running' && !runOwnerAlive(run)) {
          // A crash can leave the final JSON line incomplete.
          await truncate(path, end);
          const record: RunRecord = {
            type: 'finished',
            status: 'interrupted',
            finishedAt: Date.now(),
            durationMs: lastTimestamp,
            output: null,
            usage: run.usage,
            error: 'The server stopped before this run finished.'
          };
          await appendFile(path, `${JSON.stringify(record)}\n`);
          applyStoredRecord(run, record, false);
          serverLog('warn', 'run.interrupted', { runId: id, runReference: run.reference });
        }
        this.runs.set(id, summary(run));
      } catch (cause) {
        serverLog('error', 'run.restore_failed', { runId: id, error: cause });
      }
    }
  }

  list(): RunSummary[] {
    return [...this.runs.values()]
      .sort((a, b) => b.startedAt - a.startedAt)
      .slice(0, 100)
      .map((run) => {
        const detail = this.details.get(run.id);
        return { ...run, activity: detail ? summarizeRunActivity(detail) : null };
      });
  }

  async get(id: string): Promise<RunDetail | undefined> {
    if (!validId.test(id) || !this.runs.has(id)) return undefined;
    return this.details.get(id) ?? (await this.read(id, true)).run;
  }

  cancel(id: string): boolean {
    const controller = this.controllers.get(id);
    if (!controller) return false;
    controller.abort(new Error('Workflow cancelled by user.'));
    return true;
  }

  /** Interrupt active workflows and flush their records. Explicit user cancellations stay cancelled. */
  async close(): Promise<void> {
    this.closing = true;
    // After HTTP and watchers close, cleanup can depend only on unref'ed
    // deadlines. A promise alone does not keep Node alive to flush the journal.
    const keepAlive = setInterval(() => {}, 1000);
    try {
      for (const controller of this.controllers.values()) controller.abort(this.shutdownReason);
      await Promise.allSettled(this.executions);
    } finally {
      clearInterval(keepAlive);
    }
  }

  private async read(id: string, includeDetails: boolean) {
    let run: RunDetail | undefined;
    let end = 0;
    let lastTimestamp = 0;
    for await (const line of readRunJournal(resolve(this.directory, `${id}.jsonl`))) {
      end += line.bytes;
      const record = line.record;
      if (!record) continue;
      if (!run) {
        if (record.type !== 'started' || record.run.id !== id) break;
        run = includeDetails
          ? record.run
          : {
              ...record.run,
              input: null,
              output: null,
              error: null,
              events: []
            };
      } else {
        applyStoredRecord(run, record, includeDetails);
        if (record.type === 'event') lastTimestamp = record.event.timestamp;
      }
    }
    return { run, end, lastTimestamp };
  }

  subscribe(listener: Listener): () => void {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  }

  private publish(id: string, record: RunRecord): void {
    for (const listener of this.listeners) listener(id, record);
  }

  private append(id: string, record: RunRecord): Promise<void> {
    // Journal writes finish in order, so their callbacks apply records in order.
    return this.journals
      .get(id)!
      .append(record)
      .then(() => {
        const run = this.details.get(id)!;
        applyStoredRecord(run, record);
        this.runs.set(id, summary(run));
        this.publish(id, record);
        if (record.type === 'finished') this.details.delete(id);
      });
  }

  async start<Input, Output>(
    webhook: string,
    workflow: WebhookTask<Input, Output>,
    input: Input,
    source: 'webhook' | 'web' | 'source'
  ) {
    if (this.closing) throw new Error('Run store is stopping');
    // Reserve before the first await so concurrent starts cannot share a reference.
    let reference: string | undefined;
    for (let attempt = 0; attempt < 100 && !reference; attempt++) {
      const candidate = this.createReference();
      if (!this.references.has(candidate)) reference = candidate;
    }
    if (!reference) throw new Error('Cannot allocate a unique run reference');
    this.references.add(reference);
    const run = newRun({
      webhook,
      workflow: workflow.name,
      source,
      ...(source === 'source' ? { sourceName: webhook } : {}),
      input,
      reference
    });
    const { id } = run;
    const started: RunRecord = { type: 'started', run };
    try {
      this.journals.set(id, await createRunJournal(this.directory, run));
    } catch (error) {
      this.references.delete(reference);
      throw error;
    }
    this.runs.set(id, summary(run));
    this.details.set(id, run);
    const controller = new AbortController();
    this.controllers.set(id, controller);
    this.publish(id, started);
    serverLog('info', 'run.started', {
      runId: id,
      runReference: run.reference,
      webhook,
      workflow: workflow.name,
      source
    });
    const completion = this.execute(id, workflow, input, controller.signal);
    this.executions.add(completion);
    void completion.then(
      () => this.executions.delete(completion),
      () => this.executions.delete(completion)
    );
    // Background runs must always have a rejection handler, even after the HTTP client leaves.
    void completion.catch((cause) =>
      serverLog('error', 'run.error', { runId: id, runReference: run.reference, error: cause })
    );
    return { id, completion };
  }

  private async execute<Input, Output>(
    id: string,
    workflow: WebhookTask<Input, Output>,
    input: Input,
    signal: AbortSignal
  ): Promise<WorkflowExecution> {
    const base = performance.now();
    const activityLog = createServerActivityLog(id, this.runs.get(id)?.reference);
    const execution = await runWorkflow(workflow, {
      input,
      signal,
      run: { id, reference: this.runs.get(id)?.reference },
      onEvent: (event) => {
        activityLog(event);
        // A write failure is handled when the finished record is written.
        void this.append(id, eventRecord(event, base)).catch(() => {});
      }
    }).finally(() => activityLog.dispose());
    this.controllers.delete(id);
    const status = signal.aborted
      ? signal.reason === this.shutdownReason
        ? 'interrupted'
        : 'cancelled'
      : execution.ok
        ? 'completed'
        : 'failed';
    try {
      await this.append(id, {
        type: 'finished',
        status,
        finishedAt: Date.now(),
        durationMs: execution.durationMs,
        usage: execution.usage,
        output: execution.output,
        error: execution.error
      });
    } catch (cause) {
      const record: RunRecord = {
        type: 'finished',
        status: 'failed',
        finishedAt: Date.now(),
        durationMs: execution.durationMs,
        usage: execution.usage,
        output: null,
        error: `Cannot save run history: ${cause instanceof Error ? cause.message : String(cause)}`
      };
      const run = this.details.get(id)!;
      applyStoredRecord(run, record);
      this.runs.set(id, summary(run));
      this.publish(id, record);
      throw new Error(record.error!);
    } finally {
      this.journals.delete(id);
    }
    serverLog(
      status === 'failed'
        ? 'error'
        : status === 'cancelled' || status === 'interrupted'
          ? 'warn'
          : 'info',
      'run.finished',
      {
        runId: id,
        runReference: this.runs.get(id)?.reference,
        status,
        durationMs: execution.durationMs,
        usage: execution.usage
      }
    );
    return execution;
  }
}

// Preserve active executions through Vite module reloads.
const state = globalThis as typeof globalThis & {
  __runlingRunStore?: Promise<RunStore>;
};
export function getRunStore(): Promise<RunStore> {
  state.__runlingRunStore ??= (async () => {
    const configPath = process.env.RUNLING_WEB_CONFIG;
    if (!configPath) throw new Error('RUNLING_WEB_CONFIG is required for run history');
    const cwd = dirname(configPath);
    const store = new RunStore(await runJournalDirectory(cwd));
    await store.init();
    return store;
  })();
  return state.__runlingRunStore;
}
