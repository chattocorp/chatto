import { createReadStateAPI, type MarkRoomAsReadResult } from '$lib/api-client/readState';
import { useServerScope } from '$lib/state/server/scope.svelte';
import { useUnreadMarker, type UnreadMarkerEvent } from './useUnreadMarker.svelte';

/**
 * Room-specific unread marker wrapper. The shared unread marker hook owns the
 * focus/refocus lifecycle; this wrapper only wires room read-state mutation
 * and room-list unread clearing.
 *
 * Must be called during component initialization (uses context).
 */
export function useRoomUnread(
  getProps: () => {
    roomId: string;
    events: readonly UnreadMarkerEvent[];
    canReadMessages?: boolean;
  }
) {
  const serverScope = useServerScope();
  const roomUnreadStore = serverScope.store.roomUnread;

  const unread = useUnreadMarker(() => getProps().roomId, {
    markAsRead: async (targetRoomId: string, upToEventId?: string) => {
      if (getProps().canReadMessages === false) return null;

      const optimisticRead = roomUnreadStore.beginOptimisticRead(targetRoomId);

      try {
        const result = await serverScope.connection
          .getAPI(createReadStateAPI)
          .markRoomAsRead({ roomId: targetRoomId, upToEventId });
        optimisticRead.commit();
        return result;
      } catch (err) {
        optimisticRead.rollback();
        console.error('Failed to mark room as read:', err);
        return null;
      }
    },
    markerWindowFromReadResult: (result: MarkRoomAsReadResult, markedAtMs: number) => {
      if (!result.previousLastReadAt || !result.lastReadAt) return null;
      if (result.previousLastReadAt === result.lastReadAt) return null;
      return {
        afterTime: result.previousLastReadAt,
        beforeTime: markedAtMs
      };
    },
    getMarkerEvents: () => getProps().events
  });

  return {
    get unreadMarkerEventId() {
      return unread.unreadMarkerEventId;
    },
    markRoomAsRead: unread.markAsRead,
    clearUnreadMarker: unread.clearUnreadMarker
  };
}
