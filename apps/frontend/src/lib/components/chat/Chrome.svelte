<script lang="ts">
  import type { Snippet } from 'svelte';
  import { serverUi } from '$lib/state/server/serverUi';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import ServerSidebar from '$lib/components/ServerSidebar.svelte';
  import { LoadingDots, ScrollFader } from '$lib/ui';
  import RoomList from '$lib/RoomList.svelte';
  import ServerHeader from './ServerHeader.svelte';
  import ServerBanner from './ServerBanner.svelte';
  import ServerSubscriptionMarker from './ServerSubscriptionMarker.svelte';
  import SidebarNav from '$lib/components/SidebarNav.svelte';
  import MyThreadsNavItem from './MyThreadsNavItem.svelte';
  import { MessageSearchState } from '$lib/state/server/messageSearch';
  import { getSettingsNavGroups } from '$lib/navigation/settingsNav';
  import { getAdminNavItems } from './adminNav';
  import { m } from '$lib/i18n/messages';
  import {
    DEFAULT_SERVER_SETTINGS_ROUTE,
    SERVER_SETTINGS_ROOT_ROUTE
  } from '$lib/navigation/settingsRoutes';

  let { children }: { children?: Snippet } = $props();

  const serverScope = useServerScope();
  const serverSegment = $derived(serverIdToSegment(serverScope.serverId));
  const activeStore = serverScope.store;

  // All server- and resource-scoped management screens share one shell.
  const serverManagementPrefix = $derived(
    resolve('/chat/[serverId]/manage/server', { serverId: serverSegment })
  );
  const managementPrefix = $derived(serverManagementPrefix.slice(0, -'/server'.length));
  const isManageMode = $derived(
    page.url.pathname === managementPrefix || page.url.pathname.startsWith(`${managementPrefix}/`)
  );

  // Server preferences and permission-gated server administration share one
  // settings shell, regardless of which route family owns the content page.
  const settingsPrefix = $derived(resolve(SERVER_SETTINGS_ROOT_ROUTE, { serverId: serverSegment }));
  const isSettingsMode = $derived(page.url.pathname.startsWith(settingsPrefix));
  const isServerSettingsMode = $derived(isSettingsMode || isManageMode);

  // Detect if we're on the server Overview page
  const isHomeActive = $derived(
    page.url.pathname === resolve('/chat/[serverId]/overview', { serverId: serverSegment })
  );

  const searchHref = $derived(resolve('/chat/[serverId]/search', { serverId: serverSegment }));
  const isSearchActive = $derived(page.url.pathname === searchHref);
  // Wait for remote discovery, but allow best-effort feature requests.
  const serverCanConnect = $derived(activeStore.serverInfo.canConnect);
  const messageSearchAvailable = $derived(
    serverCanConnect &&
      (serverUi(activeStore).messageSearch.statusError ||
        (serverUi(activeStore).messageSearch.statusLoaded &&
          serverUi(activeStore).messageSearch.status.state !== MessageSearchState.DISABLED))
  );

  $effect(() => {
    if (serverCanConnect && activeStore.isAuthenticated)
      void serverUi(activeStore).messageSearch.ensureStatus();
  });

  // Detect if we're on the My Threads page
  const isMyThreadsActive = $derived(
    page.url.pathname === resolve('/chat/[serverId]/threads', { serverId: serverSegment })
  );

  // Server chrome is part of the canonical retained projection. Switching a
  // warm server selects this state synchronously; only a genuinely cold
  // projection renders the quiet loading branch below.
  const serverData = $derived(
    activeStore.realtimeSync.hasDisplayableView
      ? { name: activeStore.serverInfo.name, bannerUrl: activeStore.serverInfo.bannerUrl }
      : null
  );

  // Server updates mutate the retained projection, so these derived values
  // update without a separate validation query.
  let serverName = $derived(serverData?.name ?? null);
  let bannerUrl = $derived(serverData?.bannerUrl ?? null);

  // Admin navigation items - filtered based on permissions
  const adminNavItems = $derived(
    getAdminNavItems({ serverSegment, permissions: activeStore.permissions })
  );
  const managedRoom = $derived(
    page.params.roomId
      ? (serverUi(activeStore).navigation.rooms.find((room) => room.id === page.params.roomId) ??
          null)
      : null
  );
  const managedGroup = $derived(
    page.params.groupId
      ? (serverUi(activeStore).navigation.roomGroups.find(
          (group) => group.id === page.params.groupId
        ) ?? null)
      : null
  );
  const managementNavItems = $derived(
    adminNavItems.length > 0
      ? adminNavItems
      : managedRoom?.viewerCanManageRoom
        ? [
            {
              href: resolve('/chat/[serverId]/manage/rooms/[roomId]', {
                serverId: serverSegment,
                roomId: managedRoom.id
              }),
              label: m('room_list.open_room_settings'),
              icon: 'iconify icon-[uil--setting]'
            }
          ]
        : managedGroup?.viewerCanManageGroup
          ? [
              {
                href: resolve('/chat/[serverId]/manage/room-groups/[groupId]', {
                  serverId: serverSegment,
                  groupId: managedGroup.id
                }),
                label: m('room_list.group_settings', { group: managedGroup.name }),
                icon: 'iconify icon-[uil--setting]'
              }
            ]
          : []
  );
  const settingsNavGroups = $derived(
    getSettingsNavGroups(serverScope.serverId, serverSegment, managementNavItems)
  );
  const overviewNavItems = $derived([
    {
      href: resolve(DEFAULT_SERVER_SETTINGS_ROUTE, { serverId: serverSegment }),
      label: m('settings.overview.title'),
      icon: 'iconify icon-[uil--apps]'
    }
  ]);
