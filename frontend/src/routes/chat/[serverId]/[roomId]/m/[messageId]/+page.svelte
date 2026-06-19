<!--
  Message link resolver. Fetches the event and redirects to the correct
  room (or thread) URL, with the highlight intent delivered via
  PendingHighlightStore so the destination URL stays clean (refresh won't
  re-fire the highlight). Renders nothing — the goto() fires on mount.
-->
<script lang="ts" module>
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import type { PendingHighlightStore } from '$lib/state/server/pendingHighlight.svelte';
  import { wireEventBusManager } from '$lib/state/server/wireEventBus.svelte';
  import { GetRoomEventRequest } from '$lib/pb/chatto/api/v1/chat_pb';

  /**
   * Fetch a message by ID and redirect to the appropriate room or thread URL.
   * If the message is a thread reply, opens the thread pane. If not found or
   * on error, falls back to the room URL.
   */
  export async function resolveAndRedirect(
    serverId: string,
    pendingHighlights: PendingHighlightStore,
    serverSegment: string,
    roomId: string,
    messageId: string
  ): Promise<void> {
    const roomParams = { serverId: serverSegment, roomId };

    try {
      const client = wireEventBusManager.getClient(serverId);
      if (!client) {
        pendingHighlights.set(roomId, null, messageId);
        goto(resolve('/chat/[serverId]/[roomId]', roomParams), { replaceState: true });
        return;
      }

      const response = await client.getRoomEvent(
        new GetRoomEventRequest({ roomId, eventId: messageId })
      );
      const event = response.event;
      if (!event) {
        pendingHighlights.set(roomId, null, messageId);
        goto(resolve('/chat/[serverId]/[roomId]', roomParams), { replaceState: true });
        return;
      }

      const payload = event.event?.payload;
      const threadRoot =
        payload?.case === 'messagePosted' ? (payload.value.threadRootEventId ?? null) : null;

      if (threadRoot) {
        pendingHighlights.set(roomId, threadRoot, messageId);
        goto(
          resolve('/chat/[serverId]/[roomId]/[threadId]', {
            ...roomParams,
            threadId: threadRoot
          }),
          { replaceState: true }
        );
        return;
      }

      pendingHighlights.set(roomId, null, messageId);
      goto(resolve('/chat/[serverId]/[roomId]', roomParams), { replaceState: true });
    } catch {
      goto(resolve('/chat/[serverId]/[roomId]', roomParams), { replaceState: true });
    }
  }
</script>

<script lang="ts">
  import { page } from '$app/state';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';

  const activeServerId = $derived(getActiveServer());
  const stores = $derived(serverRegistry.getStore(activeServerId));

  // Wait for the active server's rooms store to settle before redirecting,
  // so a deep-link to a DM doesn't briefly resolve as a missing channel
  // room and trigger the not-found redirect.
  const roomsStore = $derived(stores.rooms);

  $effect(() => {
    if (roomsStore.isInitialLoading) return;
    resolveAndRedirect(
      activeServerId,
      stores.pendingHighlights,
      page.params.serverId!,
      page.params.roomId!,
      page.params.messageId!
    );
  });
</script>
