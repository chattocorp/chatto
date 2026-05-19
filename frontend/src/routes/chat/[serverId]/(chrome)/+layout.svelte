<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { untrack } from 'svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import { graphql } from '$lib/gql';
  import { clearLastRoom } from '$lib/storage/lastRoom';
  import { useActiveEvent, useReconnectCallback } from '$lib/hooks';
  import SecondarySidebar from '$lib/components/SecondarySidebar.svelte';
  import { createChromePermissions } from '$lib/state/space';
  import { getServerPermissions } from '$lib/state/server/permissions.svelte';
  import RoomList from '$lib/RoomList.svelte';
  import SpaceHeader from './SpaceHeader.svelte';
  import SpaceBanner from './SpaceBanner.svelte';
  import ServerEventProvider from '$lib/components/chat/ServerEventProvider.svelte';
  import SidebarNav from '$lib/components/SidebarNav.svelte';
  import MyThreadsNavItem from './MyThreadsNavItem.svelte';

  let { children } = $props();

  const connection = useConnection();
  const serverSegment = $derived(serverIdToSegment(getActiveServer()));

  // Detect if we're in space admin mode based on URL (use startsWith to avoid
  // false positives from rooms or other paths that happen to contain "admin")
  const adminPrefix = $derived(
    resolve('/chat/[serverId]/(chrome)/server-admin', { serverId: serverSegment })
  );
  const isAdminMode = $derived(page.url.pathname.startsWith(adminPrefix));

  // Detect if we're in user settings mode
  const settingsPrefix = $derived(
    resolve('/chat/[serverId]/(chrome)/settings', { serverId: serverSegment })
  );
  const isSettingsMode = $derived(page.url.pathname.startsWith(settingsPrefix));

  // User-settings navigation items
  const settingsNavItems = $derived([
    {
      href: resolve('/chat/[serverId]/(chrome)/settings', { serverId: serverSegment }),
      label: 'Profile',
      icon: 'iconify uil--user'
    },
    {
      href: resolve('/chat/[serverId]/(chrome)/settings/preferences', { serverId: serverSegment }),
      label: 'Preferences',
      icon: 'iconify uil--clock'
    },
    {
      href: resolve('/chat/[serverId]/(chrome)/settings/account', { serverId: serverSegment }),
      label: 'Account',
      icon: 'iconify uil--setting'
    },
    {
      href: resolve('/chat/[serverId]/(chrome)/settings/notifications', { serverId: serverSegment }),
      label: 'Notifications',
      icon: 'iconify uil--bell'
    }
  ]);

  // Detect if we're on the server Overview page
  const isHomeActive = $derived(
    page.url.pathname === resolve('/chat/[serverId]/(chrome)/overview', { serverId: serverSegment })
  );

  // Detect if we're on the My Threads page
  const isMyThreadsActive = $derived(
    page.url.pathname === resolve('/chat/[serverId]/(chrome)/threads', { serverId: serverSegment })
  );

  // Detect if we're on the Preferences page
  const isPreferencesActive = $derived(
    page.url.pathname === resolve('/chat/[serverId]/(chrome)/preferences', { serverId: serverSegment })
  );

  // Create space permissions context (must be synchronous during init)
  const updateChromePermissions = createChromePermissions();

  type SpaceData = {
    name: string;
    bannerUrl: string | null;
    hasAnyAdminPermission: boolean;
    canManage: boolean;
    canManageRooms: boolean;
    canManageRoles: boolean;
    canAssignRoles: boolean;
  };

  // Validate access to the active server. Returns server data on success,
  // null if the server says it's not accessible, or 'transient' on network
  // failure (treat as "try again later", not as access denial).
  async function validateSpace(): Promise<SpaceData | null | 'transient'> {
    const result = await connection()
      .client.query(
        graphql(`
          query ValidateSpaceAccess {
            server {
              config {
                serverName
                bannerUrl(width: 480, height: 252)
              }
              viewerHasAnyAdminPermission
              viewerCanManageInstance
              viewerCanManageRooms
              viewerCanManageRoles
              viewerCanAssignRoles
            }
          }
        `),
        {}
      )
      .toPromise();

    // Transient network failure (e.g., wake-from-sleep) — caller should
    // preserve existing data and storage, and rely on the reconnect handler
    // to revalidate.
    if (result.error?.networkError) {
      return 'transient';
    }

    if (!result.data?.server) {
      return null;
    }

    const inst = result.data.server;
    return {
      name: inst.config.serverName,
      bannerUrl: inst.config.bannerUrl ?? null,
      hasAnyAdminPermission: inst.viewerHasAnyAdminPermission,
      canManage: inst.viewerCanManageInstance,
      canManageRooms: inst.viewerCanManageRooms,
      canManageRoles: inst.viewerCanManageRoles,
      canAssignRoles: inst.viewerCanAssignRoles
    };
  }

  // Space validation state - uses $state instead of async $derived to avoid race conditions
  // See egg t4x5m3 for the pattern explanation
  let spaceData = $state<SpaceData | null>(null);
  let validationLoadId = { current: 0 };

  // Force re-validation after genuine WebSocket reconnections (not instance switches).
  // This is separate from the main validation effect to avoid coupling reconnectCount
  // as a dependency — reconnectCount changes during instance switches (different client
  // = different count) which would falsely trigger validation with a stale spaceId.
  let revalidationCounter = $state(0);
  useReconnectCallback(() => {
    revalidationCounter++;
  });

  // Fetch server data on instance change or after WebSocket reconnection.
  $effect(() => {
    const currentInstance = getActiveServer();
    const currentRevalidation = revalidationCounter;

    // Skip if already validated for this instance in this revalidation cycle
    if (
      untrack(() => lastValidatedInstance) === currentInstance &&
      currentRevalidation === untrack(() => lastRevalidation)
    ) {
      return;
    }

    // Only clear data when switching to a different instance.
    if (untrack(() => lastValidatedInstance) !== currentInstance) {
      spaceData = null;
    }

    const thisLoadId = ++validationLoadId.current;

    validateSpace()
      .then((result) => {
        if (validationLoadId.current !== thisLoadId) return;

        // Transient network error — keep prior state visible (or skeleton if
        // none) and let the reconnect handler retry. Don't redirect or wipe
        // storage; the user's place must survive a brief offline blip.
        if (result === 'transient') {
          console.warn(
            '[validateSpace] networkError, ignoring (spaceData stays at prior value)',
            { instance: currentInstance }
          );
          return;
        }

        spaceData = result;
        lastValidatedInstance = currentInstance;
        lastRevalidation = currentRevalidation;

        // Genuine "no access" — clear the last-room hint so we don't loop
        // back here, then redirect away.
        if (result === null) {
          clearLastRoom(getActiveServer());
          goto(resolve('/chat/[serverId]', { serverId: serverSegment }), { replaceState: true });
        }
      })
      .catch((error) => {
        if (validationLoadId.current !== thisLoadId) return;
        console.error('Space validation failed:', error);
        spaceData = null;
      });
  });
  let lastRevalidation = -1;
  let lastValidatedInstance = '';

  // Update space permissions context when spaceData changes
  $effect(() => {
    if (spaceData) {
      updateChromePermissions({
        hasAnyAdminPermission: spaceData.hasAnyAdminPermission,
        canManage: spaceData.canManage,
        canManageRooms: spaceData.canManageRooms,
        canManageRoles: spaceData.canManageRoles,
        canAssignRoles: spaceData.canAssignRoles
      });
    }
  });

  // Server name and banner — derived from spaceData, which is updated both by
  // the initial fetch and by live ServerUpdatedEvent events.
  let spaceName = $derived(spaceData?.name ?? null);
  let bannerUrl = $derived(spaceData?.bannerUrl ?? null);

  // Listen for server updates on the active instance's event bus.
  // Uses useActiveEvent (not useEvent) so that when the user
  // switches to a remote instance, this handler receives events from that
  // instance's bus rather than the home instance's context-based bus.
  useActiveEvent((event) => {
    if (!event.event) return; // Skip unknown event types for forward/backward compatibility
    if (event.event.__typename === 'ServerUpdatedEvent') {
      spaceData = { ...spaceData!, name: event.event.name, bannerUrl: event.event.bannerUrl || null };
    }
  });

  // Read instance permissions for admin-flavoured nav items (system, runtime).
  const serverPerms = getServerPermissions();

  // Whether the user can access ANY admin/settings feature (used to decide
  // whether to show the gear cog in the SpaceHeader).
  const canAccessAnySettings = $derived(
    !!spaceData?.hasAnyAdminPermission || serverPerms.current.canViewAdmin
  );

  // Admin navigation items - filtered based on permissions
  const adminNavItems = $derived.by(() => {
    if (!spaceData) return [];

    const items: { href: string; label: string; icon: string }[] = [];

    // Home is always visible (landing page for admin)
    const adminBase = resolve('/chat/[serverId]/(chrome)/server-admin', { serverId: serverSegment });

    items.push({
      href: adminBase,
      label: 'Dashboard',
      icon: 'iconify uil--dashboard'
    });

    if (spaceData.canManage) {
      items.push({
        href: resolve('/chat/[serverId]/(chrome)/server-admin/general', { serverId: serverSegment }),
        label: 'General',
        icon: 'iconify uil--setting'
      });
    }

    if (spaceData.canAssignRoles || serverPerms.current.canAdminViewUsers) {
      items.push({
        href: resolve('/chat/[serverId]/(chrome)/server-admin/members', { serverId: serverSegment }),
        label: 'Members',
        icon: 'iconify uil--users-alt'
      });
    }

    if (spaceData.canManageRooms) {
      items.push({
        href: resolve('/chat/[serverId]/(chrome)/server-admin/rooms', { serverId: serverSegment }),
        label: 'Rooms',
        icon: 'iconify uil--apps'
      });
    }

    if (spaceData.canManageRoles || serverPerms.current.canAdminViewRoles) {
      items.push({
        href: resolve('/chat/[serverId]/(chrome)/server-admin/roles', { serverId: serverSegment }),
        label: 'Roles',
        icon: 'iconify uil--shield-check'
      });
    }

    if (serverPerms.current.canViewAdmin) {
      items.push({
        href: resolve('/chat/[serverId]/(chrome)/server-admin/security', { serverId: serverSegment }),
        label: 'Security',
        icon: 'iconify uil--shield-exclamation'
      });
    }

    if (serverPerms.current.canAdminViewSystem) {
      items.push({
        href: resolve('/chat/[serverId]/(chrome)/server-admin/system', { serverId: serverSegment }),
        label: 'System',
        icon: 'iconify uil--server'
      });
    }

    return items;
  });

  // Check if an admin nav item is active (custom logic for nested URLs)
  function isAdminNavActive(href: string, _items: unknown): boolean {
    const adminBase = resolve('/chat/[serverId]/(chrome)/server-admin', { serverId: serverSegment });
    if (href === adminBase) {
      return page.url.pathname === adminBase;
    }
    return page.url.pathname.startsWith(href);
  }
