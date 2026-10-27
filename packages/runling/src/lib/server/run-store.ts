/** Persist web runs and bridge their lifecycle to live console subscribers. */
import type { RunHandle, SourceRun } from 'runling/web';
import { parseBoundary } from '../../runtime/schema.ts';
import { serverLog } from '../../runtime/server-log.ts';
import { createServerActivityLog } from '../../runtime/server-activity.ts';
import { mkdir, readdir, appendFile, truncate, rm } from 'node:fs/promises';
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
import type { AnyTask, Channel, WorkflowExecution } from 'runling';

import { runtime } from './runtime.ts';
const { createChannel, runWorkflow } = runtime;
import {
  continueRun,
  projectRecord,
  runUsage,
  type RunDetail,
  type RunRecord,
  type RunSummary
} from '../runs.ts';

import { summarizeRunActivity } from '../run-activity.ts';
import { WorkflowStateStore } from '../../runtime/workflow-state.ts';
import { emptyTokenUsage } from '../../runtime/usage.ts';

const validId = /^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/;
const DAY_MS = 24 * 60 * 60_000;
/** Retention starts at terminal closure, never at workflow creation or shutdown interruption. */
const RETENTION_MS = 30 * DAY_MS;
type Listener = (id: string, record: RunRecord) => void;

function summary(run: RunDetail): RunSummary {
  const { input: _, output: _o, error: _e, events: _v, ...value } = run;
  return value;
}

// Server-owned event arrays can grow in place. Browser state uses applyRecord.
function applyStoredRecord(run: RunDetail, record: RunRecord, includeDetails = true) {
  record = projectRecord(run, record);
  if (record.type === 'event') {
    if (record.event.type === 'run.title') run.title = record.event.title;
    if (includeDetails) run.events.push(record.event);
    if (record.event.type === 'usage.updated') run.usage = runUsage(run, record.event.usage);
  } else if (record.type === 'finished') {
    const { type: _, output, error, ...result } = record;
    Object.assign(run, result, { usage: runUsage(run, result.usage) });
    if (includeDetails) Object.assign(run, { output, error });
  }
  return record;
}

/** Keeps the run history of one journal directory. The server writes its own runs; runs that
 * `runling run` writes are read at startup. */
export class RunStore {
  private runs = new Map<string, RunSummary>();
  private details = new Map<string, RunDetail>();
  private journals = new Map<string, RunJournal>();
  private controllers = new Map<string, AbortController>();
  /** Messages from the host to each active run's top-level task. */
  private inboxes = new Map<string, { task: AnyTask; channel: Channel<unknown> }>();
  private executions = new Set<Promise<WorkflowExecution>>();
  private handles = new Map<string, RunHandle>();
  /** Reservations cover asynchronous registration as well as task execution. */
  private registrations = new Set<Promise<unknown>>();
  private reactivating = new Set<string>();
  private listeners = new Set<Listener>();
  private references = new Set<string>();
  private readonly shutdownReason = new Error('Runling server stopped.');
  private closing = false;
  private workflows: Record<string, AnyTask> = {};
  private recovery: Promise<void> = Promise.resolve();
  private nextCleanup = 0;
  private cleaning?: Promise<void>;
  /** The only persisted mutable state object owned by this server. */
  readonly state: WorkflowStateStore;

  constructor(
    readonly directory: string,
    private readonly createReference = randomId
  ) {
    this.state = new WorkflowStateStore(resolve(directory, 'state.json'));
  }

