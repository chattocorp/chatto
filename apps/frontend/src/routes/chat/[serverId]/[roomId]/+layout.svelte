<script lang="ts">
  import { page } from '$app/state';
  import { roomRouteAccess } from '$lib/navigation/roomLinkAccess';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import Room from './Room.svelte';
  import RoomJoinScreen from './RoomJoinScreen.svelte';

  let { data, children } = $props();

  let { roomId } = $derived(data);

  const serverScope = useServerScope();
  const activeServerId = $derived(serverScope.serverId);

  // Wait for the active server projection to contain its viewer prefix before
  // treating room absence as authoritative.
  const serverStore = $derived(serverScope.store);
  const navigation = $derived(serverStore.navigation);
  const ready = $derived(
    !navigation.isInitialLoading &&
      !!serverStore.currentUser.user?.id &&
      navigation.currentUserId === serverStore.currentUser.user.id
  );

  let threadId = $derived(page.params.threadId);

  const isMessageLinkMode = $derived(page.route.id === '/chat/[serverId]/[roomId]/m/[messageId]');
  const roomAccess = $derived.by(() => {
    if (!ready || !roomId) return { kind: 'unknown' } as const;
    return roomRouteAccess({
      rooms: navigation.rooms,
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
