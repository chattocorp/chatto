import { SvelteMap, SvelteSet } from 'svelte/reactivity';

/**
 * Tracks which rooms have unread messages, organized by space.
 * Space-level unread status is derived from this.
 *
 * Two flat fields:
 * - `unreadByRoom`: known unread rooms keyed by space.
 * - `spacesWithUnknownUnread`: spaces flagged as having unread without
 *   per-room data (e.g. on initial load before rooms are queried).
 *
 * Updated by:
 * - `NewMessageInSpaceEvent` → `setRoomUnread(_, _, true)`
 * - Marking a room as read (posting or entering) → `setRoomUnread(_, _, false)`
 * - Initial load with full room data → `initSpaceRooms`
 * - Initial load with only space-level signal → `setSpaceHasUnread`
 */
export class RoomUnreadStore {
  // Specific rooms known to be unread, keyed by space.
  private unreadByRoom = new SvelteMap<string, SvelteSet<string>>();
  // Spaces flagged as having unread but without per-room data yet.
  private spacesWithUnknownUnread = new SvelteSet<string>();

  /**
   * Set unread status for a specific room.
   */
  setRoomUnread(spaceId: string, roomId: string, unread: boolean): void {
    if (unread) {
      let rooms = this.unreadByRoom.get(spaceId);
      if (!rooms) {
        rooms = new SvelteSet<string>();
        this.unreadByRoom.set(spaceId, rooms);
      }
      rooms.add(roomId);
    } else {
      const rooms = this.unreadByRoom.get(spaceId);
      if (rooms) {
        rooms.delete(roomId);
        if (rooms.size === 0) this.unreadByRoom.delete(spaceId);
      }
      // Reading a specific room implies we now have concrete knowledge for
      // this space — drop the unknown-unread flag.
      this.spacesWithUnknownUnread.delete(spaceId);
    }
  }

  /**
   * Check if a space has any unread rooms (or is flagged with unknown unread).
   */
  spaceHasUnread(spaceId: string): boolean {
    const rooms = this.unreadByRoom.get(spaceId);
    if (rooms && rooms.size > 0) return true;
    return this.spacesWithUnknownUnread.has(spaceId);
  }

  /**
   * Get the first known unread room ID for a space.
   * Returns null if only the unknown-unread flag is set (no specific rooms).
   */
  getFirstUnreadRoomId(spaceId: string): string | null {
    const rooms = this.unreadByRoom.get(spaceId);
    if (!rooms || rooms.size === 0) return null;
    for (const roomId of rooms) return roomId;
    return null;
  }

  /**
   * Check if a specific room is unread.
   */
  roomIsUnread(spaceId: string, roomId: string): boolean {
    return this.unreadByRoom.get(spaceId)?.has(roomId) ?? false;
  }

  /**
   * Initialize unread state for a space from room data.
   * Call this when loading rooms for a space.
   */
  initSpaceRooms(spaceId: string, rooms: Array<{ id: string; hasUnread: boolean }>): void {
    this.unreadByRoom.delete(spaceId);
    this.spacesWithUnknownUnread.delete(spaceId);
    for (const room of rooms) {
      if (room.hasUnread) this.setRoomUnread(spaceId, room.id, true);
    }
  }

  /**
   * Flag (or unflag) a space as having unread when only the space-level signal
   * is known (initial load, before rooms are queried).
   */
  setSpaceHasUnread(spaceId: string, hasUnread: boolean): void {
    if (hasUnread) {
      this.spacesWithUnknownUnread.add(spaceId);
    } else {
      this.spacesWithUnknownUnread.delete(spaceId);
      this.unreadByRoom.delete(spaceId);
    }
  }

  /**
   * Number of distinct spaces with any unread signal.
   */
  get unreadSpaceCount(): number {
    let count = this.spacesWithUnknownUnread.size;
    for (const spaceId of this.unreadByRoom.keys()) {
      if (!this.spacesWithUnknownUnread.has(spaceId)) count++;
    }
    return count;
  }

  /**
   * Whether any space has unread rooms (for PWA flag badge).
   */
  get hasAnyUnread(): boolean {
    return this.unreadByRoom.size > 0 || this.spacesWithUnknownUnread.size > 0;
  }

  /**
   * Clear all unread state.
   */
  clear(): void {
    this.unreadByRoom.clear();
    this.spacesWithUnknownUnread.clear();
  }
}
