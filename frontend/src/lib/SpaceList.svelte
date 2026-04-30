<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { instanceIdToSegment, segmentToInstanceId } from '$lib/navigation';

  // SpaceList renders in the root layout (above [instanceId]),
  // so it cannot use getActiveInstance(). Derive instance from URL.
  const originInstanceId = $derived(instanceRegistry.originInstance?.id ?? '');
  import { getCurrentUser } from '$lib/auth/currentUser.svelte';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { getInstancePermissions, type InstancePermissions, type ViewerData } from '$lib/state/instance/permissions.svelte';
  import SpaceIcon from './SpaceIcon.svelte';
  import UserAvatar from './components/UserAvatar.svelte';
  import InstanceSpaceSection from './InstanceSpaceSection.svelte';
  import { notificationTarget } from '$lib/state/instance/notifications.svelte';
  import type { SpaceIndicator } from '$lib/state/instance/store.svelte';

  // Context-based current user — set by the root layout, populated by
  // AuthenticatedChatProvider. Used as fallback when the instance store's
  // currentUser isn't populated yet (e.g. immediately after login, before
  // the origin instance is fully registered in the store).
  const currentUserCtx = getCurrentUser();

  const DM_SPACE_ID = 'DM';

  let {
    activeSpaceId,
    onPermissionsLoaded
  }: {
    activeSpaceId?: string;
    /** Callback to update instance permissions when the combined query completes. */
    onPermissionsLoaded?: (viewer: ViewerData) => void;
  } = $props();

  // Derive the active instance from the URL. Checks both instanceId
  // (routes under /chat/[instanceId]/...) and instanceSegment (DM routes
  // under /chat/dm/[instanceSegment]/...). On instance-agnostic routes
  // (e.g. /chat/spaces) falls back to the origin instance.
  const instanceSegment = $derived(page.params.instanceId ?? page.params.instanceSegment);
  const activeInstanceId = $derived(
    (instanceSegment ? segmentToInstanceId(instanceSegment) : null)
    ?? originInstanceId
  );
  const originInstanceSegment = $derived(instanceIdToSegment(originInstanceId));

  // Get the current user for the active instance (reactive — updates on
  // avatar/name changes and when navigating between instances).
  // Falls back to context user for the origin instance (covers the setup
  // wizard flow where the store may not be populated yet).
  const activeInstanceUser = $derived(
    instanceRegistry.tryGetStore(activeInstanceId)?.currentUser.user
    ?? (activeInstanceId === originInstanceId ? currentUserCtx.user : undefined)
  );

  // Check if we're on DM pages (unified route: /chat/dm/...)
  let isDMActive = $derived(page.url.pathname.startsWith(resolve('/chat/dm')));

  // Check if we're on Browse Spaces page
  let isBrowseSpacesActive = $derived(page.url.pathname === resolve('/chat/spaces'));

  // Check if we're on Admin pages
  let isAdminActive = $derived(page.url.pathname.startsWith(resolve('/chat/[instanceId]/admin', { instanceId: originInstanceSegment })));

  // Check if we're on Create Space page
  let isCreateSpaceActive = $derived(page.url.pathname === resolve('/chat/spaces/new'));

  // Read permissions from centralized instance permissions context
  const instancePerms = getInstancePermissions();
  let canViewAdmin = $derived(instancePerms.current.canViewAdmin);

  // Check whether any authenticated instance grants a permission.
  // Optimistically returns true while permissions are still loading.
  // Unauthenticated instances are skipped entirely.
  function anyInstanceHasPermission(key: keyof InstancePermissions): boolean {
    return instanceRegistry.instances.some((i) => {
      const store = instanceRegistry.tryGetStore(i.id);
      if (!store) return false;

      // Origin's currentUser is populated reactively by AuthenticatedChatProvider,
      // but during the gap between probeOrigin and that mount the context user
      // is the only signal — fall through to it for the origin slot.
      const authed =
        store.isAuthenticated ||
        (instanceRegistry.isOriginInstance(i.id) && !!currentUserCtx.user);
      if (!authed) return false;

      const perms = store.permissions;
      return !perms.loaded || perms[key];
    });
  }

  let anyCanViewDMs = $derived(anyInstanceHasPermission('canViewDMs'));
  let anyCanCreateSpace = $derived(anyInstanceHasPermission('canCreateSpace'));
  let anyCanBrowseSpaces = $derived(anyInstanceHasPermission('canListSpaces'));

  // The DM space icon represents DMs across ALL authenticated instances
  // (the bell aggregates across instances too, so the DM dot must as well —
  // a remote DM should still light up the icon).
  let dmIndicator = $derived.by((): SpaceIndicator => {
    let result: SpaceIndicator = null;
    for (const instance of instanceRegistry.instances) {
      const stores = instanceRegistry.tryGetStore(instance.id);
      if (!stores?.isAuthenticated) continue;
      const ind = stores.dmIndicator();
      if (ind === 'notification') return 'notification';
      if (ind === 'unread') result = 'unread';
    }
    return result;
  });

  // Find the first authenticated instance with a DM notification or unread
  // and return both the instance and a candidate conversation id (or null).
  function findDMTarget(kind: 'notification' | 'unread'):
    | { instanceId: string; conversationId: string | null }
    | null {
    for (const instance of instanceRegistry.instances) {
      const stores = instanceRegistry.tryGetStore(instance.id);
      if (!stores?.isAuthenticated) continue;
      if (kind === 'notification') {
        if (stores.notifications.hasDMNotifications()) {
          const n = stores.notifications.getDMNotification();
          const t = n ? notificationTarget(n) : null;
          return { instanceId: instance.id, conversationId: t?.roomId ?? null };
        }
      } else {
        if (stores.roomUnread.spaceHasUnread(DM_SPACE_ID)) {
          return {
            instanceId: instance.id,
            conversationId: stores.roomUnread.getFirstUnreadRoomId(DM_SPACE_ID)
          };
        }
      }
    }
    return null;
  }

  // Handle click on DM unread dot - navigate to first unread DM conversation
  // on whichever instance has it.
  async function handleDMUnreadClick() {
    const target = findDMTarget('unread');
    if (!target) {
      await goto(resolve('/chat/dm'));
      return;
    }
    const seg = instanceIdToSegment(target.instanceId);
    if (target.conversationId) {
      await goto(resolve('/chat/dm/[instanceSegment]/[conversationId]', { instanceSegment: seg, conversationId: target.conversationId }));
    } else {
      await goto(resolve('/chat/dm'));
    }
  }

  // Handle click on DM notification dot - dismiss and navigate to the
  // instance + conversation the notification points to.
  async function handleDMNotificationClick() {
    const target = findDMTarget('notification');
    if (!target) return;
    const stores = instanceRegistry.getStore(target.instanceId);
    const notification = stores.notifications.getDMNotification();
    if (!notification) return;

    void stores.notifications.dismiss(notification.id);

    const path = stores.notifications.getCleanPath(target.instanceId, notification);
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- path from getCleanPath() is already resolved
    await goto(path);
  }

  function handleDMIndicatorClick(kind: 'notification' | 'unread') {
    if (kind === 'notification') return handleDMNotificationClick();
    return handleDMUnreadClick();
  }