  async init(): Promise<void> {
    await mkdir(this.directory, { recursive: true });
    await this.state.load();
    for (const file of await readdir(this.directory)) {
      const id = file.replace(/\.jsonl$/, '');
      if (!file.endsWith('.jsonl') || !validId.test(id)) continue;
      const path = resolve(this.directory, file);
      try {
        const { run, end, lastTimestamp, attemptUsage } = await this.read(id, false);
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
            usage: attemptUsage,
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
    await this.cleanup();
    this.state.start(
      () => serverLog('error', 'state.save_failed', { message: 'Cannot save workflow state' }),
      60_000,
      () => this.cleanup()
    );
  }

  /** Resolve saved root keys against current configuration. Re-entry keeps the run ID;
   * it never recreates child tasks, restores queues, or replays external effects. */
  restore(workflows: Record<string, AnyTask> = {}): Promise<void> {
    const next = this.recovery.then(() => this.recover(workflows));
    this.recovery = next.catch(() => {});
    return next;
  }

  /** Run once at startup, then at most daily from the persistence timer.
   * Failures leave enough journal data to retry and never block normal state saves. */
  cleanup(): Promise<void> {
    if (this.cleaning) return this.cleaning;
    if (this.closing || Date.now() < this.nextCleanup) return Promise.resolve();
    this.nextCleanup = Date.now() + DAY_MS;
    this.cleaning = this.purgeExpired()
      .catch(() => {
        serverLog('error', 'run.cleanup_failed', { message: 'Cannot clean expired runs' });
      })
      .finally(() => {
        this.cleaning = undefined;
      });
    return this.cleaning;
  }

  /** Read current journals, including CLI runs that finished since startup. Persist removal
   * from the snapshot before deleting journals, so a crash cannot leave orphaned saved state. */
  private async purgeExpired(): Promise<void> {
    const cutoff = Date.now() - RETENTION_MS;
    const expired: RunDetail[] = [];
    for (const file of await readdir(this.directory)) {
      const id = file.replace(/\.jsonl$/, '');
      if (
        !file.endsWith('.jsonl') ||
        !validId.test(id) ||
        this.controllers.has(id) ||
        this.reactivating.has(id) ||
        this.journals.has(id)
      )
        continue;
      try {
        const { run } = await this.read(id, false);
        if (
          !run ||
          !['completed', 'failed', 'cancelled'].includes(run.status) ||
          !Number.isFinite(run.finishedAt) ||
          run.finishedAt! >= cutoff
        )
          continue;
        expired.push(run);
      } catch {
        serverLog('error', 'run.cleanup_failed', {
          runId: id,
          message: 'Cannot clean expired run'
        });
      }
    }
    if (!expired.length) return;
    const removed = expired.flatMap(({ id }) => {
      const saved = this.state.data.workflows[id];
      delete this.state.data.workflows[id];
      return saved ? [[id, saved] as const] : [];
    });
    try {
      // Also save an empty snapshot when the last retained run expires.
      await this.state.save();
    } catch (error) {
      for (const [id, saved] of removed) this.state.data.workflows[id] = saved;
      throw error;
    }
    for (const run of expired) {
      try {
        await rm(resolve(this.directory, run.id), { recursive: true, force: true });
        await rm(resolve(this.directory, `${run.id}.jsonl`), { force: true });
        this.runs.delete(run.id);
        this.details.delete(run.id);
        if (run.reference) this.references.delete(run.reference);
      } catch {
        serverLog('error', 'run.cleanup_failed', {
          runId: run.id,
          message: 'Cannot remove expired run files'
        });
      }
    }
  }

  private async recover(workflows: Record<string, AnyTask>): Promise<void> {
    if (this.closing) return;
    this.workflows = workflows;
    for (const [id, saved] of Object.entries(this.state.data.workflows)) {
      if (!saved.open || this.controllers.has(id)) continue;
      const prior = this.runs.get(id);
      if (!prior || runOwnerAlive(prior)) continue;
      // A terminal journal is newer evidence than a periodic snapshot.
      if (prior.status !== 'interrupted' && prior.status !== 'running') {
        saved.open = false;
        continue;
      }
      const task =
        saved.task !== null && Object.hasOwn(workflows, saved.task)
          ? workflows[saved.task]
          : undefined;
      if (!task) {
        serverLog('warn', 'run.recovery_unavailable', { runId: id });
        continue;
      }
      const { run: previous, end } = await this.read(id, true);
      if (!previous) continue;
      await truncate(resolve(this.directory, `${id}.jsonl`), end);
      const run: RunDetail = {
        ...previous,
        pid: process.pid,
        status: 'running',
        startedAt: Date.now(),
        executionStartedAt: undefined,
        finishedAt: undefined,
        durationMs: undefined,
        output: null,
        error: null,
        events: [],
        previousUsage: previous.usage,
        usage: previous.usage
      };
      this.journals.set(id, await createRunJournal(this.directory, run, true));
      this.launch(continueRun(previous, run), task, saved.input, true);
    }
  }

  list(): RunSummary[] {
    return [...this.runs.values()]
      .sort((a, b) => (b.executionStartedAt ?? b.startedAt) - (a.executionStartedAt ?? a.startedAt))
      .slice(0, 100)
      .map((run) => {
        const detail = this.details.get(run.id);
        return { ...run, activity: detail ? summarizeRunActivity(detail) : null };
      });
  }

  /** Source routing can include retained executions without starting them again. */
  sourceRuns(name: string, options: { includeFinished?: boolean } = {}): SourceRun[] {
    return [...this.runs].flatMap(([id, run]) => {
      const handle = this.handles.get(id);
      const saved = this.state.data.workflows[id];
      return run.source === 'source' &&
        run.sourceName === name &&
        (handle || (options.includeFinished && saved && run.status !== 'running'))
        ? [
            {
              ...(handle ?? {
                id,
                steer: async () => {
                  throw new RunNotActiveError();
                },
                closeSteering: () => {},
                cancel: () => {},
                finished: Promise.resolve()
              }),
              input: saved?.input,
              workflow: saved?.task ?? run.workflow
            }
          ]
        : [];
    });
  }

  async get(id: string): Promise<RunDetail | undefined> {
    if (!validId.test(id) || !this.runs.has(id)) return undefined;
    return this.details.get(id) ?? (await this.read(id, true)).run;
  }

  /** Validate a message and queue it for an active run's top-level task. */
  async send(id: string, message: unknown): Promise<void> {
    const inbox = this.inboxes.get(id);
    // A cancelled run's inbox fails with the cancellation reason; report the run state instead.
    if (!inbox || this.controllers.get(id)?.signal.aborted) throw new RunNotActiveError();
    await parseBoundary(inbox.task.name, 'message', inbox.task.steering, message);
    await inbox.channel.send(message);
  }

  /** Tell an active run's top-level task that no more messages come. */
  closeSteering(id: string): void {
    this.inboxes.get(id)?.channel.close();
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
    this.state.stop();
    // After HTTP and watchers close, cleanup can depend only on unref'ed
    // deadlines. A promise alone does not keep Node alive to flush the journal.
    const keepAlive = setInterval(() => {}, 1000);
    try {
      await this.recovery;
      await Promise.allSettled(this.registrations);
      await this.cleaning;
      for (const controller of this.controllers.values()) controller.abort(this.shutdownReason);
      await Promise.allSettled(this.executions);
      await this.state.close();
    } finally {
      clearInterval(keepAlive);
    }
  }

  private async read(id: string, includeDetails: boolean) {
    let run: RunDetail | undefined;
    let end = 0;
    let lastTimestamp = 0;
    let attemptUsage = emptyTokenUsage();
    for await (const line of readRunJournal(resolve(this.directory, `${id}.jsonl`))) {
      end += line.bytes;
      const record = line.record;
      if (!record) continue;
      if (!run || record.type === 'started') {
        if (record.type !== 'started' || record.run.id !== id) break;
        lastTimestamp = 0;
        attemptUsage = emptyTokenUsage();
        // Derive older journals' baseline too. A started record contains no work
        // from its new attempt, even when it carries totals from prior attempts.
        const previousUsage = run?.usage ?? record.run.previousUsage;
        const previous = run;
        run = includeDetails
          ? { ...record.run }
          : {
              ...record.run,
              input: null,
              output: null,
              error: null,
              events: []
            };
        if (previous) run = continueRun(previous, run);
        if (!includeDetails) run.events = [];
        run.previousUsage = previousUsage;
        run.usage = { ...(previousUsage ?? record.run.usage) };
      } else {
        if (record.type === 'finished') attemptUsage = record.usage;
        else if (record.event.type === 'usage.updated') attemptUsage = record.event.usage;
        applyStoredRecord(run, record, includeDetails);
        if (record.type === 'event') lastTimestamp = record.event.timestamp;
      }
    }
    return { run, end, lastTimestamp, attemptUsage };
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
        const projected = applyStoredRecord(run, record);
        this.runs.set(id, summary(run));
        this.publish(id, projected);
        if (record.type === 'finished') this.details.delete(id);
      });
  }

