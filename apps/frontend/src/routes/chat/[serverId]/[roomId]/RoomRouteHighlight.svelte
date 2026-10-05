<!--
@component

Converts a room's explicit URL target into a pending highlight on activation.
The parent keys this component by the room, thread, message, and query target,
and mounts it only when the destination's room data is ready. In-app requests
take precedence over a query target. Navigation guards prevent repeat jumps
when room hydration mounts the same URL again.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import type { RoomNavigationState } from './roomNavigationState.svelte';

  let {
    roomId,
    threadId,
    messageId,
    highlightParam,
    navigation,
    onQueryConsumed
  }: {
    roomId: string;
    threadId?: string;
    messageId?: string;
    highlightParam: string | null;
    navigation: RoomNavigationState;
    onQueryConsumed: (roomId: string, threadId: string | undefined, eventId: string) => void;
  } = $props();

  onMount(() => {
    const threadTarget = navigation.consumeThreadMessageRoute(roomId, threadId, messageId);
    if (threadTarget !== undefined) {
      if (threadTarget) navigation.beginHighlight(roomId, threadId ?? null, threadTarget);
      return;
    }

    if (navigation.highlightFor(roomId, threadId ?? null)) return;
    const fromQuery = navigation.consumeHighlightParam(roomId, threadId, highlightParam);
    if (!fromQuery) return;

    navigation.beginHighlight(roomId, threadId ?? null, fromQuery);
    onQueryConsumed(roomId, threadId, fromQuery);
  });
</script>
