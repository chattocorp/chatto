<!--
@component

Projections section of the System page: a summary of the projections on the
replica that handled the request, and the state, lag, and memory use of
each projection. Failed projections come first.
-->
<script lang="ts">
  import { formatBytes, formatNumber } from '$lib/components/admin';
  import { DataTable, Hint, Panel, Pill } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { useSystem } from '../systemContext';

  const system = useSystem();

  const projections = $derived(
    [...system.info.projections].sort((a, b) => {
      if (a.failed !== b.failed) return a.failed ? -1 : 1;
      if (a.estimatedBytes !== b.estimatedBytes) return b.estimatedBytes - a.estimatedBytes;
      return a.name.localeCompare(b.name);
    })
  );
  const totalEstimatedBytes = $derived(
    projections.reduce((sum, projection) => sum + projection.estimatedBytes, 0)
  );
  const totalEntries = $derived(
    projections.reduce((sum, projection) => sum + projection.entryCount, 0)
  );
  const laggingCount = $derived(projections.filter((projection) => projection.lag > 0).length);
  const failedProjectionCount = $derived(
    projections.filter((projection) => projection.failed).length
  );
  const averageProjectionEntryBytes = $derived(
    totalEntries > 0 ? totalEstimatedBytes / totalEntries : 0
  );

  function formatDurationSeconds(seconds: number | null | undefined): string {
    if (seconds == null) return m('admin.system.pending_state');
    if (seconds < 0.001) return '<1 ms';
    if (seconds < 1) return `${Math.round(seconds * 1000)} ms`;
    if (seconds < 10) return `${seconds.toFixed(2)} s`;
    if (seconds < 60) return `${seconds.toFixed(1)} s`;

    const minutes = Math.floor(seconds / 60);
    const remainingSeconds = Math.round(seconds % 60);
    return `${minutes}m ${remainingSeconds}s`;
  }
</script>

{#if system.info.projectionsAvailable}
  <Panel title={m('admin.system.projection_summary')} icon="iconify icon-[uil--layers]">
    <div class="grid grid-cols-2 gap-x-6 gap-y-4 md:grid-cols-3">
      <div>
        <div class="text-sm text-muted">{m('admin.system.projections')}</div>
        <div class="font-mono text-lg">{formatNumber(projections.length)}</div>
      </div>
      <div>
        <div class="text-sm text-muted">{m('admin.system.entries')}</div>
        <div class="font-mono text-lg">{formatNumber(totalEntries)}</div>
      </div>
      <div>
        <div class="text-sm text-muted">{m('admin.system.projection_memory')}</div>
        <div class="font-mono text-lg">{formatBytes(totalEstimatedBytes)}</div>
      </div>
      <div>
        <div class="text-sm text-muted">{m('admin.system.average_entry_size')}</div>
        <div class="font-mono text-lg">{formatBytes(averageProjectionEntryBytes)}</div>
      </div>
      <div>
        <div class="text-sm text-muted">{m('admin.system.projection_failures')}</div>
        <div class={['font-mono text-lg', failedProjectionCount > 0 ? 'text-danger' : '']}>
          {formatNumber(failedProjectionCount)}
        </div>
      </div>
      <div>
        <div class="text-sm text-muted">{m('admin.system.projection_lag')}</div>
        <div class={['font-mono text-lg', laggingCount > 0 ? 'text-warning' : '']}>
          {formatNumber(laggingCount)}
        </div>
      </div>
    </div>
  </Panel>

  <Panel title={m('admin.system.projections')} icon="iconify icon-[uil--chart-line]" noPadding>
    <DataTable items={projections} columns={7} emptyMessage={m('admin.system.no_projections')}>
      {#snippet header()}
        <th class="table-header-cell">{m('admin.system.projection')}</th>
        <th class="table-header-cell">{m('admin.system.state')}</th>
        <th class="table-header-cell">{m('admin.system.startup')}</th>
        <th class="table-header-cell">{m('admin.system.applied')}</th>
        <th class="table-header-cell">{m('admin.system.lag')}</th>
        <th class="table-header-cell">{m('admin.system.entries')}</th>
        <th class="table-header-cell">{m('admin.system.memory')}</th>
      {/snippet}
      {#snippet row(projection)}
        <td class="px-4 py-3">
          <div class="font-medium">{projection.name}</div>
        </td>
        <td class="px-4 py-3">
          <div class="flex flex-wrap gap-1">
            <Pill tone={projection.failed ? 'danger' : projection.started ? 'success' : 'muted'}>
              {projection.failed
                ? m('admin.system.failed')
                : projection.started
                  ? m('admin.system.started')
                  : m('admin.system.stopped')}
            </Pill>
          </div>
          {#if projection.failed}
            <div class="mt-1 max-w-[28rem] font-mono text-xs break-words text-danger">
              {projection.failure}
            </div>
          {/if}
        </td>
        <td class="px-4 py-3 font-mono text-sm whitespace-nowrap">
          <span class={[projection.startupDurationSeconds == null ? 'text-muted' : '']}>
            {formatDurationSeconds(projection.startupDurationSeconds)}
          </span>
        </td>
        <td class="px-4 py-3 font-mono text-sm whitespace-nowrap">
          {projection.lastAppliedSequence}
          <span class="text-muted">/ {projection.matchingStreamSequence}</span>
          {#if projection.failed}
            <div class="text-xs text-danger">
              {m('admin.system.failed_at', { sequence: projection.failedSequence })}
            </div>
          {/if}
        </td>
        <td class="px-4 py-3">
          <span class={[projection.lag > 0 ? 'font-semibold text-warning' : '']}>
            {formatNumber(projection.lag)}
          </span>
        </td>
        <td class="px-4 py-3 font-mono text-sm">{formatNumber(projection.entryCount)}</td>
        <td class="px-4 py-3">
          <div class="font-mono text-sm whitespace-nowrap">
            {formatBytes(projection.estimatedBytes)}
          </div>
          <div class="text-xs whitespace-nowrap text-muted">
            {m('admin.system.average_entry_value', {
              size: formatBytes(projection.averageEntryBytes)
            })}
          </div>
        </td>
      {/snippet}
    </DataTable>
  </Panel>
{:else}
  <Hint>{m('admin.system.projections_unavailable')}</Hint>
{/if}
