<script lang="ts">
  import { pushState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { version } from '$app/environment';
  import { sidebarNav, quickSwitcher } from '$lib/state/globals.svelte';
  import * as m from '$lib/i18n/messages';
  import UnreadDot from '$lib/ui/UnreadDot.svelte';
  import MotdContent from '$lib/ui/MotdContent.svelte';

  // MOTD follows the active server; the connection-lost icon below stays
  // bound to the origin store since it reflects the SPA host's own connection.
  const motd = $derived(serverRegistry.tryGetStore(getActiveServer())?.serverInfo.motd);
  const originStore = $derived(serverRegistry.tryGetStore(serverRegistry.originServer?.id ?? ''));

  // Aggregate notification count across all servers.
  const totalNotificationCount = $derived(
    serverRegistry.servers.reduce(
      (sum, instance) => sum + serverRegistry.getStore(instance.id).notifications.count,
      0
    )
  );

  // Show sign-out button when any server is registered
  const hasInstances = $derived(serverRegistry.servers.length > 0);

  function handleSignOut() {
    pushState('', { modal: { type: 'logout' } });
  }
</script>

<header class="app-header flex items-center justify-between gap-2 p-2 text-muted md:text-sm">
  <!-- Leading: Sidebar toggle + Notifications -->
  <div class="flex items-center gap-3">
    <!-- Hamburger - 44px tap target for mobile accessibility -->
    <button
      type="button"
      class="app-header-icon"
      onclick={() => sidebarNav.toggle()}
      aria-label={m['ui.toggle_sidebar']()}
      aria-expanded={sidebarNav.isOpen}
      title={m['ui.toggle_sidebar']()}
    >
      <span class="iconify text-xl uil--bars"></span>
    </button>

    <!-- Notification bell - 44px tap target for mobile accessibility -->
    <a
      href={resolve('/chat/notifications')}
      aria-label={m['ui.notifications']()}
      title={m['ui.notifications']()}
      class="relative app-header-icon"
    >
      <span class="iconify text-lg uil--bell"></span>
      {#if totalNotificationCount > 0}
        <UnreadDot class="absolute top-2 right-2" />
      {/if}
    </a>

    <!-- Quick switcher trigger -->
    {#if hasInstances}
      <button
        type="button"
        class="app-header-icon"
        onclick={() => quickSwitcher.open()}
        aria-label={m['ui.open_quick_switcher']()}
        title={m['ui.quick_switcher_shortcut']()}
      >
        <span class="iconify text-lg uil--apps"></span>
      </button>
    {/if}

    <!-- Connection lost indicator: only show when an authenticated server has lost connection.
         Skip the origin server if the user isn't authenticated (no WebSocket expected). -->
    {#if originStore?.currentUser.user && serverConnectionManager.originClient.showConnectionLostIcon}
      <span
        class={[
          'iconify text-lg uil--wifi-slash',
          serverConnectionManager.originClient.showConnectionLostBanner
            ? 'text-warning'
            : 'animate-pulse'
        ]}
        title={m['ui.realtime_paused']()}
      ></span>
    {/if}
  </div>

  <!-- MOTD -->
  {#if motd}
    <MotdContent {motd} />
  {:else}
    <span class="flex-1"></span>
  {/if}

  <!-- Actions: Version + Logout -->
  <div class="flex items-center gap-3">
    {#if version}
      <span class="text-muted">v{version}</span>
    {/if}

    {#if hasInstances}
      <button
        type="button"
        class="iconify cursor-pointer uil--signout hover:text-text"
        onclick={handleSignOut}
        title={m['ui.sign_out']()}
      >
      </button>
    {/if}
  </div>
</header>

<style>
  /* Tauri window dragging - header is draggable, interactive elements are not */
  .app-header {
    -webkit-app-region: drag;
  }
  .app-header :global(a),
  .app-header :global(button) {
    -webkit-app-region: no-drag;
  }
</style>
