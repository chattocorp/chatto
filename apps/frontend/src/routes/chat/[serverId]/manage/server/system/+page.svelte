<script lang="ts">
  import { errorMessage } from '$lib/utils/errorMessage';
  import { getAdminSystemInfo } from '$lib/api/adminDiagnostics';
  import { formatBytes, formatNumber } from '$lib/components/admin';
  import {
    DataTable,
    Panel,
    Hint,
    PaneContent,
    Pill,
    StatCard,
    LoadingFog,
    PaneHeader,
    PageTitle,
    HeaderIconButton
  } from '$lib/ui';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createQuery } from '$lib/query/client';
  import { m } from '$lib/i18n/messages';
  import AssetCleanupPanel from './AssetCleanupPanel.svelte';
  import BrokerPanel from './BrokerPanel.svelte';
  import DurableWorkersPanel from './DurableWorkersPanel.svelte';
  import JetStreamUsagePanel from './JetStreamUsagePanel.svelte';
  import SystemHealthPanel from './SystemHealthPanel.svelte';

  const serverScope = useServerScope();

  const systemInfoQuery = createQuery(() => {
    const serverId = serverScope.serverId;
    const activeConnection = serverScope.connection;
    return {
      queryKey: adminQueryKeys.systemInfo(serverId, activeConnection),
      queryFn: ({ signal }) => getAdminSystemInfo(activeConnection.apiConfig, { signal })
    };
  });

  const systemInfo = $derived(systemInfoQuery.data ?? null);
  const loading = $derived(systemInfoQuery.isPending);
  const error = $derived(systemInfoQuery.error ? errorMessage(systemInfoQuery.error) : null);

  const streams = $derived(systemInfo?.nats?.streams ?? []);
  const consumers = $derived(systemInfo?.nats?.consumers ?? []);
  const projections = $derived(
    [...(systemInfo?.projections ?? [])].sort((a, b) => {
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
  /** Chatto's event log; other streams hold derived or bounded data. */
  const eventStream = $derived(streams.find((stream) => stream.name === 'EVT') ?? null);
  const averageEventBytes = $derived(
    eventStream && eventStream.messages > 0 ? eventStream.bytes / eventStream.messages : 0
  );
  const averageProjectionEntryBytes = $derived(
    totalEntries > 0 ? totalEstimatedBytes / totalEntries : 0
  );
  function consumerFilters(consumer: {
    filterSubject: string;
    filterSubjects: string[];
  }): string[] {
    if (consumer.filterSubjects.length > 0) return consumer.filterSubjects;
    if (consumer.filterSubject) return [consumer.filterSubject];
    return [m('admin.system.all_subjects')];
  }

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

<PageTitle title={m('admin.common.page_title', { title: m('admin.system.title') })} />

<div class="pane-page">
  <PaneHeader title={m('admin.system.title')} subtitle={m('admin.system.subtitle')}>
    {#snippet actions()}
      <HeaderIconButton
        icon="icon-[uil--sync]"
        label={m('admin.system.refresh')}
        disabled={systemInfoQuery.isFetching}
        onclick={() => systemInfoQuery.refetch()}
      />
    {/snippet}
  </PaneHeader>

  <PaneContent>
    <div class="flex flex-col gap-6">
      {#if loading}
        <LoadingFog class="h-40 w-full" label={m('admin.system.loading')} />
      {:else if error}
        <Hint tone="danger">{error}</Hint>
      {:else if systemInfo}
        <SystemHealthPanel info={systemInfo} />

        {#if systemInfo.statsAvailable || systemInfo.natsAvailable}
          <div class="grid grid-cols-2 gap-4 xl:grid-cols-4">
            {#if systemInfo.statsAvailable}
              <StatCard
                value={formatNumber(systemInfo.stats.userCount)}
                label={m('admin.system.overview.users')}
                icon="iconify icon-[uil--users-alt]"
              />
              <StatCard
                value={formatNumber(systemInfo.stats.channelRoomCount)}
                label={m('admin.system.overview.rooms')}
                icon="iconify icon-[uil--comments]"
                subtitle={m('admin.system.overview.direct_messages_count', {
                  count: systemInfo.stats.dmRoomCount
                })}
              />
            {/if}
            {#if systemInfo.natsAvailable}
              <StatCard
                value={formatNumber(eventStream?.messages ?? 0)}
                label={m('admin.system.overview.events')}
                icon="iconify icon-[uil--history]"
                subtitle={m('admin.system.average_message_size', {
                  size: formatBytes(averageEventBytes)
                })}
              />
              <StatCard
                value={formatBytes(systemInfo.nats.totalBytes)}
                label={m('admin.system.overview.stored_data')}
                icon="iconify icon-[uil--database]"
                subtitle={m('admin.system.overview.stored_data_subtitle_count', {
                  count: streams.length
                })}
              />
            {/if}
          </div>
        {/if}

        <div class="grid items-start gap-6 lg:grid-cols-2">
          <JetStreamUsagePanel
            account={systemInfo.account}
            accountAvailable={systemInfo.accountAvailable}
            nats={systemInfo.nats}
            natsAvailable={systemInfo.natsAvailable}
          />
          <BrokerPanel connection={systemInfo.connection} />
        </div>

        {#if systemInfo.natsAvailable}
          <Panel title={m('admin.system.streams')} icon="iconify icon-[uil--exchange]" noPadding>
            <DataTable items={streams} columns={6} emptyMessage={m('admin.system.no_streams')}>
              {#snippet header()}
                <th class="table-header-cell">{m('admin.system.stream')}</th>
                <th class="table-header-cell">{m('admin.system.storage')}</th>
                <th class="table-header-cell">{m('admin.system.messages')}</th>
                <th class="table-header-cell">{m('admin.system.bytes')}</th>
                <th class="table-header-cell">{m('admin.system.consumers')}</th>
                <th class="table-header-cell">{m('admin.system.replicas')}</th>
              {/snippet}
              {#snippet row(stream)}
                <td class="px-4 py-3">
                  <div class="font-medium">{stream.name}</div>
                  {#if stream.description}
                    <div class="text-xs text-muted">{stream.description}</div>
                  {/if}
                </td>
                <td class="px-4 py-3">{stream.storage}</td>
                <td class="px-4 py-3 font-mono text-sm">{formatNumber(stream.messages)}</td>
                <td class="px-4 py-3 font-mono text-sm">{formatBytes(stream.bytes)}</td>
                <td class="px-4 py-3 font-mono text-sm">{formatNumber(stream.consumerCount)}</td>
                <td class="px-4 py-3">
                  <div class="font-mono text-sm">{formatNumber(stream.replicas)}</div>
                  {#if stream.clusterLeader}
                    <div class="text-xs text-muted">{stream.clusterLeader}</div>
                  {/if}
                </td>
              {/snippet}
            </DataTable>
          </Panel>

          <Panel title={m('admin.system.consumers')} icon="iconify icon-[uil--users-alt]" noPadding>
            <DataTable items={consumers} columns={7} emptyMessage={m('admin.system.no_consumers')}>
              {#snippet header()}
                <th class="table-header-cell">{m('admin.system.consumer')}</th>
                <th class="table-header-cell">{m('admin.system.mode')}</th>
                <th class="table-header-cell">{m('admin.system.filters')}</th>
                <th class="table-header-cell">{m('admin.system.pending')}</th>
                <th class="table-header-cell">{m('admin.system.ack_pending')}</th>
                <th class="table-header-cell">{m('admin.system.redelivered')}</th>
                <th class="table-header-cell">{m('admin.system.acked_through')}</th>
              {/snippet}
              {#snippet row(consumer)}
                <td class="px-4 py-3">
                  <div class="font-medium">{consumer.name}</div>
                  <div class="font-mono text-xs text-muted">{consumer.stream}</div>
                  {#if consumer.durable}
                    <div class="text-xs text-muted">
                      {m('admin.system.durable', { name: consumer.durable })}
                    </div>
                  {/if}
                </td>
                <td class="px-4 py-3">
                  <div class="flex flex-wrap gap-1">
                    <Pill tone={consumer.pullBased ? 'neutral' : 'muted'}>
                      {consumer.pullBased ? m('admin.system.pull') : m('admin.system.push')}
                    </Pill>
                    {#if !consumer.pullBased}
                      <Pill tone={consumer.pushBound ? 'success' : 'danger'}>
                        {consumer.pushBound ? m('admin.system.bound') : m('admin.system.unbound')}
                      </Pill>
                    {/if}
                  </div>
                  <div class="mt-1 text-xs text-muted">{consumer.ackPolicy}</div>
                </td>
                <td class="px-4 py-3">
                  <div class="flex flex-wrap gap-1">
                    {#each consumerFilters(consumer) as filter (filter)}
                      <span
                        class="rounded border border-border px-1.5 py-0.5 font-mono text-[11px] text-muted"
                      >
                        {filter}
                      </span>
                    {/each}
                  </div>
                </td>
                <td class="px-4 py-3">
                  <span class={[consumer.pending > 0 ? 'font-semibold text-warning' : '']}>
                    {formatNumber(consumer.pending)}
                  </span>
                </td>
                <td class="px-4 py-3">
                  <span class={[consumer.ackPending > 0 ? 'font-semibold text-warning' : '']}>
                    {formatNumber(consumer.ackPending)}
                  </span>
                </td>
                <td class="px-4 py-3 font-mono text-sm">{formatNumber(consumer.redelivered)}</td>
                <td class="px-4 py-3 whitespace-nowrap">
                  <div class="font-mono text-sm">
                    {m('admin.system.ack_floor_stream', {
                      sequence: consumer.ackFloorStreamSequence
                    })}
                  </div>
                  <div class="font-mono text-xs text-muted">
                    {m('admin.system.ack_floor_consumer', {
                      sequence: consumer.ackFloorConsumerSequence
                    })}
                  </div>
                </td>
              {/snippet}
            </DataTable>
          </Panel>
        {:else}
          <Hint>{m('admin.system.jetstream_unavailable')}</Hint>
        {/if}

        {#if systemInfo.projectionsAvailable}
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

          <Panel
            title={m('admin.system.projections')}
            icon="iconify icon-[uil--chart-line]"
            noPadding
          >
            <DataTable
              items={projections}
              columns={7}
              emptyMessage={m('admin.system.no_projections')}
            >
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
                    <Pill
                      tone={projection.failed ? 'danger' : projection.started ? 'success' : 'muted'}
                    >
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

        <DurableWorkersPanel workers={systemInfo.durableWorkers} />
        <AssetCleanupPanel status={systemInfo.assetCleanup} />
      {/if}
    </div>
  </PaneContent>
</div>
