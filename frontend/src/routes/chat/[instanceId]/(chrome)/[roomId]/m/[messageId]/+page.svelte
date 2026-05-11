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
  import type { PendingHighlightStore } from '$lib/state/instance/pendingHighlight.svelte';

  const ResolveMessageLinkQuery = graphql(`
    query ResolveMessageLink($roomId: ID!, $eventId: ID!) {
      roomEventByEventId(roomId: $roomId, eventId: $eventId) {
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
    pendingHighlights: PendingHighlightStore,
    instanceSegment: string,
    roomId: string,
    messageId: string
  ): Promise<void> {
    const roomParams = { instanceId: instanceSegment, roomId };

    try {
      const result = await client
        .query(ResolveMessageLinkQuery, { roomId, eventId: messageId }, { requestPolicy: 'network-only' })
        .toPromise();

      const event = result.data?.roomEventByEventId;
      if (!event) {
        pendingHighlights.set(roomId, null, messageId);
        goto(resolve('/chat/[instanceId]/(chrome)/[roomId]', roomParams), { replaceState: true });
        return;
      }

      const inner = event.event;
      const threadRoot =
        inner?.__typename === 'MessagePostedEvent' ? inner.inThread : null;

      if (threadRoot) {
        pendingHighlights.set(roomId, threadRoot, messageId);
        goto(
          resolve('/chat/[instanceId]/(chrome)/[roomId]/[threadId]', {
            ...roomParams,
            threadId: threadRoot
          }),
          { replaceState: true }
        );
        return;
      }

      pendingHighlights.set(roomId, null, messageId);
      goto(resolve('/chat/[instanceId]/(chrome)/[roomId]', roomParams), { replaceState: true });
    } catch {
      goto(resolve('/chat/[instanceId]/(chrome)/[roomId]', roomParams), { replaceState: true });
    }
  }
</script>

<script lang="ts">
  import { page } from '$app/state';
  import { useConnection } from '$lib/state/instance/connection.svelte';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { useEffectiveSpaceId } from '$lib/hooks';

  const connection = useConnection();
  const getInstanceId = getActiveInstance();
  const stores = $derived(instanceRegistry.getStore(getInstanceId()));

  // Used as a "rooms store ready" gate — returns null while loading. We only
  // need the room ID for the resolve query, so the resolved space ID itself
  // is never read; we just wait for the store to settle before redirecting.
  const effective = useEffectiveSpaceId(() => page.params.roomId);

  $effect(() => {
    if (!effective.current) return;
    resolveAndRedirect(
      connection().client,
      stores.pendingHighlights,
      page.params.instanceId!,
      page.params.roomId!,
      page.params.messageId!
    );
  });
</script>
