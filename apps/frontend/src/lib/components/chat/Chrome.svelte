<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { viewerResponseToState } from '$lib/api-client/viewer';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import ServerSidebar from '$lib/components/ServerSidebar.svelte';
  import ScrollFader from '$lib/ui/ScrollFader.svelte';
  import { createChromePermissions } from '$lib/state/server/chromePermissions.svelte';
  import { getServerPermissions } from '$lib/state/server/permissions.svelte';
  import RoomList from '$lib/RoomList.svelte';
  import ServerHeader from './ServerHeader.svelte';
  import ServerBanner from './ServerBanner.svelte';
  import ServerEventProvider from './ServerEventProvider.svelte';
  import SidebarNav from '$lib/components/SidebarNav.svelte';
  import MyThreadsNavItem from './MyThreadsNavItem.svelte';
  import { getAdminNavItems } from './adminNav';
  import * as m from '$lib/i18n/messages';

  let { children } = $props();

  const serverSegment = $derived(serverIdToSegment(getActiveServer()));
  const activeStore = $derived(serverRegistry.getStore(getActiveServer()));

  // Detect if we're in server admin mode based on URL (use startsWith to avoid
  // false positives from rooms or other paths that happen to contain "admin")
  const adminPrefix = $derived(
    resolve('/chat/[serverId]/server-admin', { serverId: serverSegment })
  );
  const isAdminMode = $derived(page.url.pathname.startsWith(adminPrefix));

  // Detect if we're in user settings mode
  const settingsPrefix = $derived(
    resolve('/chat/[serverId]/settings', { serverId: serverSegment })
  );
  const isSettingsMode = $derived(page.url.pathname.startsWith(settingsPrefix));

  // User-settings navigation items
  const settingsNavItems = $derived([
    {
      href: resolve('/chat/[serverId]/settings', { serverId: serverSegment }),
      label: m['settings.nav.profile'](),
      icon: 'iconify uil--user'
    },
    {
      href: resolve('/chat/[serverId]/settings/preferences', { serverId: serverSegment }),
      label: m['settings.nav.display'](),
      icon: 'iconify uil--clock'
    },
    {
      href: resolve('/chat/[serverId]/settings/notifications', { serverId: serverSegment }),
      label: m['settings.nav.notifications'](),
      icon: 'iconify uil--bell'
    },
    {
      href: resolve('/chat/[serverId]/settings/account', { serverId: serverSegment }),
      label: m['settings.nav.account'](),
      icon: 'iconify uil--setting'
    }
  ]);

  // Detect if we're on the server Overview page
  const isHomeActive = $derived(
    page.url.pathname === resolve('/chat/[serverId]/overview', { serverId: serverSegment })
  );

  // Detect if we're on the My Threads page
  const isMyThreadsActive = $derived(
    page.url.pathname === resolve('/chat/[serverId]/threads', { serverId: serverSegment })
  );

  // Create server chrome permissions context (must be synchronous during init)
  const updateChromePermissions = createChromePermissions();

  type ServerChromeData = {
    name: string;
    bannerUrl: string | null;
    canViewAdmin: boolean;
    canManage: boolean;
    canManageRooms: boolean;
    canManageRoles: boolean;
    canAssignRoles: boolean;
    canManageUserAccounts: boolean;
    canManageUserPermissions: boolean;
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

  // Update server chrome permissions context when serverData changes
  $effect(() => {
    if (serverData) {
      updateChromePermissions({
        canViewAdmin: serverData.canViewAdmin,
        canManage: serverData.canManage,
        canManageRooms: serverData.canManageRooms,
        canManageRoles: serverData.canManageRoles,
        canAssignRoles: serverData.canAssignRoles,
        canManageUserAccounts: serverData.canManageUserAccounts,
        canManageUserPermissions: serverData.canManageUserPermissions
      });
    }
  });

  // Server updates mutate the retained projection, so these derived values
  // update without a separate validation query.
  let serverName = $derived(serverData?.name ?? null);
  let bannerUrl = $derived(serverData?.bannerUrl ?? null);

  // Read server-wide permissions for admin-flavoured nav items (system, audit).
  const serverPerms = getServerPermissions();

  // Admin navigation items - filtered based on permissions
  const adminNavItems = $derived(
    getAdminNavItems({
      serverSegment,
      chrome: serverData,
      server: serverPerms.current
    })
  );
  const adminHref = $derived(adminNavItems[0]?.href);

  function isAdminNavActive(href: string, _items: unknown): boolean {
    return page.url.pathname.startsWith(href);
  }

</script>

<ServerEventProvider>
  <!-- Sidebar -->
  <ServerSidebar>
    {#if isSettingsMode}
      <SidebarNav
        title={m['settings.nav.title']()}
        items={settingsNavItems}
        backHref={resolve('/chat/[serverId]', { serverId: serverSegment })}
        backLabel={m['settings.nav.back_to_server']()}
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
    {:else if isAdminMode}
      <SidebarNav
        title={serverName ?? m['chat.server_nav.server_fallback']()}
        items={adminNavItems}
        backHref={resolve('/chat/[serverId]', { serverId: serverSegment })}
        backLabel={m['chat.server_nav.back_to_server']()}
        isActive={isAdminNavActive}
      />
    {:else}
      <!-- Server header - fixed at top -->
      <ServerHeader serverName={serverName ?? ''} {adminHref} />

      <!-- Scrollable area for room list sidebar -->
      <ScrollFader top bottom>
        {#if bannerUrl}
          <ServerBanner url={bannerUrl} />
        {/if}

        <nav class="sidebar-nav p-2">
          <a
            href={resolve('/chat/[serverId]/overview', { serverId: serverSegment })}
            class={['sidebar-item', isHomeActive ? 'bg-surface' : '']}
          >
            <span class="sidebar-icon iconify uil--estate"></span>
            {m['chat.overview.title']()}
          </a>
          <MyThreadsNavItem active={isMyThreadsActive} />
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
</ServerEventProvider>
