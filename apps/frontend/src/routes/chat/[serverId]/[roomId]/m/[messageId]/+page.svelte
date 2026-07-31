<!--
  Message link resolver. Fetches the event and redirects to the correct
  room (or thread) URL, with the highlight intent delivered via
  PendingHighlightStore so the destination URL stays clean (refresh won't
  re-fire the highlight). Renders nothing — the goto() fires on mount.
-->
<script lang="ts" module>
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { TimelineEventKind } from '$lib/render/timelineEvents';
  import { createRoomTimelineAPI, type RoomTimelineAPI } from '$lib/api-client/roomTimeline';
  import type { PendingHighlightStore } from '$lib/state/server/pendingHighlight.svelte';

  /**
   * Fetch a message by ID and redirect to the appropriate room or thread URL.
   * If the message is a thread reply, opens the thread pane. If not found or
   * on error, falls back to the room URL.
   */
  export async function resolveAndRedirect(
    api: Pick<RoomTimelineAPI, 'getMessage'>,
    pendingHighlights: PendingHighlightStore,
    serverSegment: string,
    roomId: string,
    messageId: string,
    isCurrent: () => boolean = () => true
  ): Promise<void> {
    const roomParams = { serverId: serverSegment, roomId };

    try {
      const target = await api.getMessage({
        roomId,
        eventId: messageId
      });
      if (!isCurrent()) return;

      if (!target) {
        pendingHighlights.set(roomId, null, messageId);
        goto(resolve('/chat/[serverId]/[roomId]', roomParams), { replaceState: true });
        return;
      }

      const threadRootEventId =
        target.event.kind === TimelineEventKind.MessagePosted
          ? (target.event.threadRootEventId ?? null)
          : null;

      if (threadRootEventId) {
        pendingHighlights.set(roomId, threadRootEventId, messageId);
        goto(
          resolve('/chat/[serverId]/[roomId]/[threadId]', {
            ...roomParams,
            threadId: threadRootEventId
          }),
          { replaceState: true }
        );
        return;
      }

      pendingHighlights.set(roomId, null, messageId);
      goto(resolve('/chat/[serverId]/[roomId]', roomParams), { replaceState: true });
    } catch {
      if (!isCurrent()) return;
      goto(resolve('/chat/[serverId]/[roomId]', roomParams), { replaceState: true });
    }
  }
</script>

<script lang="ts">
  import { page } from '$app/state';
  import { useServerScope } from '$lib/state/server/scope.svelte';

  const serverScope = useServerScope();
  const stores = $derived(serverScope.store);

  // Wait for the active server projection to settle before redirecting,
  // so a deep-link to a DM doesn't briefly resolve as a missing channel
  // room and trigger the not-found redirect.
  const navigation = $derived(stores.navigation);

  $effect(() => {
    if (navigation.isInitialLoading) return;
    const serverSegment = page.params.serverId!;
    const roomId = page.params.roomId!;
    const messageId = page.params.messageId!;
    resolveAndRedirect(
      serverScope.connection.getAPI(createRoomTimelineAPI),
      stores.pendingHighlights,
      serverSegment,
      roomId,
      messageId,
      () =>
        serverScope.isCurrent() &&
        serverSegment === page.params.serverId &&
        roomId === page.params.roomId &&
        messageId === page.params.messageId
    );
  });
</script>
