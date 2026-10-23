<!--
  Message link resolver. Fetches the event and redirects to the correct
  room (or thread) URL, with the highlight intent delivered via
  PendingHighlightStore so the destination URL stays clean (refresh won't
  re-fire the highlight). Renders nothing — the goto() fires on mount.
-->
<script lang="ts">
  import { serverUi } from '$lib/state/server/serverUi';
  import { createRoomTimelineAPI } from '@chatto/client/api/roomTimeline';
  import { conversationPath } from '$lib/navigation/conversationPath';
  import { resolveAndRedirect } from '$lib/navigation/resolveMessageDestination';
  import { page } from '$app/state';
  import { useServerScope } from '$lib/state/server/scope.svelte';

  const serverScope = useServerScope();
  const stores = serverScope.store;

  // Wait for the active server projection to settle before redirecting,
  // so a deep-link to a DM doesn't briefly resolve as a missing channel
  // room and trigger the not-found redirect.
  const navigation = $derived(serverUi(stores).navigation);

  $effect(() => {
    if (navigation.isInitialLoading) return;
    const serverSegment = page.params.serverId!;
    const roomId = page.params.roomId!;
    const messageId = page.params.messageId!;
    resolveAndRedirect(
      serverScope.connection.getAPI(createRoomTimelineAPI),
      serverUi(stores).pendingHighlights,
      (threadId) => conversationPath(serverScope.serverId, roomId, { threadId, store: stores }),
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
