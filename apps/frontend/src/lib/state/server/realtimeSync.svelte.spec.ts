import { describe, expect, it } from 'vitest';
import { RealtimeProjectionSyncState } from './realtimeSync.svelte';

describe('RealtimeProjectionSyncState', () => {
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
