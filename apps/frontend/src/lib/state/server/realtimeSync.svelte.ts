import { SvelteSet } from 'svelte/reactivity';

/** How current one server's retained client projection is. */
export type RealtimeProjectionPhase = 'empty' | 'hydrating' | 'ready' | 'stale';

export const MAX_RETAINED_ROOM_TIMELINES = 64;
const PROJECTION_REFRESH_TIMEOUT_MS = 30_000;

type CatchUpWaiter = {
  afterGeneration: number;
  authorizationRefreshGeneration: number;
  resolve: (caughtUp: boolean) => void;
  timeout: ReturnType<typeof setTimeout>;
};

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
  #authorizationRefreshGeneration = 0;
  #completedAuthorizationRefreshGeneration = 0;
  #caughtUpGeneration = 0;
  #catchUpWaiters = new SvelteSet<CatchUpWaiter>();

  get resumeCursor(): string | null {
    return this.#resumeCursor;
  }

  /** Whether the next resumed transport must replace effective permission state. */
  get authorizationRefreshRequired(): boolean {
    return (
      this.#completedAuthorizationRefreshGeneration < this.#authorizationRefreshGeneration
    );
  }

  /** Generation that a reconnect must reconcile, or zero when none is pending. */
  get pendingAuthorizationRefreshGeneration(): number {
    return this.authorizationRefreshRequired ? this.#authorizationRefreshGeneration : 0;
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

  markCaughtUp(cursor: string | undefined, authorizationRefreshGeneration = 0): void {
    if (cursor) this.#resumeCursor = cursor;
    this.#completedAuthorizationRefreshGeneration = Math.max(
      this.#completedAuthorizationRefreshGeneration,
      authorizationRefreshGeneration
    );
    this.phase = 'ready';
    this.lastCaughtUpAt = Date.now();
    this.#caughtUpGeneration++;
    for (const waiter of this.#catchUpWaiters) {
      if (waiter.afterGeneration >= this.#caughtUpGeneration) continue;
      if (
        waiter.authorizationRefreshGeneration >
        this.#completedAuthorizationRefreshGeneration
      )
        continue;
      clearTimeout(waiter.timeout);
      this.#catchUpWaiters.delete(waiter);
      waiter.resolve(true);
    }
  }

  /** Wait for a later transport to finish applying its projection prefix. */
  waitForNextCaughtUp(timeoutMs = PROJECTION_REFRESH_TIMEOUT_MS): Promise<boolean> {
    return this.#waitForCaughtUp(0, timeoutMs);
  }

  /** Wait until a transport reconciles at least the requested authority generation. */
  waitForAuthorizationRefresh(
    authorizationRefreshGeneration: number,
    timeoutMs = PROJECTION_REFRESH_TIMEOUT_MS
  ): Promise<boolean> {
    return this.#waitForCaughtUp(authorizationRefreshGeneration, timeoutMs);
  }

  #waitForCaughtUp(
    authorizationRefreshGeneration: number,
    timeoutMs: number
  ): Promise<boolean> {
    const afterGeneration = this.#caughtUpGeneration;
    return new Promise<boolean>((resolve) => {
      const waiter: CatchUpWaiter = {
        afterGeneration,
        authorizationRefreshGeneration,
        resolve,
        timeout: setTimeout(() => {
          this.#catchUpWaiters.delete(waiter);
          resolve(false);
        }, timeoutMs)
      };
      this.#catchUpWaiters.add(waiter);
    });
  }

  markStale(): void {
    if (this.phase === 'ready') this.phase = 'stale';
  }

  /** Keep mounted state while the next transport refreshes effective permissions. */
  invalidateAuthorization(): number {
    this.markStale();
    this.lastCaughtUpAt = null;
    this.#authorizationRefreshGeneration++;
    return this.#authorizationRefreshGeneration;
  }

  reset(): void {
    this.phase = 'empty';
    this.lastCaughtUpAt = null;
    this.#resumeCursor = null;
    this.#desiredRoomIds.clear();
    this.#materializedRoomIds.clear();
    this.#pendingTransportEvictions = [];
    this.#authorizationRefreshGeneration = 0;
    this.#completedAuthorizationRefreshGeneration = 0;
    for (const waiter of this.#catchUpWaiters) {
      clearTimeout(waiter.timeout);
      waiter.resolve(false);
    }
    this.#catchUpWaiters.clear();
  }
}
