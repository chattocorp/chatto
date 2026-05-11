/**
 * Server-level and per-room notification level preferences.
 *
 * Tracks the viewer's notification level for the server and individual rooms.
 * Used by the room list to suppress unread indicators for muted rooms, and
 * by the preferences page to display and edit settings.
 *
 * Post-PR(b) the API surface no longer carries `spaceId`, so the previous
 * "per-space" dimension collapses to a single server-level preference.
 */

import { SvelteMap } from 'svelte/reactivity';
import { NotificationLevel } from '$lib/gql/graphql';

export class NotificationLevelStore {
  /** Server-level preference. */
  private server = $state<{ level: NotificationLevel; effectiveLevel: NotificationLevel }>({
    level: NotificationLevel.Default,
    effectiveLevel: NotificationLevel.Normal
  });

  /** Room-level preferences: roomId -> { level, effectiveLevel } */
  private roomLevels = new SvelteMap<
    string,
    { level: NotificationLevel; effectiveLevel: NotificationLevel }
  >();

  /**
   * Set the viewer's server-level notification preference.
   */
  setServerPreference(level: NotificationLevel, effectiveLevel: NotificationLevel): void {
    this.server = { level, effectiveLevel };
  }

  /**
   * Set the viewer's notification preference for a room.
   */
  setRoomPreference(
    roomId: string,
    level: NotificationLevel,
    effectiveLevel: NotificationLevel
  ): void {
    this.roomLevels.set(roomId, { level, effectiveLevel });
  }

  /**
   * Get the viewer's server-level notification preference.
   * Returns DEFAULT/NORMAL if not set.
   */
  getServerPreference(): { level: NotificationLevel; effectiveLevel: NotificationLevel } {
    return this.server;
  }

  /**
   * Get the viewer's notification preference for a room.
   * Returns DEFAULT with the server's effective level if not set.
   */
  getRoomPreference(
    roomId: string
  ): { level: NotificationLevel; effectiveLevel: NotificationLevel } {
    const roomPref = this.roomLevels.get(roomId);
    if (roomPref) return roomPref;
    return {
      level: NotificationLevel.Default,
      effectiveLevel: this.server.effectiveLevel
    };
  }

  /**
   * Get the effective notification level for a room.
   * Resolves: room-level -> server-level -> NORMAL.
   */
  getEffectiveLevel(roomId: string): NotificationLevel {
    return this.getRoomPreference(roomId).effectiveLevel;
  }

  /**
   * Check if a room is muted (no notifications, no unread markers).
   */
  isRoomMuted(roomId: string): boolean {
    return this.getEffectiveLevel(roomId) === NotificationLevel.Muted;
  }

  /**
   * Check if the server is fully muted (server-level muted, no room overrides).
   */
  isServerMuted(): boolean {
    return this.server.effectiveLevel === NotificationLevel.Muted;
  }

  /**
   * Clear all preferences. Called on logout.
   */
  clear(): void {
    this.server = {
      level: NotificationLevel.Default,
      effectiveLevel: NotificationLevel.Normal
    };
    this.roomLevels.clear();
  }
}
