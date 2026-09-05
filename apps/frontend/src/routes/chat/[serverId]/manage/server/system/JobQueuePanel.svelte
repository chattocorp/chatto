<script lang="ts">
  import type { AdminNatsStreamInfo } from '$lib/api-client/adminDiagnostics';
  import { formatBytes, formatNumber } from '$lib/components/admin';
  import { m } from '$lib/i18n/messages';
  import { getReactiveLocale } from '$lib/i18n/state.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import { Hint } from '$lib/ui';

  let { queue }: { queue: AdminNatsStreamInfo | undefined } = $props();

  function duration(seconds: number | null | undefined): string {
    if (seconds == null) return '—';
    const [scale, unit]: [number, string] =
      seconds >= 86400
        ? [86400, 'day']
        : seconds >= 3600
          ? [3600, 'hour']
          : seconds >= 60
            ? [60, 'minute']
            : [1, 'second'];
    return new Intl.NumberFormat(getReactiveLocale(), {
      style: 'unit',
      unit,
      unitDisplay: 'short',
      maximumFractionDigits: 1
    }).format(seconds / scale);
  }
</script>

<!-- @component Current background work retained by the shared queue, without job contents. -->
<Panel title={m('admin.system.jobs.title')} icon="iconify icon-[uil--briefcase]">
  {#if queue}
    <dl class="grid grid-cols-2 gap-4 lg:grid-cols-4" data-testid="job-queue-stats">
      <div>
        <dt class="text-muted">{m('admin.system.jobs.outstanding')}</dt>
        <dd class="font-mono">{formatNumber(queue.messages)}</dd>
      </div>
      <div>
        <dt class="text-muted">{m('admin.system.jobs.oldest')}</dt>
        <dd class="font-mono">{duration(queue.oldestMessageAgeSeconds)}</dd>
      </div>
      <div>
        <dt class="text-muted">{m('admin.system.jobs.storage')}</dt>
        <dd class="font-mono">{formatBytes(queue.bytes)}</dd>
      </div>
      <div>
        <dt class="text-muted">{m('admin.system.jobs.retention')}</dt>
        <dd class="font-mono">
          {queue.maxAgeSeconds === 0 ? m('admin.system.unlimited') : duration(queue.maxAgeSeconds)}
        </dd>
      </div>
    </dl>
    <p class="mt-4 text-muted">{m('admin.system.jobs.description')}</p>
  {:else}
    <Hint>{m('admin.system.asset_cleanup_unavailable')}</Hint>
  {/if}
</Panel>
