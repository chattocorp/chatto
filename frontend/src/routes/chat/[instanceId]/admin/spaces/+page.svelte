<script lang="ts">
  import { graphql } from '$lib/gql';
  import { useQuery } from '$lib/hooks';
  import { Panel, DataTable, CopyId } from '$lib/components/admin';
  import { Hint } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  const AdminSpacesListQuery = graphql(`
    query AdminSpacesList {
      spaces {
        id
        name
        description
        memberCount
        roomCount
        assetCount
      }
    }
  `);

  const spacesQuery = useQuery(AdminSpacesListQuery, () => ({}));

  let spaces = $derived(spacesQuery.data?.spaces ?? []);
  let loading = $derived(spacesQuery.loading);
  let error = $derived(spacesQuery.error ?? null);
</script>

<PageTitle title="Spaces | Admin" />

<PaneHeader title="Spaces" subtitle="View all spaces on this instance" showMobileNav />

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  {#if loading}
    <div class="text-muted">Loading spaces...</div>
  {:else if error}
    <Hint tone="danger">{error}</Hint>
  {:else}
    <Panel noPadding>
      <DataTable items={spaces} columns={6} emptyMessage="No spaces found">
        {#snippet header()}
          <th class="px-4 py-3 font-medium">Name</th>
          <th class="px-4 py-3 font-medium">Description</th>
          <th class="px-4 py-3 text-right font-medium">Members</th>
          <th class="px-4 py-3 text-right font-medium">Rooms</th>
          <th class="px-4 py-3 text-right font-medium">Assets</th>
          <th class="px-4 py-3 font-medium">ID</th>
        {/snippet}
        {#snippet row(space)}
          <td class="px-4 py-3 font-medium">{space.name}</td>
          <td class="px-4 py-3 text-muted">{space.description || '—'}</td>
          <td class="px-4 py-3 text-right font-mono text-sm">{space.memberCount}</td>
          <td class="px-4 py-3 text-right font-mono text-sm">{space.roomCount}</td>
          <td class="px-4 py-3 text-right font-mono text-sm">{space.assetCount}</td>
          <td class="px-4 py-3 text-muted"><CopyId value={space.id} /></td>
        {/snippet}
      </DataTable>
    </Panel>

    <div class="text-sm text-muted">{spaces.length} space(s) total</div>
  {/if}
</div>
