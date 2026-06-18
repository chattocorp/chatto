<script lang="ts">
  import { page } from '$app/state';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { roomIDFromURLSegment } from '$lib/roomUrls';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import Room from './Room.svelte';

  let { data, children }: { data: { roomId?: string }; children?: import('svelte').Snippet } =
    $props();

  const activeServerId = $derived(getActiveServer());
  const roomsStore = $derived(serverRegistry.getStore(activeServerId).rooms);
  const ready = $derived(!roomsStore.isInitialLoading);

  let roomId = $derived(data.roomId ? roomIDFromURLSegment(data.roomId) : undefined);
  let threadId = $derived(page.params.threadId);

  const isMessageLinkMode = $derived(/\/m\/[^/]+$/.test(page.url.pathname));
</script>

{#if ready && roomId}
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
