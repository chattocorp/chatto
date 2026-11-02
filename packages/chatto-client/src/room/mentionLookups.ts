import { LRUCache } from 'lru-cache';
import { signal } from '../reactivity/index.js';

/** One handle's consumers, request, and outcome. Failed reads settle without a result. */
type MentionLookup = {
  readers: number;
  state: 'idle' | 'pending' | 'settled';
  result: string | false | undefined;
  controller?: AbortController;
};

/**
 * Shared lookups for handles retained by visible room content. Results contain
 * IDs only; the room reads current profiles from its normal profile owner.
 * At most four reads run at once. Unused results have a bounded memory cache.
 */
export class MentionLookups {
  readonly #entries = new Map<string, MentionLookup>();
  readonly #cache = new LRUCache<string, string | false>({ max: 256 });
  readonly #ids = signal<string[]>([]);
  #running = 0;
  #disposed = false;

  constructor(
    private readonly lookup: (handle: string, signal: AbortSignal) => Promise<string | null>,
    private readonly canRead: () => boolean
  ) {}

  /** IDs resolved for currently retained handles. */
  get ids(): string[] {
    return this.#ids.get();
  }

  /** Retain a set of handles until the returned, idempotent release runs. */
  retain(handles: readonly string[]): () => void {
    if (this.#disposed) return () => {};
    const keys = [...new Set(handles.map((handle) => handle.toLowerCase()))];
    for (const key of keys) {
      let entry = this.#entries.get(key);
      if (!entry) {
        const result = this.#cache.get(key);
        this.#cache.delete(key);
        entry = { readers: 0, state: result === undefined ? 'idle' : 'settled', result };
        this.#entries.set(key, entry);
      }
      entry.readers++;
    }
    this.resume();
    let released = false;
    return () => {
      if (released) return;
      released = true;
      for (const key of keys) {
        const entry = this.#entries.get(key);
        if (entry && --entry.readers === 0) {
          // A retained refresh result is not yet verified. Do not let an
          // unmounted consumer turn it into a settled cache entry.
          if (entry.state === 'settled' && entry.result !== undefined)
            this.#cache.set(key, entry.result);
          if (entry.controller) entry.controller.abort();
          else this.#entries.delete(key);
        }
      }
      this.publish();
    };
  }

  /** Fence selected reads and matches after profile, membership, or authority changes.
   * Recovery can retain results until a fresh lookup replaces them. */
  invalidate(
    affected: (handle: string, result: string | false | undefined) => boolean = () => true,
    retainResults = false
  ): void {
    for (const [handle, entry] of this.#entries) {
      if (!affected(handle, entry.result)) continue;
      if (!retainResults) entry.result = undefined;
      if (entry.controller) entry.controller.abort();
      else entry.state = 'idle';
    }
    for (const [handle, result] of this.#cache) {
      if (affected(handle, result)) this.#cache.delete(handle);
    }
    this.publish();
  }

  /** Resume retained lookups after the room's authority has been checked. */
  resume(): void {
    if (this.#disposed || !this.canRead()) return;
    this.publish();
    for (const [handle, entry] of this.#entries) {
      if (this.#running >= 4) break;
      if (!entry.readers || entry.state !== 'idle') continue;
      const controller = new AbortController();
      entry.controller = controller;
      entry.state = 'pending';
      this.#running++;
      void this.lookup(handle, controller.signal)
        .then((id) => {
          if (!controller.signal.aborted) entry.result = id ?? false;
        })
        .catch(() => {
          // A failed read keeps its previous result, if any. A later
          // invalidation, or release followed by retain, can retry it.
        })
        .finally(() => {
          this.#running--;
          entry.controller = undefined;
          entry.state = controller.signal.aborted ? 'idle' : 'settled';
          if (!entry.readers) this.#entries.delete(handle);
          this.resume();
        });
    }
  }

  /** Cancel pending reads and release all retained content. */
  dispose(): void {
    this.#disposed = true;
    this.invalidate();
    this.#entries.clear();
  }

  private publish(): void {
    const ids = [
      ...new Set(
        [...this.#entries.values()].flatMap(({ readers, result }) =>
          readers && result ? [result] : []
        )
      )
    ];
    const previous = this.#ids.get();
    if (ids.length !== previous.length || ids.some((id, index) => id !== previous[index])) {
      this.#ids.set(ids);
    }
  }
}
