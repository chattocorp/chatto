/** One ordinary state object and periodic snapshot for a Runling server instance.
 * This is a recovery hint, not a transaction log: abrupt exit can lose recent mutations. */
import { mkdir, readFile, rename, rm, writeFile } from 'node:fs/promises';
import { dirname } from 'node:path';
import type { WorkflowState } from './context.ts';

/** Enough information to re-enter a registered root. Child tasks and messages are not saved. */
export interface SavedWorkflow {
  /** Stable configuration key, or null for a root without a recovery registration. */
  task: string | null;
  input: unknown;
  state: WorkflowState;
  open: boolean;
}

interface Snapshot {
  version: 1;
  workflows: Record<string, SavedWorkflow>;
}

/** Saves complete snapshots with ordered, atomic file replacement. One process owns a file. */
export class WorkflowStateStore {
  readonly data: Snapshot = { version: 1, workflows: {} };
  private writes: Promise<void> = Promise.resolve();
  private timer?: ReturnType<typeof setTimeout>;
  private stopped = false;
  private tick?: Promise<void>;

  constructor(readonly path: string) {}

  /** Reject malformed snapshots rather than replacing recovery data with an empty object. */
  async load(): Promise<void> {
    let content: string;
    try {
      content = await readFile(this.path, 'utf8');
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === 'ENOENT') return;
      throw error;
    }
    const saved: unknown = JSON.parse(content);
    if (!isObject(saved) || saved.version !== 1 || !isObject(saved.workflows))
      throw new Error('Invalid workflow state snapshot');
    for (const [id, entry] of Object.entries(saved.workflows)) {
      if (
        !/^[0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12}$/.test(id) ||
        !isObject(entry) ||
        (entry.task !== null && (typeof entry.task !== 'string' || !entry.task)) ||
        typeof entry.open !== 'boolean' ||
        !isObject(entry.state)
      )
        throw new Error('Invalid saved workflow');
    }
    Object.assign(this.data.workflows, saved.workflows);
  }

  /** The next interval begins after maintenance and the write settle; timers never overlap.
   * Maintenance failures are reported before saving. close() waits for an in-flight tick. */
  start(
    onError: (error: unknown) => void,
    intervalMs = 60_000,
    beforeSave?: () => Promise<void>
  ): void {
    if (this.timer || this.stopped) return;
    const schedule = () => {
      if (this.stopped) return;
      this.timer = setTimeout(() => {
        this.tick = (async () => {
          await beforeSave?.().catch(onError);
          if (!this.stopped) await this.save();
        })()
          .catch(onError)
          .finally(schedule);
      }, intervalMs);
      this.timer.unref();
    };
    schedule();
  }

  /** Capture synchronously, then queue the disk operation. Later mutations belong to the
   * next snapshot. A failed save leaves the previous file intact and rejects to its caller. */
  save(): Promise<void> {
    let content: string;
    try {
      for (const entry of Object.values(this.data.workflows)) assertJson(entry.state);
      content = JSON.stringify(this.data);
    } catch (error) {
      return Promise.reject(error);
    }
    const next = this.writes.then(async () => {
      const temporary = `${this.path}.${process.pid}.tmp`;
      try {
        await mkdir(dirname(this.path), { recursive: true });
        await writeFile(temporary, content, { mode: 0o600 });
        await rename(temporary, this.path);
      } finally {
        await rm(temporary, { force: true });
      }
    });
    this.writes = next.catch(() => {});
    return next;
  }

  /** Stop periodic writes before task cleanup; close() saves the final mutations afterwards. */
  stop(): void {
    this.stopped = true;
    clearTimeout(this.timer);
  }

  async close(): Promise<void> {
    this.stop();
    await this.tick;
    await this.save();
  }
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

/** Catch silent JSON conversions as well as cycles, so a bad state never replaces a good one. */
function assertJson(value: unknown, parents = new Set<object>()): void {
  if (value === null || typeof value === 'string' || typeof value === 'boolean') return;
  if (typeof value === 'number' && Number.isFinite(value)) return;
  if (
    typeof value !== 'object' ||
    value === null ||
    (!Array.isArray(value) &&
      Object.getPrototypeOf(value) !== Object.prototype &&
      Object.getPrototypeOf(value) !== null) ||
    parents.has(value)
  )
    throw new TypeError('Workflow state must contain only JSON data');
  parents.add(value);
  for (const item of Object.values(value)) assertJson(item, parents);
  parents.delete(value);
}