</script>

<div class="space-list flex min-h-0 flex-1 flex-col border-r border-border">
  <!-- Scrollable area for spaces and navigation -->
  <div
    class="scrollbar-hide flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto p-2"
    data-sidebar-scroll
  >
    <!-- Direct Messages -->
    {#if anyCanViewDMs}
      <div data-testid="dm-icon">
        <SpaceIcon
          icon="iconify uil--comment-alt-lines"
          title="Direct Messages"
          href={resolve('/chat/dm')}
          selected={isDMActive}
          indicator={dmIndicator}
          onIndicatorClick={handleDMIndicatorClick}
        />
      </div>
    {/if}

    <!-- Per-instance space sections (only for authenticated instances) -->
    {#each instanceRegistry.instances as instance (instance.id)}
      {@const isOrigin = instanceRegistry.isOriginInstance(instance.id)}
      {@const store = instanceRegistry.tryGetStore(instance.id)}
      {@const instanceUser = store?.currentUser.user ?? (isOrigin ? currentUserCtx.user : undefined)}
      {#if store?.isAuthenticated || (isOrigin && currentUserCtx.user)}
        <InstanceSpaceSection
          instanceId={instance.id}
          {activeSpaceId}
          currentUserId={instanceUser?.id}
          onPermissionsLoaded={isOrigin ? onPermissionsLoaded : undefined}
        />
      {/if}
    {/each}

    <!-- Add Instance -->
    <a
      href={resolve('/instances/add')}
      title="Add Instance"
      class={['space-list-item', page.url.pathname === '/instances/add' && 'space-list-item-active']}
    >
      <span class="iconify uil--plus"></span>
    </a>

    <!-- Create Space (visible when any instance grants space.create) -->
    {#if anyCanCreateSpace}
      <a
        href={resolve('/chat/spaces/new')}
        title="Create Space"
        class={['space-list-item', isCreateSpaceActive && 'space-list-item-active']}
      >
        <span class="iconify uil--create-dashboard"></span>
      </a>
    {/if}

    <!-- Explore Spaces -->
    {#if anyCanBrowseSpaces}
      <a
        href={resolve('/chat/spaces')}
        title="Explore Spaces"
        class={['space-list-item', isBrowseSpacesActive && 'space-list-item-active']}
      >
        <span class="iconify uil--compass"></span>
      </a>
    {/if}

    <!-- Admin Panel (only if user has permission) -->
    {#if canViewAdmin}
      <a
        href={resolve('/chat/[instanceId]/admin', { instanceId: originInstanceSegment })}
        title="Admin Panel"
        class={['space-list-item', isAdminActive && 'space-list-item-active']}
      >
        <span class="iconify uil--setting"></span>
      </a>
    {/if}
  </div>

  <!-- User avatar - shows the user for the currently active instance -->
  {#if activeInstanceUser}
    <a
      href={resolve('/chat/[instanceId]/settings', { instanceId: instanceIdToSegment(activeInstanceId) })}
      title="User Settings"
      class="m-2 mt-2 h-12 w-12 shrink-0 cursor-pointer rounded-full"
    >
      <UserAvatar user={activeInstanceUser} size="lg" showPresence={false} />
    </a>
  {/if}
</div>
