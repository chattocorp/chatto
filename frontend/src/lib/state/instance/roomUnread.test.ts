import { describe, it, expect, beforeEach } from 'vitest';
import { RoomUnreadStore } from './roomUnread.svelte';

describe('RoomUnreadStore', () => {
  let store: RoomUnreadStore;

  beforeEach(() => {
    store = new RoomUnreadStore();
  });

  describe('setRoomUnread', () => {
    it('marks a room as unread', () => {
      store.setRoomUnread('space1', 'room1', true);
      expect(store.roomIsUnread('space1', 'room1')).toBe(true);
    });

    it('marks a room as read', () => {
      store.setRoomUnread('space1', 'room1', true);
      store.setRoomUnread('space1', 'room1', false);
      expect(store.roomIsUnread('space1', 'room1')).toBe(false);
    });

    it('tracks multiple rooms per space', () => {
      store.setRoomUnread('space1', 'room1', true);
      store.setRoomUnread('space1', 'room2', true);
      expect(store.roomIsUnread('space1', 'room1')).toBe(true);
      expect(store.roomIsUnread('space1', 'room2')).toBe(true);
    });

    it('tracks rooms across spaces independently', () => {
      store.setRoomUnread('space1', 'room1', true);
      store.setRoomUnread('space2', 'room1', false);
      expect(store.roomIsUnread('space1', 'room1')).toBe(true);
      expect(store.roomIsUnread('space2', 'room1')).toBe(false);
    });
  });

  describe('spaceHasUnread', () => {
    it('returns false for empty store', () => {
      expect(store.spaceHasUnread('space1')).toBe(false);
    });

    it('returns true when any room is unread', () => {
      store.setRoomUnread('space1', 'room1', true);
      expect(store.spaceHasUnread('space1')).toBe(true);
    });

    it('returns false when all rooms are read', () => {
      store.setRoomUnread('space1', 'room1', true);
      store.setRoomUnread('space1', 'room1', false);
      expect(store.spaceHasUnread('space1')).toBe(false);
    });
  });

  describe('getFirstUnreadRoomId', () => {
    it('returns null when no unread rooms', () => {
      expect(store.getFirstUnreadRoomId('space1')).toBeNull();
    });

    it('returns a room id when rooms are unread', () => {
      store.setRoomUnread('space1', 'room1', true);
      expect(store.getFirstUnreadRoomId('space1')).toBe('room1');
    });

    it('returns null when only the unknown-unread flag is set', () => {
      store.setSpaceHasUnread('space1', true);
      expect(store.getFirstUnreadRoomId('space1')).toBeNull();
    });
  });

  describe('initSpaceRooms', () => {
    it('initializes unread state from room data', () => {
      store.initSpaceRooms('space1', [
        { id: 'room1', hasUnread: true },
        { id: 'room2', hasUnread: false },
        { id: 'room3', hasUnread: true }
      ]);
      expect(store.roomIsUnread('space1', 'room1')).toBe(true);
      expect(store.roomIsUnread('space1', 'room2')).toBe(false);
      expect(store.roomIsUnread('space1', 'room3')).toBe(true);
    });

    it('clears previous state for the space', () => {
      store.setRoomUnread('space1', 'roomOld', true);
      store.initSpaceRooms('space1', [{ id: 'roomNew', hasUnread: true }]);
      expect(store.roomIsUnread('space1', 'roomOld')).toBe(false);
      expect(store.roomIsUnread('space1', 'roomNew')).toBe(true);
    });
  });

  describe('setSpaceHasUnread (unknown-unread flag)', () => {
    it('sets space-level unread without specific rooms', () => {
      store.setSpaceHasUnread('space1', true);
      expect(store.spaceHasUnread('space1')).toBe(true);
    });

    it('unknown-unread flag is cleared when marking a specific room as read', () => {
      store.setSpaceHasUnread('space1', true);
      store.setRoomUnread('space1', 'room1', false);
      expect(store.spaceHasUnread('space1')).toBe(false);
    });

    it('clears space unread', () => {
      store.setSpaceHasUnread('space1', true);
      store.setSpaceHasUnread('space1', false);
      expect(store.spaceHasUnread('space1')).toBe(false);
    });
  });

  describe('global counters', () => {
    it('counts unread spaces', () => {
      expect(store.unreadSpaceCount).toBe(0);
      store.setRoomUnread('space1', 'room1', true);
      expect(store.unreadSpaceCount).toBe(1);
      store.setRoomUnread('space2', 'room1', true);
      expect(store.unreadSpaceCount).toBe(2);
    });

    it('hasAnyUnread reflects state', () => {
      expect(store.hasAnyUnread).toBe(false);
      store.setRoomUnread('space1', 'room1', true);
      expect(store.hasAnyUnread).toBe(true);
    });

    it('clear removes all state', () => {
      store.setRoomUnread('space1', 'room1', true);
      store.setRoomUnread('space2', 'room2', true);
      store.clear();
      expect(store.hasAnyUnread).toBe(false);
      expect(store.unreadSpaceCount).toBe(0);
    });
  });
});
