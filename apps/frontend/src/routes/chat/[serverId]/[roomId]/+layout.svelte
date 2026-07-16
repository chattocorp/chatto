<script lang="ts">
  import { page } from '$app/state';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { roomRouteAccess } from '$lib/navigation/roomLinkAccess';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import Room from './Room.svelte';
  import RoomJoinScreen from './RoomJoinScreen.svelte';

  let { data, children } = $props();

  let { roomId } = $derived(data);

  const activeServerId = $derived(getActiveServer());

  // Wait for the active server's merged rooms store (channels + DMs) to
  // settle before letting children mount. Without this, a freshly-loaded
  // room page can fire queries against the URL roomId before the store has
  // decided whether the room exists, briefly showing the not-found redirect.
  const serverStore = $derived(serverRegistry.getStore(activeServerId));
  const roomsStore = $derived(serverStore.rooms);
  const ready = $derived(
    !roomsStore.isInitialLoading &&
      !!serverStore.currentUser.user?.id &&
      roomsStore.currentUserId === serverStore.currentUser.user.id
  );

  let threadId = $derived(page.params.threadId);

  const isMessageLinkMode = $derived(page.route.id === '/chat/[serverId]/[roomId]/m/[messageId]');
  const roomAccess = $derived.by(() => {
    if (!ready || !roomId) return { kind: 'unknown' } as const;
    return roomRouteAccess({
      rooms: roomsStore.rooms,
      roomId
    });
  });
  const canRenderRoom = $derived(
    ready &&
      roomId &&
      (roomAccess.kind === 'member' || (roomAccess.kind === 'unknown' && !isMessageLinkMode))
  );
</script>

{#if ready && roomId && roomAccess.kind === 'nonmember'}
  {#key roomAccess.room.id}
    <RoomJoinScreen room={roomAccess.room} serverSegment={data.serverSegment} />
  {/key}
{:else if canRenderRoom && roomId}
  {#if isMessageLinkMode}
    <!-- Message link resolver: renders +page.svelte which fetches + redirects -->
    {@render children?.()}
  {:else}
    <!--
			Room is rendered in the layout so it stays mounted when navigating
			between room and thread URLs. This prevents unnecessary reloads.
		-->
    {#key activeServerId}
      <Room {roomId} {threadId} routeMessageId={page.params.messageId} />
    {/key}
  {/if}
{/if}
