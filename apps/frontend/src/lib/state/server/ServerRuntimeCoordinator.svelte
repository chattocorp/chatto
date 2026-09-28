<script lang="ts">
  /**
   * Runs the `@chatto/client` runtime for the app's lifetime and keeps the
   * URL-active server's realtime transport live.
   */
  import { untrack } from 'svelte';
  import { page } from '$app/state';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { startClientRuntime, type ClientRuntime } from '@chatto/client/server/runtime';

  const activeServerId = $derived(page.route.id?.startsWith('/chat') ? getActiveServer() : '');

  let runtime: ClientRuntime | null = null;

  $effect(() => {
    // Starting reads client state; it must not make this effect restart.
    const started = untrack(startClientRuntime);
    runtime = started;
    return () => {
      started.stop();
      if (runtime === started) runtime = null;
    };
  });

  $effect(() => {
    const serverId = activeServerId || null;
    untrack(() => runtime?.setActiveServer(serverId));
  });
</script>
