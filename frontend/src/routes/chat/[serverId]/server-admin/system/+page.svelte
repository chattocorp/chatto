<script lang="ts">
  import { graphql } from '$lib/gql';
  import { useQuery } from '$lib/hooks';
  import { Panel, StatCard, DataTable, formatBytes, formatNumber } from '$lib/components/admin';
  import { Hint, Pill } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import * as m from '$lib/i18n/messages';

  const AdminSystemInfoQuery = graphql(`
    query AdminSystemInfo {
      admin {
        systemInfo {
          connection {
            connected
            serverId
            serverName
            version
            maxPayload
            rtt
          }
          account {
            memory
            memoryUsed
            storage
            storageUsed
            streams
            streamsUsed
            consumers
            consumersUsed
          }
          nats {
            totalMessages
            totalBytes
            totalConsumerPending
            totalAckPending
            streams {
              name
              description
              subjects
              storage
              messages
              bytes
              firstSequence
              lastSequence
              consumerCount
              replicas
              clusterLeader
            }
            consumers {
              stream
              name
              durable
              filterSubject
              filterSubjects
              ackPolicy
              pullBased
              pushBound
              pending
              ackPending
              redelivered
              waiting
              deliveredConsumerSequence
              deliveredStreamSequence
              ackFloorConsumerSequence
              ackFloorStreamSequence
            }
          }
        }
        projections {
          name
          subjects
          started
          startupDurationSeconds
          lastAppliedSequence
          matchingStreamSequence
          streamLastSequence
          lag
          failed
          failedSequence
          failure
          entryCount
          estimatedBytes
          averageEntryBytes
        }
      }
    }
  `);

  const systemQuery = useQuery(AdminSystemInfoQuery, () => ({}));

  const systemInfo = $derived(systemQuery.data?.admin?.systemInfo ?? null);
  const streams = $derived(systemInfo?.nats.streams ?? []);
  const consumers = $derived(systemInfo?.nats.consumers ?? []);
  const projections = $derived(
    [...(systemQuery.data?.admin?.projections ?? [])].sort((a, b) => {
      if (a.failed !== b.failed) return a.failed ? -1 : 1;
      if (a.estimatedBytes !== b.estimatedBytes) return b.estimatedBytes - a.estimatedBytes;
      return a.name.localeCompare(b.name);
    })
  );
  const loading = $derived(systemQuery.loading);
  const error = $derived(systemQuery.error ?? null);
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

  function formatLimit(limit: number, formatter: (n: number) => string = String): string {
    return limit <= 0 ? m['admin.system.unlimited']() : formatter(limit);
  }

  function consumerFilters(consumer: {
    filterSubject: string;
    filterSubjects: string[];
  }): string[] {
    if (consumer.filterSubjects.length > 0) return consumer.filterSubjects;
    if (consumer.filterSubject) return [consumer.filterSubject];
    return [m['admin.system.all_subjects']()];
  }

  function formatDurationSeconds(seconds: number | null | undefined): string {
    if (seconds == null) return m['admin.system.pending_state']();
    if (seconds < 0.001) return '<1 ms';
    if (seconds < 1) return `${Math.round(seconds * 1000)} ms`;
    if (seconds < 10) return `${seconds.toFixed(2)} s`;
    if (seconds < 60) return `${seconds.toFixed(1)} s`;

    const minutes = Math.floor(seconds / 60);
    const remainingSeconds = Math.round(seconds % 60);
    return `${minutes}m ${remainingSeconds}s`;
  }
</script>

