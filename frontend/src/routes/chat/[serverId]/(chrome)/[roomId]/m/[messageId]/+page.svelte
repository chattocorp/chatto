<!--
  Message link resolver. Fetches the event and redirects to the correct
  room (or thread) URL, with the highlight intent delivered via
  PendingHighlightStore so the destination URL stays clean (refresh won't
  re-fire the highlight). Renders nothing — the goto() fires on mount.
-->
<script lang="ts" module>
  import { graphql } from '$lib/gql';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import type { Client } from '@urql/svelte';
  import type { PendingHighlightStore } from '$lib/state/server/pendingHighlight.svelte';

  const ResolveMessageLinkQuery = graphql(`
    query ResolveMessageLink($roomId: ID!, $eventId: ID!) {
      room(roomId: $roomId) {
        event(eventId: $eventId) {
          id
          event {
            __typename
            ... on MessagePostedEvent {
              inThread
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
    const roomParams = { serverId: serverSegment, roomId };

    try {
      const result = await client
        .query(ResolveMessageLinkQuery, { roomId, eventId: messageId }, { requestPolicy: 'network-only' })
        .toPromise();

      const event = result.data?.room?.event;
      if (!event) {
        pendingHighlights.set(roomId, null, messageId);
        goto(resolve('/chat/[serverId]/(chrome)/[roomId]', roomParams), { replaceState: true });
        return;
      }

      const inner = event.event;
      const threadRoot =
        inner?.__typename === 'MessagePostedEvent' ? inner.inThread : null;

      if (threadRoot) {
        pendingHighlights.set(roomId, threadRoot, messageId);
        goto(
          resolve('/chat/[serverId]/(chrome)/[roomId]/[threadId]', {
            ...roomParams,
            threadId: threadRoot
          }),
          { replaceState: true }
        );
        return;
      }

      pendingHighlights.set(roomId, null, messageId);
      goto(resolve('/chat/[serverId]/(chrome)/[roomId]', roomParams), { replaceState: true });
    } catch {
      goto(resolve('/chat/[serverId]/(chrome)/[roomId]', roomParams), { replaceState: true });
    }
  }
</script>

<script lang="ts">
  import { page } from '$app/state';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { getSpaceRoomsStore } from '$lib/state/space';

  const connection = useConnection();
  const getServerId = getActiveServer();
  const stores = $derived(serverRegistry.getStore(getServerId()));

  // Wait for the rooms store to settle before redirecting, so a deep-link to
  // a DM doesn't briefly resolve as a missing channel room and trigger the
  // not-found redirect.
  const roomsStore = getSpaceRoomsStore();

  $effect(() => {
    if (roomsStore.isInitialLoading) return;
    resolveAndRedirect(
      connection().client,
      stores.pendingHighlights,
      page.params.serverId!,
      page.params.roomId!,
      page.params.messageId!
    );
  });
</script>
