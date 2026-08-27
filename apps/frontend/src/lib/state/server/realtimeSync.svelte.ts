import { SvelteSet } from 'svelte/reactivity';

/** How current one server's retained client projection is. */
export type RealtimeProjectionPhase = 'empty' | 'hydrating' | 'ready' | 'stale';

export const MAX_RETAINED_ROOM_TIMELINES = 64;

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
  #desiredRoomIds = new SvelteSet<string>();
  #materializedRoomIds = new SvelteSet<string>();
  #pendingTransportEvictions: string[] = [];

  get resumeCursor(): string | null {
    return this.#resumeCursor;
  }

  get hasUsableProjection(): boolean {
    return this.phase === 'ready' || this.phase === 'stale';
  }

  /** Materialized room timelines safe to advertise alongside this cursor. */
  get retainedRoomIds(): string[] {
    return [...this.#materializedRoomIds];
  }

  /** Room timelines the UI wants, including hydration requests still in flight. */
  get desiredRoomIds(): string[] {
    return [...this.#desiredRoomIds];
  }

  /** Retain one room and return the least-recent room evicted at the wire limit. */
  retainRoom(roomId: string): string | null {
    if (!roomId) return null;
    if (this.#desiredRoomIds.delete(roomId)) {
      // Refresh insertion order so the bounded set behaves as an LRU.
      this.#desiredRoomIds.add(roomId);
      return null;
    }
    let evictedRoomId: string | null = null;
    if (this.#desiredRoomIds.size >= MAX_RETAINED_ROOM_TIMELINES) {
      evictedRoomId = this.#desiredRoomIds.values().next().value ?? null;
      if (evictedRoomId) {
        this.#desiredRoomIds.delete(evictedRoomId);
        this.#materializedRoomIds.delete(evictedRoomId);
        this.#pendingTransportEvictions.push(evictedRoomId);
      }
    }
    this.#desiredRoomIds.add(roomId);
    return evictedRoomId;
  }

  /** Evictions that require replacing an already-subscribed socket. */
  takeTransportEvictions(): string[] {
    return this.#pendingTransportEvictions.splice(0);
  }

  /** Mark a requested timeline as present in the reducer-owned projection. */
  confirmRoom(roomId: string): void {
    if (this.#desiredRoomIds.has(roomId)) this.#materializedRoomIds.add(roomId);
  }

  beginCatchUp(): void {
    if (this.phase === 'empty') this.phase = 'hydrating';
  }

  /** Advance only after every projection reducer accepted the event. */
  acceptProjectionEvent(cursor: string | undefined, reset: boolean): void {
    if (reset) {
      this.phase = 'hydrating';
      this.#materializedRoomIds.clear();
    }
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
    this.#desiredRoomIds.clear();
    this.#materializedRoomIds.clear();
    this.#pendingTransportEvictions = [];
  }
}
