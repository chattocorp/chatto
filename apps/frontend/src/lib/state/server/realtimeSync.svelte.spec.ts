import { describe, expect, it, vi } from 'vitest';
import { RealtimeProjectionSyncState } from './realtimeSync.svelte';

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
    state.markCaughtUp('cursor-before');

    state.invalidateAuthorization();

    expect(state.phase).toBe('stale');
    expect(state.hasUsableProjection).toBe(true);
    expect(state.resumeCursor).toBe('cursor-before');
    expect(state.authorizationRefreshRequired).toBe(true);
    expect(state.pendingAuthorizationRefreshGeneration).toBe(1);

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
    const refreshed = state.waitForAuthorizationRefresh(requestedGeneration).then((result) => {
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

  it('keeps one opaque cursor for the resource snapshot it advances', () => {
    const sync = new RealtimeProjectionSyncState();
    sync.beginCatchUp();
    sync.acceptProjectionEvent('event-cursor', false);
    sync.markCaughtUp('boundary-cursor');
    expect(sync.resumeCursor).toBe('boundary-cursor');
    expect(sync.phase).toBe('ready');
  });

  it('marks a disconnected ready snapshot stale', () => {
    vi.spyOn(Date, 'now').mockReturnValue(123);
    const sync = new RealtimeProjectionSyncState();
    sync.markCaughtUp('cursor');
    sync.markStale();
    expect(sync.phase).toBe('stale');
    expect(sync.lastCaughtUpAt).toBe(123);
  });
});
