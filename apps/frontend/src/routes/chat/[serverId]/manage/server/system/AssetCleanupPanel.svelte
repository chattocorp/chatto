<script lang="ts">
  import type { AdminAssetCleanupStatus } from '$lib/api-client/adminDiagnostics';
  import { Panel } from '$lib/components/admin';
  import { Pill } from '$lib/ui';
  import { m } from '$lib/i18n/messages';

  let { status }: { status: AdminAssetCleanupStatus } = $props();

  const caughtUp = $derived(status.lastInspectedSequence === status.latestDeletionSequence);
  const tone = $derived(
    status.health === 'healthy'
      ? 'success'
      : status.health === 'retrying' || status.health === 'initializing'
        ? 'action'
        : status.health === 'unavailable'
          ? 'muted'
          : 'danger'
  );
  const healthLabel = $derived(
    status.health === 'healthy'
      ? m('admin.system.asset_cleanup_healthy')
      : status.health === 'retrying'
        ? m('admin.system.asset_cleanup_retrying')
        : status.health === 'initializing'
          ? m('admin.system.asset_cleanup_initializing')
          : status.health === 'stalled'
            ? m('admin.system.asset_cleanup_stalled')
            : status.health === 'inactive'
              ? m('admin.system.asset_cleanup_inactive')
              : m('admin.system.asset_cleanup_unavailable')
  );
  const summary = $derived(
    status.health === 'healthy'
      ? m('admin.system.asset_cleanup_healthy_summary')
      : status.health === 'retrying'
        ? m('admin.system.asset_cleanup_retrying_summary')
        : status.health === 'initializing'
          ? m('admin.system.asset_cleanup_initializing_summary')
          : status.health === 'stalled'
            ? m('admin.system.asset_cleanup_stalled_summary')
            : status.health === 'inactive'
              ? m('admin.system.asset_cleanup_inactive_summary')
              : m('admin.system.asset_cleanup_unavailable_summary')
  );

</script>

<Panel title={m('admin.system.asset_cleanup')} icon="iconify icon-[uil--trash-alt]">
  <div class="flex flex-col gap-4">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div class="min-w-0">
        <div class="text-sm text-muted">{m('admin.common.status')}</div>
        <div class="mt-1"><Pill {tone}>{healthLabel}</Pill></div>
      </div>
      {#if status.health === 'healthy' || status.health === 'retrying' || status.health === 'unavailable'}
        <div class="max-w-2xl text-sm text-muted">{summary}</div>
      {/if}
    </div>

    <div class="grid grid-cols-2 gap-x-6 gap-y-4">
      <div>
        <div class="text-sm text-muted">{m('admin.system.asset_cleanup_pending')}</div>
        <div class={['font-mono text-lg', status.pendingCount > 0 ? 'text-warning' : '']}>
          {status.available ? status.pendingCount.toLocaleString() : '-'}
        </div>
      </div>
      <div>
        <div class="text-sm text-muted">{m('admin.system.asset_cleanup_event_scan')}</div>
        <div class={['text-sm', !caughtUp && status.available ? 'text-warning' : '']}>
          {!status.available
            ? '-'
            : caughtUp
              ? m('admin.system.asset_cleanup_caught_up')
              : m('admin.system.asset_cleanup_events_waiting')}
        </div>
      </div>
    </div>
  </div>
</Panel>
