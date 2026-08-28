<script lang="ts">
  import { getAdminSystemInfo } from '$lib/api-client/adminDiagnostics';
  import { StatCard, formatBytes, formatNumber } from '$lib/components/admin';
  import DataTable from '$lib/ui/DataTable.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import { Hint, PaneContent, Pill } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createQuery } from '@tanstack/svelte-query';
  import { adminQueryKeys } from '$lib/query/admin';
  import { queryClient } from '$lib/query/client';
  import { m } from '$lib/i18n/messages';
  import AssetCleanupPanel from './AssetCleanupPanel.svelte';
  import DurableWorkersPanel from './DurableWorkersPanel.svelte';

  const serverScope = useServerScope();

  const systemInfoQuery = createQuery(
    () => {
      const serverId = serverScope.serverId;
      const activeConnection = serverScope.connection;
      return {
        queryKey: adminQueryKeys.systemInfo(serverId, activeConnection),
        queryFn: ({ signal }) => getAdminSystemInfo(activeConnection.apiConfig, { signal })
      };
    },
    () => queryClient
  );

  const systemInfo = $derived(systemInfoQuery.data ?? null);
  const loading = $derived(systemInfoQuery.isPending);
  const error = $derived(
    systemInfoQuery.error instanceof Error
      ? systemInfoQuery.error.message
      : systemInfoQuery.error == null
        ? null
        : String(systemInfoQuery.error)
  );

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
  const consumersWithBacklog = $derived(
    consumers.filter((consumer) => consumer.pending > 0).length
  );
  const fileStreamCount = $derived(streams.filter((stream) => stream.storage === 'File').length);
  const memoryStreamCount = $derived(
    streams.filter((stream) => stream.storage === 'Memory').length
  );
  const pullConsumerCount = $derived(consumers.filter((consumer) => consumer.pullBased).length);
  const pushConsumerCount = $derived(consumers.length - pullConsumerCount);
  const unboundPushConsumerCount = $derived(
    consumers.filter((consumer) => !consumer.pullBased && !consumer.pushBound).length
  );
  const totalRedelivered = $derived(
    consumers.reduce((sum, consumer) => sum + consumer.redelivered, 0)
  );
  const averageEventBytes = $derived(
    systemInfo?.natsAvailable && systemInfo.nats.totalMessages > 0
      ? systemInfo.nats.totalBytes / systemInfo.nats.totalMessages
      : 0
  );
  const averageProjectionEntryBytes = $derived(
    totalEntries > 0 ? totalEstimatedBytes / totalEntries : 0
  );
  const largestStream = $derived.by(() => {
    let largest = streams[0] ?? null;
    for (const stream of streams) {
      if (!largest || stream.bytes > largest.bytes) largest = stream;
    }
    return largest;
  });

  function formatLimit(limit: number, formatter: (n: number) => string = String): string {
    return limit <= 0 ? m('admin.system.unlimited') : formatter(limit);
  }

  function formatPercent(used: number, limit: number): string {
    if (limit <= 0) return m('admin.system.unlimited');
    return `${Math.round((used / limit) * 100)}%`;
  }

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
  <PaneHeader title={m('admin.system.title')} subtitle={m('admin.system.subtitle')} showMobileNav />

  <PaneContent>
    <div class="flex flex-col gap-6">
      {#if loading}
        <div class="text-muted">{m('admin.system.loading')}</div>
      {:else if error}
        <Hint tone="danger">{error}</Hint>
      {:else if systemInfo}
        <Panel title={m('admin.system.broker')} icon="iconify icon-[uil--server]">
          <div class="grid gap-4 lg:grid-cols-[minmax(0,1.2fr)_minmax(0,2fr)]">
            <div class="rounded-lg border border-border bg-surface/70 p-4">
              <div class="text-sm text-muted">{m('admin.common.status')}</div>
              <div class="mt-1 flex items-center gap-2 text-xl font-semibold">
                <span
                  class={[
                    'h-2.5 w-2.5 rounded-full',
                    systemInfo.connection.connected ? 'bg-success' : 'bg-danger'
                  ]}
                ></span>
                {systemInfo.connection.connected
                  ? m('admin.system.connected')
                  : m('admin.system.disconnected')}
              </div>
              <div
                class="mt-3 truncate font-mono text-xs text-muted"
                title={systemInfo.connection.serverId}
              >
                {systemInfo.connection.serverId || '-'}
              </div>
            </div>

            <div class="grid grid-cols-2 gap-x-6 gap-y-4 md:grid-cols-4">
              <div>
                <div class="text-sm text-muted">{m('admin.common.version')}</div>
                <div class="font-mono text-sm">{systemInfo.connection.version || '-'}</div>
              </div>
              <div>
                <div class="text-sm text-muted">{m('admin.system.rtt')}</div>
                <div class="font-mono text-sm">{systemInfo.connection.rtt || '-'}</div>
              </div>
              <div>
                <div class="text-sm text-muted">{m('admin.system.max_payload')}</div>
                <div class="font-mono text-sm">{formatBytes(systemInfo.connection.maxPayload)}</div>
              </div>
              <div>
                <div class="text-sm text-muted">{m('admin.system.server_name')}</div>
                <div class="truncate font-mono text-sm" title={systemInfo.connection.serverName}>
                  {systemInfo.connection.serverName || '-'}
                </div>
              </div>
            </div>
          </div>
        </Panel>

        {#if systemInfo.accountAvailable}
        <div>
          <h2 class="mb-3 text-sm font-semibold text-muted uppercase">
            {m('admin.system.jetstream_account')}
          </h2>
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <StatCard
              value={formatBytes(systemInfo.account.storageUsed)}
              label={m('admin.system.account_storage')}
              icon="iconify icon-[uil--hdd]"
              color="action"
              subtitle={m('admin.system.limit', {
                limit: formatLimit(systemInfo.account.storage, formatBytes)
              })}
            />
            <StatCard
              value={formatBytes(systemInfo.account.memoryUsed)}
              label={m('admin.system.account_memory')}
              icon="iconify icon-[uil--processor]"
              color="success"
              subtitle={m('admin.system.limit', {
                limit: formatLimit(systemInfo.account.memory, formatBytes)
              })}
            />
            <StatCard
              value={formatPercent(systemInfo.account.streamsUsed, systemInfo.account.streams)}
              label={m('admin.system.stream_capacity')}
              icon="iconify icon-[uil--exchange]"
              color="warning"
              subtitle={m('admin.system.used_of_limit', {
                used: formatNumber(systemInfo.account.streamsUsed),
                limit: formatLimit(systemInfo.account.streams)
              })}
            />
            <StatCard
              value={formatPercent(systemInfo.account.consumersUsed, systemInfo.account.consumers)}
              label={m('admin.system.consumer_capacity')}
              icon="iconify icon-[uil--users-alt]"
              color="danger"
              subtitle={m('admin.system.used_of_limit', {
                used: formatNumber(systemInfo.account.consumersUsed),
                limit: formatLimit(systemInfo.account.consumers)
              })}
            />
          </div>
        </div>
        {:else}
          <Hint>{m('admin.system.asset_cleanup_unavailable')}</Hint>
        {/if}

        {#if systemInfo.natsAvailable}
        <div>
          <h2 class="mb-3 text-sm font-semibold text-muted uppercase">
            {m('admin.system.stream_activity')}
          </h2>
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <StatCard
              value={formatNumber(systemInfo.nats.totalMessages)}
              label={m('admin.system.messages_stored')}
              icon="iconify icon-[uil--database]"
              color="action"
              subtitle={m('admin.system.average_message_size', {
                size: formatBytes(averageEventBytes)
              })}
            />
            <StatCard
              value={formatBytes(systemInfo.nats.totalBytes)}
              label={m('admin.system.stream_bytes')}
              icon="iconify icon-[uil--hdd]"
              color="success"
              subtitle={m('admin.system.storage_mix', {
                file: formatNumber(fileStreamCount),
                memory: formatNumber(memoryStreamCount)
              })}
            />
            <StatCard
              value={formatNumber(systemInfo.nats.totalConsumerPending)}
              label={m('admin.system.consumer_backlog')}
              icon="iconify icon-[uil--clock]"
              color={systemInfo.nats.totalConsumerPending > 0 ? 'warning' : 'success'}
              subtitle={m('admin.system.consumer_backlog_subtitle', {
                count: formatNumber(consumersWithBacklog)
              })}
            />
            <StatCard
              value={formatNumber(systemInfo.nats.totalAckPending)}
              label={m('admin.system.ack_pending')}
              icon="iconify icon-[uil--check-circle]"
              color={systemInfo.nats.totalAckPending > 0 ? 'warning' : 'success'}
              subtitle={m('admin.system.redelivered_total', {
                count: formatNumber(totalRedelivered)
              })}
            />
          </div>
        </div>

        <div class="grid gap-4 lg:grid-cols-2">
          <Panel title={m('admin.system.stream_summary')} icon="iconify icon-[uil--chart-line]">
            <div class="grid grid-cols-2 gap-x-6 gap-y-4">
              <div>
                <div class="text-sm text-muted">{m('admin.system.file_streams')}</div>
                <div class="font-mono text-lg">{formatNumber(fileStreamCount)}</div>
              </div>
              <div>
                <div class="text-sm text-muted">{m('admin.system.memory_streams')}</div>
                <div class="font-mono text-lg">{formatNumber(memoryStreamCount)}</div>
              </div>
              <div class="col-span-2">
                <div class="text-sm text-muted">{m('admin.system.largest_stream')}</div>
                {#if largestStream}
                  <div class="min-w-0">
                    <div class="truncate font-medium" title={largestStream.name}>
                      {largestStream.name}
                    </div>
                    <div class="font-mono text-sm text-muted">
                      {formatBytes(largestStream.bytes)} / {formatNumber(largestStream.messages)}
                      {m('admin.system.messages_lower')}
                    </div>
                  </div>
                {:else}
                  <div class="font-mono text-sm text-muted">-</div>
                {/if}
              </div>
            </div>
          </Panel>

          <Panel title={m('admin.system.consumer_summary')} icon="iconify icon-[uil--users-alt]">
            <div class="grid grid-cols-2 gap-x-6 gap-y-4">
              <div>
                <div class="text-sm text-muted">{m('admin.system.pull_consumers')}</div>
                <div class="font-mono text-lg">{formatNumber(pullConsumerCount)}</div>
              </div>
              <div>
                <div class="text-sm text-muted">{m('admin.system.push_consumers')}</div>
                <div class="font-mono text-lg">{formatNumber(pushConsumerCount)}</div>
              </div>
              <div>
                <div class="text-sm text-muted">{m('admin.system.unbound_push_consumers')}</div>
                <div
                  class={['font-mono text-lg', unboundPushConsumerCount > 0 ? 'text-warning' : '']}
                >
                  {formatNumber(unboundPushConsumerCount)}
                </div>
              </div>
              <div>
                <div class="text-sm text-muted">{m('admin.system.redelivered')}</div>
                <div class={['font-mono text-lg', totalRedelivered > 0 ? 'text-warning' : '']}>
                  {formatNumber(totalRedelivered)}
                </div>
              </div>
            </div>
          </Panel>

        </div>

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
                <div class="font-mono text-sm">stream {consumer.ackFloorStreamSequence}</div>
                <div class="font-mono text-xs text-muted">
                  consumer {consumer.ackFloorConsumerSequence}
                </div>
              </td>
            {/snippet}
          </DataTable>
        </Panel>
        {:else}
          <Hint>{m('admin.system.asset_cleanup_unavailable')}</Hint>
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
                  {formatBytes(projection.averageEntryBytes)} avg
                </div>
              </td>
            {/snippet}
          </DataTable>
        </Panel>
        {:else}
          <Hint>{m('admin.system.asset_cleanup_unavailable')}</Hint>
        {/if}

        <DurableWorkersPanel workers={systemInfo.durableWorkers} />
        <AssetCleanupPanel status={systemInfo.assetCleanup} />
      {/if}
    </div>
  </PaneContent>
</div>
