<script lang="ts">
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getCurrentUser } from '$lib/auth/currentUser.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import type { ServerPermissions } from '$lib/state/server/permissions.svelte';
  import UserAvatar from './components/UserAvatar.svelte';
  import ServerSpaceSection from './ServerSpaceSection.svelte';
  import AddServerDialog from './components/AddServerDialog.svelte';

  // Context-based current user — set by the root layout, populated by
  // AuthenticatedChatProvider. Used as fallback when the instance store's
  // currentUser isn't populated yet (e.g. immediately after login, before
  // the origin instance is fully registered in the store).
  const currentUserCtx = getCurrentUser();

  const originServerId = $derived(serverRegistry.originServer?.id ?? '');
  const getServerId = getActiveServer();
  const activeInstanceId = $derived(getServerId());
  // Get the current user for the active instance (reactive — updates on
  // avatar/name changes and when navigating between instances).
  // Falls back to context user for the origin instance (covers the setup
  // wizard flow where the store may not be populated yet).
  const activeInstanceUser = $derived(
    serverRegistry.tryGetStore(activeInstanceId)?.currentUser.user
    ?? (activeInstanceId === originServerId ? currentUserCtx.user : undefined)
  );

  // Check whether any authenticated instance grants a permission.
  // Optimistically returns true while permissions are still loading.
  // Unauthenticated instances are skipped entirely.
  function anyInstanceHasPermission(key: keyof ServerPermissions): boolean {
    return serverRegistry.instances.some((i) => {
      const store = serverRegistry.tryGetStore(i.id);
      if (!store) return false;

      // Origin's currentUser is populated reactively by AuthenticatedChatProvider,
      // but during the gap between probeOrigin and that mount the context user
      // is the only signal — fall through to it for the origin slot.
      const authed =
        store.isAuthenticated ||
        (serverRegistry.isOriginInstance(i.id) && !!currentUserCtx.user);
      if (!authed) return false;

      const perms = store.permissions;
      return !perms.loaded || perms[key];
    });
  }

  void anyInstanceHasPermission;

  let addInstanceDialogVisible = $state(false);
</script>

<div class="space-list flex min-h-0 flex-1 flex-col border-r border-border">
  <!-- Scrollable area for spaces and navigation -->
  <div
    class="scrollbar-hide flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto p-2"
    data-sidebar-scroll
  >
    <!-- Per-instance space sections (only for authenticated instances) -->
    {#each serverRegistry.instances as instance (instance.id)}
      {@const isOrigin = serverRegistry.isOriginInstance(instance.id)}
      {@const store = serverRegistry.tryGetStore(instance.id)}
      {@const serverUser = store?.currentUser.user ?? (isOrigin ? currentUserCtx.user : undefined)}
      {#if store?.isAuthenticated || (isOrigin && currentUserCtx.user)}
        <ServerSpaceSection
          serverId={instance.id}
          currentUserId={serverUser?.id}
        />
      {/if}
    {/each}

    <!-- Add Server -->
    <button
      type="button"
      onclick={() => (addInstanceDialogVisible = true)}
      title="Add Server"
      class={['space-list-item', addInstanceDialogVisible && 'space-list-item-active']}
    >
      <span class="iconify uil--plus"></span>
    </button>

  </div>

  <!-- User avatar - shows the user for the currently active instance -->
  {#if activeInstanceUser}
    <a
      href={resolve('/chat/[serverId]/settings', { serverId: serverIdToSegment(activeInstanceId) })}
      title="User Settings"
      class="m-2 mt-2 h-12 w-12 shrink-0 cursor-pointer rounded-full"
    >
      <UserAvatar user={activeInstanceUser} size="lg" showPresence={false} />
    </a>
  {/if}
</div>

<AddServerDialog
  bind:visible={addInstanceDialogVisible}
  onclose={() => (addInstanceDialogVisible = false)}
/>
