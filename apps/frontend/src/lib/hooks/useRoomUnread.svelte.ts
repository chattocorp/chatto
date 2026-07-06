import { createReadStateAPI, type MarkRoomAsReadResult } from '$lib/api-client/readState';
import { useConnection } from '$lib/state/server/connection.svelte';
import { serverRegistry } from '$lib/state/server/registry.svelte';
import { getActiveServer } from '$lib/state/activeServer.svelte';
import { useUnreadMarker } from './useUnreadMarker.svelte';

/**
 * Room-specific unread marker wrapper. The shared unread marker hook owns the
 * focus/refocus lifecycle; this wrapper only wires room read-state mutation
 * and room-list unread clearing.
 *
 * Must be called during component initialization (uses context).
 */
export function useRoomUnread(getProps: () => { roomId: string }) {
  const connection = useConnection();
  const roomUnreadStore = serverRegistry.getStore(getActiveServer()).roomUnread;

  const unread = useUnreadMarker(() => getProps().roomId, {
    markAsRead: async (targetRoomId: string, upToEventId?: string) => {
      roomUnreadStore.setRoomUnread(targetRoomId, false);

      try {
        const conn = connection();
        return await createReadStateAPI({
          serverId: conn.serverId ?? getActiveServer(),
          baseUrl: conn.connectBaseUrl,
          bearerToken: conn.bearerToken
        }).markRoomAsRead({ roomId: targetRoomId, upToEventId });
      } catch (err) {
        console.error('Failed to mark room as read:', err);
        return null;
      }
    },
    markerWindowFromReadResult: (result: MarkRoomAsReadResult) => {
      if (!result.previousLastReadAt || !result.lastReadAt) return null;
      return {
        afterTime: result.previousLastReadAt,
        beforeTime: result.lastReadAt
      };
    }
  });

  return {
    get unreadMarkerEventId() {
      return unread.unreadMarkerEventId;
    },
    get unreadMarkerWindow() {
      return unread.unreadMarkerWindow;
    },
    markRoomAsRead: unread.markAsRead,
    noteAwayEvent: unread.noteAwayEvent,
    setUnreadMarkerEventId: unread.setUnreadMarkerEventId,
    clearUnreadMarker: unread.clearUnreadMarker
  };
}
