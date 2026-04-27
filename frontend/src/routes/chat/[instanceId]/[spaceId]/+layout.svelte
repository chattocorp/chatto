<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { untrack } from 'svelte';
  import { useConnection } from '$lib/state/instance/connection.svelte';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { instanceIdToSegment } from '$lib/navigation';
  import { graphql } from '$lib/gql';
  import { clearLastRoom, clearLastSpace, setLastSpace } from '$lib/storage/lastRoom';
  import { useActiveInstanceEvent, useReconnectCallback } from '$lib/hooks';
  import SecondarySidebar from '$lib/components/SecondarySidebar.svelte';
  import { createSpacePermissions } from '$lib/state/space';
  import RoomList from '$lib/RoomList.svelte';
  import SpaceHeader from './SpaceHeader.svelte';
  import SpaceBanner from './SpaceBanner.svelte';
  import SpaceEventProvider from '$lib/components/chat/SpaceEventProvider.svelte';
  import SidebarNav from '$lib/components/SidebarNav.svelte';
  import MyThreadsNavItem from './MyThreadsNavItem.svelte';

  let { data, children } = $props();

  const connection = useConnection();
  const getInstanceId = getActiveInstance();
  const instanceSegment = $derived(instanceIdToSegment(getInstanceId()));

  // Detect if we're in space admin mode based on URL (use startsWith to avoid
  // false positives from rooms or other paths that happen to contain "admin")
  const adminPrefix = $derived(
    data.spaceId ? resolve('/chat/[instanceId]/[spaceId]/admin', { instanceId: instanceSegment, spaceId: data.spaceId }) : ''
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
    data.spaceId
      ? page.url.pathname === resolve('/chat/[instanceId]/[spaceId]/rooms', { instanceId: instanceSegment, spaceId: data.spaceId })
      : false
  );

  // Detect if we're on the My Threads page
  const isMyThreadsActive = $derived(
    data.spaceId
      ? page.url.pathname === resolve('/chat/[instanceId]/[spaceId]/threads', { instanceId: instanceSegment, spaceId: data.spaceId })
      : false
  );

  // Detect if we're on the Preferences page
  const isPreferencesActive = $derived(
    data.spaceId
      ? page.url.pathname === resolve('/chat/[instanceId]/[spaceId]/preferences', { instanceId: instanceSegment, spaceId: data.spaceId })
      : false
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

  // Validate space access. Returns the space data on success, null if the
  // server says the space genuinely doesn't exist / isn't accessible, or
  // 'transient' if the request failed with a network error (treat as
  // "try again later", not as access denial).
  async function validateSpace(spaceId: string): Promise<SpaceData | null | 'transient'> {
    const result = await connection()
      .client.query(
        graphql(`
          query ValidateSpaceAccess($spaceId: ID!) {
            space(id: $spaceId) {
              id
              name
              bannerUrl(width: 512, height: 384)
              viewerIsMember
              viewerHasAnyAdminPermission
              viewerCanManageSpace
              viewerCanBrowseRooms
              viewerCanManageRooms
              viewerCanManageRoles
              viewerCanAssignRoles
              viewerCanInviteMembers
            }
          }
        `),
        { spaceId }
      )
      .toPromise();

    // Transient network failure (e.g., wake-from-sleep) — caller should
    // preserve existing data and storage, and rely on the reconnect handler
    // to revalidate.
    if (result.error?.networkError) {
      return 'transient';
    }

    // Space doesn't exist or no access
    if (!result.data?.space) {
      return null;
    }

    // User is not a member of this space
    if (!result.data.space.viewerIsMember) {
      return null;
    }

    return {
      id: result.data.space.id,
      name: result.data.space.name,
      bannerUrl: result.data.space.bannerUrl ?? null,
      hasAnyAdminPermission: result.data.space.viewerHasAnyAdminPermission,
      canManage: result.data.space.viewerCanManageSpace,
      canBrowseRooms: result.data.space.viewerCanBrowseRooms,
      canManageRooms: result.data.space.viewerCanManageRooms,
      canManageRoles: result.data.space.viewerCanManageRoles,
      canAssignRoles: result.data.space.viewerCanAssignRoles,
      canInviteMembers: result.data.space.viewerCanInviteMembers
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
  // Dependencies: data.spaceId and revalidationCounter only.
  // spaceData is read via untrack() to avoid re-triggering when the guard effect clears it.
  $effect(() => {
    const currentSpaceId = data.spaceId;
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

        // Genuine "no access" — clear stored navigation hints for this space
        // so we don't loop back here, then redirect away.
        if (result === null) {
          clearLastSpace(getInstanceId());
          clearLastRoom(getInstanceId(), currentSpaceId);
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

  // Remember this space as the last visited (only if valid)
  $effect(() => {
    if (data.spaceId && spaceData) {
      setLastSpace(getInstanceId(), data.spaceId);
    }
  });

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

  // Space name and banner — derived from spaceData, which is updated both by
  // the initial fetch and by live SpaceUpdatedEvent events.
  let spaceName = $derived(spaceData?.name ?? null);
  let bannerUrl = $derived(spaceData?.bannerUrl ?? null);

  // Listen for space updates on the active instance's event bus.
  // Uses useActiveInstanceEvent (not useInstanceEvent) so that when the user
  // switches to a remote instance, this handler receives events from that
  // instance's bus rather than the home instance's context-based bus.
  useActiveInstanceEvent((event) => {
    if (!event.event) return; // Skip unknown event types for forward/backward compatibility
    if (event.event.__typename === 'SpaceUpdatedEvent' && event.event.spaceId === data.spaceId) {
      spaceData = { ...spaceData!, name: event.event.name, bannerUrl: event.event.bannerUrl || null };
    }
  });

  // Whether the user can access ANY space settings feature (use the new hasAnyAdminPermission)
  const canAccessAnySettings = $derived(spaceData?.hasAnyAdminPermission);

  // Admin navigation items - filtered based on permissions
  const adminNavItems = $derived.by(() => {
    if (!data.spaceId || !spaceData) return [];

    const items: { href: string; label: string; icon: string }[] = [];

    // Home is always visible (landing page for admin)
    const adminBase = resolve('/chat/[instanceId]/[spaceId]/admin', { instanceId: instanceSegment, spaceId: data.spaceId });

    items.push({
      href: adminBase,
      label: 'Home',
      icon: 'iconify uil--home'
    });

    if (spaceData.canManage) {
      items.push({
        href: resolve('/chat/[instanceId]/[spaceId]/admin/general', { instanceId: instanceSegment, spaceId: data.spaceId }),
        label: 'General',
        icon: 'iconify uil--setting'
      });
    }

    if (spaceData.canAssignRoles) {
      items.push({
        href: resolve('/chat/[instanceId]/[spaceId]/admin/members', { instanceId: instanceSegment, spaceId: data.spaceId }),
        label: 'Members',
        icon: 'iconify uil--users-alt'
      });
    }

    if (spaceData.canInviteMembers) {
      items.push({
        href: resolve('/chat/[instanceId]/[spaceId]/admin/invites', { instanceId: instanceSegment, spaceId: data.spaceId }),
        label: 'Invites',
        icon: 'iconify uil--link'
      });
    }

    if (spaceData.canManageRooms) {
      items.push({
        href: resolve('/chat/[instanceId]/[spaceId]/admin/rooms', { instanceId: instanceSegment, spaceId: data.spaceId }),
        label: 'Rooms',
        icon: 'iconify uil--apps'
      });
    }

    if (spaceData.canManageRoles) {
      items.push({
        href: resolve('/chat/[instanceId]/[spaceId]/admin/roles', { instanceId: instanceSegment, spaceId: data.spaceId }),
        label: 'Roles',
        icon: 'iconify uil--shield-check'
      });
    }

    return items;
  });

  // Check if an admin nav item is active (custom logic for space-specific URLs)
  function isAdminNavActive(href: string, _items: unknown): boolean {
    if (!data.spaceId) return false;
    const adminBase = resolve('/chat/[instanceId]/[spaceId]/admin', { instanceId: instanceSegment, spaceId: data.spaceId });
    if (href === adminBase) {
      return page.url.pathname === adminBase;
    }
    return page.url.pathname.startsWith(href);
  }
</script>

{#key data.spaceId}
  {#if data.spaceId}
    <SpaceEventProvider spaceId={data.spaceId}>
      <!-- Sidebar -->
      {#if !isRoomSettingsMode}
        <SecondarySidebar>
          {#if !spaceData}
            <!-- Skeleton sidebar while space data is loading -->
            <SpaceHeader spaceId={data.spaceId} spaceName="" loading />

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
              title="Space Admin"
              subtitle={spaceName ?? undefined}
              items={adminNavItems}
              backHref={resolve('/chat/[instanceId]/[spaceId]', { instanceId: instanceSegment, spaceId: data.spaceId })}
              backLabel="Back to Space"
              isActive={isAdminNavActive}
            />
          {:else}
            <!-- Space header - fixed at top -->
            <SpaceHeader
              spaceId={data.spaceId}
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
                    href={resolve('/chat/[instanceId]/[spaceId]/rooms', { instanceId: instanceSegment, spaceId: data.spaceId })}
                    class={['sidebar-item', isBrowseRoomsActive ? 'bg-surface-100' : 'text-muted']}
                  >
                    <span class="sidebar-icon iconify uil--search-alt"></span>
                    Browse Rooms
                  </a>
                {/if}
                <MyThreadsNavItem spaceId={data.spaceId} active={isMyThreadsActive} />
                <a
                  href={resolve('/chat/[instanceId]/[spaceId]/preferences', { instanceId: instanceSegment, spaceId: data.spaceId })}
                  class={['sidebar-item', isPreferencesActive ? 'bg-surface-100' : 'text-muted']}
                >
                  <span class="sidebar-icon iconify uil--bell"></span>
                  Preferences
                </a>
              </nav>

              <hr class="border-border" />

              <!-- Room List - always visible to space members (shows rooms user has joined) -->
              <RoomList spaceId={data.spaceId} />
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
