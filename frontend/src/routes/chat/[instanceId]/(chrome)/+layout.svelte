<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { untrack } from 'svelte';
  import { useConnection } from '$lib/state/instance/connection.svelte';
  import { getActiveInstance, getActiveInstanceSpaceId } from '$lib/state/activeInstance.svelte';
  import { instanceIdToSegment } from '$lib/navigation';
  import { graphql } from '$lib/gql';
  import { clearLastRoom } from '$lib/storage/lastRoom';
  import { useActiveInstanceEvent, useReconnectCallback } from '$lib/hooks';
  import SecondarySidebar from '$lib/components/SecondarySidebar.svelte';
  import { createSpacePermissions } from '$lib/state/space';
  import { getServerPermissions } from '$lib/state/instance/permissions.svelte';
  import RoomList from '$lib/RoomList.svelte';
  import SpaceHeader from './SpaceHeader.svelte';
  import SpaceBanner from './SpaceBanner.svelte';
  import SpaceEventProvider from '$lib/components/chat/SpaceEventProvider.svelte';
  import SidebarNav from '$lib/components/SidebarNav.svelte';
  import MyThreadsNavItem from './MyThreadsNavItem.svelte';

  let { data, children } = $props();

  const connection = useConnection();
  const getInstanceId = getActiveInstance();
  const getSpaceId = getActiveInstanceSpaceId();
  const instanceSegment = $derived(instanceIdToSegment(getInstanceId()));
  const spaceId = $derived(getSpaceId());

  // The chat-root URL (/chat/<instanceSeg>) is the only (chrome) page that
  // renders without a SpaceEventProvider — it shows the welcome / empty
  // state. Used by the no-spaceId branch below to decide whether to render
  // children or a loading shell.
  const isChatRoot = $derived(
    page.url.pathname === resolve('/chat/[instanceId]', { instanceId: instanceSegment })
  );

  // Detect if we're in space admin mode based on URL (use startsWith to avoid
  // false positives from rooms or other paths that happen to contain "admin")
  const adminPrefix = $derived(
    spaceId ? resolve('/chat/[instanceId]/(chrome)/server-admin', { instanceId: instanceSegment }) : ''
  );
  const isAdminMode = $derived(adminPrefix ? page.url.pathname.startsWith(adminPrefix) : false);

  // Detect if we're in room settings mode (separate from space admin mode)
  // Room settings: /chat/[spaceId]/[roomId]/settings
  // Space admin: /chat/[spaceId]/admin
  const isRoomSettingsMode = $derived(
    data.roomId && page.url.pathname.includes(`/${data.roomId}/settings`)
  );

  // Detect if we're on the Browse Rooms page
  const isBrowseRoomsActive = $derived(
    page.url.pathname === resolve('/chat/[instanceId]/(chrome)/rooms', { instanceId: instanceSegment })
  );

  // Detect if we're on the My Threads page
  const isMyThreadsActive = $derived(
    page.url.pathname === resolve('/chat/[instanceId]/(chrome)/threads', { instanceId: instanceSegment })
  );

  // Detect if we're on the Preferences page
  const isPreferencesActive = $derived(
    page.url.pathname === resolve('/chat/[instanceId]/(chrome)/preferences', { instanceId: instanceSegment })
  );

  // Create space permissions context (must be synchronous during init)
  const updateSpacePermissions = createSpacePermissions();

  type SpaceData = {
    id: string;
    name: string;
    bannerUrl: string | null;
    hasAnyAdminPermission: boolean;
    canManage: boolean;
    canBrowseRooms: boolean;
    canManageRooms: boolean;
    canManageRoles: boolean;
    canAssignRoles: boolean;
    canInviteMembers: boolean;
  };

  // Validate access to the active instance. Returns instance data on success,
  // null if the server says it's not accessible, or 'transient' on network
  // failure (treat as "try again later", not as access denial).
  async function validateSpace(spaceId: string): Promise<SpaceData | null | 'transient'> {
    const result = await connection()
      .client.query(
        graphql(`
          query ValidateSpaceAccess {
            server {
              primarySpaceId
              config {
                serverName
                bannerUrl(width: 480, height: 252)
              }
              viewerHasAnyAdminPermission
              viewerCanManageInstance
              viewerCanBrowseRooms
              viewerCanManageRooms
              viewerCanManageRoles
              viewerCanAssignRoles
              viewerCanInviteMembers
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

    if (!result.data?.server || !result.data.server.primarySpaceId) {
      return null;
    }

    const inst = result.data.server;
    return {
      id: inst.primarySpaceId,
      name: inst.config.serverName,
      bannerUrl: inst.config.bannerUrl ?? null,
      hasAnyAdminPermission: inst.viewerHasAnyAdminPermission,
      canManage: inst.viewerCanManageInstance,
      canBrowseRooms: inst.viewerCanBrowseRooms,
      canManageRooms: inst.viewerCanManageRooms,
      canManageRoles: inst.viewerCanManageRoles,
      canAssignRoles: inst.viewerCanAssignRoles,
      canInviteMembers: inst.viewerCanInviteMembers
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

  // Validate space when spaceId changes or after WebSocket reconnection.
  // Dependencies: spaceId and revalidationCounter only.
  // spaceData is read via untrack() to avoid re-triggering when the guard effect clears it.
  $effect(() => {
    const currentSpaceId = spaceId;
    const currentRevalidation = revalidationCounter;

    if (!currentSpaceId) {
      spaceData = null;
      return;
    }

    // Skip if already validated for this spaceId in this revalidation cycle
    if (
      untrack(() => spaceData?.id) === currentSpaceId &&
      currentRevalidation === untrack(() => lastRevalidation)
    ) {
      return;
    }

    // Only show skeleton when switching to a different space.
    // On revalidation (same space), keep existing data visible while refetching.
    if (untrack(() => spaceData?.id) !== currentSpaceId) {
      spaceData = null;
    }

    const thisLoadId = ++validationLoadId.current;

    validateSpace(currentSpaceId)
      .then((result) => {
        // Skip if spaceId changed while validating
        if (validationLoadId.current !== thisLoadId) return;

        // Transient network error — keep prior state visible (or skeleton if
        // none) and let the reconnect handler retry. Don't redirect or wipe
        // storage; the user's place must survive a brief offline blip.
        // Logged so a stuck-skeleton-sidebar incident leaves a fingerprint.
        if (result === 'transient') {
          console.warn(
            '[validateSpace] networkError, ignoring (spaceData stays at prior value)',
            { spaceId: currentSpaceId }
          );
          return;
        }

        spaceData = result;
        lastRevalidation = currentRevalidation;

        // Genuine "no access" — clear the last-room hint so we don't loop
        // back here, then redirect away.
        if (result === null) {
          clearLastRoom(getInstanceId());
          goto(resolve('/chat/[instanceId]', { instanceId: instanceSegment }), { replaceState: true });
        }
      })
      .catch((error) => {
        if (validationLoadId.current !== thisLoadId) return;
        console.error('Space validation failed:', error);
        spaceData = null;
      });
  });
  let lastRevalidation = -1;

  // Update space permissions context when spaceData changes
  $effect(() => {
    if (spaceData) {
      updateSpacePermissions({
        hasAnyAdminPermission: spaceData.hasAnyAdminPermission,
        canManage: spaceData.canManage,
        canBrowseRooms: spaceData.canBrowseRooms,
        canManageRooms: spaceData.canManageRooms,
        canManageRoles: spaceData.canManageRoles,
        canAssignRoles: spaceData.canAssignRoles,
        canInviteMembers: spaceData.canInviteMembers
      });
    }
  });

  // Server name and banner — derived from spaceData, which is updated both by
  // the initial fetch and by live ServerUpdatedEvent events.
  let spaceName = $derived(spaceData?.name ?? null);
  let bannerUrl = $derived(spaceData?.bannerUrl ?? null);

  // Listen for server updates on the active instance's event bus.
  // Uses useActiveInstanceEvent (not useInstanceEvent) so that when the user
  // switches to a remote instance, this handler receives events from that
  // instance's bus rather than the home instance's context-based bus.
  useActiveInstanceEvent((event) => {
    if (!event.event) return; // Skip unknown event types for forward/backward compatibility
    if (event.event.__typename === 'ServerUpdatedEvent') {
      spaceData = { ...spaceData!, name: event.event.name, bannerUrl: event.event.bannerUrl || null };
    }
  });

  // Read instance permissions for admin-flavoured nav items (system, runtime).
  const instancePerms = getServerPermissions();

  // Whether the user can access ANY admin/settings feature (used to decide
  // whether to show the gear cog in the SpaceHeader).
  const canAccessAnySettings = $derived(
    !!spaceData?.hasAnyAdminPermission || instancePerms.current.canViewAdmin
  );

  // Admin navigation items - filtered based on permissions
  const adminNavItems = $derived.by(() => {
    if (!spaceId || !spaceData) return [];

    const items: { href: string; label: string; icon: string }[] = [];

    // Home is always visible (landing page for admin)
    const adminBase = resolve('/chat/[instanceId]/(chrome)/server-admin', { instanceId: instanceSegment });

    items.push({
      href: adminBase,
      label: 'Dashboard',
      icon: 'iconify uil--dashboard'
    });

    if (spaceData.canManage) {
      items.push({
        href: resolve('/chat/[instanceId]/(chrome)/server-admin/general', { instanceId: instanceSegment }),
        label: 'General',
        icon: 'iconify uil--setting'
      });
    }

    if (spaceData.canAssignRoles || instancePerms.current.canAdminViewUsers) {
      items.push({
        href: resolve('/chat/[instanceId]/(chrome)/server-admin/members', { instanceId: instanceSegment }),
        label: 'Members',
        icon: 'iconify uil--users-alt'
      });
    }

    if (spaceData.canManageRooms) {
      items.push({
        href: resolve('/chat/[instanceId]/(chrome)/server-admin/rooms', { instanceId: instanceSegment }),
        label: 'Rooms',
        icon: 'iconify uil--apps'
      });
    }

    if (spaceData.canManageRoles || instancePerms.current.canAdminViewRoles) {
      items.push({
        href: resolve('/chat/[instanceId]/(chrome)/server-admin/roles', { instanceId: instanceSegment }),
        label: 'Roles',
        icon: 'iconify uil--shield-check'
      });
      items.push({
        href: resolve('/chat/[instanceId]/(chrome)/server-admin/inspector', { instanceId: instanceSegment }),
        label: 'Inspector',
        icon: 'iconify uil--search'
      });
    }

    if (instancePerms.current.canViewAdmin) {
      items.push({
        href: resolve('/chat/[instanceId]/(chrome)/server-admin/security', { instanceId: instanceSegment }),
        label: 'Security',
        icon: 'iconify uil--shield-exclamation'
      });
    }

    if (instancePerms.current.canAdminViewSystem) {
      items.push({
        href: resolve('/chat/[instanceId]/(chrome)/server-admin/system', { instanceId: instanceSegment }),
        label: 'System',
        icon: 'iconify uil--server'
      });
    }

    return items;
  });

  // Check if an admin nav item is active (custom logic for space-specific URLs)
  function isAdminNavActive(href: string, _items: unknown): boolean {
    if (!spaceId) return false;
    const adminBase = resolve('/chat/[instanceId]/(chrome)/server-admin', { instanceId: instanceSegment });
    if (href === adminBase) {
      return page.url.pathname === adminBase;
    }
    return page.url.pathname.startsWith(href);
  }
</script>

{#key spaceId}
  {#if !spaceId}
    <!-- No primary space (fresh install / no joined spaces yet). Skip the
         space chrome (banner, sidebar, RoomList — they all need a space).
         Only render children if we're at the chat-root URL — that's the
         welcome / empty-state page that's designed to handle no spaceId.
         Other (chrome) pages (rooms, threads, [roomId], server-admin)
         depend on SpaceEventProvider and would crash on missing context;
         show a brief loading shell instead and let the {#key spaceId}
         remount the proper tree once primarySpaceId arrives. -->
    {#if isChatRoot}
      <div class="flex min-h-0 min-w-0 flex-1 flex-col">
        {@render children?.()}
      </div>
    {/if}
  {:else}
    <SpaceEventProvider>
      <!-- Sidebar -->
      {#if !isRoomSettingsMode}
        <SecondarySidebar>
          {#if !spaceData}
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
              backHref={resolve('/chat/[instanceId]', { instanceId: instanceSegment })}
              backLabel="Back to Server"
              isActive={isAdminNavActive}
            />
          {:else}
            <!-- Space header - fixed at top -->
            <SpaceHeader
              spaceName={spaceName ?? ''}
              canAccessSettings={canAccessAnySettings}
            />

            <!-- Scrollable area for room list sidebar -->
            <div class="flex min-h-0 flex-1 flex-col overflow-x-hidden overflow-y-auto">
              {#if bannerUrl}
                <SpaceBanner url={bannerUrl} />
              {/if}

              <nav class="sidebar-nav p-2">
                {#if spaceData.canBrowseRooms}
                  <a
                    href={resolve('/chat/[instanceId]/(chrome)/rooms', { instanceId: instanceSegment })}
                    class={['sidebar-item', isBrowseRoomsActive ? 'bg-surface-100' : 'text-muted']}
                  >
                    <span class="sidebar-icon iconify uil--search-alt"></span>
                    Browse Rooms
                  </a>
                {/if}
                <MyThreadsNavItem active={isMyThreadsActive} />
                <a
                  href={resolve('/chat/[instanceId]/(chrome)/preferences', { instanceId: instanceSegment })}
                  class={['sidebar-item', isPreferencesActive ? 'bg-surface-100' : 'text-muted']}
                >
                  <span class="sidebar-icon iconify uil--bell"></span>
                  Preferences
                </a>
              </nav>

              <hr class="border-border" />

              <!-- Room List - always visible to space members (shows rooms user has joined) -->
              <RoomList />
            </div>
          {/if}
        </SecondarySidebar>
      {/if}

      <!-- Main content - always renders so room can load in parallel -->
      <div class="flex min-h-0 min-w-0 flex-1 flex-col">
        {@render children?.()}
      </div>
    </SpaceEventProvider>
  {/if}
{/key}
