import { describe, expect, it } from 'vitest';
import { RoomUnreadStore } from './roomUnread.svelte';

describe('RoomUnreadStore', () => {
  it('initializes room unread state from an authoritative directory snapshot', () => {
    const store = new RoomUnreadStore();

    store.initRooms([
      { id: 'read', hasUnread: false },
      { id: 'unread', hasUnread: true }
    ]);

    expect(store.roomIsUnread('read')).toBe(false);
    expect(store.roomIsUnread('unread')).toBe(true);
    expect(store.getFirstUnreadRoomId()).toBe('unread');
    expect(store.hasAnyUnread).toBe(true);
  });

  it('merges partial room snapshots without dropping other known unread rooms', () => {
    const store = new RoomUnreadStore();
    store.initRooms([{ id: 'channel', hasUnread: true }]);

    store.updateRooms([{ id: 'dm', hasUnread: true }]);

    expect(store.roomIsUnread('channel')).toBe(true);
    expect(store.roomIsUnread('dm')).toBe(true);
  });

  it('hides unread state immediately and restores it on rollback', () => {
    const store = new RoomUnreadStore();
    store.setRoomUnread('room-1', true);

    const read = store.beginOptimisticRead('room-1');

    expect(store.roomIsUnread('room-1')).toBe(false);
    expect(store.hasAnyUnread).toBe(false);

    read.rollback();

    expect(store.roomIsUnread('room-1')).toBe(true);
    expect(store.hasAnyUnread).toBe(true);
  });

  it('does not let rollback erase a newer unread message', () => {
    const store = new RoomUnreadStore();
    store.setRoomUnread('room-1', true);

    const read = store.beginOptimisticRead('room-1');
    store.setRoomUnread('room-1', true);
    read.rollback();

    expect(store.roomIsUnread('room-1')).toBe(true);
  });

  it('does not let rollback restore unread after an authoritative read event', () => {
    const store = new RoomUnreadStore();
    store.setRoomUnread('room-1', true);

    const read = store.beginOptimisticRead('room-1');
    store.setRoomUnread('room-1', false);
    read.rollback();

    expect(store.roomIsUnread('room-1')).toBe(false);
  });

  it('lets only the latest overlapping read settle the optimistic state', () => {
    const store = new RoomUnreadStore();
    store.setRoomUnread('room-1', true);

    const first = store.beginOptimisticRead('room-1');
    const second = store.beginOptimisticRead('room-1');
    first.commit();
    second.rollback();

    expect(store.roomIsUnread('room-1')).toBe(true);

    const latest = store.beginOptimisticRead('room-1');
    latest.commit();

    expect(store.roomIsUnread('room-1')).toBe(false);
  });

  it('restores an unknown server unread signal when an optimistic read fails', () => {
    const store = new RoomUnreadStore();
    store.setServerHasUnread(true);

    const read = store.beginOptimisticRead('room-1');
    expect(store.hasAnyUnread).toBe(false);

    read.rollback();
    expect(store.hasAnyUnread).toBe(true);
  });
});