  /** Register and start a run. Resolves after registration, with a handle for its messages. */
  start(
    webhook: string,
    workflow: AnyTask,
    input: unknown,
    source: 'webhook' | 'web' | 'source',
    runId?: string
  ): Promise<RunHandle & { completion: Promise<WorkflowExecution> }> {
    const registration =
      runId === undefined
        ? this.startNew(webhook, workflow, input, source)
        : this.reactivate(runId, webhook, workflow, input, source);
    this.registrations.add(registration);
    void registration.finally(() => this.registrations.delete(registration)).catch(() => {});
    return registration;
  }

  /** Re-enter a finished root with fresh input, retaining its identity and state. */
  private async reactivate(
    id: string,
    webhook: string,
    workflow: AnyTask,
    input: unknown,
    source: 'webhook' | 'web' | 'source'
  ) {
    if (this.closing) throw new Error('Run store is stopping');
    if (this.reactivating.has(id) || this.handles.has(id) || this.journals.has(id))
      throw new Error('This run is still active');
    this.reactivating.add(id);
    try {
      await this.cleaning;
      const saved = this.state.data.workflows[id];
      const prior = this.runs.get(id);
      if (
        !prior ||
        !saved ||
        prior.source !== source ||
        prior.webhook !== webhook ||
        saved.task === null ||
        this.workflows[saved.task] !== workflow
      )
        throw new Error('Run is not available for this route and task');
      if (prior.status === 'running' || runOwnerAlive(prior))
        throw new Error('This run is still active');
      await parseBoundary(workflow.name, 'input', workflow.input, input);
      const { run: previous, end } = await this.read(id, true);
      if (!previous) throw new Error('Run history is unavailable');
      const run: RunDetail = {
        ...previous,
        workflow: workflow.name,
        input: input === undefined ? null : JSON.parse(JSON.stringify(input)),
        pid: process.pid,
        status: 'running',
        startedAt: Date.now(),
        executionStartedAt: undefined,
        finishedAt: undefined,
        durationMs: undefined,
        output: null,
        error: null,
        events: [],
        previousUsage: previous.usage,
        usage: previous.usage
      };
      await truncate(resolve(this.directory, `${id}.jsonl`), end);
      this.journals.set(id, await createRunJournal(this.directory, run, true));
      saved.input = input === undefined ? undefined : run.input;
      saved.open = true;
      return this.launch(continueRun(previous, run), workflow, input);
    } finally {
      this.reactivating.delete(id);
    }
  }

