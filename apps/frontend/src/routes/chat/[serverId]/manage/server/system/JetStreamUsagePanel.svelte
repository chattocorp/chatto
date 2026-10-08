<!--
@component

Shows JetStream account usage against its limits, with a meter for each
limited resource, and the largest stream. All values come from the broker,
so every replica reports the same data.
-->
<script lang="ts">
  import type { AdminAccountInfo, AdminNatsStats } from '$lib/api/adminDiagnostics';
  import { formatBytes, formatNumber } from '$lib/components/admin';
  import { Hint, Panel } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { STORAGE_CRITICAL_RATIO, STORAGE_WARNING_RATIO } from './systemHealth';

  let {
    account,
    accountAvailable,
    nats,
    natsAvailable
  }: {
    account: AdminAccountInfo;
    accountAvailable: boolean;
    nats: AdminNatsStats;
    natsAvailable: boolean;
  } = $props();

  type Meter = {
    id: string;
    label: string;
    used: number;
    limit: number;
    format: (value: number) => string;
    note?: string;
    warning?: string;
  };

  const fileStreams = $derived(nats.streams.filter((stream) => stream.storage === 'File'));
  const fileStreamCount = $derived(fileStreams.length);
  const memoryStreamCount = $derived(nats.streams.filter((s) => s.storage === 'Memory').length);
  const pullConsumerCount = $derived(nats.consumers.filter((c) => c.pullBased).length);
  const pushConsumerCount = $derived(nats.consumers.length - pullConsumerCount);
  const unboundPushConsumerCount = $derived(
    nats.consumers.filter((c) => !c.pullBased && !c.pushBound).length
  );
  const largestStream = $derived(
    nats.streams.reduce<(typeof nats.streams)[number] | null>(
      (largest, stream) => (!largest || stream.bytes > largest.bytes ? stream : largest),
      null
    )
  );

  // JetStream counts every replica's copy against the account storage, so
  // a replicated stream uses a multiple of its own size.
  const fileStreamBytes = $derived(fileStreams.reduce((sum, stream) => sum + stream.bytes, 0));
  const includesReplicaCopies = $derived(
    natsAvailable && fileStreams.some((stream) => stream.replicas > 1)
  );

  const meters = $derived<Meter[]>([
    {
      id: 'storage',
      label: m('admin.system.account_storage'),
      used: account.storageUsed,
      limit: account.storage,
      format: formatBytes,
      note: includesReplicaCopies
        ? m('admin.system.usage.replica_copies_note', { size: formatBytes(fileStreamBytes) })
        : undefined
    },
    {
      id: 'memory',
      label: m('admin.system.account_memory'),
      used: account.memoryUsed,
      limit: account.memory,
      format: formatBytes
    },
    {
      id: 'streams',
      label: m('admin.system.streams'),
      used: account.streamsUsed,
      limit: account.streams,
      format: formatNumber,
      note: natsAvailable
        ? m('admin.system.usage.stream_mix', {
            file: formatNumber(fileStreamCount),
            memory: formatNumber(memoryStreamCount)
          })
        : undefined
    },
    {
      id: 'consumers',
      label: m('admin.system.consumers'),
      used: account.consumersUsed,
      limit: account.consumers,
      format: formatNumber,
      note: natsAvailable
        ? m('admin.system.usage.consumer_mix', {
            pull: formatNumber(pullConsumerCount),
            push: formatNumber(pushConsumerCount)
          })
        : undefined,
      warning:
        unboundPushConsumerCount > 0
          ? m('admin.system.usage.unbound_count', { count: unboundPushConsumerCount })
          : undefined
    }
  ]);

  function ratio(meter: Meter): number {
    return meter.limit > 0 ? Math.min(meter.used / meter.limit, 1) : 0;
  }

  function barTone(value: number): string {
    if (value >= STORAGE_CRITICAL_RATIO) return 'bg-danger';
    if (value >= STORAGE_WARNING_RATIO) return 'bg-warning';
    return 'bg-action';
  }
</script>

<Panel title={m('admin.system.usage.title')} icon="iconify icon-[uil--hdd]">
  <div class="flex flex-col gap-5">
    {#if accountAvailable}
      <div class="flex flex-col gap-4">
        {#each meters as meter (meter.id)}
          <div>
            <div class="flex items-baseline justify-between gap-3">
              <span class="text-sm text-muted">{meter.label}</span>
              <span class="text-sm whitespace-nowrap">
                {#if meter.limit > 0}
                  {m('admin.system.used_of_limit', {
                    used: meter.format(meter.used),
                    limit: meter.format(meter.limit)
                  })}
                {:else}
                  <span class="font-medium tabular-nums">{meter.format(meter.used)}</span>
                  <span class="text-muted">· {m('admin.system.usage.no_limit')}</span>
                {/if}
              </span>
            </div>
            {#if meter.limit > 0}
              <div
                class="mt-1.5 h-1.5 overflow-hidden rounded-full bg-surface-emphasized"
                role="meter"
                aria-label={meter.label}
                aria-valuemin={0}
                aria-valuemax={meter.limit}
                aria-valuenow={Math.min(meter.used, meter.limit)}
                aria-valuetext={m('admin.system.used_of_limit', {
                  used: meter.format(meter.used),
                  limit: meter.format(meter.limit)
                })}
              >
                <div
                  class={['h-full rounded-full', barTone(ratio(meter))]}
                  style:width="{ratio(meter) * 100}%"
                ></div>
              </div>
            {/if}
            {#if meter.note || meter.warning}
              <div class="mt-1 text-xs text-muted">
                {meter.note}
                {#if meter.warning}
                  <span class="text-warning">· {meter.warning}</span>
                {/if}
              </div>
            {/if}
          </div>
        {/each}
      </div>
    {:else}
      <Hint>{m('admin.system.account_unavailable')}</Hint>
    {/if}

    {#if natsAvailable && largestStream}
      <div class="border-t border-border pt-4">
        <div class="text-sm text-muted">{m('admin.system.largest_stream')}</div>
        <div class="text-sm">
          <span class="font-medium">{largestStream.name}</span>
          <span class="text-muted">
            · {formatBytes(largestStream.bytes)} ·
            {m('admin.system.usage.messages_count', { count: largestStream.messages })}
          </span>
        </div>
      </div>
    {/if}
  </div>
</Panel>
