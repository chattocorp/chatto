/** Durable Botto work. One process must own each server/account data file. */
import { createHash } from 'node:crypto';
import { mkdirSync, readFileSync, renameSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import type { IncomingMessage } from './messages.ts';

interface Work {
  message: IncomingMessage;
  /** Running work can have external effects and must not be repeated automatically. */
  state: 'queued' | 'running';
}

interface State {
  version: 1;
  since: number;
  work: Work[];
  completed: Record<string, number>;
}

/** Save before enqueueing and before external effects. Atomic replacement protects
 * against process termination during a write. A corrupt file stops startup. */
export class Delivery {
  private readonly path: string;
  private state: State;

  constructor(directory: string, identity: readonly string[], now = Date.now()) {
    mkdirSync(directory, { recursive: true, mode: 0o700 });
    const key = createHash('sha256').update(JSON.stringify(identity)).digest('hex');
    this.path = join(directory, `delivery-${key}.json`);
    try {
      const data = JSON.parse(readFileSync(this.path, 'utf8')) as State;
      if (
        data.version !== 1 ||
        !Number.isFinite(data.since) ||
        !Array.isArray(data.work) ||
        !data.completed ||
        typeof data.completed !== 'object' ||
        data.work.some((item) => !item.message?.id || !['queued', 'running'].includes(item.state))
      )
        throw new Error('Invalid Botto delivery state');
      this.state = data;
    } catch (error) {
      if (!error || typeof error !== 'object' || !('code' in error) || error.code !== 'ENOENT')
        throw error;
      this.state = { version: 1, since: now, work: [], completed: {} };
      this.save(this.state);
    }
  }

  get since(): number {
    return this.state.since;
  }
  get queued(): IncomingMessage[] {
    return this.state.work.filter((item) => item.state === 'queued').map((item) => item.message);
  }
  get uncertain(): number {
    return this.state.work.filter((item) => item.state === 'running').length;
  }

  has(id: string): boolean {
    return (
      Object.hasOwn(this.state.completed, id) ||
      this.state.work.some((item) => item.message.id === id)
    );
  }

  /** Keep overflow on disk until the bounded in-memory inbox has capacity. */
  accept(message: IncomingMessage): void {
    if (this.has(message.id)) return;
    this.save({ ...this.state, work: [...this.state.work, { message, state: 'queued' }] });
  }

  /** Call immediately before the first operation that can have external effects. */
  start(messages: readonly IncomingMessage[]): void {
    const ids = new Set(messages.map((item) => item.id));
    this.save({
      ...this.state,
      work: this.state.work.map((item) =>
        ids.has(item.message.id) ? { ...item, state: 'running' } : item
      )
    });
  }

  complete(messages: readonly IncomingMessage[]): void {
    const ids = new Set(messages.map((item) => item.id));
    const completed = { ...this.state.completed };
    for (const id of ids) completed[id] = Date.now();
    this.save({
      ...this.state,
      completed,
      work: this.state.work.filter((item) => !ids.has(item.message.id))
    });
  }

  /** Keep the initial boundary: notification pages use live offsets, so removals
   * can move a source past a page boundary. A later complete scan repairs that
   * omission. Server retention bounds the scan to at most 90 days. */
  prune(): void {
    const oldest = Date.now() - 90 * 24 * 60 * 60 * 1000;
    const entries = Object.entries(this.state.completed);
    const retained = entries.filter(([, time]) => time >= oldest);
    if (retained.length !== entries.length)
      this.save({ ...this.state, completed: Object.fromEntries(retained) });
  }

  /** Publish new state only after the file replacement succeeds. Failed writes
   * leave the previous in-memory state intact, without a rollback copy. */
  private save(state: State): void {
    writeFileSync(`${this.path}.new`, JSON.stringify(state), { mode: 0o600, flush: true });
    renameSync(`${this.path}.new`, this.path);
    this.state = state;
  }
}
