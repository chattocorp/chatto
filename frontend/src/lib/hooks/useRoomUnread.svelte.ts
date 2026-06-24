import { createReadStateAPI } from '$lib/api/readState';
import { useConnection } from '$lib/state/server/connection.svelte';
import { serverRegistry } from '$lib/state/server/registry.svelte';
import { getActiveServer } from '$lib/state/activeServer.svelte';
import { appState } from '$lib/state/globals.svelte';

/**
 * Manages room unread state: marks the room as read on entry and every time
 * the user transitions back to "present" on the room (window refocus, tab
 * reveal). Tracks the unread separator position so a refocus shows what
 * arrived while the user was away.
 *
 * Must be called during component initialization (uses context).
 */
export function useRoomUnread(getProps: () => { roomId: string }) {
  const connection = useConnection();
  const roomUnreadStore = serverRegistry.getStore(getActiveServer()).roomUnread;

  let unreadAfterTime = $state<string | null>(null);
  let unreadBeforeTime = $state<string | null>(null);

  // The server's most recent read cursor (`lastReadAt`) for this room.
  // Updated from every markRoomAsRead result; used to capture where the
  // unread separator should be anchored when the user leaves.
  let lastCursor: string | null = null;
  // Captured while the app is hidden/unfocused, but intentionally not exposed
  // to the timeline until the user returns. This lets unread dots update in
  // the background without repainting the open room's separator while the user
  // is looking at another app.
  let pendingAwayAfterTime: string | null = null;
  let pendingAwayHasEvents = false;

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
   * this hook — notably when the user posts their own message, since
   * PostMessage auto-marks the room read on the server.
   *
   * Also advances open-upper-bound separator anchors past the new cursor
   * whenever it lands beyond the anchor, regardless of presence. Two scenarios
   * both need this:
   *
   * 1. Own message lands while away (e.g. posted from another device): the
   *    presence-false anchor was set from a stale cursor; without this the
   *    separator would render above the user's own latest message on
   *    refocus.
   * 2. Own message lands after a background → refocus → post sequence on
   *    the same room: the presence-true effect deliberately doesn't
   *    overwrite the separator on a same-room refocus, so without this the
   *    separator stays stuck at the prior cursor and renders BETWEEN the
   *    user's two own posts (one before bg, one after refocus).
   *
   * Gating the visible anchor on `unreadBeforeTime === null` keeps the closed
   * (after, before] window from a fresh room entry intact — that window
   * represents "this is what you missed last time", and the user posting
   * doesn't change what they previously hadn't seen.
   */
  function noteReadCursor(timestamp: string) {
    const ts = Date.parse(timestamp);
    if (lastCursor && ts <= Date.parse(lastCursor)) return;
    lastCursor = timestamp;

    if (pendingAwayAfterTime !== null && ts > Date.parse(pendingAwayAfterTime)) {
      pendingAwayAfterTime = timestamp;
    }

    if (
      unreadAfterTime !== null &&
      unreadBeforeTime === null &&
      ts > Date.parse(unreadAfterTime)
    ) {
      unreadAfterTime = timestamp;
    }
  }

  function noteAwayEvent() {
    if (pendingAwayAfterTime !== null) {
      pendingAwayHasEvents = true;
    }
  }

  // Fire markRoomAsRead on every presence-true edge (fresh entry OR
  // refocus/tab-reveal) and on room changes while present. The mutation
  // result drives the unread separator on room entry; same-room refocus uses
  // the deferred away anchor so the separator does not appear in the
  // background before the user returns.
  let lastFiredRoomId = '';
  let wasPresent = false;

  $effect(() => {
    const { roomId } = getProps();
    const present = appState.isPresent;

    if (!present) {
      // Presence-false edge: capture the current read cursor, but do not
      // expose it to the rendered timeline yet. Messages can keep streaming
      // in and sidebar unread dots can light up while the user is away; the
      // in-room separator appears only on the presence-true edge below.
      if (wasPresent && lastCursor) {
        pendingAwayAfterTime = lastCursor;
        pendingAwayHasEvents = false;
      }
      wasPresent = false;
      return;
    }

    if (wasPresent && lastFiredRoomId === roomId) return;

    const isRoomChange = lastFiredRoomId !== roomId;
    wasPresent = true;
    lastFiredRoomId = roomId;

    // On a room change, clear the previous room's separator so it can't
    // flash in the new room while the mutation below is in flight. On a
    // refocus of the *same* room, reveal any deferred away anchor and leave
    // the mutation result ignored below so the marker stays stable.
    if (isRoomChange) {
      pendingAwayAfterTime = null;
      pendingAwayHasEvents = false;
      unreadAfterTime = null;
      unreadBeforeTime = null;
    } else if (pendingAwayAfterTime && pendingAwayHasEvents) {
      unreadAfterTime = pendingAwayAfterTime;
      unreadBeforeTime = null;
      pendingAwayAfterTime = null;
      pendingAwayHasEvents = false;
    } else {
      pendingAwayAfterTime = null;
      pendingAwayHasEvents = false;
    }

    markRoomAsRead(roomId).then((result) => {
      const current = getProps();
      if (current.roomId === roomId && result) {
        // Only adopt the server's (previousLastReadAt, lastReadAt] window on
        // a fresh room entry. On a same-room refocus the separator was just
        // revealed from the deferred away anchor with an open upper bound —
        // overwriting it here collapses the window to empty whenever the
        // server cursor hasn't moved (e.g. only non-message events like
        // joins/leaves arrived while away), making the separator vanish on
        // focus and reappear on every blur.
        if (isRoomChange && result.previousLastReadAt && result.lastReadAt) {
          unreadAfterTime = result.previousLastReadAt;
          unreadBeforeTime = result.lastReadAt;
        }
      }
    });
  });

  return {
    get unreadAfterTime() {
      return unreadAfterTime;
    },
    get unreadBeforeTime() {
      return unreadBeforeTime;
    },
    markRoomAsRead,
    noteReadCursor,
    noteAwayEvent
  };
}
