import { graphql } from '$lib/gql';
import { useConnection } from '$lib/state/server/connection.svelte';
import { serverRegistry } from '$lib/state/server/registry.svelte';
import { getActiveServer } from '$lib/state/activeServer.svelte';
import { appState } from '$lib/state/globals.svelte';

/**
 * Manages room unread state: marks the room as read on entry and when new
 * messages arrive from other users. Tracks the unread separator position.
 *
 * Must be called during component initialization (uses context).
 */
export function useRoomUnread(getProps: () => { roomId: string }) {
  const connection = useConnection();
  const getInstanceId = getActiveServer();
  const roomUnreadStore = serverRegistry.getStore(getInstanceId()).roomUnread;

  let unreadAfterTime = $state<string | null>(null);
  let unreadBeforeTime = $state<string | null>(null);

  async function markRoomAsRead(targetRoomId: string) {
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
          { input: { roomId: targetRoomId } }
        )
        .toPromise();

      return result.data?.markRoomAsRead ?? null;
    } catch (err) {
      console.error('Failed to mark room as read:', err);
      return null;
    }
  }

  let previousRoomId: string | undefined;

  // Mark as read when entering the room
  $effect(() => {
    const { roomId } = getProps();

    unreadAfterTime = null;
    unreadBeforeTime = null;

    if (!appState.isFocused) return;
    if (previousRoomId === roomId) return;
    previousRoomId = roomId;

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
