<!--
@component

Shared frame of the System page sections. It loads the diagnostics snapshot,
renders the header with the refresh action and the section tabs, and provides
the snapshot to the Overview, Streams, Projections, Workers, and LiveKit pages
through `systemContext`. A tab shows a status dot when a health check that
its section explains reports a warning or a problem.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { getAdminSystemInfo } from '$lib/api/adminDiagnostics';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createQuery } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import {
    HeaderIconButton,
    Hint,
    LoadingFog,
    PageTitle,
    PaneContent,
    PaneHeader,
    TabNav,
    type TabNavItem
  } from '$lib/ui';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { provideSystem } from './systemContext';
  import { systemHealthChecks } from './systemHealth';
  import {
    systemSectionRoutes,
    systemSectionStatus,
    visibleSystemSections,
    type SystemSection
  } from './systemSections';

  let { children }: { children: Snippet } = $props();

  const serverScope = useServerScope();
  const serverSegment = serverIdToSegment(serverScope.serverId);

  const systemInfoQuery = createQuery(() => {
    const serverId = serverScope.serverId;
    const activeConnection = serverScope.connection;
    return {
      queryKey: adminQueryKeys.systemInfo(serverId, activeConnection),
      queryFn: ({ signal }) => getAdminSystemInfo(activeConnection.apiConfig, { signal })
    };
  });

  const systemInfo = $derived(systemInfoQuery.data ?? null);
  const loading = $derived(systemInfoQuery.isPending);
  const error = $derived(systemInfoQuery.error ? errorMessage(systemInfoQuery.error) : null);

  provideSystem({
    get info() {
      return systemInfo!;
    }
  });

  const sectionLabel = $derived<Record<SystemSection, string>>({
    overview: m('admin.system.tabs.overview'),
    streams: m('admin.system.tabs.streams'),
    projections: m('admin.system.tabs.projections'),
    workers: m('admin.system.tabs.workers'),
    livekit: m('admin.system.tabs.livekit')
  });

  const sectionIcon: Record<SystemSection, string> = {
    overview: 'icon-[uil--heart-rate]',
    streams: 'icon-[uil--exchange]',
    projections: 'icon-[uil--layers]',
    workers: 'icon-[uil--cog]',
    livekit: 'icon-[uil--video]'
  };

  const sectionTabs = $derived.by<TabNavItem[]>(() => {
    const checks = systemInfo ? systemHealthChecks(systemInfo) : [];
    return visibleSystemSections(systemInfo).map((section) => {
      const status = systemSectionStatus(checks, section);
      return {
        href: resolve(systemSectionRoutes[section], { serverId: serverSegment }),
        label: sectionLabel[section],
        icon: sectionIcon[section],
        current: page.route.id === systemSectionRoutes[section],
        status: status
          ? status === 'critical'
            ? { tone: 'danger', label: m('admin.system.health.status_critical') }
            : { tone: 'warning', label: m('admin.system.health.status_warning') }
          : undefined
      };
    });
  });
</script>

<PageTitle title={m('admin.common.page_title', { title: m('admin.system.title') })} />

<div class="pane-page">
  <PaneHeader title={m('admin.system.title')} subtitle={m('admin.system.subtitle')}>
    {#snippet actions()}
      <HeaderIconButton
        icon="icon-[uil--sync]"
        label={m('admin.system.refresh')}
        disabled={systemInfoQuery.isFetching}
        onclick={() => systemInfoQuery.refetch()}
      />
    {/snippet}
    {#snippet tabs()}
      <TabNav label={m('admin.system.tabs.label')} items={sectionTabs} />
    {/snippet}
  </PaneHeader>

  <PaneContent>
    <div class="flex flex-col gap-6">
      {#if loading}
        <LoadingFog class="h-40 w-full" label={m('admin.system.loading')} />
      {:else if error}
        <Hint tone="danger">{error}</Hint>
      {:else if systemInfo}
        {@render children()}
      {/if}
    </div>
  </PaneContent>
</div>
