/** How current one server's retained client projection is. */
export type RealtimeProjectionPhase = 'empty' | 'hydrating' | 'ready' | 'stale';

/**
 * Session-local resume state for one server projection.
 *
 * The opaque cursor is deliberately owned by the projection rather than a
 * WebSocket. It is never persisted: a cursor without the exact in-memory
 * projection it advances is meaningless and must not survive a page reload.
 */
export class RealtimeProjectionSyncState {
  phase = $state<RealtimeProjectionPhase>('empty');
  lastCaughtUpAt = $state<number | null>(null);
  #resumeCursor = $state<string | null>(null);

  get resumeCursor(): string | null {
    return this.#resumeCursor;
  }

  get hasUsableProjection(): boolean {
    return this.phase === 'ready' || this.phase === 'stale';
  }

  beginCatchUp(): void {
    if (this.phase === 'empty') this.phase = 'hydrating';
  }

  /** Advance only after every projection reducer accepted the event. */
  acceptProjectionEvent(cursor: string | undefined, reset: boolean): void {
    if (reset) this.phase = 'hydrating';
    if (cursor) this.#resumeCursor = cursor;
  }

  markCaughtUp(cursor: string | undefined): void {
    if (cursor) this.#resumeCursor = cursor;
    this.phase = 'ready';
    this.lastCaughtUpAt = Date.now();
  }

  markStale(): void {
    if (this.phase === 'ready') this.phase = 'stale';
  }

  reset(): void {
    this.phase = 'empty';
    this.lastCaughtUpAt = null;
    this.#resumeCursor = null;
  }
}
