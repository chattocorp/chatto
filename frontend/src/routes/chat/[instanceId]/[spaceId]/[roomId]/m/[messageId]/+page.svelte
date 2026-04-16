<!--
  Message link resolver. Fetches the event and redirects to the correct
  room (or thread) URL with ?highlight= so the jump-to-message mechanism
  scrolls to it. Renders nothing — the goto() fires on mount.
-->
<script lang="ts" module>
  import { graphql } from '$lib/gql';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import type { Client } from '@urql/svelte';

  const ResolveMessageLinkQuery = graphql(`
    query ResolveMessageLink($spaceId: ID!, $roomId: ID!, $eventId: ID!) {
      roomEventByEventId(spaceId: $spaceId, roomId: $roomId, eventId: $eventId) {
        id
        event {
          __typename
          ... on MessagePostedEvent {
            inThread
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
    instanceSegment: string,
    spaceId: string,
    roomId: string,
    messageId: string
  ): Promise<void> {
    const roomPath = resolve('/chat/[instanceId]/[spaceId]/[roomId]', {
      instanceId: instanceSegment,
      spaceId,
      roomId
    });

    try {
      const result = await client
        .query(ResolveMessageLinkQuery, { spaceId, roomId, eventId: messageId }, { requestPolicy: 'network-only' })
        .toPromise();

      const event = result.data?.roomEventByEventId;
      if (!event) {
        goto(`${roomPath}?highlight=${encodeURIComponent(messageId)}`, { replaceState: true });
        return;
      }

      const inner = event.event;
      const threadRoot =
        inner?.__typename === 'MessagePostedEvent' ? inner.inThread : null;

      if (threadRoot) {
        const threadPath = resolve('/chat/[instanceId]/[spaceId]/[roomId]/[threadId]', {
          instanceId: instanceSegment,
          spaceId,
          roomId,
          threadId: threadRoot
        });
        goto(`${threadPath}?highlight=${encodeURIComponent(messageId)}`, { replaceState: true });
        return;
      }

      goto(`${roomPath}?highlight=${encodeURIComponent(messageId)}`, { replaceState: true });
    } catch {
      goto(roomPath, { replaceState: true });
    }
  }
</script>

<script lang="ts">
  import { page } from '$app/state';
  import { useConnection } from '$lib/state/instance/connection.svelte';

  const connection = useConnection();

  $effect(() => {
    resolveAndRedirect(
      connection().client,
      page.params.instanceId!,
      page.params.spaceId!,
      page.params.roomId!,
      page.params.messageId!
    );
  });
</script>
