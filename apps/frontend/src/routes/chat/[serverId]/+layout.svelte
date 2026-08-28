<script lang="ts">
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import ServerScopeProvider from '$lib/state/server/ServerScopeProvider.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import Chrome from '$lib/components/chat/Chrome.svelte';

  let { children } = $props();

  // The root layout resolves the active instance from the URL and provides
  // it via context; we just consume it here.
  const serverId = $derived(getActiveServer());

  // Guard: if the instance ID couldn't be resolved (e.g., "-" with no origin
  // instance registered), the layout load redirects before this component mounts.
  const serverStore = $derived(serverId ? serverRegistry.tryGetStore(serverId) : undefined);

  // Reauthentication is a live session state. The load guard handles initial
  // access; this keeps private route content unmounted if a session expires.
  const reauthRequired = $derived(
    !!serverStore && serverRegistry.getServer(serverId)?.reauthRequiredAt != null
  );
</script>

<!-- Authentication replacement recreates same-ID server resources, so key by
     store identity rather than only by the URL-selected server ID. -->
{#key serverStore}
  {#if serverStore}
    <ServerScopeProvider
      {serverId}
      connection={serverConnectionManager.getClient(serverId)}
      store={serverStore}
    >
      {#if reauthRequired}
        <Chrome />
      {:else}
        <Chrome>
          {@render children?.()}
        </Chrome>
      {/if}
    </ServerScopeProvider>
  {/if}
{/key}
