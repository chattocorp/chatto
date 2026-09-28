import { createReadStateAPI, type MarkRoomAsReadResult } from '$lib/api-client/readState';
import { useServerScope } from '$lib/state/server/scope.svelte';
import { useUnreadMarker, type UnreadMarkerEvent } from './useUnreadMarker.svelte';

/**
 * Room-specific unread marker wrapper. The shared unread marker hook owns the
 * entry and foreground lifecycle; this wrapper only wires read-state mutation
 * and room-list unread clearing.
 *
 * Must be called during component initialization (uses context).
 */
export function useRoomUnread(
  getProps: () => {
    roomId: string;
    events: readonly UnreadMarkerEvent[];
    canReadMessages?: boolean;
    /** See the `useUnreadMarker` option of the same name. */
    getLifecycleUpToEventId?: () => string | undefined | null;
    /** See the `useUnreadMarker` option of the same name. */
    onLifecycleRead?: (upToEventId: string | undefined) => void;
  }
) {
  const serverScope = useServerScope();
  const roomUnreadStore = serverScope.store.roomUnread;

  const unread = useUnreadMarker(() => getProps().roomId, {
    markAsRead: async (
      targetRoomId: string,
      upToEventId: string | undefined,
      signal: AbortSignal
    ) => {
      // Only a read through the latest message is known to clear the room.
      // The live read state reconciles a partial read.
      const optimisticRead =
        upToEventId === undefined ? roomUnreadStore.beginOptimisticRead(targetRoomId) : null;

      try {
        const result = await serverScope.connection
          .getAPI(createReadStateAPI)
          .markRoomAsRead({ roomId: targetRoomId, upToEventId }, { signal });
        optimisticRead?.commit();
        return result;
      } catch (err) {
        optimisticRead?.rollback();
        throw err;
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
    getMarkerEvents: () => getProps().events,
    getMarkerSkipActorId: () => serverScope.store.viewerId,
    canMarkAsRead: () => serverScope.store.isAuthenticated && getProps().canReadMessages !== false,
    onMarkAsReadError: (error) => console.error('Failed to mark room as read:', error),
    getLifecycleUpToEventId: () => {
      const getter = getProps().getLifecycleUpToEventId;
      return getter ? getter() : undefined;
    },
    onLifecycleRead: (upToEventId) => getProps().onLifecycleRead?.(upToEventId)
  });

  return {
    get unreadMarkerEventId() {
      return unread.unreadMarkerEventId;
    },
    markAsRead: unread.markAsRead,
    clearUnreadMarker: unread.clearUnreadMarker,
    markArrivalWhileAway: unread.markArrivalWhileAway
  };
}
