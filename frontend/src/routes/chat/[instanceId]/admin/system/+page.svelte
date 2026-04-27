<script lang="ts">
  import { graphql } from '$lib/gql';
  import { useQuery } from '$lib/hooks';
  import { Panel, StatCard, formatBytes } from '$lib/components/admin';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  const AdminSystemInfoQuery = graphql(`
    query AdminSystemInfo {
      admin {
        systemInfo {
          connection {
            connected
            serverID
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
        }
      }
    }
  `);

  const systemQuery = useQuery(AdminSystemInfoQuery, () => ({}));

  let systemInfo = $derived(systemQuery.data?.admin?.systemInfo ?? null);
  let loading = $derived(systemQuery.loading);
  let error = $derived(systemQuery.error ?? null);

  function formatLimit(
    _used: number,
    limit: number,
    formatter: (n: number) => string = String
  ): string {
    // NATS uses -1 for unlimited storage/memory, and 0 for unlimited streams/consumers
    return limit <= 0 ? 'unlimited' : formatter(limit);
  }
</script>

<PageTitle title="System | Admin" />

<PaneHeader title="System" subtitle="NATS/JetStream status and aggregate usage" showMobileNav />

<div class="flex flex-col gap-6 overflow-auto p-6">
  {#if loading}
    <div class="text-muted">Loading system information...</div>
  {:else if error}
    <div class="rounded-xl border border-danger/20 bg-danger/10 p-4 text-danger">{error}</div>
  {:else if systemInfo}
    <!-- Connection Info -->
    <Panel title="Connection" icon="iconify uil--plug">
      <div class="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-5">
        <div>
          <div class="text-sm text-muted">Status</div>
          <div class="flex items-center gap-2">
            {systemInfo.connection.connected ? 'Connected' : 'Disconnected'}
            <span
              class="h-2 w-2 rounded-full"
              class:bg-success={systemInfo.connection.connected}
              class:bg-danger={!systemInfo.connection.connected}
            ></span>
          </div>
        </div>
        <div>
          <div class="text-sm text-muted">Version</div>
          <div class="font-mono text-sm">{systemInfo.connection.version}</div>
        </div>
        <div>
          <div class="text-sm text-muted">RTT</div>
          <div class="font-mono text-sm">{systemInfo.connection.rtt || '—'}</div>
        </div>
        <div>
          <div class="text-sm text-muted">Max Payload</div>
          <div class="font-mono text-sm">{formatBytes(systemInfo.connection.maxPayload)}</div>
        </div>
        <div>
          <div class="text-sm text-muted">Server ID</div>
          <div class="truncate font-mono text-xs" title={systemInfo.connection.serverID}>
            {systemInfo.connection.serverID.slice(0, 12)}...
          </div>
        </div>
      </div>
    </Panel>

    <!-- Account Usage -->
    <div class="grid grid-cols-2 gap-4 md:grid-cols-4">
      <StatCard
        value={formatBytes(systemInfo.account.storageUsed)}
        label="Storage"
        icon="iconify uil--hdd"
        color="primary"
        subtitle="of {formatLimit(
          systemInfo.account.storageUsed,
          systemInfo.account.storage,
          formatBytes
        )}"
      />
      <StatCard
        value={formatBytes(systemInfo.account.memoryUsed)}
        label="Memory"
        icon="iconify uil--processor"
        color="success"
        subtitle="of {formatLimit(
          systemInfo.account.memoryUsed,
          systemInfo.account.memory,
          formatBytes
        )}"
      />
      <StatCard
        value={systemInfo.account.streamsUsed}
        label="Streams"
        icon="iconify uil--exchange"
        color="warning"
        subtitle="of {formatLimit(systemInfo.account.streamsUsed, systemInfo.account.streams)}"
      />
      <StatCard
        value={systemInfo.account.consumersUsed}
        label="Consumers"
        icon="iconify uil--users-alt"
        color="danger"
        subtitle="of {formatLimit(systemInfo.account.consumersUsed, systemInfo.account.consumers)}"
      />
    </div>

    <!--
      Per-stream / per-bucket / per-object-store breakdowns intentionally
      removed: those leaked structural information (room IDs, user IDs,
      bucket names) without serving an operator use case the chatto CLI
      doesn't already cover. For raw NATS inspection, use the chatto-debugging
      skill (operator shell + nats CLI).
    -->
  {/if}
</div>
