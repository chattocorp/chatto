<!--
@component

Summarizes System page diagnostics as one overall status and a short list of
health checks. See `systemHealth.ts` for the rules behind each status.
-->
<script lang="ts">
  import type { AdminSystemInfo } from '$lib/api/adminDiagnostics';
  import { Panel } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import {
    overallHealth,
    systemHealthChecks,
    type HealthCheck,
    type HealthCheckId,
    type HealthStatus
  } from './systemHealth';

  let { info }: { info: AdminSystemInfo } = $props();

  const checks = $derived(systemHealthChecks(info));
  const overall = $derived(overallHealth(checks));

  const statusIcon: Record<HealthStatus, string> = {
    ok: 'icon-[uil--check-circle] text-success',
    warning: 'icon-[uil--exclamation-triangle] text-warning',
    critical: 'icon-[uil--times-circle] text-danger',
    unknown: 'icon-[uil--question-circle] text-muted'
  };

  const detailTone: Record<HealthStatus, string> = {
    ok: 'text-muted',
    warning: 'text-warning',
    critical: 'text-danger',
    unknown: 'text-muted'
  };

  const statusLabel = $derived<Record<HealthStatus, string>>({
    ok: m('admin.system.health.status_ok'),
    warning: m('admin.system.health.status_warning'),
    critical: m('admin.system.health.status_critical'),
    unknown: m('admin.system.health.status_unknown')
  });

  const overallTitle = $derived(
    overall === 'critical'
      ? m('admin.system.health.overall_critical')
      : overall === 'warning'
        ? m('admin.system.health.overall_warning')
        : m('admin.system.health.overall_ok')
  );

  const overallSummary = $derived(
    overall !== 'ok'
      ? m('admin.system.health.overall_problem_summary')
      : checks.some((check) => check.status === 'unknown')
        ? m('admin.system.health.overall_partial_summary')
        : m('admin.system.health.overall_ok_summary')
  );

  const checkLabel = $derived<Record<HealthCheckId, string>>({
    broker: m('admin.system.health.broker'),
    projections: m('admin.system.health.projections'),
    workers: m('admin.system.health.workers'),
    backlog: m('admin.system.health.backlog'),
    storage: m('admin.system.health.storage'),
    voice_calls: m('admin.system.health.voice_calls')
  });

  function checkDetail({ detail }: HealthCheck): string {
    switch (detail.kind) {
      case 'unavailable':
        return m('admin.system.health.unavailable');
      case 'broker_connected':
        return m('admin.system.health.broker_connected', { version: detail.version || '-' });
      case 'broker_disconnected':
        return m('admin.system.health.broker_disconnected');
      case 'projections_failed':
        return m('admin.system.health.projections_failed_count', { count: detail.count });
      case 'projections_catching_up':
        return m('admin.system.health.projections_catching_up_count', { count: detail.count });
      case 'projections_running':
        return m('admin.system.health.projections_running_count', { count: detail.count });
      case 'workers_failing':
        return m('admin.system.health.workers_failing_count', { count: detail.count });
      case 'workers_unconfirmed':
        return m('admin.system.health.workers_unconfirmed_count', { count: detail.count });
      case 'workers_running':
        return m('admin.system.health.workers_running_count', { count: detail.count });
      case 'backlog_waiting':
        return m('admin.system.health.backlog_waiting_count', { count: detail.count });
      case 'backlog_clear':
        return m('admin.system.health.backlog_clear');
      case 'storage_used':
        return m('admin.system.health.storage_used', { percent: detail.percent });
      case 'storage_unlimited':
        return m('admin.system.health.storage_unlimited');
      case 'livekit_connected':
        return m('admin.system.health.livekit_connected');
      case 'livekit_unreachable':
        return m('admin.system.health.livekit_unreachable');
      case 'livekit_unauthorized':
        return m('admin.system.health.livekit_unauthorized');
      case 'livekit_error':
        return m('admin.system.health.livekit_error');
      case 'livekit_insecure_url':
        return m('admin.system.health.livekit_insecure_url');
      case 'livekit_webhooks_rejected':
        return m('admin.system.health.livekit_webhooks_rejected');
    }
  }
</script>

<Panel title={m('admin.system.health.title')} icon="iconify icon-[uil--heart-rate]">
  <div class="grid items-center gap-5 md:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
    <div class="flex items-center gap-4 md:flex-col md:items-start md:gap-3">
      <span
        aria-hidden="true"
        class={['iconify shrink-0 text-5xl', statusIcon[overall]]}
        data-testid="system-health-overall-icon"
      ></span>
      <div class="min-w-0">
        <div class="text-xl font-semibold" data-testid="system-health-overall">{overallTitle}</div>
        <div class="mt-1 text-sm text-muted">{overallSummary}</div>
      </div>
    </div>

    <ul class="divide-y divide-border surface-box">
      {#each checks as check (check.id)}
        <li
          class="flex flex-wrap items-center gap-x-3 gap-y-0.5 px-3 py-2.5"
          data-health={check.status}
        >
          <span
            role="img"
            aria-label={statusLabel[check.status]}
            class={['iconify shrink-0 text-lg', statusIcon[check.status]]}
          ></span>
          <span class="font-medium">{checkLabel[check.id]}</span>
          <span class={['ms-auto text-sm', detailTone[check.status]]}>{checkDetail(check)}</span>
        </li>
      {/each}
    </ul>
  </div>
</Panel>
