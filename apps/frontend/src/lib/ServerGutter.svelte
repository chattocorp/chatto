<!--
@component

The **Server Gutter** — narrow leftmost column listing every server the user
is connected to, plus the add-server button pinned to the bottom. See the
"UI" section of `docs/GLOSSARY.md`.
-->
<script lang="ts">
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import type { ServerPermissions } from '$lib/state/server/permissions.svelte';
  import * as m from '$lib/i18n/messages';
  import ScrollFader from '$lib/ui/ScrollFader.svelte';
  import ServerSidebarEntry from './ServerSidebarEntry.svelte';

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
  let addServerDialogModule: Promise<typeof import('./components/AddServerDialog.svelte')> | null =
    null;

  function loadAddServerDialog() {
    addServerDialogModule ??= import('./components/AddServerDialog.svelte');
    return addServerDialogModule;
  }
</script>

<div class="server-gutter flex min-h-0 flex-1 flex-col border-r border-border">
  <ScrollFader top bottom scrollClass="scrollbar-hide">
    <div class="flex flex-col gap-2 p-2 max-md:pl-3">
      {#each serverRegistry.servers as server (server.id)}
        {@const store = serverRegistry.tryGetStore(server.id)}
        {#if store && (store.isAuthenticated || server.reauthRequiredAt != null)}
          <ServerSidebarEntry serverId={server.id} currentUserId={store.currentUser.user?.id} />
        {/if}
      {/each}
    </div>
  </ScrollFader>

  <!-- Add Server - pinned to the bottom -->
  <div class="flex shrink-0 justify-center p-2 max-md:pl-3">
    <button
      type="button"
      onclick={() => (addServerDialogVisible = true)}
      title={m['chat.server_gutter.add_server']()}
      class={[
        'server-gutter-item cursor-pointer',
        addServerDialogVisible && 'server-gutter-item-active'
      ]}
    >
      <span class="iconify uil--plus"></span>
    </button>
  </div>
</div>

{#if addServerDialogVisible}
  {#await loadAddServerDialog() then { default: AddServerDialog }}
    <AddServerDialog
      bind:visible={addServerDialogVisible}
      onclose={() => (addServerDialogVisible = false)}
    />
  {/await}
{/if}