<PageTitle title={m['admin.common.page_title']({ title: m['admin.system.title']() })} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title={m['admin.system.title']()}
    subtitle={m['admin.system.subtitle']()}
    showMobileNav
  />

  <div class="min-h-0 flex-1 overflow-y-auto">
    <div class="flex flex-col gap-6 p-6">
      {#if loading}
        <div class="text-muted">{m['admin.system.loading']()}</div>
      {:else if error}
        <Hint tone="danger">{error}</Hint>
      {:else if systemInfo}
        <Panel title={m['admin.system.connection']()} icon="iconify uil--plug">
          <div class="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-5">
            <div>
              <div class="text-sm text-muted">{m['admin.common.status']()}</div>
              <div class="flex items-center gap-2">
                {systemInfo.connection.connected
                  ? m['admin.system.connected']()
                  : m['admin.system.disconnected']()}
                <span
                  class={[
                    'h-2 w-2 rounded-full',
                    systemInfo.connection.connected ? 'bg-success' : 'bg-danger'
                  ]}
                ></span>
              </div>
            </div>
            <div>
              <div class="text-sm text-muted">{m['admin.common.version']()}</div>
              <div class="font-mono text-sm">{systemInfo.connection.version}</div>
            </div>
            <div>
              <div class="text-sm text-muted">{m['admin.system.rtt']()}</div>
              <div class="font-mono text-sm">{systemInfo.connection.rtt || '-'}</div>
            </div>
            <div>
              <div class="text-sm text-muted">{m['admin.system.max_payload']()}</div>
              <div class="font-mono text-sm">{formatBytes(systemInfo.connection.maxPayload)}</div>
            </div>
            <div>
              <div class="text-sm text-muted">{m['admin.system.server_id']()}</div>
              <div class="truncate font-mono text-xs" title={systemInfo.connection.serverId}>
                {systemInfo.connection.serverId.slice(0, 12)}...
              </div>
            </div>
          </div>
        </Panel>

        <div class="grid grid-cols-2 gap-4 md:grid-cols-4">
          <StatCard
            value={formatBytes(systemInfo.account.storageUsed)}
            label={m['admin.system.account_storage']()}
            icon="iconify uil--hdd"
            color="primary"
            subtitle={m['admin.system.limit']({
              limit: formatLimit(systemInfo.account.storage, formatBytes)
            })}
          />
          <StatCard
            value={formatBytes(systemInfo.account.memoryUsed)}
            label={m['admin.system.memory']()}
            icon="iconify uil--processor"
            color="success"
            subtitle={m['admin.system.limit']({
              limit: formatLimit(systemInfo.account.memory, formatBytes)
            })}
          />
          <StatCard
            value={systemInfo.account.streamsUsed}
            label={m['admin.system.streams']()}
            icon="iconify uil--exchange"
            color="warning"
            subtitle={m['admin.system.limit']({ limit: formatLimit(systemInfo.account.streams) })}
          />
          <StatCard
            value={systemInfo.account.consumersUsed}
            label={m['admin.system.consumers']()}
            icon="iconify uil--users-alt"
            color="danger"
            subtitle={m['admin.system.limit']({
              limit: formatLimit(systemInfo.account.consumers)
            })}
          />
        </div>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-4">
          <StatCard
            value={formatNumber(systemInfo.nats.totalMessages)}
            label={m['admin.system.events']()}
            icon="iconify uil--database"
            color="primary"
          />
          <StatCard
            value={formatBytes(systemInfo.nats.totalBytes)}
            label={m['admin.system.event_bytes']()}
            icon="iconify uil--hdd"
            color="success"
          />
          <StatCard
            value={formatNumber(systemInfo.nats.totalConsumerPending)}
            label={m['admin.system.consumer_backlog']()}
            icon="iconify uil--clock"
            color={systemInfo.nats.totalConsumerPending > 0 ? 'warning' : 'success'}
            subtitle={m['admin.system.consumer_backlog_subtitle']({
              count: formatNumber(consumersWithBacklog)
            })}
          />
          <StatCard
            value={formatNumber(systemInfo.nats.totalAckPending)}
            label={m['admin.system.ack_pending']()}
            icon="iconify uil--check-circle"
            color={systemInfo.nats.totalAckPending > 0 ? 'warning' : 'success'}
          />
        </div>

        <Panel title={m['admin.system.streams']()} icon="iconify uil--exchange" noPadding>
          <DataTable items={streams} columns={6} emptyMessage={m['admin.system.no_streams']()}>
            {#snippet header()}
              <th class="px-4 py-3 font-medium">{m['admin.system.stream']()}</th>
              <th class="px-4 py-3 font-medium">{m['admin.system.storage']()}</th>
              <th class="px-4 py-3 font-medium">{m['admin.system.messages']()}</th>
              <th class="px-4 py-3 font-medium">{m['admin.system.bytes']()}</th>
              <th class="px-4 py-3 font-medium">{m['admin.system.consumers']()}</th>
              <th class="px-4 py-3 font-medium">{m['admin.system.replicas']()}</th>
            {/snippet}
            {#snippet row(stream)}
              <td class="px-4 py-3">
                <div class="font-medium">{stream.name}</div>
                {#if stream.description}
                  <div class="text-xs text-muted">{stream.description}</div>
                {/if}
                <div class="mt-1 flex flex-wrap gap-1">
                  {#each stream.subjects as subject (subject)}
                    <span
                      class="rounded border border-border px-1.5 py-0.5 font-mono text-[11px] text-muted"
                    >
                      {subject}
                    </span>
                  {/each}
                </div>
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

        <Panel title={m['admin.system.consumers']()} icon="iconify uil--users-alt" noPadding>
          <DataTable items={consumers} columns={7} emptyMessage={m['admin.system.no_consumers']()}>
            {#snippet header()}
              <th class="px-4 py-3 font-medium">{m['admin.system.consumer']()}</th>
              <th class="px-4 py-3 font-medium">{m['admin.system.mode']()}</th>
              <th class="px-4 py-3 font-medium">{m['admin.system.filters']()}</th>
              <th class="px-4 py-3 font-medium">{m['admin.system.pending']()}</th>
              <th class="px-4 py-3 font-medium">{m['admin.system.ack_pending']()}</th>
              <th class="px-4 py-3 font-medium">{m['admin.system.redelivered']()}</th>
              <th class="px-4 py-3 font-medium">{m['admin.system.acked_through']()}</th>
            {/snippet}
            {#snippet row(consumer)}
              <td class="px-4 py-3">
                <div class="font-medium">{consumer.name}</div>
                <div class="font-mono text-xs text-muted">{consumer.stream}</div>
                {#if consumer.durable}
                  <div class="text-xs text-muted">
                    {m['admin.system.durable']({ name: consumer.durable })}
                  </div>
                {/if}
              </td>
              <td class="px-4 py-3">
                <div class="flex flex-wrap gap-1">
                  <Pill tone={consumer.pullBased ? 'primary' : 'muted'}>
                    {consumer.pullBased ? m['admin.system.pull']() : m['admin.system.push']()}
                  </Pill>
                  {#if !consumer.pullBased}
                    <Pill tone={consumer.pushBound ? 'success' : 'danger'}>
                      {consumer.pushBound ? m['admin.system.bound']() : m['admin.system.unbound']()}
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

        <div class="grid grid-cols-1 gap-4 md:grid-cols-4">
          <StatCard
            value={formatNumber(projections.length)}
            label={m['admin.system.projections']()}
            icon="iconify uil--layers"
            color="primary"
          />
          <StatCard
            value={formatBytes(totalEstimatedBytes)}
            label={m['admin.system.projection_memory']()}
            icon="iconify uil--processor"
            color="success"
            subtitle={m['admin.system.projection_entries']({ count: formatNumber(totalEntries) })}
          />
          <StatCard
            value={formatNumber(failedProjectionCount)}
            label={m['admin.system.projection_failures']()}
            icon="iconify uil--exclamation-triangle"
            color={failedProjectionCount > 0 ? 'danger' : 'success'}
          />
          <StatCard
            value={formatNumber(laggingCount)}
            label={m['admin.system.projection_lag']()}
            icon="iconify uil--clock"
            color={laggingCount > 0 ? 'warning' : 'success'}
          />
        </div>

        <Panel title={m['admin.system.projections']()} icon="iconify uil--chart-line" noPadding>
          <DataTable
            items={projections}
            columns={7}
            emptyMessage={m['admin.system.no_projections']()}
          >
            {#snippet header()}
              <th class="px-4 py-3 font-medium">{m['admin.system.projection']()}</th>
              <th class="px-4 py-3 font-medium">{m['admin.system.state']()}</th>
              <th class="px-4 py-3 font-medium">{m['admin.system.startup']()}</th>
              <th class="px-4 py-3 font-medium">{m['admin.system.applied']()}</th>
              <th class="px-4 py-3 font-medium">{m['admin.system.lag']()}</th>
              <th class="px-4 py-3 font-medium">{m['admin.system.entries']()}</th>
              <th class="px-4 py-3 font-medium">{m['admin.system.memory']()}</th>
            {/snippet}
            {#snippet row(projection)}
              <td class="px-4 py-3">
                <div class="font-medium">{projection.name}</div>
                <div class="mt-1 flex flex-wrap gap-1">
                  {#each projection.subjects as subject (subject)}
                    <span
                      class="rounded border border-border px-1.5 py-0.5 font-mono text-[11px] text-muted"
                    >
                      {subject}
                    </span>
                  {/each}
                </div>
              </td>
              <td class="px-4 py-3">
                <div class="flex flex-wrap gap-1">
                  <Pill
                    tone={projection.failed ? 'danger' : projection.started ? 'success' : 'muted'}
                  >
                    {projection.failed
                      ? m['admin.system.failed']()
                      : projection.started
                        ? m['admin.system.started']()
                        : m['admin.system.stopped']()}
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
                    {m['admin.system.failed_at']({ sequence: projection.failedSequence })}
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
      {/if}
    </div>
  </div>
</div>
