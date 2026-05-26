<script lang="ts">
  import { graphql } from '$lib/gql';
  import { useQuery } from '$lib/hooks';
  import { Panel, StatCard, DataTable, formatBytes, formatNumber } from '$lib/components/admin';
  import { Hint, Pill } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  const AdminProjectionsQuery = graphql(`
    query AdminProjections {
      admin {
        projections {
          name
          subjects
          started
          lastAppliedSequence
          matchingStreamSequence
          streamLastSequence
          lag
          entryCount
          estimatedBytes
          averageEntryBytes
        }
      }
    }
  `);

  const projectionsQuery = useQuery(AdminProjectionsQuery, () => ({}));
  const projections = $derived(
    [...(projectionsQuery.data?.admin?.projections ?? [])].sort((a, b) => {
      if (a.estimatedBytes !== b.estimatedBytes) return b.estimatedBytes - a.estimatedBytes;
      return a.name.localeCompare(b.name);
    })
  );
  const loading = $derived(projectionsQuery.loading);
  const error = $derived(projectionsQuery.error ?? null);
  const totalEstimatedBytes = $derived(
    projections.reduce((sum, projection) => sum + projection.estimatedBytes, 0)
  );
  const totalEntries = $derived(
    projections.reduce((sum, projection) => sum + projection.entryCount, 0)
  );
  const laggingCount = $derived(projections.filter((projection) => projection.lag > 0).length);
</script>

<PageTitle title="Projections | Admin" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title="Projections"
    subtitle="Runtime state for event-sourced read models"
    showMobileNav
  />

  <div class="min-h-0 flex-1 overflow-y-auto">
    <div class="flex flex-col gap-6 p-6">
      {#if loading}
        <div class="text-muted">Loading projection state...</div>
      {:else if error}
        <Hint tone="danger">{error}</Hint>
      {:else}
        <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
          <StatCard
            value={formatNumber(projections.length)}
            label="Projections"
            icon="iconify uil--layers"
            color="primary"
          />
          <StatCard
            value={formatBytes(totalEstimatedBytes)}
            label="Estimated Memory"
            icon="iconify uil--processor"
            color="success"
            subtitle={`${formatNumber(totalEntries)} projected entries`}
          />
          <StatCard
            value={formatNumber(laggingCount)}
            label="With Lag"
            icon="iconify uil--clock"
            color={laggingCount > 0 ? 'warning' : 'success'}
          />
        </div>

        <Panel noPadding>
          <DataTable
            items={projections}
            columns={6}
            emptyMessage="No projections are registered."
          >
            {#snippet header()}
              <th class="px-4 py-3 font-medium">Projection</th>
              <th class="px-4 py-3 font-medium">State</th>
              <th class="px-4 py-3 font-medium">Applied</th>
              <th class="px-4 py-3 font-medium">Lag</th>
              <th class="px-4 py-3 font-medium">Entries</th>
              <th class="px-4 py-3 font-medium">Memory</th>
            {/snippet}
            {#snippet row(projection)}
              <td class="px-4 py-3">
                <div class="font-medium">{projection.name}</div>
                <div class="mt-1 flex flex-wrap gap-1">
                  {#each projection.subjects as subject (subject)}
                    <span class="rounded border border-border px-1.5 py-0.5 font-mono text-[11px] text-muted">
                      {subject}
                    </span>
                  {/each}
                </div>
              </td>
              <td class="px-4 py-3">
                <Pill tone={projection.started ? 'success' : 'muted'}>
                  {projection.started ? 'Started' : 'Stopped'}
                </Pill>
              </td>
              <td class="whitespace-nowrap px-4 py-3 font-mono text-sm">
                {projection.lastAppliedSequence}
                <span class="text-muted">/ {projection.matchingStreamSequence}</span>
              </td>
              <td class="px-4 py-3">
                <span class:font-semibold={projection.lag > 0} class:text-warning={projection.lag > 0}>
                  {formatNumber(projection.lag)}
                </span>
              </td>
              <td class="px-4 py-3 font-mono text-sm">{formatNumber(projection.entryCount)}</td>
              <td class="px-4 py-3">
                <div class="whitespace-nowrap font-mono text-sm">{formatBytes(projection.estimatedBytes)}</div>
                <div class="whitespace-nowrap text-xs text-muted">
                  {formatBytes(projection.averageEntryBytes)} avg
                </div>
              </td>
            {/snippet}
          </DataTable>
        </Panel>

      {/if}
    </div>
  </div>
</div>
