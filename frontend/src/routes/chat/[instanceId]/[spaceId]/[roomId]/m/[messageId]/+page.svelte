<!--
  Message link resolver. Fetches the event and redirects to the correct
  room (or thread) URL with ?highlight= so the jump-to-message mechanism
  scrolls to it. Renders nothing — the goto() fires on mount.
-->
<script lang="ts" module>
  import { graphql } from '$lib/gql';

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
</script>

<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { useConnection } from '$lib/state/instance/connection.svelte';

  const connection = useConnection();

  $effect(() => {
    const instanceSegment = page.params.instanceId!;
    const spaceId = page.params.spaceId!;
    const roomId = page.params.roomId!;
    const messageId = page.params.messageId!;

    const roomPath = resolve('/chat/[instanceId]/[spaceId]/[roomId]', {
      instanceId: instanceSegment,
      spaceId,
      roomId
    });

    let cancelled = false;

    (async () => {
      try {
        const result = await connection().client
          .query(ResolveMessageLinkQuery, { spaceId, roomId, eventId: messageId }, { requestPolicy: 'network-only' })
          .toPromise();

        if (cancelled) return;

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
        if (!cancelled) {
          goto(roomPath, { replaceState: true });
        }
      }
    })();

    return () => {
      cancelled = true;
    };
  });
</script>
