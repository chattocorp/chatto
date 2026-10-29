/** Bounded, process-local batches with one execution at a time for each key. */
export interface BatchQueueOptions<Item> {
  key(item: Item): string;
  id(item: Item): string;
  /** Quiet period after the latest item or touch. May depend on the complete queued batch. */
  delay(items: readonly Item[]): number;
  run(items: readonly Item[]): Promise<void>;
  failed(error: unknown): void;
  /** Bounds include running batches and batches waiting for their quiet period. */
  maxKeys: number;
  maxItems: number;
}

interface Batch<Item> {
  items: Item[];
  timer?: ReturnType<typeof setTimeout>;
  running?: Promise<void>;
}

/** Owns serialization, quiet-period timers, capacity, and bounded duplicate detection.
 * Failed items can be admitted again. Persistence and retry policy belong to the source. */
export class BatchQueue<Item> {
  private readonly batches = new Map<string, Batch<Item>>();
  private readonly seen = new Set<string>();
  private pending = 0;
  private closed = false;

  constructor(private readonly options: BatchQueueOptions<Item>) {
    for (const value of [options.maxKeys, options.maxItems])
      if (!Number.isSafeInteger(value) || value < 1)
        throw new RangeError('Batch limits must be positive safe integers');
  }

  /** Includes queued and executing work. Idle keys do not consume capacity. */
  has(key: string): boolean {
    return this.batches.has(key);
  }

  /** Inspect only queued items. The returned array must not be modified. */
  queued(key: string): readonly Item[] {
    return this.batches.get(key)?.items ?? [];
  }

  /** Delay matching queued batches. Running work is never interrupted. */
  touch(matches: (items: readonly Item[], key: string) => boolean): void {
    if (this.closed) return;
    for (const [key, batch] of this.batches)
      if (batch.items.length && matches(batch.items, key)) this.schedule(key, batch);
  }

  /** Admit an item without waiting for execution. Full queues leave retry to the caller. */
  accept(item: Item): 'accepted' | 'duplicate' | 'full' | 'closed' {
    if (this.closed) return 'closed';
    const { key: keyOf, id: idOf, maxKeys, maxItems } = this.options;
    const key = keyOf(item),
      id = idOf(item);
    if (this.seen.has(id)) return 'duplicate';
    if (this.pending >= maxItems || (!this.batches.has(key) && this.batches.size >= maxKeys))
      return 'full';
    const batch = this.batches.get(key) ?? { items: [] };
    // Validate policy before changing admission state.
    this.delay([...batch.items, item]);
    this.seen.add(id);
    if (this.seen.size > Math.max(10_000, maxItems))
      this.seen.delete(this.seen.values().next().value!);
    this.pending++;
    this.batches.set(key, batch);
    batch.items.push(item);
    this.schedule(key, batch);
    return 'accepted';
  }

  private delay(items: readonly Item[]): number {
    const delay = this.options.delay(items);
    if (!Number.isFinite(delay) || delay < 0 || delay > 2_147_483_647)
      throw new RangeError('Batch delay must be between 0 and 2147483647');
    return delay;
  }

  private schedule(key: string, batch: Batch<Item>): void {
    const delay = this.delay(batch.items);
    clearTimeout(batch.timer);
    batch.timer = setTimeout(() => {
      batch.timer = undefined;
      this.flush(key, batch);
    }, delay);
  }

  private flush(key: string, batch: Batch<Item>): void {
    if (this.closed || batch.running || batch.timer || !batch.items.length) return;
    const items = batch.items.splice(0);
    batch.running = Promise.resolve()
      .then(() => {
        if (!this.closed) return this.options.run(items);
      })
      .catch((error) => {
        for (const item of items) this.seen.delete(this.options.id(item));
        this.options.failed(error);
      })
      .finally(() => {
        this.pending -= items.length;
        batch.running = undefined;
        if (!batch.items.length) this.batches.delete(key);
        else this.flush(key, batch);
      });
    // A throwing failure observer must not cause an unhandled rejection.
    void batch.running.catch(() => {});
  }

  /** Discard queued work and await running callbacks. The caller first signals cancellation. */
  async close(): Promise<void> {
    this.closed = true;
    for (const batch of this.batches.values()) {
      clearTimeout(batch.timer);
      this.pending -= batch.items.length;
      batch.items = [];
    }
    await Promise.allSettled([...this.batches.values()].map((batch) => batch.running));
    this.batches.clear();
  }
}
