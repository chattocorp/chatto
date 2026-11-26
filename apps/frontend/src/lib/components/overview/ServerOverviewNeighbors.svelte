<!--
@component

The Neighbors that the current server advertises, from its cached
Neighborhood. The request goes only to the current server, and the logos and
banners are copies on it, so the browser does not contact the Neighbors (see
FDR-042). A card opens the server when it is already in the app, or the
Server Directory, which owns the join flow. Renders nothing when the server
has no Neighbors or the read fails.
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { listNeighborhoodServers } from '@chatto/client/api/server';
  import ServerProfileCard from '$lib/components/ServerProfileCard.svelte';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { createQuery } from '$lib/query/client';
  import { findServerByUrl } from '$lib/serverCatalogue';
  import OverviewSection from './OverviewSection.svelte';

  let { serverOrigin }: { serverOrigin: string } = $props();

  const neighborhoodQuery = createQuery(() => ({
    queryKey: ['public', 'neighborhood', serverOrigin],
    queryFn: ({ signal }) => listNeighborhoodServers(serverOrigin, { signal })
  }));

  const neighbors = $derived(
    (neighborhoodQuery.data ?? [])
      .filter((server) => server.directNeighbor)
      .sort((a, b) => a.profile.name.localeCompare(b.profile.name))
      .map((server) => ({ ...server, known: findServerByUrl(server.origin) }))
  );
</script>

{#if neighbors.length > 0}
  <OverviewSection title={m('chat.overview.neighbors')}>
    <div class="grid grid-cols-[repeat(auto-fill,minmax(min(100%,16rem),1fr))] gap-4">
      {#each neighbors as neighbor (neighbor.origin)}
        <ServerProfileCard
          origin={neighbor.origin}
          imageOrigin={serverOrigin}
          profile={neighbor.profile}
          href={neighbor.known
            ? resolve('/chat/[serverId]', { serverId: serverIdToSegment(neighbor.known.id) })
            : resolve('/chat/servers')}
          actionLabel={neighbor.known
            ? m('chat.overview.open_neighbor', { server: neighbor.profile.name })
            : m('chat.overview.view_neighbor', { server: neighbor.profile.name })}
          testId="server-overview-neighbor"
        />
      {/each}
    </div>
  </OverviewSection>
{/if}
