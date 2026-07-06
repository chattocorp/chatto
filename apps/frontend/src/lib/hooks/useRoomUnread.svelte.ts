import { createReadStateAPI } from '$lib/api-client/readState';
import { useConnection } from '$lib/state/server/connection.svelte';
import { serverRegistry } from '$lib/state/server/registry.svelte';
import { getActiveServer } from '$lib/state/activeServer.svelte';
import { appState } from '$lib/state/globals.svelte';

export type UnreadMarkerWindow = {
  afterTime: string;
  beforeTime: string;
};

/**
 * Manages room unread state: marks the room as read on entry and every time
 * the user transitions back to "present" on the room (window refocus, tab
 * reveal). The rendered unread separator is an explicit event-id marker:
 * once visible, late read-state responses and background events do not move it.
 *
 * Must be called during component initialization (uses context).
 */
export function useRoomUnread(getProps: () => { roomId: string }) {
  const connection = useConnection();
  const roomUnreadStore = serverRegistry.getStore(getActiveServer()).roomUnread;

  let unreadMarkerEventId = $state<string | null>(null);
  let unreadMarkerWindow = $state<UnreadMarkerWindow | null>(null);

  // The server's most recent read cursor (`lastReadAt`) for this room.
  let lastCursor: string | null = null;
  // Captured while hidden/unfocused and revealed only when the user returns.
  let pendingAwayEventId: string | null = null;

  async function markRoomAsRead(targetRoomId: string, upToEventId?: string) {
    roomUnreadStore.setRoomUnread(targetRoomId, false);

    try {
      const conn = connection();
      const data = await createReadStateAPI({
        serverId: conn.serverId ?? getActiveServer(),
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      }).markRoomAsRead({ roomId: targetRoomId, upToEventId });

      if (data?.lastReadAt && getProps().roomId === targetRoomId) {
        lastCursor = data.lastReadAt;
      }
      return data;
    } catch (err) {
      console.error('Failed to mark room as read:', err);
      return null;
    }
  }

  /**
   * Advance the tracked read cursor without issuing a mutation. Used when
   * the read cursor moves server-side without a markRoomAsRead call from
   * this hook, notably when the user posts their own message.
   */
  function noteReadCursor(timestamp: string) {
    const ts = Date.parse(timestamp);
    if (lastCursor && ts <= Date.parse(lastCursor)) return;
    lastCursor = timestamp;
  }

  function noteAwayEvent(eventId: string) {
    if (unreadMarkerEventId !== null || pendingAwayEventId !== null) return;
    pendingAwayEventId = eventId;
  }

  function setUnreadMarkerEventId(eventId: string | null) {
    unreadMarkerEventId = eventId;
    if (eventId !== null) {
      unreadMarkerWindow = null;
    }
  }

  function clearUnreadMarker() {
    unreadMarkerEventId = null;
    unreadMarkerWindow = null;
    pendingAwayEventId = null;
  }

  // Fire markRoomAsRead on every presence-true edge (fresh entry OR
  // refocus/tab-reveal) and on room changes while present. Fresh room entry
  // may produce a timestamp window; RoomEventsPane resolves it once to an
  // explicit event id. Same-room refocus reveals only the pending event id.
  let lastFiredRoomId = '';
  let wasPresent = false;

  $effect(() => {
    const { roomId } = getProps();
    const present = appState.isPresent;

    if (!present) {
      wasPresent = false;
      return;
    }

    if (wasPresent && lastFiredRoomId === roomId) return;

    const isRoomChange = lastFiredRoomId !== roomId;
    wasPresent = true;
    lastFiredRoomId = roomId;

    if (isRoomChange) {
      clearUnreadMarker();
    } else if (pendingAwayEventId) {
      setUnreadMarkerEventId(pendingAwayEventId);
      pendingAwayEventId = null;
    } else {
      pendingAwayEventId = null;
    }

    markRoomAsRead(roomId).then((result) => {
      const current = getProps();
      if (current.roomId !== roomId || !result) return;

      if (isRoomChange && result.previousLastReadAt && result.lastReadAt) {
        unreadMarkerWindow = {
          afterTime: result.previousLastReadAt,
          beforeTime: result.lastReadAt
        };
      }
    });
  });

  return {
    get unreadMarkerEventId() {
      return unreadMarkerEventId;
    },
    get unreadMarkerWindow() {
      return unreadMarkerWindow;
    },
    markRoomAsRead,
    noteReadCursor,
    noteAwayEvent,
    setUnreadMarkerEventId,
    clearUnreadMarker
  };
}
