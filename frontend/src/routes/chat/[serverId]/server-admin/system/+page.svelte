<script lang="ts">
  import { onMount } from 'svelte';
  import { Panel, StatCard, DataTable, formatBytes, formatNumber } from '$lib/components/admin';
  import { Hint, Pill } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import {
    GetAdminSystemInfoRequest,
    type AdminNatsConsumerInfoView,
    type AdminNatsStreamInfoView,
    type AdminProjectionStateView,
    type AdminSystemInfoView
  } from '$lib/pb/chatto/api/v1/chat_pb';
  import { withActiveServerWireClient } from '$lib/wire/activeServerClient';

  type StreamInfo = {
    name: string;
    description: string;
    subjects: string[];
    storage: string;
    messages: number;
    bytes: number;
    firstSequence: string;
    lastSequence: string;
    consumerCount: number;
    replicas: number;
    clusterLeader: string;
  };

  type ConsumerInfo = {
    stream: string;
    name: string;
    durable: string;
    filterSubject: string;
    filterSubjects: string[];
    ackPolicy: string;
    pullBased: boolean;
    pushBound: boolean;
    pending: number;
    ackPending: number;
    redelivered: number;
    waiting: number;
    deliveredConsumerSequence: string;
    deliveredStreamSequence: string;
    ackFloorConsumerSequence: string;
    ackFloorStreamSequence: string;
  };

  type SystemInfo = {
    connection: {
      connected: boolean;
      serverId: string;
      serverName: string;
      version: string;
      maxPayload: number;
      rtt: string;
    };
    account: {
      memory: number;
      memoryUsed: number;
      storage: number;
      storageUsed: number;
      streams: number;
      streamsUsed: number;
      consumers: number;
      consumersUsed: number;
    };
    nats: {
      totalMessages: number;
      totalBytes: number;
      totalConsumerPending: number;
      totalAckPending: number;
      streams: StreamInfo[];
      consumers: ConsumerInfo[];
    };
  };

  type ProjectionState = {
    name: string;
    subjects: string[];
    started: boolean;
    lastAppliedSequence: string;
    matchingStreamSequence: string;
    streamLastSequence: string;
    lag: number;
    failed: boolean;
    failedSequence: string;
    failure: string;
    entryCount: number;
    estimatedBytes: number;
    averageEntryBytes: number;
  };

  let systemInfo = $state<SystemInfo | null>(null);
  let projections = $state<ProjectionState[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);

  onMount(() => {
    void loadSystemInfo();
  });

  const streams = $derived(systemInfo?.nats.streams ?? []);
  const consumers = $derived(systemInfo?.nats.consumers ?? []);
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

  async function loadSystemInfo() {
    loading = true;
    error = null;

    try {
      const response = await withActiveServerWireClient((client) =>
        client.getAdminSystemInfo(new GetAdminSystemInfoRequest())
      );
      if (!response.systemInfo) {
        systemInfo = null;
        projections = [];
        error = 'System information is unavailable';
        return;
      }

      systemInfo = mapSystemInfo(response.systemInfo);
      projections = response.projections.map(mapProjectionState).sort(compareProjections);
    } catch (err) {
      systemInfo = null;
      projections = [];
      error = 'Failed to load system information';
      console.error('Failed to load system information', err);
    } finally {
      loading = false;
    }
  }

  function protoNumber(value: bigint | number | undefined): number {
    if (typeof value === 'bigint') return Number(value);
    return value ?? 0;
  }

  function mapSystemInfo(info: AdminSystemInfoView): SystemInfo {
    const connection = info.connection;
    const account = info.account;
    const nats = info.nats;

    return {
      connection: {
        connected: connection?.connected ?? false,
        serverId: connection?.serverId ?? '',
        serverName: connection?.serverName ?? '',
        version: connection?.version ?? '',
        maxPayload: protoNumber(connection?.maxPayload),
        rtt: connection?.rtt ?? ''
      },
      account: {
        memory: protoNumber(account?.memory),
        memoryUsed: protoNumber(account?.memoryUsed),
        storage: protoNumber(account?.storage),
        storageUsed: protoNumber(account?.storageUsed),
        streams: account?.streams ?? 0,
        streamsUsed: account?.streamsUsed ?? 0,
        consumers: account?.consumers ?? 0,
        consumersUsed: account?.consumersUsed ?? 0
      },
      nats: {
        totalMessages: protoNumber(nats?.totalMessages),
        totalBytes: protoNumber(nats?.totalBytes),
        totalConsumerPending: protoNumber(nats?.totalConsumerPending),
        totalAckPending: nats?.totalAckPending ?? 0,
        streams: nats?.streams.map(mapStreamInfo) ?? [],
        consumers: nats?.consumers.map(mapConsumerInfo) ?? []
      }
    };
  }

  function mapStreamInfo(stream: AdminNatsStreamInfoView): StreamInfo {
    return {
      name: stream.name,
      description: stream.description,
      subjects: [...stream.subjects],
      storage: stream.storage,
      messages: protoNumber(stream.messages),
      bytes: protoNumber(stream.bytes),
      firstSequence: stream.firstSequence,
      lastSequence: stream.lastSequence,
      consumerCount: stream.consumerCount,
      replicas: stream.replicas,
      clusterLeader: stream.clusterLeader
    };
  }

  function mapConsumerInfo(consumer: AdminNatsConsumerInfoView): ConsumerInfo {
    return {
      stream: consumer.stream,
      name: consumer.name,
      durable: consumer.durable,
      filterSubject: consumer.filterSubject,
      filterSubjects: [...consumer.filterSubjects],
      ackPolicy: consumer.ackPolicy,
      pullBased: consumer.pullBased,
      pushBound: consumer.pushBound,
      pending: protoNumber(consumer.pending),
      ackPending: consumer.ackPending,
      redelivered: consumer.redelivered,
      waiting: consumer.waiting,
      deliveredConsumerSequence: consumer.deliveredConsumerSequence,
      deliveredStreamSequence: consumer.deliveredStreamSequence,
      ackFloorConsumerSequence: consumer.ackFloorConsumerSequence,
      ackFloorStreamSequence: consumer.ackFloorStreamSequence
    };
  }

  function mapProjectionState(projection: AdminProjectionStateView): ProjectionState {
    return {
      name: projection.name,
      subjects: [...projection.subjects],
      started: projection.started,
      lastAppliedSequence: projection.lastAppliedSequence,
      matchingStreamSequence: projection.matchingStreamSequence,
      streamLastSequence: projection.streamLastSequence,
      lag: protoNumber(projection.lag),
      failed: projection.failed,
      failedSequence: projection.failedSequence,
      failure: projection.failure,
      entryCount: protoNumber(projection.entryCount),
      estimatedBytes: protoNumber(projection.estimatedBytes),
      averageEntryBytes: protoNumber(projection.averageEntryBytes)
    };
  }

  function compareProjections(a: ProjectionState, b: ProjectionState) {
    if (a.failed !== b.failed) return a.failed ? -1 : 1;
    if (a.estimatedBytes !== b.estimatedBytes) return b.estimatedBytes - a.estimatedBytes;
    return a.name.localeCompare(b.name);
  }

  function formatLimit(limit: number, formatter: (n: number) => string = String): string {
    return limit <= 0 ? 'unlimited' : formatter(limit);
  }

  function consumerFilters(consumer: {
    filterSubject: string;
    filterSubjects: string[];
  }): string[] {
    if (consumer.filterSubjects.length > 0) return consumer.filterSubjects;
    if (consumer.filterSubject) return [consumer.filterSubject];
    return ['all subjects'];
  }
