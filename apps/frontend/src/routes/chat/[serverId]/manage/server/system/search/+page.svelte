<!--
@component

Search section of the System page: the status of each message search
provider process. Availability and index freshness show separately, so a
provider can be ready while its index is stalled. Each provider with a
problem gets a short hint about what to check.
-->
<script lang="ts">
  import type { AdminSearchProvider } from '$lib/api/adminDiagnostics';
  import { formatNumber, formatSeconds } from '$lib/components/admin';
  import { DataTable, Hint, Panel, Pill } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { useSystem } from '../systemContext';
  import { searchCheck, searchIndexCheck, type HealthStatus } from '../systemHealth';
  import { healthDetailLabel } from '../systemHealthLabels';

  type Tone = 'success' | 'warning' | 'danger' | 'muted';

  const system = useSystem();
  const search = $derived(system.info.search);
  const serverScope = useServerScope();
  const userSettings = $derived(
    timeFormatSettingsFor(serverScope.store.currentUser.user?.settings)
  );

  const stateTone: Record<AdminSearchProvider['state'], Tone> = {
    unknown: 'muted',
    starting: 'muted',
    indexing: 'warning',
    ready: 'success',
    degraded: 'warning',
    unavailable: 'danger'
  };

  const freshnessTone: Record<AdminSearchProvider['freshness'], Tone> = {
    unknown: 'muted',
    current: 'success',
    behind: 'muted',
    stalled: 'warning'
  };

  const probeTone: Record<AdminSearchProvider['queryProbe']['result'], Tone> = {
    none: 'muted',
    succeeded: 'success',
    failed: 'danger',
    timed_out: 'danger'
  };

  /** The providers that most need attention come first. */
  const severity: Record<AdminSearchProvider['state'], number> = {
    unavailable: 0,
    degraded: 1,
    unknown: 2,
    indexing: 3,
    starting: 4,
    ready: 5
  };

  const providers = $derived(
    [...(search?.providers ?? [])].sort(
      (a, b) =>
        severity[a.state] - severity[b.state] ||
        a.host.localeCompare(b.host) ||
        a.instanceId.localeCompare(b.instanceId)
    )
  );

  const checks = $derived(search ? [searchCheck(search), searchIndexCheck(search)] : []);
  const checkTone: Record<HealthStatus, Tone> = {
    ok: 'success',
    warning: 'warning',
    critical: 'danger',
    unknown: 'muted'
  };

  function formatTimestamp(value: Date | null): string {
    return value ? formatDateTime(value, userSettings, getLocale()) : '-';
  }

  function formatCount(value: string): string {
    try {
      return BigInt(value).toLocaleString();
    } catch {
      return value;
    }
  }

  /** Returns what an operator should check first, or null when nothing is wrong. */
  function providerHint(provider: AdminSearchProvider): { tone: Tone; text: string } | null {
    if (provider.state === 'unknown') {
      return { tone: 'warning', text: m('admin.system.search.hint_unknown') };
    }
    if (provider.state === 'unavailable') {
      return { tone: 'danger', text: m('admin.system.search.hint_unavailable') };
    }
    if (provider.queryProbe.result === 'failed' || provider.queryProbe.result === 'timed_out') {
      return { tone: 'danger', text: m('admin.system.search.hint_probe') };
    }
    if (provider.freshness === 'stalled') {
      return { tone: 'warning', text: m('admin.system.search.hint_stalled') };
    }
    if (provider.state === 'degraded') {
      return { tone: 'warning', text: m('admin.system.search.hint_degraded') };
    }
    return null;
  }
</script>