</script>

<ServerSubscriptionMarker />
<!-- Sidebar -->
<ServerSidebar>
  {#if isServerSettingsMode}
    <SidebarNav
      title={m('settings.nav.title')}
      subtitle={serverName ?? activeStore.serverInfo.name}
      items={overviewNavItems}
      groups={settingsNavGroups}
      backHref={resolve('/chat/[serverId]', { serverId: serverSegment })}
      backLabel={m('settings.nav.back_to_server')}
    />
  {:else if !serverData}
    <ServerHeader serverName={activeStore.serverInfo.name} />
    <LoadingDots class="min-h-0 flex-1 items-center" />
  {:else}
    <!-- Server header - fixed at top -->
    <ServerHeader serverName={serverName ?? ''} />

    <!-- Scrollable area for room list sidebar -->
    <ScrollFader top bottom>
      {#if bannerUrl}
        <ServerBanner url={bannerUrl} />
      {/if}

      <nav class="sidebar-nav p-2">
        <a
          href={resolve('/chat/[serverId]/overview', { serverId: serverSegment })}
          aria-current={isHomeActive ? 'page' : undefined}
          class="sidebar-item"
        >
          <span aria-hidden="true" class="iconify sidebar-icon icon-[uil--estate]"></span>
          {m('chat.overview.title')}
        </a>
        {#if messageSearchAvailable}
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- searchHref is resolved above -->
          <a
            href={searchHref}
            aria-current={isSearchActive ? 'page' : undefined}
            class="sidebar-item"
          >
            <span class="iconify sidebar-icon icon-[uil--search]" aria-hidden="true"></span>
            {m('search.action')}
          </a>
        {/if}
        <MyThreadsNavItem active={isMyThreadsActive} />
      </nav>

      <hr class="border-border" />

      <!-- Room List - always visible to server members (shows rooms user has joined) -->
      <RoomList canReorderGroups={activeStore.permissions.canManageRooms} />
    </ScrollFader>
  {/if}
</ServerSidebar>

<!-- Main content - always renders so room can load in parallel -->
<div class="flex min-h-0 min-w-0 flex-1 flex-col" data-navigation-content>
  {@render children?.()}
</div>
