import { describe, expect, it } from 'vitest';
import {
  MAX_RETAINED_ROOM_TIMELINES,
  RealtimeProjectionSyncState
} from './realtimeSync.svelte';

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
