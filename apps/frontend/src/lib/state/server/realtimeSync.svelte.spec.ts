import { describe, expect, it, vi } from 'vitest';
import { RealtimeProjectionSyncState } from './realtimeSync.svelte';

describe('RealtimeProjectionSyncState', () => {
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
