<!--
@component

The **Server Gutter** — narrow leftmost column listing every server the user
is connected to, plus the add-server button pinned to the bottom. See the
"UI" section of `docs/GLOSSARY.md`.
-->
<script lang="ts">
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import type { ServerPermissions } from '$lib/state/server/permissions.svelte';
  import ServerSpaceSection from './ServerSpaceSection.svelte';
  import AddServerDialog from './components/AddServerDialog.svelte';

  // Check whether any authenticated server grants a permission.
  // Optimistically returns true while permissions are still loading.
  // Unauthenticated servers are skipped entirely.
  function anyServerHasPermission(key: keyof ServerPermissions): boolean {
    return serverRegistry.servers.some((s) => {
      const store = serverRegistry.tryGetStore(s.id);
      if (!store?.isAuthenticated) return false;

      const perms = store.permissions;
      return !perms.loaded || perms[key];
    });
  }

  void anyServerHasPermission;

  let addServerDialogVisible = $state(false);
</script>

<div class="server-gutter flex min-h-0 flex-1 flex-col border-r border-border">
  <!-- Scrollable area for servers and navigation -->
  <div
    class="scrollbar-hide flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto p-2"
    data-sidebar-scroll
  >
    <!-- Per-server room sections (only for authenticated servers) -->
    {#each serverRegistry.servers as server (server.id)}
      {@const store = serverRegistry.tryGetStore(server.id)}
      {#if store?.isAuthenticated}
        <ServerSpaceSection
          serverId={server.id}
          currentUserId={store.currentUser.user?.id}
        />
      {/if}
    {/each}
  </div>

  <!-- Add Server - pinned to the bottom; the top border lines up with the
       Server Sidebar's current-user bar. -->
  <div class="flex shrink-0 justify-center border-t border-border p-2">
    <button
      type="button"
      onclick={() => (addServerDialogVisible = true)}
      title="Add Server"
      class={['server-gutter-item cursor-pointer', addServerDialogVisible && 'server-gutter-item-active']}
    >
      <span class="iconify uil--plus"></span>
    </button>
  </div>
</div>

<AddServerDialog
  bind:visible={addServerDialogVisible}
  onclose={() => (addServerDialogVisible = false)}
/>
