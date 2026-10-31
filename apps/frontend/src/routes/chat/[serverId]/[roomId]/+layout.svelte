<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import type { ResolvedPathname } from '$app/types';
  import { conversationPath, conversationUserId, userPath } from '$lib/navigation/conversationPath';
  import { createRoomTimelineAPI } from '@chatto/client/api/roomTimeline';
  import { resolveAndRedirect } from '$lib/navigation/resolveMessageDestination';
  import { serverUi } from '$lib/state/server/serverUi';
  import { roomRouteAccess } from '$lib/navigation/roomLinkAccess';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { captureMutationCompletion } from '$lib/navigation/mutationCompletion';
  import Room from './Room.svelte';
  import RoomInvitationScreen from './RoomInvitationScreen.svelte';
  import RoomJoinScreen from './RoomJoinScreen.svelte';
  import { PageTitle } from '$lib/ui';

  let { data, children } = $props();

  let { roomId } = $derived(data);

  const serverScope = useServerScope();
  const activeServerId = serverScope.serverId;

  const serverStore = serverScope.store;
  const timelineAPI = serverScope.connection.getAPI(createRoomTimelineAPI);
  const highlights = serverUi(serverStore).pendingHighlights;
  const navigation = $derived(serverUi(serverStore).navigation);
  // Displayed membership belongs to this store's viewer. The connection owns
  // session verification and command readiness.
  const ready = $derived(
    !navigation.isInitialLoading &&
      (!serverStore.projectionViewerId || serverStore.projectionViewerId === serverStore.viewerId)
  );

  let threadId = $derived(page.params.threadId);
  const userId = $derived(ready && roomId ? conversationUserId(serverStore, roomId) : null);
  const isMessageLinkMode = $derived(page.route.id === '/chat/[serverId]/[roomId]/m/[messageId]');
  const roomAccess = $derived.by(() => {
    if (!ready || !roomId) return { kind: 'unknown' } as const;
    return roomRouteAccess({
      rooms: navigation.rooms,
      roomId
    });
  });
  const isRoomMember = $derived(roomAccess.kind === 'member');
  const canRenderRoom = $derived(
    (ready || serverStore.realtimeSync.isRecoveringSnapshot) &&
      roomId &&
      (isRoomMember || (roomAccess.kind === 'unknown' && !isMessageLinkMode))
  );

  // Projection readiness and membership can change without navigation (snapshot
  // recovery, inline joins, or another device). One owner selects the final URL.
  $effect(() => {
    if (!ready || !roomId) return;
    const selectedRoomId = roomId;
    if (isMessageLinkMode) {
      if (!isRoomMember) return;
      const isCurrent = captureMutationCompletion(serverScope);
      let cancelled = false;
      void resolveAndRedirect(
        timelineAPI,
        highlights,
        (threadId) =>
          conversationPath(activeServerId, selectedRoomId, { threadId, store: serverStore }),
        selectedRoomId,
        page.params.messageId!,
        () => !cancelled && isCurrent()
      );
      return () => {
        cancelled = true;
      };
    }
    if (userId) {
      const destination =
        `${userPath(activeServerId, userId, { threadId: page.params.threadId, messageId: page.params.messageId })}${page.url.search}${page.url.hash}` as ResolvedPathname;
      void goto(destination, { replaceState: true });
    }
  });
</script>

{#if userId && !isMessageLinkMode}
  <PageTitle />
{:else if ready && roomId && roomAccess.kind === 'nonmember'}
  {#key roomAccess.room.id}
    {#if roomAccess.room.viewerInvitation}
      <RoomInvitationScreen room={roomAccess.room} serverSegment={data.serverSegment} />
    {:else}
      <RoomJoinScreen room={roomAccess.room} serverSegment={data.serverSegment} />
    {/if}
  {/key}
{:else if canRenderRoom && roomId && !isMessageLinkMode}
  <!-- Keep the conversation mounted across room and thread URLs. -->
  <Room {roomId} {threadId} routeMessageId={page.params.messageId} />
{:else}
  <PageTitle />
{/if}

{@render children?.()}
