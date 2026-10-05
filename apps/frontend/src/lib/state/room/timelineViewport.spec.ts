import { describe, expect, it, vi } from 'vitest';
import type { MessagesStore, TimelineAnchor } from '@chatto/client/room/messages/MessagesStore';
import {
  clearTimelineViewport,
  isLoadingOlder,
  loadOlder,
  recoveryViewport,
  setTimelineViewport
} from './timelineViewport';

function fakeStore() {
  const state = {
    accept: true,
    anchor: null as string | null,
    recovery: null as TimelineAnchor | null
  };
  const store = {
    setAnchor: vi.fn((eventId: string | null) => {
      if (!state.accept) return false;
      state.anchor = eventId;
      return true;
    }),
    clearAnchor: vi.fn(() => {
      state.anchor = null;
      state.recovery = null;
    }),
    get recoveryAnchor() {
      return state.recovery;
    },
    isLoadingMore: false,
    loadMore: vi.fn(async () => {})
  };
  return { store: store as unknown as MessagesStore, state, loadMore: store.loadMore };
}

describe('timeline viewport', () => {
  it('keeps the offset of an accepted anchor for recovery', () => {
    const { store, state } = fakeStore();
    setTimelineViewport(store, { eventId: 'E1', offset: 17 });
    state.accept = false;
    // A recovering store keeps its anchor, so the offset stays too.
    setTimelineViewport(store, { eventId: 'E2', offset: 99 });
    state.recovery = { eventId: 'E1', hasNewer: true };
    expect(recoveryViewport(store)).toEqual({ eventId: 'E1', offset: 17, hasNewer: true });

    clearTimelineViewport(store);
    expect(recoveryViewport(store)).toBeNull();
  });

  it('pages older events once until the list settled', async () => {
    const { store, loadMore } = fakeStore();
    await Promise.all([loadOlder(store), loadOlder(store)]);
    expect(loadMore).toHaveBeenCalledOnce();
    await loadOlder(store);
    expect(loadMore).toHaveBeenCalledTimes(2);
  });

  it('reports loading until the list settled one frame after the page', async () => {
    const { store, loadMore } = fakeStore();
    let finish!: () => void;
    loadMore.mockImplementationOnce(() => new Promise<void>((resolve) => (finish = resolve)));
    const loading = loadOlder(store);
    expect(isLoadingOlder(store)).toBe(true);
    finish();
    // The page arrived; the list still settles, so the flag stays true.
    await Promise.resolve();
    await Promise.resolve();
    expect(isLoadingOlder(store)).toBe(true);
    await loading;
    expect(isLoadingOlder(store)).toBe(false);
  });
});
