import { graphql } from '$lib/gql';
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
  // Updated from every markRoomAsRead result; used to anchor the unread
  // separator the instant the user leaves (see the presence-false edge).
  let lastCursor: string | null = null;

  async function markRoomAsRead(targetRoomId: string, upToEventId?: string) {
    roomUnreadStore.setRoomUnread(targetRoomId, false);

    try {
      const result = await connection()
        .client.mutation(
          graphql(`
            mutation MarkRoomAsRead($input: MarkRoomAsReadInput!) {
              markRoomAsRead(input: $input) {
                previousLastReadAt
                lastReadAt
              }
            }
          `),
          { input: { roomId: targetRoomId, upToEventId } }
        )
        .toPromise();

      const data = result.data?.markRoomAsRead ?? null;
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
   * PostMessage auto-marks the room read on the server. Keeps the
   * presence-false anchor accurate so backgrounding the tab doesn't strand
   * the user's own latest message below the "new messages" separator.
   */
  function noteReadCursor(timestamp: string) {
    const ts = new Date(timestamp).getTime();
    if (lastCursor && ts <= new Date(lastCursor).getTime()) return;
    lastCursor = timestamp;

    // If this lands while the user is away — e.g. their own message's
    // subscription event arrives just after they backgrounded the tab — the
    // presence-false anchor was already set from a now-stale cursor. Advance
    // it too, so their own message isn't stranded below the separator.
    if (
      !appState.isPresent &&
      unreadAfterTime !== null &&
      ts > new Date(unreadAfterTime).getTime()
    ) {
      unreadAfterTime = timestamp;
    }
  }

  // Fire markRoomAsRead on every presence-true edge (fresh entry OR
  // refocus/tab-reveal) and on room changes while present. The mutation
  // result drives the unread separator so a refocus shows what arrived
  // while the user was away.
  let lastFiredRoomId = '';
  let wasPresent = false;

  $effect(() => {
    const { roomId } = getProps();
    const present = appState.isPresent;

    if (!present) {
      // Presence-false edge: anchor the unread separator at the current
      // read cursor with no upper bound, so messages arriving while the
      // user is away pile up below the marker in real time — it's already
      // there the moment the tab comes back on-screen (or right away if
      // the window is visible but unfocused). The presence-true edge below
      // refines it on return.
      if (wasPresent && lastCursor) {
        unreadAfterTime = lastCursor;
        unreadBeforeTime = null;
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
    // refocus of the *same* room, leave the existing separator in place —
    // it was anchored on the presence-false edge and the mutation result
    // is deliberately ignored below so the marker stays stable.
    if (isRoomChange) {
      unreadAfterTime = null;
      unreadBeforeTime = null;
    }

    markRoomAsRead(roomId).then((result) => {
      const current = getProps();
      if (current.roomId === roomId && result) {
        // Only adopt the server's (previousLastReadAt, lastReadAt] window on
        // a fresh room entry. On a same-room refocus the separator was just
        // anchored on the presence-false edge against `lastCursor` with an
        // open upper bound — overwriting it here collapses the window to
        // empty whenever the server cursor hasn't moved (e.g. only non-
        // message events like joins/leaves arrived while away), making the
        // separator vanish on focus and reappear on every blur.
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
    noteReadCursor
  };
}
