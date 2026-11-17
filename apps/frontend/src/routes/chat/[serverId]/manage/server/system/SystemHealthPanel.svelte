<!--
@component

Summarizes System page diagnostics as one overall status and a short list of
health checks. See `systemHealth.ts` for the rules behind each status. A
check whose details are in a different System section links to that section.
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import type { AdminSystemInfo } from '$lib/api/adminDiagnostics';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Panel } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import {
    overallHealth,
    systemHealthChecks,
    type HealthCheckId,
    type HealthStatus
  } from './systemHealth';
  import { healthDetailLabel } from './systemHealthLabels';
  import { healthCheckSection, systemSectionRoutes } from './systemSections';

  let { info }: { info: AdminSystemInfo } = $props();

  const serverSegment = serverIdToSegment(useServerScope().serverId);

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

  const overallTitle = $derived<Record<HealthStatus, string>>({
    ok: m('admin.system.health.overall_ok'),
    warning: m('admin.system.health.overall_warning'),
    critical: m('admin.system.health.overall_critical'),
    unknown: m('admin.system.health.overall_unknown')
  });

  const overallSummary = $derived(
    overall === 'unknown'
      ? m('admin.system.health.overall_unknown_summary')
      : overall !== 'ok'
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
    voice_calls: m('admin.system.health.voice_calls'),
    search: m('admin.system.health.search'),
    search_index: m('admin.system.health.search_index')
  });
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
        <div class="text-xl font-semibold" data-testid="system-health-overall">
          {overallTitle[overall]}
        </div>
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
          {#if healthCheckSection[check.id] === 'overview'}
            <span class="font-medium">{checkLabel[check.id]}</span>
          {:else}
            <a
              class="font-medium link"
              href={resolve(systemSectionRoutes[healthCheckSection[check.id]], {
                serverId: serverSegment
              })}
            >
              {checkLabel[check.id]}
            </a>
          {/if}
          <span class={['ms-auto text-sm', detailTone[check.status]]}
            >{healthDetailLabel(check.detail)}</span
          >
        </li>
      {/each}
    </ul>
  </div>
</Panel>
