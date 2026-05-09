import { SvelteSet } from 'svelte/reactivity';

/**
 * Tracks which rooms on the server have unread messages.
 *
 * Post-PR(b) the API surface no longer carries `spaceId`, so unread state
 * collapses from "rooms-by-space" to a flat per-room set with a single
 * server-level "unknown unread" sentinel.
 *
 * Updated by:
 * - `NewMessageInSpaceEvent` → `setRoomUnread(_, true)`
 * - Marking a room as read (posting or entering) → `setRoomUnread(_, false)`
 * - Initial load with full room data → `initRooms`
 * - Initial load with only a server-level signal → `setServerHasUnread`
 */
export class RoomUnreadStore {
  // Specific rooms known to be unread.
  private unreadRooms = new SvelteSet<string>();
  // Server-level unknown-unread flag (set when we know there's unread but
  // not which room — e.g. on initial load before rooms are queried).
  private serverHasUnknownUnread = $state(false);

  /**
   * Set unread status for a specific room.
   */
  setRoomUnread(roomId: string, unread: boolean): void {
    if (unread) {
      this.unreadRooms.add(roomId);
    } else {
      this.unreadRooms.delete(roomId);
      // Reading a specific room implies we now have concrete knowledge —
      // drop the unknown-unread flag.
      this.serverHasUnknownUnread = false;
    }
  }

  /**
   * Check if the server has any unread rooms (or is flagged with unknown unread).
   */
  get hasAnyUnread(): boolean {
    return this.unreadRooms.size > 0 || this.serverHasUnknownUnread;
  }

  /**
   * Get the first known unread room ID, or null if only the unknown-unread
   * flag is set (no specific rooms).
   */
  getFirstUnreadRoomId(): string | null {
    for (const roomId of this.unreadRooms) return roomId;
    return null;
  }

  /**
   * Check if a specific room is unread.
   */
  roomIsUnread(roomId: string): boolean {
    return this.unreadRooms.has(roomId);
  }

  /**
   * Initialize unread state from room data.
   * Call this when loading rooms.
   */
  initRooms(rooms: Array<{ id: string; hasUnread: boolean }>): void {
    this.unreadRooms.clear();
    this.serverHasUnknownUnread = false;
    for (const room of rooms) {
      if (room.hasUnread) this.unreadRooms.add(room.id);
    }
  }

  /**
   * Flag (or unflag) the server as having unread when only the server-level
   * signal is known (initial load, before rooms are queried).
   */
  setServerHasUnread(hasUnread: boolean): void {
    if (hasUnread) {
      this.serverHasUnknownUnread = true;
    } else {
      this.serverHasUnknownUnread = false;
      this.unreadRooms.clear();
    }
  }

  /**
   * Clear all unread state.
   */
  clear(): void {
    this.unreadRooms.clear();
    this.serverHasUnknownUnread = false;
  }
}
