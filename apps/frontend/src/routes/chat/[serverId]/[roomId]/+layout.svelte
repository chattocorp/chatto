<script lang="ts">
  import { page } from '$app/state';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
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
