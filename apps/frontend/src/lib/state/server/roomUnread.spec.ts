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

  it('preserves an optimistic read across a stale directory refresh', () => {
    const store = new RoomUnreadStore();
    store.setRoomUnread('room-1', true);
    const snapshotRevision = store.captureSnapshotRevision();

    const read = store.beginOptimisticRead('room-1');
    store.initRooms([{ id: 'room-1', hasUnread: true }], false, snapshotRevision);

    expect(store.roomIsUnread('room-1')).toBe(false);

    read.commit();

    expect(store.roomIsUnread('room-1')).toBe(false);
  });

  it('reveals refreshed unread state when an optimistic read rolls back', () => {
    const store = new RoomUnreadStore();
    const snapshotRevision = store.captureSnapshotRevision();

    const read = store.beginOptimisticRead('room-1');
    store.initRooms([{ id: 'room-1', hasUnread: true }], false, snapshotRevision);
    read.rollback();

    expect(store.roomIsUnread('room-1')).toBe(true);
  });

  it('does not let a stale directory refresh overwrite a successful read', () => {
    const store = new RoomUnreadStore();
    store.setRoomUnread('room-1', true);
    const snapshotRevision = store.captureSnapshotRevision();

    const read = store.beginOptimisticRead('room-1');
    read.commit();
    store.initRooms([{ id: 'room-1', hasUnread: true }], false, snapshotRevision);

    expect(store.roomIsUnread('room-1')).toBe(false);
  });

  it('does not let a stale directory refresh overwrite a live read event', () => {
    const store = new RoomUnreadStore();
    store.setRoomUnread('room-1', true);
    const snapshotRevision = store.captureSnapshotRevision();

    const read = store.beginOptimisticRead('room-1');
    store.setRoomUnread('room-1', false);
    store.initRooms([{ id: 'room-1', hasUnread: true }], false, snapshotRevision);
    read.rollback();

    expect(store.roomIsUnread('room-1')).toBe(false);
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

  it('preserves an unrelated unknown unread during a room read', () => {
    const store = new RoomUnreadStore();
    store.initRooms([{ id: 'room-1', hasUnread: false }], true);

    const read = store.beginOptimisticRead('room-1');
    expect(store.roomIsUnread('room-1')).toBe(false);
    expect(store.hasAnyUnread).toBe(true);

    read.commit();
    expect(store.hasAnyUnread).toBe(true);

    store.resolveUnknownUnread();
    expect(store.hasAnyUnread).toBe(false);
  });

  it('does not add an unknown sentinel when the aggregate is represented', () => {
    const store = new RoomUnreadStore();
    store.initRooms([{ id: 'channel', hasUnread: true }]);

    const read = store.beginOptimisticRead('channel');

    expect(store.roomIsUnread('channel')).toBe(false);
    expect(store.hasAnyUnread).toBe(false);

    read.commit();
    expect(store.hasAnyUnread).toBe(false);
  });

  it('keeps a channel aggregate when only a DM unread is concrete', () => {
    const store = new RoomUnreadStore();
    store.initRooms([{ id: 'dm', hasUnread: true }], true);

    const read = store.beginOptimisticRead('dm');

    expect(store.roomIsUnread('dm')).toBe(false);
    expect(store.hasAnyUnread).toBe(true);

    read.commit();
    expect(store.hasAnyUnread).toBe(true);
  });

  it('keeps a room read optimistic when a coarse unread signal arrives', () => {
    const store = new RoomUnreadStore();
    store.setRoomUnread('room-1', true);

    const read = store.beginOptimisticRead('room-1');
    store.setServerHasUnread(true);

    expect(store.roomIsUnread('room-1')).toBe(false);

    read.rollback();
    expect(store.roomIsUnread('room-1')).toBe(true);
  });
});
