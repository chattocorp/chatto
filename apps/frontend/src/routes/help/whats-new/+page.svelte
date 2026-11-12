<!-- @component Help retains an explicit server selection for private release acknowledgements. -->
<script lang="ts">
  import { page } from '$app/state';
  import { serverRegistry, serverConnectionManager } from '$lib/client';
  import { segmentToServerId } from '$lib/navigation';
  import { firstAuthenticatedServerId } from '$lib/serverCatalogue';
  import ReleaseHighlights from '$lib/help/highlights/ReleaseHighlights.svelte';
  import ServerScopeProvider from '$lib/state/server/ServerScopeProvider.svelte';

  const serverId = $derived(
    page.url.searchParams.has('server')
      ? segmentToServerId(page.url.searchParams.get('server') ?? '')
      : firstAuthenticatedServerId()
  );
  const store = $derived(serverId ? serverRegistry.tryGetStore(serverId) : undefined);
  const connection = $derived(
    serverId && store ? serverConnectionManager.getClient(serverId) : undefined
  );
</script>

{#if serverId && store && connection}
  {#key store}
    <ServerScopeProvider {serverId} {store} {connection}>
      <ReleaseHighlights />
    </ServerScopeProvider>
  {/key}
{:else}
  <ReleaseHighlights />
{/if}
