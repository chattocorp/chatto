<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { roomLinkAccessRedirect } from '$lib/navigation/roomLinkAccess';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import Room from './Room.svelte';

  let { data, children } = $props();

  let { roomId } = $derived(data);

  const activeServerId = $derived(getActiveServer());

  // Wait for the active server's merged rooms store (channels + DMs) to
  // settle before letting children mount. Without this, a freshly-loaded
  // room page can fire queries against the URL roomId before the store has
  // decided whether the room exists, briefly showing the not-found redirect.
  const roomsStore = $derived(serverRegistry.getStore(activeServerId).rooms);
  const ready = $derived(!roomsStore.isInitialLoading);
  const fallbackPath = $derived(resolve('/chat/[serverId]', { serverId: data.serverSegment }));
  const targetPath = $derived(`${page.url.pathname}${page.url.search}${page.url.hash}`);

  let threadId = $derived(page.params.threadId);

  const isMessageLinkMode = $derived(/\/m\/[^/]+$/.test(page.url.pathname));
  const roomAccess = $derived.by(() => {
    if (!ready || !roomId) return { kind: 'allow' } as const;
    return roomLinkAccessRedirect({
      rooms: roomsStore.rooms,
      roomId,
      targetPath,
      fallbackPath
    });
  });
  const canRenderRoom = $derived(ready && roomId && roomAccess.kind === 'allow');

  let lastAccessRedirectKey = $state<string | null>(null);

  $effect(() => {
    if (!ready || !roomId || roomAccess.kind !== 'redirect') return;

    const key = JSON.stringify([roomAccess.path, roomAccess.state]);
    if (lastAccessRedirectKey === key) return;
    lastAccessRedirectKey = key;
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- roomAccess.path is the resolved server fallback path.
    goto(roomAccess.path, { replaceState: true, state: roomAccess.state });
  });
</script>

{#if canRenderRoom && roomId}
  {#if isMessageLinkMode}
    <!-- Message link resolver: renders +page.svelte which fetches + redirects -->
    {@render children?.()}
  {:else}
    <!--
			Room is rendered in the layout so it stays mounted when navigating
			between room and thread URLs. This prevents unnecessary reloads.
		-->
    {#key activeServerId}
      <Room {roomId} {threadId} />
    {/key}
  {/if}
{/if}
