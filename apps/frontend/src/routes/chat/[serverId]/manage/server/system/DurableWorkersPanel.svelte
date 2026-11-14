<script lang="ts">
  import type { AdminDurableWorkerStatus } from '$lib/api/adminDiagnostics';
  import { DataTable, Panel, Pill } from '$lib/ui';
  import { m } from '$lib/i18n/messages';

  let { workers }: { workers: AdminDurableWorkerStatus[] } = $props();

  // Keep the broker key visible for support while naming the new job pools.
  function workerLabel(key: string): string | null {
    switch (key) {
      case 'jobs':
        return m('admin.jobs.title');
      case 'media_jobs':
        return m('admin.jobs.media');
      default:
        return null;
    }
  }

  function healthLabel(health: AdminDurableWorkerStatus['health']): string {
    switch (health) {
      case 'healthy':
        return m('admin.system.asset_cleanup_healthy');
      case 'working':
        return m('admin.system.asset_cleanup_in_progress');
      case 'unconfirmed':
        return m('admin.system.worker_unconfirmed');
      case 'stalled':
        return m('admin.system.asset_cleanup_stalled');
      case 'inactive':
        return m('admin.system.asset_cleanup_inactive');
      default:
        return m('admin.system.asset_cleanup_unavailable');
    }
  }

  function healthTone(
    health: AdminDurableWorkerStatus['health']
  ): 'success' | 'action' | 'danger' | 'muted' {
    switch (health) {
      case 'healthy':
        return 'success';
      case 'working':
        return 'action';
      case 'unconfirmed':
        return 'muted';
      case 'stalled':
        return 'danger';
      default:
        return 'muted';
    }
  }

  function formatCount(value: string): string {
    try {
      return BigInt(value).toLocaleString();
    } catch {
      return value;
    }
  }
</script>

<Panel title={m('admin.system.durable_workers')} icon="iconify icon-[uil--cog]" noPadding>
  <DataTable items={workers} columns={6} emptyMessage={m('admin.system.no_durable_workers')}>
    {#snippet header()}
      <th class="table-header-cell">{m('admin.system.consumer')}</th>
      <th class="table-header-cell">{m('admin.system.state')}</th>
      <th class="table-header-cell">{m('admin.system.pending')}</th>
      <th class="table-header-cell">{m('admin.system.ack_pending')}</th>
      <th class="table-header-cell">{m('admin.system.redelivered')}</th>
      <th class="table-header-cell">{m('admin.system.acked_through')}</th>
    {/snippet}
    {#snippet row(worker)}
      {@const label = workerLabel(worker.key)}
      <td class="px-4 py-3">
        {#if label}<div class="font-medium">{label}</div>{/if}
        <div class="font-mono text-sm">{worker.key}</div>
      </td>
      <td class="px-4 py-3">
        <Pill tone={healthTone(worker.health)}>{healthLabel(worker.health)}</Pill>
      </td>
      <td class="px-4 py-3 font-mono text-sm">
        <span class={[worker.pendingCount !== '0' ? 'font-semibold text-warning' : '']}>
          {formatCount(worker.pendingCount)}
        </span>
      </td>
      <td class="px-4 py-3 font-mono text-sm">
        {formatCount(worker.ackPendingCount)}
      </td>
      <td class="px-4 py-3 font-mono text-sm">
        {formatCount(worker.redeliveredCount)}
      </td>
      <td class="px-4 py-3 whitespace-nowrap">
        <div class="font-mono text-sm">{worker.ackFloorSequence}</div>
        <div class="font-mono text-xs text-muted">/ {worker.lastDeliveredSequence}</div>
      </td>
    {/snippet}
  </DataTable>
</Panel>
