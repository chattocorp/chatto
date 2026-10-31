import { LRUCache } from 'lru-cache';
import { signal } from '../reactivity/index.js';

/**
 * Shared lookups for handles retained by visible room content. Results contain
 * IDs only; the room reads current profiles from its normal profile owner.
 * At most four reads run at once. Unused results have a bounded memory cache.
 */
export class MentionLookups {
  readonly #retained = new Map<string, number>();
  readonly #cache = new LRUCache<string, string | false>({ max: 256 });
  readonly #matches = new Map<string, string | false>();
  readonly #pending = new Map<string, AbortController>();
  readonly #attempted = new Set<string>();
  readonly #ids = signal<string[]>([]);
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
      if (!this.#retained.has(key)) this.#attempted.delete(key);
      this.#retained.set(key, (this.#retained.get(key) ?? 0) + 1);
      const cached = this.#cache.get(key);
      if (cached !== undefined) this.#matches.set(key, cached);
    }
    this.resume();
    let released = false;
    return () => {
      if (released) return;
      released = true;
      for (const key of keys) {
        const count = (this.#retained.get(key) ?? 1) - 1;
        if (count) this.#retained.set(key, count);
        else {
          this.#retained.delete(key);
          this.#matches.delete(key);
          this.#attempted.delete(key);
          this.#pending.get(key)?.abort();
        }
      }
      this.publish();
    };
  }

  /** Fence old reads and discard matches after membership or authority changes. */
  invalidate(): void {
    for (const controller of this.#pending.values()) controller.abort();
    this.#cache.clear();
    this.#matches.clear();
    this.#attempted.clear();
    this.#ids.set([]);
  }

  /** Resume retained lookups after the room's authority has been checked. */
  resume(): void {
    if (this.#disposed || !this.canRead()) return;
    this.publish();
    for (const handle of this.#retained.keys()) {
      if (this.#pending.size >= 4) break;
      if (this.#matches.has(handle) || this.#pending.has(handle) || this.#attempted.has(handle))
        continue;
      const controller = new AbortController();
      this.#pending.set(handle, controller);
      this.#attempted.add(handle);
      void this.lookup(handle, controller.signal)
        .then((id) => {
          if (!controller.signal.aborted) {
            this.#cache.set(handle, id ?? false);
            this.#matches.set(handle, id ?? false);
          }
        })
        .catch(() => {
          // A failed lookup leaves plain text. A later retain or invalidation retries it.
        })
        .finally(() => {
          this.#pending.delete(handle);
          if (controller.signal.aborted) this.#attempted.delete(handle);
          this.resume();
        });
    }
  }

  /** Cancel pending reads and release all retained content. */
  dispose(): void {
    this.#disposed = true;
    this.invalidate();
    this.#retained.clear();
  }

  private publish(): void {
    const ids = [
      ...new Set(
        [...this.#retained.keys()].flatMap((handle) => {
          const id = this.#matches.get(handle);
          return id ? [id] : [];
        })
      )
    ];
    const previous = this.#ids.get();
    if (ids.length !== previous.length || ids.some((id, index) => id !== previous[index])) {
      this.#ids.set(ids);
    }
  }
}
