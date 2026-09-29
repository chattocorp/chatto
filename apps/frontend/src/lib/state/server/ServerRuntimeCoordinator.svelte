<script lang="ts">
  /**
   * Runs the `@chatto/client` runtime for the app's lifetime and keeps the
   * URL-active server's realtime transport live.
   */
  import { untrack } from 'svelte';
  import { page } from '$app/state';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { client } from '$lib/client';

  const activeServerId = $derived(page.route.id?.startsWith('/chat') ? getActiveServer() : '');

  $effect(() => {
    // Starting reads client state; it must not make this effect restart.
    untrack(() => client.start());
    return () => client.stop();
  });

  $effect(() => {
    const serverId = activeServerId || null;
    untrack(() => client.setActiveServer(serverId));
  });
</script>
