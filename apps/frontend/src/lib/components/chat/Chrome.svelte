<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { Code, ConnectError } from '@connectrpc/connect';
  import { untrack } from 'svelte';
  import { getAuthenticatedServerState } from '$lib/api-client/serverState';
  import { getViewerStateViaConnect } from '$lib/api-client/viewer';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import { clearLastRoom } from '$lib/storage/lastRoom';
  import { useActiveEvent, useReconnectCallback } from '$lib/hooks';
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
  import { RoomEventKind, roomEventKind } from '$lib/render/eventKinds';

  let { children } = $props();

  const connection = useConnection();
  const serverSegment = $derived(serverIdToSegment(getActiveServer()));

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

  // Validate access to the active server. Returns server data on success,
  // null if the server says it's not accessible, or 'transient' on network
  // failure (treat as "try again later", not as access denial).
  async function validateServer(): Promise<ServerChromeData | null | 'transient'> {
    const currentConnection = connection();
    const config = {
      baseUrl: currentConnection.connectBaseUrl,
      bearerToken: currentConnection.bearerToken
    };

    try {
      const [server, viewer] = await Promise.all([
        getAuthenticatedServerState(config),
        getViewerStateViaConnect(config)
      ]);

      return {
        name: server.name,
        bannerUrl: server.bannerUrl,
        canViewAdmin: viewer.canViewAdmin,
        canManage: server.viewerCanManageServer,
        canManageRooms: server.viewerCanManageRooms,
        canManageRoles: viewer.canAdminManageRoles,
        canAssignRoles: viewer.canAssignRoles,
        canManageUserAccounts: viewer.canAdminManageAccounts,
        canManageUserPermissions: viewer.canManageUserPermissions
      };
    } catch (error) {
      if (isTransientValidationError(error)) {
        return 'transient';
      }
      if (isAccessDeniedValidationError(error)) {
        return null;
      }
      throw error;
    }
  }

  // Server validation state - uses $state instead of async $derived to avoid race conditions
  // See egg t4x5m3 for the pattern explanation
  let serverData = $state<ServerChromeData | null>(null);
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
      serverData = null;
    }

    const thisLoadId = ++validationLoadId.current;

    validateServer()
      .then((result) => {
        if (validationLoadId.current !== thisLoadId) return;

        // Transient network error — keep prior state visible (or skeleton if
        // none) and let the reconnect handler retry. Don't redirect or wipe
        // storage; the user's place must survive a brief offline blip.
        if (result === 'transient') {
          console.warn(
            '[validateServer] networkError, ignoring (serverData stays at prior value)',
            { instance: currentInstance }
          );
          return;
        }

        serverData = result;
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
        console.error('Server validation failed:', error);
        serverData = null;
      });
  });
  let lastRevalidation = -1;
  let lastValidatedInstance = '';

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

  // Server name and banner — derived from serverData, which is updated both by
  // the initial fetch and by live ServerUpdatedEvent events.
  let serverName = $derived(serverData?.name ?? null);
  let bannerUrl = $derived(serverData?.bannerUrl ?? null);

  // Listen for server updates on the active instance's event bus.
  // Uses useActiveEvent (not useEvent) so that when the user
  // switches to a remote instance, this handler receives events from that
  // instance's bus rather than the home instance's context-based bus.
  useActiveEvent((event) => {
    if (!event.event) return; // Skip unknown event types for forward/backward compatibility
    if (roomEventKind(event.event) === RoomEventKind.ServerUpdated) {
      revalidationCounter++;
    }
  });

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

  function isTransientValidationError(error: unknown): boolean {
    if (error instanceof ConnectError) {
      return error.code === Code.Unavailable || error.code === Code.DeadlineExceeded;
    }
    return error instanceof TypeError;
  }

  function isAccessDeniedValidationError(error: unknown): boolean {
    return (
      error instanceof ConnectError &&
      (error.code === Code.Unauthenticated ||
        error.code === Code.PermissionDenied ||
        error.code === Code.NotFound)
    );
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
            class={['sidebar-item', isHomeActive ? 'bg-surface-100' : '']}
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
