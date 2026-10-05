import { describe, expect, it } from 'vitest';
import { PendingHighlightStore } from './pendingHighlight';

describe('PendingHighlightStore', () => {
  it('retains a stable request until the destination acknowledges it', () => {
    const store = new PendingHighlightStore();
    store.set('room-1', null, 'message-1', 'notification-1');
    const request = store.peek('room-1', null)!;

    expect(store.peek('room-1', null)).toBe(request);
    expect(store.has('room-1', null)).toBe(true);
    expect(store.peek('room-1', 'thread-1')).toBeNull();
    store.complete(request);
    expect(store.current).toBeNull();
  });

  it('ignores a late completion after another click on the same message', () => {
    const store = new PendingHighlightStore();
    store.set('room-1', null, 'message-1');
    const first = store.current!;
    store.set('room-1', null, 'message-1');
    const second = store.current!;

    expect(second).not.toBe(first);
    store.complete(first);
    expect(store.peek('room-1', null)).toBe(second);
  });

  it('replaces the previous destination when the user selects a new target', () => {
    const store = new PendingHighlightStore();
    store.set('room-1', null, 'message-1');
    const first = store.current!;
    store.set('room-2', 'thread-2', 'message-2');
    store.complete(first);

    expect(store.has('room-1', null)).toBe(false);
    expect(store.peek('room-2', 'thread-2')?.eventId).toBe('message-2');
  });
});
