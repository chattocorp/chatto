<!--
@component

Overview section of the System page: the health summary, server counts,
JetStream account usage, and the broker connection. Each health check links
to the section that shows its details.
-->
<script lang="ts">
  import { formatBytes, formatNumber } from '$lib/components/admin';
  import { StatCard } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import BrokerPanel from './BrokerPanel.svelte';
  import JetStreamUsagePanel from './JetStreamUsagePanel.svelte';
  import SystemHealthPanel from './SystemHealthPanel.svelte';
  import { useSystem } from './systemContext';

  const system = useSystem();
  const systemInfo = $derived(system.info);

  const streams = $derived(systemInfo.nats.streams);
  /** Chatto's event log; other streams hold derived or bounded data. */
  const eventStream = $derived(streams.find((stream) => stream.name === 'EVT') ?? null);
  const averageEventBytes = $derived(
    eventStream && eventStream.messages > 0 ? eventStream.bytes / eventStream.messages : 0
  );
</script>

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