{#if !search}
  <Hint>{m('admin.system.search.unavailable')}</Hint>
{:else}
  <Panel title={m('admin.system.search.title')} icon="iconify icon-[uil--search]" noPadding>
    {#if providers.length === 0}
      <div class="p-4">
        <Hint tone="danger">{m('admin.system.search.none')}</Hint>
      </div>
    {:else}
      <div class="flex flex-wrap gap-2 px-4 pt-4" data-testid="search-summary">
        {#each checks as check (check.id)}
          <Pill tone={checkTone[check.status]}>{healthDetailLabel(check.detail)}</Pill>
        {/each}
      </div>
      <DataTable items={providers} columns={4} getKey={(provider) => provider.instanceId}>
        {#snippet header()}
          <th class="table-header-cell">{m('admin.system.search.provider')}</th>
          <th class="table-header-cell">{m('admin.system.state')}</th>
          <th class="table-header-cell">{m('admin.system.search.index')}</th>
          <th class="table-header-cell">{m('admin.system.search.query_probe')}</th>
        {/snippet}
        {#snippet row(provider)}
          {@const hint = providerHint(provider)}
          <td class="px-4 py-3 align-top" data-search-provider={provider.state}>
            <div class="font-medium">
              {provider.host || m('admin.system.search.unnamed_provider')}
            </div>
            <div class="font-mono text-xs text-muted">
              {provider.instanceId || '-'}
              {#if provider.implementationVersion}
                · {provider.implementationVersion}
              {/if}
            </div>
            {#if hint}
              <div
                class={[
                  'mt-1 max-w-[28rem] text-sm',
                  hint.tone === 'danger' ? 'text-danger' : 'text-warning'
                ]}
              >
                {hint.text}
              </div>
            {/if}
          </td>
          <td class="px-4 py-3 align-top">
            <Pill tone={stateTone[provider.state]}>
              {m(`admin.system.search.state_${provider.state}`)}
            </Pill>
            {#if provider.indexedEventCount != null && provider.targetEventCount != null && provider.state !== 'ready'}
              <div class="mt-1 text-sm text-muted">
                {m('admin.system.search.progress', {
                  indexed: formatCount(provider.indexedEventCount),
                  target: formatCount(provider.targetEventCount)
                })}
              </div>
            {/if}
            {#if provider.currentOperation}
              <div class="mt-1 font-mono text-xs whitespace-nowrap text-muted">
                {provider.currentOperation}
                {#if provider.currentOperationSeconds != null}
                  · {formatSeconds(provider.currentOperationSeconds)}
                {/if}
              </div>
            {/if}
          </td>
          <td class="px-4 py-3 align-top">
            <Pill tone={freshnessTone[provider.freshness]}>
              {m(`admin.system.search.freshness_${provider.freshness}`)}
            </Pill>
            {#if provider.committedSequence != null}
              <div class="mt-1 font-mono text-sm whitespace-nowrap">
                {provider.committedSequence}
                {#if provider.observedSequence != null}
                  <span class="text-muted">/ {provider.observedSequence}</span>
                {/if}
              </div>
            {/if}
            {#if provider.pendingSeconds != null}
              <div
                class={[
                  'text-sm',
                  provider.freshness === 'stalled' ? 'text-warning' : 'text-muted'
                ]}
              >
                {m('admin.system.search.pending_for', {
                  duration: formatSeconds(provider.pendingSeconds)
                })}
              </div>
            {/if}
            <div class="text-sm text-muted">
              {m('admin.system.search.last_progress', {
                time: formatTimestamp(provider.lastCommitAt)
              })}
            </div>
          </td>
          <td class="px-4 py-3 align-top">
            <Pill tone={probeTone[provider.queryProbe.result]}>
              {m(`admin.system.search.probe_${provider.queryProbe.result}`)}
            </Pill>
            {#if provider.queryProbe.consecutiveFailures > 0}
              <div class="mt-1 text-sm text-danger">
                {m('admin.system.search.probe_failures', {
                  count: formatNumber(provider.queryProbe.consecutiveFailures)
                })}
              </div>
            {/if}
            {#if provider.queryProbe.inFlightSince}
              <div class="mt-1 text-sm text-muted">
                {m('admin.system.search.probe_running_since', {
                  time: formatTimestamp(provider.queryProbe.inFlightSince)
                })}
              </div>
            {/if}
            {#if provider.queryProbe.completedAt}
              <div class="text-sm text-muted">
                {m('admin.system.search.probe_last_run', {
                  time: formatTimestamp(provider.queryProbe.completedAt)
                })}
              </div>
            {/if}
          </td>
        {/snippet}
      </DataTable>
    {/if}
  </Panel>
  <p class="text-sm text-muted">{m('admin.system.search.scope_help')}</p>
{/if}
