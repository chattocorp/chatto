import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { OptimisticMutationRegistry } from '$lib/state/optimisticMutations';

export type OptimisticRoomReadHandle = {
  commit(): void;
  rollback(): void;
};

/**
 * Tracks which rooms on the server have unread messages.
 *
 * Post-PR(b) the API surface no longer carries `spaceId`, so unread state
 * collapses from "rooms-by-space" to a flat per-room group with a single
 * server-level "unknown unread" sentinel.
 *
 * Updated by:
 * - Root `MessagePostedEvent` for any room on the server → `setRoomUnread(_, true)`
 * - Marking a room as read (posting or entering) → `setRoomUnread(_, false)`
 * - Initial load with full room data → `initRooms`
 * - Initial load with only a server-level signal → `setServerHasUnread`
 */
export class RoomUnreadStore {
  // Specific rooms authoritatively known to be unread.
  private unreadRooms = new SvelteSet<string>();
  // Provisional reads hide the underlying unread fact until they settle.
  private optimisticReadRooms = new SvelteSet<string>();
  private optimisticReads = new OptimisticMutationRegistry();
  private roomRevisions = new SvelteMap<string, number>();
  // Server-level unknown-unread flag (set when we know there's unread but
  // not which room — e.g. on initial load before rooms are queried).
  private serverHasUnknownUnread = $state(false);

  private optimisticReadKey(roomId: string): string {
    return `room:${roomId}`;
  }

  private roomRevision(roomId: string): number {
    return this.roomRevisions.get(roomId) ?? 0;
  }

  private invalidateOptimisticRead(roomId: string): void {
    this.optimisticReads.clear(this.optimisticReadKey(roomId));
    this.optimisticReadRooms.delete(roomId);
  }

  /**
   * Set unread status for a specific room.
   */
  setRoomUnread(roomId: string, unread: boolean): void {
    this.roomRevisions.set(roomId, this.roomRevision(roomId) + 1);
    this.invalidateOptimisticRead(roomId);

    if (unread) {
      this.unreadRooms.add(roomId);
    } else {
      this.unreadRooms.delete(roomId);
    }
  }

  /**
   * Provisionally mark a room read while retaining the underlying unread fact.
   * New room state invalidates the handle so rollback cannot overwrite it.
   */
  beginOptimisticRead(roomId: string): OptimisticRoomReadHandle {
    const token = this.optimisticReads.createToken();
    const roomRevision = this.roomRevision(roomId);
    const key = this.optimisticReadKey(roomId);

    this.optimisticReads.mark(key, token);
    this.optimisticReadRooms.add(roomId);

    return {
      commit: () => {
        if (!this.optimisticReads.isCurrent(key, token)) return;
        if (this.roomRevision(roomId) !== roomRevision) return;

        this.unreadRooms.delete(roomId);
        this.optimisticReads.clear(key);
        this.optimisticReadRooms.delete(roomId);
      },
      rollback: () => {
        if (!this.optimisticReads.isCurrent(key, token)) return;
        this.optimisticReads.clear(key);
        this.optimisticReadRooms.delete(roomId);
      }
    };
  }

  /**
   * Check if the server has any unread rooms (or is flagged with unknown unread).
   */
  get hasAnyUnread(): boolean {
    for (const roomId of this.unreadRooms) {
      if (!this.optimisticReadRooms.has(roomId)) return true;
    }
    return this.serverHasUnknownUnread;
  }

  /**
   * Get the first known unread room ID, or null if only the unknown-unread
   * flag is set (no specific rooms).
   */
  getFirstUnreadRoomId(): string | null {
    for (const roomId of this.unreadRooms) {
      if (!this.optimisticReadRooms.has(roomId)) return roomId;
    }
    return null;
  }

  /**
   * Check if a specific room is unread.
   */
  roomIsUnread(roomId: string): boolean {
    return this.unreadRooms.has(roomId) && !this.optimisticReadRooms.has(roomId);
  }

  /**
   * Initialize unread state from room data.
   * Call this when loading rooms.
   */
  initRooms(
    rooms: Array<{ id: string; hasUnread: boolean }>,
    serverHasUnknownUnread = false
  ): void {
    this.optimisticReads.clearAll();
    this.optimisticReadRooms.clear();
    this.roomRevisions.clear();
    this.unreadRooms.clear();
    this.serverHasUnknownUnread = false;
    this.updateRooms(rooms);
    this.serverHasUnknownUnread = serverHasUnknownUnread;
  }

  /** Merge an authoritative partial room snapshot without dropping other rooms. */
  updateRooms(rooms: Array<{ id: string; hasUnread: boolean }>): void {
    for (const room of rooms) {
      this.roomRevisions.set(room.id, this.roomRevision(room.id) + 1);
      this.invalidateOptimisticRead(room.id);
      if (room.hasUnread) this.unreadRooms.add(room.id);
      else this.unreadRooms.delete(room.id);
    }
  }

  /** Clear the server-level sentinel after all relevant room scopes are known. */
  resolveUnknownUnread(): void {
    this.serverHasUnknownUnread = false;
  }

  /**
   * Flag (or unflag) the server as having unread when only the server-level
   * signal is known (initial load, before rooms are queried).
   */
  setServerHasUnread(hasUnread: boolean): void {
    if (hasUnread) {
      this.serverHasUnknownUnread = true;
    } else {
      this.optimisticReads.clearAll();
      this.optimisticReadRooms.clear();
      this.serverHasUnknownUnread = false;
      this.unreadRooms.clear();
    }
  }

  /**
   * Clear all unread state.
   */
  clear(): void {
    this.optimisticReads.clearAll();
    this.optimisticReadRooms.clear();
    this.roomRevisions.clear();
    this.unreadRooms.clear();
    this.serverHasUnknownUnread = false;
  }
}
