<script lang="ts">
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import type { ServerPermissions } from '$lib/state/server/permissions.svelte';
  import ServerSpaceSection from './ServerSpaceSection.svelte';
  import AddServerDialog from './components/AddServerDialog.svelte';

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
  </div>

  <!-- Add Server - pinned to the bottom; the top border lines up with the
       secondary sidebar's current-user bar. -->
  <div class="flex shrink-0 justify-center border-t border-border p-2">
    <button
      type="button"
      onclick={() => (addInstanceDialogVisible = true)}
      title="Add Server"
      class={['space-list-item cursor-pointer', addInstanceDialogVisible && 'space-list-item-active']}
    >
      <span class="iconify uil--plus"></span>
    </button>
  </div>
</div>

<AddServerDialog
  bind:visible={addInstanceDialogVisible}
  onclose={() => (addInstanceDialogVisible = false)}
/>
