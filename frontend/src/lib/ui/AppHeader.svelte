<script lang="ts">
  import { pushState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { graphqlClientManager } from '$lib/state/server/graphqlClient.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { renderMarkdown } from '$lib/markdown';
  import { version } from '$app/environment';
  import { sidebarNav, quickSwitcher } from '$lib/state/globals.svelte';
  import UnreadDot from '$lib/ui/UnreadDot.svelte';

  // MOTD follows the active instance; the connection-lost icon below stays
  // bound to the origin store since it reflects the SPA host's own connection.
  const getInstanceId = getActiveServer();
  const motd = $derived(serverRegistry.tryGetStore(getInstanceId())?.instance.motd);
  const originStore = $derived(
    serverRegistry.tryGetStore(serverRegistry.originServer?.id ?? '')
  );

  // Aggregate notification count across all instances.
  const totalNotificationCount = $derived(
    serverRegistry.instances.reduce(
      (sum, instance) => sum + serverRegistry.getStore(instance.id).notifications.count,
      0
    )
  );

  // Show sign-out button when any instance is registered
  const hasInstances = $derived(serverRegistry.instances.length > 0);

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
      aria-label="Toggle sidebar"
      aria-expanded={sidebarNav.isOpen}
      title="Toggle sidebar"
    >
      <span class="iconify text-xl uil--bars"></span>
    </button>

    <!-- Notification bell - 44px tap target for mobile accessibility -->
    <a
      href={resolve('/chat/notifications')}
      aria-label="Notifications"
      title="Notifications"
      class="app-header-icon relative"
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
        aria-label="Open quick switcher"
        title="Quick switcher (⌘K)"
      >
        <span class="iconify text-lg uil--apps"></span>
      </button>
    {/if}

    <!-- Connection lost indicator: only show when an authenticated instance has lost connection.
         Skip the origin instance if the user isn't authenticated (no WebSocket expected). -->
    {#if originStore?.currentUser.user && graphqlClientManager.originClient.showConnectionLostIcon}
      <span
        class={[
          'iconify text-lg uil--wifi-slash',
          graphqlClientManager.originClient.showConnectionLostBanner ? 'text-warning' : 'animate-pulse'
        ]}
        title="Real-time updates paused. Reconnecting..."
      ></span>
    {/if}
  </div>

  <!-- MOTD -->
  {#if motd}
    <span
      data-testid="motd-content"
      class="prose prose-compact max-w-none flex-1 truncate text-center text-sm"
    >
      {#await renderMarkdown(motd)}
        {motd}
      {:then html}
        <!-- eslint-disable-next-line svelte/no-at-html-tags -->
        {@html html}
      {/await}
    </span>
  {:else}
    <span class="flex-1"></span>
  {/if}

  <!-- Actions: Version + Logout -->
  <div class="flex items-center gap-3">
    {#if version}
      <span class="text-text/50">v{version}</span>
    {/if}

    {#if hasInstances}
      <button
        type="button"
        class="iconify cursor-pointer uil--signout hover:text-text"
        onclick={handleSignOut}
        title="Sign out"
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
