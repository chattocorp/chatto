<!--
@component

Shared frame of the System page sections. For diagnostic viewers it loads the snapshot,
renders the header with the refresh action and the section tabs, and provides
the snapshot to the Overview, Streams, Projections, Workers, and LiveKit pages
through `systemContext`. A tab shows a status dot when a health check that
its section explains reports a warning or a problem. Jobs uses its own admin API.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { resolve } from '$app/paths';
  import { goto } from '$app/navigation';
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
    LoadingDots,
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

  const access = $derived({
    diagnostics: serverScope.store.permissions.canAdminViewSystem,
    jobs:
      serverScope.store.permissions.canManageServer &&
      !serverScope.store.serverInfo.limitedFeatures.includes('backgroundJobs')
  });
  const systemInfoQuery = createQuery(() => {
    const activeConnection = serverScope.connection;
    return {
      enabled: access.diagnostics,
      queryKey: adminQueryKeys.systemInfo(serverScope),
      queryFn: ({ signal }) => getAdminSystemInfo(activeConnection.apiConfig, { signal })
    };
  });

  const systemInfo = $derived(access.diagnostics ? (systemInfoQuery.data ?? null) : null);
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
    jobs: m('chat.management_nav.jobs'),
    livekit: m('admin.system.tabs.livekit')
  });

  const sectionIcon: Record<SystemSection, string> = {
    overview: 'icon-[uil--heart-rate]',
    streams: 'icon-[uil--exchange]',
    projections: 'icon-[uil--layers]',
    workers: 'icon-[uil--cog]',
    jobs: 'icon-[uil--process]',
    livekit: 'icon-[uil--video]'
  };

  const currentSection = $derived(
    page.route.id === '/chat/[serverId]/manage/server/system/jobs/[id]'
      ? 'jobs'
      : ((Object.keys(systemSectionRoutes) as SystemSection[]).find(
          (section) => systemSectionRoutes[section] === page.route.id
        ) ?? null)
  );

  // Permissions belong to the active client session and can change on this route.
  $effect(() => {
    if (currentSection === 'overview' && access.jobs && !access.diagnostics) {
      void goto(resolve(systemSectionRoutes.jobs, { serverId: serverSegment }), {
        replaceState: true
      });
    }
  });

  const sectionTabs = $derived.by<TabNavItem[]>(() => {
    const checks = systemInfo ? systemHealthChecks(systemInfo) : [];
    return visibleSystemSections(access, systemInfo, currentSection).map((section) => {
      const status = systemSectionStatus(checks, section);
      return {
        href: resolve(systemSectionRoutes[section], { serverId: serverSegment }),
        label: sectionLabel[section],
        icon: sectionIcon[section],
        current: section === currentSection,
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
  <PaneHeader
    title={m('admin.system.title')}
    subtitle={currentSection === 'jobs' ? m('admin.jobs.subtitle') : m('admin.system.subtitle')}
  >
    {#snippet actions()}
      {#if access.diagnostics && currentSection !== 'jobs'}
        <HeaderIconButton
          icon="icon-[uil--sync]"
          label={m('admin.system.refresh')}
          disabled={systemInfoQuery.isFetching}
          onclick={() => systemInfoQuery.refetch()}
        />
      {/if}
    {/snippet}
    {#snippet tabs()}
      <TabNav label={m('admin.system.tabs.label')} items={sectionTabs} />
    {/snippet}
  </PaneHeader>

  {#if currentSection === 'jobs'}
    {@render children()}
  {:else if access.diagnostics}
    <PaneContent>
      <div class="flex flex-col gap-6">
        {#if loading}
          <LoadingDots class="h-40 w-full" label={m('admin.system.loading')} />
        {:else if error}
          <Hint tone="danger">{error}</Hint>
        {:else if systemInfo}
          {@render children()}
        {/if}
      </div>
    </PaneContent>
  {/if}
</div>
