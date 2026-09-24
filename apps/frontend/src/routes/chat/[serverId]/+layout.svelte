<script lang="ts">
  import { page } from '$app/state';
  import { supportsSavedViewRoute } from '$lib/navigation/chatRoomRoute';
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
      <Chrome>
        {#if serverStore.realtimeSync.hasDisplayableView && (!serverStore.startupPresentationOnly || supportsSavedViewRoute(page.route.id))}
          <div class="contents" aria-busy={serverStore.realtimeSync.isRecoveringSnapshot}>
            {@render children?.()}
          </div>
        {/if}
      </Chrome>
    </ServerScopeProvider>
  {/if}
{/key}