  private async startNew(
    webhook: string,
    workflow: AnyTask,
    input: unknown,
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
    try {
      this.journals.set(id, await createRunJournal(this.directory, run));
    } catch (error) {
      this.references.delete(reference);
      throw error;
    }
    const key = Object.entries(this.workflows).find(([, task]) => task === workflow)?.[0];
    this.state.data.workflows[id] = {
      task: key ?? null,
      input: input === undefined ? undefined : run.input,
      state: {},
      open: true
    };
    return this.launch(run, workflow, input);
  }

  private launch(
    run: RunDetail,
    workflow: AnyTask,
    input: unknown,
    resumed = false
  ): RunHandle & { completion: Promise<WorkflowExecution> } {
    const { id, webhook, source } = run;
    this.runs.set(id, summary(run));
    this.details.set(id, run);
    const controller = new AbortController();
    this.controllers.set(id, controller);
    this.inboxes.set(id, { task: workflow, channel: createChannel({ signal: controller.signal }) });
    this.publish(id, { type: 'started', run });
    serverLog(
      'info',
      'run.started',
      {
        runId: id,
        runReference: run.reference,
        webhook,
        workflow: workflow.name,
        source
      },
      { terminal: process.env.RUNLING_QUIET_LOG === '1' }
    );
    const completion = this.execute(id, workflow, input, controller.signal, resumed);
    this.executions.add(completion);
    void completion.then(
      () => {
        this.executions.delete(completion);
        this.handles.delete(id);
      },
      () => {
        this.executions.delete(completion);
        this.handles.delete(id);
      }
    );
    // Background runs must always have a rejection handler, even after the HTTP client leaves.
    void completion.catch((cause) =>
      serverLog('error', 'run.error', { runId: id, runReference: run.reference, error: cause })
    );
    const handle = {
      id,
      completion,
      steer: (message: unknown) =>
        this.controllers.get(id) === controller
          ? this.send(id, message)
          : Promise.reject(new RunNotActiveError()),
      closeSteering: () => {
        if (this.controllers.get(id) === controller) this.closeSteering(id);
      },
      cancel: () => {
        if (this.controllers.get(id) === controller) this.cancel(id);
      },
      finished: completion.then(
        () => {},
        () => {}
      )
    };
    this.handles.set(id, handle);
    return handle;
  }

  private async execute(
    id: string,
    workflow: AnyTask,
    input: unknown,
    signal: AbortSignal,
    resumed: boolean
  ): Promise<WorkflowExecution> {
    const base = performance.now();
    const activityLog = createServerActivityLog(id, this.runs.get(id)?.reference);
    const execution = await runWorkflow(workflow, {
      input,
      state: this.state.data.workflows[id]?.state,
      signal,
      messages: this.inboxes.get(id)!.channel,
      run: {
        id,
        reference: this.runs.get(id)?.reference,
        title: this.runs.get(id)?.title,
        resumed,
        filesDirectory: resolve(this.directory, id)
      },
      onEvent: (event) => {
        activityLog(event);
        // A write failure is handled when the finished record is written.
        void this.append(id, eventRecord(event, base)).catch(() => {});
      }
    }).finally(() => activityLog.dispose());
    this.controllers.delete(id);
    this.inboxes.get(id)?.channel.close();
    this.inboxes.delete(id);
    const status = signal.aborted
      ? signal.reason === this.shutdownReason
        ? 'interrupted'
        : 'cancelled'
      : execution.ok
        ? 'completed'
        : 'failed';
    if (status !== 'interrupted' && this.state.data.workflows[id])
      this.state.data.workflows[id]!.open = false;
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
      const projected = applyStoredRecord(run, record);
      this.runs.set(id, summary(run));
      this.publish(id, projected);
      throw new Error(record.error!);
    } finally {
      this.journals.delete(id);
    }
    // Persist the final state before the execution handle settles. A failed snapshot
    // rejects completion; the journal still describes the actual task outcome.
    await this.state.save();
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

/** The run has finished, or this server does not run it. */
export class RunNotActiveError extends Error {
  constructor() {
    super('This run is not running.');
    this.name = 'RunNotActiveError';
  }
}
