<!--
  Message link resolver. Fetches the event and redirects to the correct
  room (or thread) URL, with the highlight intent delivered via
  PendingHighlightStore so the destination URL stays clean (refresh won't
  re-fire the highlight). Renders nothing — the goto() fires on mount.
-->
<script lang="ts" module>
  import { graphql } from '$lib/gql';
  import { goto } from '$app/navigation';
  import {
    roomIDFromURLSegment,
    roomPathForSegment,
    roomThreadPathForSegment,
    roomURLSegment
  } from '$lib/roomUrls';
  import type { Client } from '@urql/svelte';
  import type { PendingHighlightStore } from '$lib/state/server/pendingHighlight.svelte';

  const ResolveMessageLinkQuery = graphql(`
    query ResolveMessageLink($roomId: ID!, $eventId: ID!) {
      room(roomId: $roomId) {
        id
        name
        type
        event(eventId: $eventId) {
          id
          event {
            __typename
            ... on MessagePostedEvent {
              threadRootEventId
            }
          }
        }
      }
    }
  `);

  /**
   * Fetch a message by ID and redirect to the appropriate room or thread URL.
   * If the message is a thread reply, opens the thread pane. If not found or
   * on error, falls back to the room URL.
   */
  export async function resolveAndRedirect(
    client: Client,
    pendingHighlights: PendingHighlightStore,
    serverSegment: string,
    roomId: string,
    messageId: string
  ): Promise<void> {
    try {
      const result = await client
        .query(ResolveMessageLinkQuery, { roomId, eventId: messageId }, { requestPolicy: 'network-only' })
        .toPromise();

      const canonicalRoomSegment = result.data?.room
        ? roomURLSegment(result.data.room)
        : roomId;
      const event = result.data?.room?.event;
      if (!event) {
        pendingHighlights.set(roomId, null, messageId);
        goto(roomPathForSegment(serverSegment, canonicalRoomSegment), { replaceState: true });
        return;
      }

      const inner = event.event;
      const threadRoot =
        inner?.__typename === 'MessagePostedEvent' ? inner.threadRootEventId : null;

      if (threadRoot) {
        pendingHighlights.set(roomId, threadRoot, messageId);
        goto(
          roomThreadPathForSegment(serverSegment, canonicalRoomSegment, threadRoot),
          { replaceState: true }
        );
        return;
      }

      pendingHighlights.set(roomId, null, messageId);
      goto(roomPathForSegment(serverSegment, canonicalRoomSegment), { replaceState: true });
    } catch {
      goto(roomPathForSegment(serverSegment, roomId), { replaceState: true });
    }
  }
</script>

<script lang="ts">
  import { page } from '$app/state';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';

  const connection = useConnection();
  const stores = $derived(serverRegistry.getStore(getActiveServer()));

  // Wait for the active server's rooms store to settle before redirecting,
  // so a deep-link to a DM doesn't briefly resolve as a missing channel
  // room and trigger the not-found redirect.
  const roomsStore = $derived(stores.rooms);

  $effect(() => {
    if (roomsStore.isInitialLoading) return;
    const roomId = roomIDFromURLSegment(page.params.roomId!);
    void resolveAndRedirect(
      connection().client,
      stores.pendingHighlights,
      page.params.serverId!,
      roomId,
      page.params.messageId!
    );
  });
</script>
