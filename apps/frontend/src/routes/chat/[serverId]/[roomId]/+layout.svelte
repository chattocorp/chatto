<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import type { ResolvedPathname } from '$app/types';
  import { conversationUserId, userPath } from '$lib/navigation/conversationPath';
  import { serverUi } from '$lib/state/server/serverUi';
  import { roomRouteAccess } from '$lib/navigation/roomLinkAccess';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import Room from './Room.svelte';
  import RoomJoinScreen from './RoomJoinScreen.svelte';
  import { PageTitle } from '$lib/ui';

  let { data, children } = $props();

  let { roomId } = $derived(data);

  const serverScope = useServerScope();
  const activeServerId = serverScope.serverId;

  const serverStore = serverScope.store;
  const navigation = $derived(serverUi(serverStore).navigation);
  // Displayed membership belongs to this store's viewer. The connection owns
  // session verification and command readiness.
  const ready = $derived(
    !navigation.isInitialLoading &&
      (!serverStore.projectionViewerId || serverStore.projectionViewerId === serverStore.viewerId)
  );

  let threadId = $derived(page.params.threadId);
  const userId = $derived(ready && roomId ? conversationUserId(serverStore, roomId) : null);
  $effect(() => {
    if (!userId) return;
    const destination =
      `${userPath(activeServerId, userId, { threadId: page.params.threadId, messageId: page.params.messageId })}${page.url.search}${page.url.hash}` as ResolvedPathname;
    void goto(destination, { replaceState: true });
  });

  const isMessageLinkMode = $derived(page.route.id === '/chat/[serverId]/[roomId]/m/[messageId]');
  const roomAccess = $derived.by(() => {
    if (!ready || !roomId) return { kind: 'unknown' } as const;
    return roomRouteAccess({
      rooms: navigation.rooms,
      roomId
    });
  });
  const canRenderRoom = $derived(
    (ready || serverStore.realtimeSync.isRecoveringSnapshot) &&
      roomId &&
      (roomAccess.kind === 'member' || (roomAccess.kind === 'unknown' && !isMessageLinkMode))
  );
</script>

{#if userId}
  <PageTitle />
{:else if ready && roomId && roomAccess.kind === 'nonmember'}
  {#key roomAccess.room.id}
    <RoomJoinScreen room={roomAccess.room} serverSegment={data.serverSegment} />
  {/key}
{:else if canRenderRoom && roomId}
  {#if isMessageLinkMode}
    <PageTitle />
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
{:else}
  <PageTitle />
{/if}
