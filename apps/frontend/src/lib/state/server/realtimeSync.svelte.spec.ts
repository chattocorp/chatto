import { describe, expect, it } from 'vitest';
import { MAX_RETAINED_ROOM_TIMELINES, RealtimeProjectionSyncState } from './realtimeSync.svelte';

describe('RealtimeProjectionSyncState', () => {
  it('resolves a refresh waiter only after a later caught-up boundary', async () => {
    const state = new RealtimeProjectionSyncState();
    state.markCaughtUp('cursor-before');
    let settled = false;
    const refreshed = state.waitForNextCaughtUp().then((result) => {
      settled = true;
      return result;
    });

    await Promise.resolve();
    expect(settled).toBe(false);
    state.markCaughtUp('cursor-after');

    await expect(refreshed).resolves.toBe(true);
    expect(state.resumeCursor).toBe('cursor-after');
  });

  it('retains mounted projection state and its cursor during an authorization refresh', () => {
    const state = new RealtimeProjectionSyncState();
    state.retainRoom('R1');
    state.confirmRoom('R1');
    state.markCaughtUp('cursor-before');

    state.invalidateAuthorization();

    expect(state.phase).toBe('stale');
    expect(state.hasUsableProjection).toBe(true);
    expect(state.resumeCursor).toBe('cursor-before');
    expect(state.authorizationRefreshRequired).toBe(true);
    expect(state.pendingAuthorizationRefreshGeneration).toBe(1);
    expect(state.desiredRoomIds).toEqual(['R1']);
    expect(state.retainedRoomIds).toEqual(['R1']);

    state.markCaughtUp('cursor-after', 1);

    expect(state.authorizationRefreshRequired).toBe(false);
  });

  it('does not lose an authorization invalidation that arrives during reconciliation', () => {
    const state = new RealtimeProjectionSyncState();
    state.markCaughtUp('cursor-before');
    state.invalidateAuthorization();
    const requestedGeneration = state.pendingAuthorizationRefreshGeneration;

    state.invalidateAuthorization();
    state.markCaughtUp('cursor-between', requestedGeneration);

    expect(state.authorizationRefreshRequired).toBe(true);
    expect(state.pendingAuthorizationRefreshGeneration).toBe(2);
    expect(state.phase).toBe('stale');
    expect(state.lastCaughtUpAt).toBeNull();

    state.markCaughtUp('cursor-after', state.pendingAuthorizationRefreshGeneration);
    expect(state.authorizationRefreshRequired).toBe(false);
  });

  it('does not complete an authorization waiter at an unrelated caught-up boundary', async () => {
    const state = new RealtimeProjectionSyncState();
    state.markCaughtUp('cursor-before');
    const requestedGeneration = state.invalidateAuthorization();
    let settled = false;
    const refreshed = state
      .waitForAuthorizationRefresh(requestedGeneration)
      .then((result) => {
        settled = true;
        return result;
      });

    state.markCaughtUp('cursor-unrelated');
    await Promise.resolve();
    expect(settled).toBe(false);
    expect(state.phase).toBe('stale');

    state.markCaughtUp('cursor-after', requestedGeneration);
    await expect(refreshed).resolves.toBe(true);
  });

  it('keeps an opaque cursor attached to the retained projection across socket lifetimes', () => {
    const state = new RealtimeProjectionSyncState();

    state.beginCatchUp();
    expect(state.phase).toBe('hydrating');
    state.acceptProjectionEvent('event-cursor', true);
    expect(state.resumeCursor).toBe('event-cursor');
    state.markCaughtUp('boundary-cursor');
    expect(state.phase).toBe('ready');
    expect(state.resumeCursor).toBe('boundary-cursor');

    state.markStale();
    expect(state.phase).toBe('stale');
    expect(state.hasUsableProjection).toBe(true);
    expect(state.resumeCursor).toBe('boundary-cursor');
  });

  it('distinguishes desired rooms from materialized rooms and clears both on reset', () => {
    const sync = new RealtimeProjectionSyncState();

    expect(sync.retainRoom('R1')).toBeNull();
    expect(sync.retainRoom('R1')).toBeNull();
    expect(sync.retainRoom('R2')).toBeNull();
    expect(sync.desiredRoomIds).toEqual(['R1', 'R2']);
    expect(sync.retainedRoomIds).toEqual([]);

    sync.confirmRoom('R1');
    sync.confirmRoom('not-requested');
    expect(sync.retainedRoomIds).toEqual(['R1']);

    sync.acceptProjectionEvent(undefined, true);
    expect(sync.desiredRoomIds).toEqual(['R1', 'R2']);
    expect(sync.retainedRoomIds).toEqual([]);

    sync.confirmRoom('R2');
    expect(sync.retainedRoomIds).toEqual(['R2']);

    sync.reset();
    expect(sync.desiredRoomIds).toEqual([]);
    expect(sync.retainedRoomIds).toEqual([]);
  });

  it('evicts the least-recent room at the server subscription limit', () => {
    const sync = new RealtimeProjectionSyncState();
    for (let index = 0; index < MAX_RETAINED_ROOM_TIMELINES; index++) {
      expect(sync.retainRoom(`R${index}`)).toBeNull();
      sync.confirmRoom(`R${index}`);
    }

    expect(sync.retainRoom('R0')).toBeNull();
    expect(sync.retainRoom('overflow')).toBe('R1');
    expect(sync.desiredRoomIds).toHaveLength(MAX_RETAINED_ROOM_TIMELINES);
    expect(sync.desiredRoomIds).not.toContain('R1');
    expect(sync.desiredRoomIds.at(-1)).toBe('overflow');
    expect(sync.retainedRoomIds).toHaveLength(MAX_RETAINED_ROOM_TIMELINES - 1);
    expect(sync.takeTransportEvictions()).toEqual(['R1']);

    sync.acceptProjectionEvent(undefined, true);
    expect(sync.desiredRoomIds).toHaveLength(MAX_RETAINED_ROOM_TIMELINES);
    expect(sync.retainedRoomIds).toEqual([]);
  });

  it('clears cursor and readiness only when the owning projection is discarded', () => {
    const state = new RealtimeProjectionSyncState();
    state.markCaughtUp('cursor');

    state.reset();

    expect(state.phase).toBe('empty');
    expect(state.hasUsableProjection).toBe(false);
    expect(state.resumeCursor).toBeNull();
    expect(state.lastCaughtUpAt).toBeNull();
  });
});