</script>

<ServerEventProvider>
      <!-- Sidebar -->
        <SecondarySidebar>
          {#if isSettingsMode}
            <SidebarNav
              title="Settings"
              items={settingsNavItems}
              backHref={resolve('/chat/[serverId]', { serverId: serverSegment })}
              backLabel="Back to Server"
            />
          {:else if !spaceData}
            <!-- Skeleton sidebar while space data is loading -->
            <SpaceHeader spaceName="" loading />

            <div class="flex min-h-0 flex-1 flex-col overflow-x-hidden overflow-y-auto">
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
            </div>
          {:else if isAdminMode}
            <SidebarNav
              title={spaceName ?? 'Space'}
              items={adminNavItems}
              backHref={resolve('/chat/[serverId]', { serverId: serverSegment })}
              backLabel="Back to Server"
              isActive={isAdminNavActive}
            />
          {:else}
            <!-- Space header - fixed at top -->
            <SpaceHeader spaceName={spaceName ?? ''} />

            <!-- Scrollable area for room list sidebar -->
            <div class="flex min-h-0 flex-1 flex-col overflow-x-hidden overflow-y-auto">
              {#if bannerUrl}
                <SpaceBanner url={bannerUrl} />
              {/if}

              <nav class="sidebar-nav p-2">
                <a
                  href={resolve('/chat/[serverId]/(chrome)/overview', { serverId: serverSegment })}
                  class={['sidebar-item', isHomeActive ? 'bg-surface-100' : '']}
                >
                  <span class="sidebar-icon iconify uil--estate"></span>
                  Overview
                </a>
                <MyThreadsNavItem active={isMyThreadsActive} />
                <a
                  href={resolve('/chat/[serverId]/(chrome)/preferences', { serverId: serverSegment })}
                  class={['sidebar-item', isPreferencesActive ? 'bg-surface-100' : '']}
                >
                  <span class="sidebar-icon iconify uil--bell"></span>
                  Preferences
                </a>
                {#if canAccessAnySettings}
                  <a
                    href={resolve('/chat/[serverId]/(chrome)/server-admin', {
                      serverId: serverSegment
                    })}
                    class={['sidebar-item', isAdminMode ? 'bg-surface-100' : '']}
                  >
                    <span class="sidebar-icon iconify uil--setting"></span>
                    Administration
                  </a>
                {/if}
              </nav>

              <hr class="border-border" />

              <!-- Room List - always visible to space members (shows rooms user has joined) -->
              <RoomList />
            </div>
          {/if}
        </SecondarySidebar>

      <!-- Main content - always renders so room can load in parallel -->
      <div class="flex min-h-0 min-w-0 flex-1 flex-col">
        {@render children?.()}
      </div>
    </ServerEventProvider>
