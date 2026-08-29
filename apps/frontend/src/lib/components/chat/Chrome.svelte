<script lang="ts">
  import type { Snippet } from 'svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { viewerResponseToState } from '$lib/api-client/viewer';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import ServerSidebar from '$lib/components/ServerSidebar.svelte';
  import { ScrollFader } from '$lib/ui';
  import {
    createChromePermissions,
    type ChromePermissions
  } from '$lib/state/server/chromePermissions.svelte';
  import RoomList from '$lib/RoomList.svelte';
  import ServerHeader from './ServerHeader.svelte';
  import ServerBanner from './ServerBanner.svelte';
  import ServerPresenceSync from './ServerPresenceSync.svelte';
  import SidebarNav from '$lib/components/SidebarNav.svelte';
  import MyThreadsNavItem from './MyThreadsNavItem.svelte';
  import { MessageSearchState } from '$lib/state/server/messageSearch.svelte';
  import { serverStorageKey } from '$lib/storage/serverStorage';
  import { getAdminNavItems } from './adminNav';
  import { m } from '$lib/i18n/messages';
  import {
    DEFAULT_SERVER_SETTINGS_ROUTE,
    SERVER_SETTINGS_ROOT_ROUTE
  } from '$lib/navigation/settingsRoutes';

  let { children }: { children?: Snippet } = $props();

  const serverScope = useServerScope();
  const serverSegment = $derived(serverIdToSegment(serverScope.serverId));
  const activeStore = $derived(serverScope.store);

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
  const settingsPrefix = $derived(
    resolve(SERVER_SETTINGS_ROOT_ROUTE, { serverId: serverSegment })
  );
  const isSettingsMode = $derived(page.url.pathname.startsWith(settingsPrefix));
  const isServerSettingsMode = $derived(isSettingsMode || isManageMode);

  const userPreferenceNavItems = $derived([
    {
      href: resolve('/chat/[serverId]/settings/account', { serverId: serverSegment }),
      label: m('settings.nav.account'),
      icon: 'iconify icon-[uil--setting]'
    },
    {
      href: resolve('/chat/[serverId]/settings/profile', { serverId: serverSegment }),
      label: m('settings.nav.profile'),
      icon: 'iconify icon-[uil--user]'
    },
    {
      href: resolve('/chat/[serverId]/settings/time', { serverId: serverSegment }),
      label: m('settings.preferences.title'),
      icon: 'iconify icon-[uil--clock]'
    },
    {
      href: resolve('/chat/[serverId]/settings/notifications', { serverId: serverSegment }),
      label: m('settings.nav.notifications'),
      icon: 'iconify icon-[uil--bell]'
    }
  ]);
  const appPreferenceNavItems = $derived([
    {
      href: resolve(DEFAULT_SERVER_SETTINGS_ROUTE, { serverId: serverSegment }),
      label: m('settings.app_preferences.appearance.title'),
      icon: 'iconify icon-[uil--palette]'
    },
    {
      href: resolve('/chat/[serverId]/settings/language', { serverId: serverSegment }),
      label: m('settings.preferences.language.title'),
      icon: 'iconify icon-[uil--language]'
    },
    {
      href: resolve('/chat/[serverId]/settings/composer', { serverId: serverSegment }),
      label: m('settings.app_preferences.composer.title'),
      icon: 'iconify icon-[uil--edit]'
    }
  ]);

  // Detect if we're on the server Overview page
  const isHomeActive = $derived(
    page.url.pathname === resolve('/chat/[serverId]/overview', { serverId: serverSegment })
  );

  const searchHref = $derived(resolve('/chat/[serverId]/search', { serverId: serverSegment }));
  const isSearchActive = $derived(page.url.pathname === searchHref);
  const supportsMessageSearch = $derived(activeStore.serverInfo.supportsFeature('messageSearch'));
  const messageSearchAvailable = $derived(
    supportsMessageSearch &&
      !activeStore.messageSearch.statusLoading &&
      (activeStore.messageSearch.statusError ||
        (activeStore.messageSearch.statusLoaded &&
          activeStore.messageSearch.status.state !== MessageSearchState.DISABLED))
  );

  $effect(() => {
    if (supportsMessageSearch) void activeStore.messageSearch.ensureStatus();
  });

  // Detect if we're on the My Threads page
  const isMyThreadsActive = $derived(
    page.url.pathname === resolve('/chat/[serverId]/threads', { serverId: serverSegment })
  );

  type ServerChromeData = ChromePermissions & {
    name: string;
    bannerUrl: string | null;
  };

  // Server chrome is part of the canonical retained projection. Switching a
  // warm server selects this state synchronously; only a genuinely cold
  // projection renders the loading branch below.
  const serverData = $derived.by<ServerChromeData | null>(() => {
    const viewerResponse = activeStore.projection.viewer;
    if (!viewerResponse || !activeStore.permissions.loaded) return null;
    const viewer = viewerResponseToState(viewerResponse);
    const can = (permission: string) => viewer.viewerPermissions[permission] ?? false;
    return {
      name: activeStore.serverInfo.name,
      bannerUrl: activeStore.serverInfo.bannerUrl,
      canViewAdmin: viewer.canViewAdmin,
      canManage: can('server.manage'),
      canManageRooms: can('room.manage'),
      canManageRoles: viewer.canAdminManageRoles,
      canAssignRoles: viewer.canAssignRoles,
      canManageUserAccounts: viewer.canAdminManageAccounts,
      canManageUserPermissions: viewer.canManageUserPermissions
    };
  });

  // Descendants read the canonical derived state directly, without a mirrored
  // permission object or post-render synchronization effect.
  createChromePermissions(() => serverData);

  // Server updates mutate the retained projection, so these derived values
  // update without a separate validation query.
  let serverName = $derived(serverData?.name ?? null);
  let bannerUrl = $derived(serverData?.bannerUrl ?? null);

  // Admin navigation items - filtered based on permissions
  const adminNavItems = $derived(
    getAdminNavItems({
      serverSegment,
      chrome: serverData,
      server: activeStore.permissions
    })
  );
  const managedRoom = $derived(
    page.params.roomId
      ? (activeStore.navigation.rooms.find((room) => room.id === page.params.roomId) ?? null)
      : null
  );
  const managedGroup = $derived(
    page.params.groupId
      ? (activeStore.navigation.roomGroups.find((group) => group.id === page.params.groupId) ??
          null)
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
              label: m('room_list.room_settings'),
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
  const settingsNavGroups = $derived([
    {
      label: m('settings.nav.app_preferences'),
      items: appPreferenceNavItems,
      persistKey: serverStorageKey(serverScope.serverId, 'collapsible:settings:app-preferences')
    },
    {
      label: m('settings.nav.your_account'),
      items: userPreferenceNavItems,
      persistKey: serverStorageKey(serverScope.serverId, 'collapsible:settings:your-account')
    },
    {
      label: m('settings.nav.server_configuration'),
      items: managementNavItems,
      persistKey: serverStorageKey(
        serverScope.serverId,
        'collapsible:settings:server-configuration'
      )
    }
  ]);
</script>

<ServerPresenceSync />
<!-- Sidebar -->
<ServerSidebar>
  {#if isServerSettingsMode}
    <SidebarNav
      title={m('settings.nav.title')}
      subtitle={serverName ?? activeStore.serverInfo.name}
      groups={settingsNavGroups}
      backHref={resolve('/chat/[serverId]', { serverId: serverSegment })}
      backLabel={m('settings.nav.back_to_server')}
    />
  {:else if !serverData}
    <!-- Skeleton sidebar while server data is loading -->
    <ServerHeader serverName="" loading />

    <ScrollFader top bottom>
      <div class="p-2">
        <div class="skeleton h-40 w-full rounded-md"></div>
      </div>

      {#each Array(2) as _, i (i)}
        <div class="flex items-center gap-2 rounded-md px-4 py-2">
          <div class="skeleton h-5 w-5 shrink-0 rounded"></div>
          <div class="skeleton h-5 flex-1 rounded"></div>
        </div>
      {/each}
      <hr class="my-2 border-border" />
      {#each Array(5) as _, i (i)}
        <div class="flex items-center gap-2 rounded-md px-4 py-2">
          <div class="skeleton h-5 w-5 shrink-0 rounded"></div>
          <div class="skeleton h-5 flex-1 rounded"></div>
        </div>
      {/each}
    </ScrollFader>
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
          <span class="iconify sidebar-icon icon-[uil--estate]"></span>
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
        <a
          href={resolve(SERVER_SETTINGS_ROOT_ROUTE, { serverId: serverSegment })}
          class="sidebar-item"
        >
          <span class="iconify sidebar-icon icon-[uil--setting]" aria-hidden="true"></span>
          {m('settings.nav.title')}
        </a>
      </nav>

      <hr class="border-border" />

      <!-- Room List - always visible to server members (shows rooms user has joined) -->
      <RoomList />
    </ScrollFader>
  {/if}
</ServerSidebar>

<!-- Main content - always renders so room can load in parallel -->
<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  {@render children?.()}
</div>
