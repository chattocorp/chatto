<script lang="ts">
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import type { ServerPermissions } from '$lib/state/server/permissions.svelte';
  import UserAvatar from './components/UserAvatar.svelte';
  import ServerSpaceSection from './ServerSpaceSection.svelte';
  import AddServerDialog from './components/AddServerDialog.svelte';

  const activeServerId = $derived(getActiveServer());
  // Get the current user for the active server (reactive — updates on
  // avatar/name changes and when navigating between servers). During the
  // setup-wizard window before the origin server is registered, this is
  // undefined; the template shows a placeholder avatar.
  const activeServerUser = $derived(
    serverRegistry.tryGetStore(activeServerId)?.currentUser.user
  );

  // Check whether any authenticated instance grants a permission.
  // Optimistically returns true while permissions are still loading.
  // Unauthenticated instances are skipped entirely.
  function anyInstanceHasPermission(key: keyof ServerPermissions): boolean {
    return serverRegistry.instances.some((i) => {
      const store = serverRegistry.tryGetStore(i.id);
      if (!store?.isAuthenticated) return false;

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
      {@const store = serverRegistry.tryGetStore(instance.id)}
      {#if store?.isAuthenticated}
        <ServerSpaceSection
          serverId={instance.id}
          currentUserId={store.currentUser.user?.id}
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
  {#if activeServerUser}
    <a
      href={resolve('/chat/[serverId]/settings', { serverId: serverIdToSegment(activeServerId) })}
      title="User Settings"
      class="m-2 mt-2 h-12 w-12 shrink-0 cursor-pointer rounded-full"
    >
      <UserAvatar user={activeServerUser} size="lg" showPresence={false} />
    </a>
  {/if}
</div>

<AddServerDialog
  bind:visible={addInstanceDialogVisible}
  onclose={() => (addInstanceDialogVisible = false)}
/>
