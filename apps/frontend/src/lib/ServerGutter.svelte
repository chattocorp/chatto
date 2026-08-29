<!--
@component

The **Server Gutter** — narrow inline-start column listing every server the user
is connected to, plus the add-server button pinned to the bottom. See the
"UI" section of `docs/GLOSSARY.md`.
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import type { ServerPermissions } from '$lib/state/server/permissions';
  import { m } from '$lib/i18n/messages';
  import { ScrollFader } from '$lib/ui';
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

  const directoryHref = resolve('/chat/servers');
  const directoryActive = $derived(page.route.id === '/chat/servers');
</script>

<div class="server-gutter flex min-h-0 flex-1 flex-col border-e border-border">
  <ScrollFader top bottom scrollClass="scrollbar-hide">
    <div class="flex flex-col gap-2 p-2 max-md:ps-3">
      {#each serverRegistry.servers as server (server.id)}
        {@const store = serverRegistry.tryGetStore(server.id)}
        {#if store}
          <!-- Authentication changes replace the per-server store. Remount the
               entry so its one-time private-data load follows the new state. -->
          {#key store}
            <ServerSidebarEntry serverId={server.id} />
          {/key}
        {/if}
      {/each}
    </div>
  </ScrollFader>

  <!-- Add Server - pinned to the bottom -->
  <div class="flex shrink-0 flex-col items-center gap-2 p-2 max-md:ps-3">
    <a
      href={directoryHref}
      title={m('chat.server_gutter.add_server')}
      aria-label={m('chat.server_gutter.add_server')}
      aria-current={directoryActive ? 'page' : undefined}
      class={[
        'server-gutter-item cursor-pointer',
        directoryActive && 'server-gutter-item-active'
      ]}
    >
      <span class="iconify icon-[uil--plus]"></span>
    </a>
  </div>
</div>
