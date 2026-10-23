<!--
@component

Streams section of the System page: the JetStream streams and their
consumers, with consumer backlog and acknowledgement state.
-->
<script lang="ts">
  import { formatBytes, formatNumber } from '$lib/components/admin';
  import { DataTable, Hint, Panel, Pill } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { useSystem } from '../systemContext';

  const system = useSystem();

  const streams = $derived(system.info.nats.streams);
  const consumers = $derived(system.info.nats.consumers);

  function consumerFilters(consumer: {
    filterSubject: string;
    filterSubjects: string[];
  }): string[] {
    if (consumer.filterSubjects.length > 0) return consumer.filterSubjects;
    if (consumer.filterSubject) return [consumer.filterSubject];
    return [m('admin.system.all_subjects')];
  }
</script>

{#if system.info.natsAvailable}
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
