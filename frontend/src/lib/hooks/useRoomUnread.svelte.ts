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

      return result.data?.markRoomAsRead ?? null;
    } catch (err) {
      console.error('Failed to mark room as read:', err);
      return null;
    }
  }

  // Fire markRoomAsRead on every presence-true edge (fresh entry OR
  // refocus/tab-reveal) and on room changes while present. The mutation
  // result drives the unread separator so a refocus shows what arrived
  // while the user was away. Presence-out leaves the existing separator
  // in place — it's the "you were away" boundary the user needs to see
  // when they come back.
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
    wasPresent = true;
    lastFiredRoomId = roomId;

    unreadAfterTime = null;
    unreadBeforeTime = null;

    markRoomAsRead(roomId).then((result) => {
      const current = getProps();
      if (current.roomId === roomId && result) {
        if (result.previousLastReadAt && result.lastReadAt) {
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
    markRoomAsRead
  };
}