</script>

<PageTitle title="System | Admin" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader title="System" subtitle="NATS, JetStream, and projection health" showMobileNav />

  <div class="min-h-0 flex-1 overflow-y-auto">
    <div class="flex flex-col gap-6 p-6">
      {#if loading}
        <div class="text-muted">Loading system information...</div>
      {:else if error}
        <Hint tone="danger">{error}</Hint>
      {:else if systemInfo}
        <Panel title="Connection" icon="iconify uil--plug">
          <div class="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-5">
            <div>
              <div class="text-sm text-muted">Status</div>
              <div class="flex items-center gap-2">
                {systemInfo.connection.connected ? 'Connected' : 'Disconnected'}
                <span
                  class={[
                    'h-2 w-2 rounded-full',
                    systemInfo.connection.connected ? 'bg-success' : 'bg-danger'
                  ]}
                ></span>
              </div>
            </div>
            <div>
              <div class="text-sm text-muted">Version</div>
              <div class="font-mono text-sm">{systemInfo.connection.version}</div>
            </div>
            <div>
              <div class="text-sm text-muted">RTT</div>
              <div class="font-mono text-sm">{systemInfo.connection.rtt || '-'}</div>
            </div>
            <div>
              <div class="text-sm text-muted">Max Payload</div>
              <div class="font-mono text-sm">{formatBytes(systemInfo.connection.maxPayload)}</div>
            </div>
            <div>
              <div class="text-sm text-muted">Server ID</div>
              <div class="truncate font-mono text-xs" title={systemInfo.connection.serverId}>
                {systemInfo.connection.serverId.slice(0, 12)}...
              </div>
            </div>
          </div>
        </Panel>

        <div class="grid grid-cols-2 gap-4 md:grid-cols-4">
          <StatCard
            value={formatBytes(systemInfo.account.storageUsed)}
            label="Account Storage"
            icon="iconify uil--hdd"
            color="primary"
            subtitle="of {formatLimit(systemInfo.account.storage, formatBytes)}"
          />
          <StatCard
            value={formatBytes(systemInfo.account.memoryUsed)}
            label="Memory"
            icon="iconify uil--processor"
            color="success"
            subtitle="of {formatLimit(systemInfo.account.memory, formatBytes)}"
          />
          <StatCard
            value={systemInfo.account.streamsUsed}
            label="Streams"
            icon="iconify uil--exchange"
            color="warning"
            subtitle="of {formatLimit(systemInfo.account.streams)}"
          />
          <StatCard
            value={systemInfo.account.consumersUsed}
            label="Consumers"
            icon="iconify uil--users-alt"
            color="danger"
            subtitle="of {formatLimit(systemInfo.account.consumers)}"
          />
        </div>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-4">
          <StatCard
            value={formatNumber(systemInfo.nats.totalMessages)}
            label="Events"
            icon="iconify uil--database"
            color="primary"
          />
          <StatCard
            value={formatBytes(systemInfo.nats.totalBytes)}
            label="Event Bytes"
            icon="iconify uil--hdd"
            color="success"
          />
          <StatCard
            value={formatNumber(systemInfo.nats.totalConsumerPending)}
            label="Consumer Backlog"
            icon="iconify uil--clock"
            color={systemInfo.nats.totalConsumerPending > 0 ? 'warning' : 'success'}
            subtitle={`${formatNumber(consumersWithBacklog)} consumer(s) with pending messages`}
          />
          <StatCard
            value={formatNumber(systemInfo.nats.totalAckPending)}
            label="Ack Pending"
            icon="iconify uil--check-circle"
            color={systemInfo.nats.totalAckPending > 0 ? 'warning' : 'success'}
          />
        </div>

        <Panel title="Streams" icon="iconify uil--exchange" noPadding>
          <DataTable items={streams} columns={6} emptyMessage="No streams are registered.">
            {#snippet header()}
              <th class="px-4 py-3 font-medium">Stream</th>
              <th class="px-4 py-3 font-medium">Storage</th>
              <th class="px-4 py-3 font-medium">Messages</th>
              <th class="px-4 py-3 font-medium">Bytes</th>
              <th class="px-4 py-3 font-medium">Consumers</th>
              <th class="px-4 py-3 font-medium">Replicas</th>
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

        <Panel title="Consumers" icon="iconify uil--users-alt" noPadding>
          <DataTable items={consumers} columns={7} emptyMessage="No consumers are registered.">
            {#snippet header()}
              <th class="px-4 py-3 font-medium">Consumer</th>
              <th class="px-4 py-3 font-medium">Mode</th>
              <th class="px-4 py-3 font-medium">Filters</th>
              <th class="px-4 py-3 font-medium">Pending</th>
              <th class="px-4 py-3 font-medium">Ack Pending</th>
              <th class="px-4 py-3 font-medium">Redelivered</th>
              <th class="px-4 py-3 font-medium">Acked Through</th>
            {/snippet}
            {#snippet row(consumer)}
              <td class="px-4 py-3">
                <div class="font-medium">{consumer.name}</div>
                <div class="font-mono text-xs text-muted">{consumer.stream}</div>
                {#if consumer.durable}
                  <div class="text-xs text-muted">Durable: {consumer.durable}</div>
                {/if}
              </td>
              <td class="px-4 py-3">
                <div class="flex flex-wrap gap-1">
                  <Pill tone={consumer.pullBased ? 'primary' : 'muted'}>
                    {consumer.pullBased ? 'Pull' : 'Push'}
                  </Pill>
                  {#if !consumer.pullBased}
                    <Pill tone={consumer.pushBound ? 'success' : 'danger'}>
                      {consumer.pushBound ? 'Bound' : 'Unbound'}
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
            label="Projections"
            icon="iconify uil--layers"
            color="primary"
          />
          <StatCard
            value={formatBytes(totalEstimatedBytes)}
            label="Projection Memory"
            icon="iconify uil--processor"
            color="success"
            subtitle={`${formatNumber(totalEntries)} projected entries`}
          />
          <StatCard
            value={formatNumber(failedProjectionCount)}
            label="Projection Failures"
            icon="iconify uil--exclamation-triangle"
            color={failedProjectionCount > 0 ? 'danger' : 'success'}
          />
          <StatCard
            value={formatNumber(laggingCount)}
            label="Projection Lag"
            icon="iconify uil--clock"
            color={laggingCount > 0 ? 'warning' : 'success'}
          />
        </div>

        <Panel title="Projections" icon="iconify uil--chart-line" noPadding>
          <DataTable items={projections} columns={6} emptyMessage="No projections are registered.">
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
                    {projection.failed ? 'Failed' : projection.started ? 'Started' : 'Stopped'}
                  </Pill>
                </div>
                {#if projection.failed}
                  <div class="mt-1 max-w-[28rem] font-mono text-xs break-words text-danger">
                    {projection.failure}
                  </div>
                {/if}
              </td>
              <td class="px-4 py-3 font-mono text-sm whitespace-nowrap">
                {projection.lastAppliedSequence}
                <span class="text-muted">/ {projection.matchingStreamSequence}</span>
                {#if projection.failed}
                  <div class="text-xs text-danger">failed at {projection.failedSequence}</div>
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
