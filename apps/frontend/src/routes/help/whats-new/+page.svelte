<!-- @component Help retains an explicit server selection for private release acknowledgements. -->
<script lang="ts">
  import { page } from '$app/state';
  import { serverRegistry, serverConnectionManager } from '$lib/client';
  import { segmentToServerId } from '$lib/navigation';
  import { firstAuthenticatedServerId } from '$lib/serverCatalogue';
  import ReleaseHighlights from '$lib/help/highlights/ReleaseHighlights.svelte';
  import ServerScopeProvider from '$lib/state/server/ServerScopeProvider.svelte';
  import type { ServerScope } from '$lib/state/server/scope.svelte';

  const serverId = $derived(
    page.url.searchParams.has('server')
      ? segmentToServerId(page.url.searchParams.get('server') ?? '')
      : firstAuthenticatedServerId()
  );
  const store = $derived(serverId ? serverRegistry.tryGetStore(serverId) : undefined);
  const scope = $derived.by((): ServerScope | undefined => {
    if (!serverId || !store) return undefined;
    const selectedId = serverId;
    const selectedStore = store;
    const connection = serverConnectionManager.getClient(selectedId);
    return {
      serverId: selectedId,
      store: selectedStore,
      connection,
      isCurrent: () =>
        serverRegistry.tryGetStore(selectedId) === selectedStore &&
        serverConnectionManager.getClient(selectedId) === connection
    };
  });
</script>

{#if scope}
  {#key scope.store}
    <ServerScopeProvider
      serverId={scope.serverId}
      store={scope.store}
      connection={scope.connection}
    >
      <ReleaseHighlights />
    </ServerScopeProvider>
  {/key}
{:else}
  <ReleaseHighlights />
{/if}
