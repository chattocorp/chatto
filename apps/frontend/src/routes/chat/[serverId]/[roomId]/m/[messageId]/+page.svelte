<!--
  Message link resolver. Fetches the event and redirects to the correct
  room (or thread) URL, with the highlight intent delivered via
  PendingHighlightStore so the destination URL stays clean (refresh won't
  re-fire the highlight). Renders nothing — the goto() fires on mount.
-->
<script lang="ts" module>
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { createRoomTimelineAPI, type RoomTimelineAPIConfig } from '$lib/api-client/roomTimeline';
  import type { PendingHighlightStore } from '$lib/state/server/pendingHighlight.svelte';

  /**
   * Fetch a message by ID and redirect to the appropriate room or thread URL.
   * If the message is a thread reply, opens the thread pane. If not found or
   * on error, falls back to the room URL.
   */
  export async function resolveAndRedirect(
    config: RoomTimelineAPIConfig,
    pendingHighlights: PendingHighlightStore,
    serverSegment: string,
    roomId: string,
    messageId: string
  ): Promise<void> {
    const roomParams = { serverId: serverSegment, roomId };

    try {
      const target = await createRoomTimelineAPI(config).resolveMessageLinkTarget({
        roomId,
        eventId: messageId
      });

      if (!target.event) {
        pendingHighlights.set(roomId, null, messageId);
        goto(resolve('/chat/[serverId]/[roomId]', roomParams), { replaceState: true });
        return;
      }

      if (target.threadRootEventId) {
        pendingHighlights.set(roomId, target.threadRootEventId, messageId);
        goto(
          resolve('/chat/[serverId]/[roomId]/[threadId]', {
            ...roomParams,
            threadId: target.threadRootEventId
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
  import { useActiveServerScope } from '$lib/state/server/activeServerScope.svelte';

  const server = useActiveServerScope();

  // Wait for the active server's rooms store to settle before redirecting,
  // so a deep-link to a DM doesn't briefly resolve as a missing channel
  // room and trigger the not-found redirect.
  const roomsStore = $derived(server.rooms);

  $effect(() => {
    if (roomsStore.isInitialLoading) return;
    const conn = server.connection;
    resolveAndRedirect(
      {
        serverId: conn.serverId,
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      },
      server.pendingHighlights,
      server.segment,
      page.params.roomId!,
      page.params.messageId!
    );
  });
</script>
