<script lang="ts">
  import { pushState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { graphqlClientManager } from '$lib/state/instance/graphqlClient.svelte';
  import { renderMarkdown } from '$lib/markdown';
  import { version } from '$app/environment';
  import { sidebarNav } from '$lib/state/globals.svelte';
  import UnreadDot from '$lib/ui/UnreadDot.svelte';

  // AppHeader renders in the root layout (above [[instanceId=hostname]]),
  // so it cannot use getActiveInstance(). Use the origin instance directly
  // for instance-specific things like MOTD.
  let originStores = $derived(instanceRegistry.originInstance ? instanceRegistry.getStore(instanceRegistry.originInstance.id) : undefined);
  let motd = $derived(originStores?.instance.motd);

  // Aggregate notification count across all instances.
  let totalNotificationCount = $derived(
    instanceRegistry.instances.reduce(
      (sum, instance) => sum + instanceRegistry.getStore(instance.id).notifications.count,
      0
    )
  );

  // Show sign-out button when any instance is registered
  let hasInstances = $derived(instanceRegistry.instances.length > 0);

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
      class="-m-2 flex h-11 w-11 cursor-pointer items-center justify-center rounded active:bg-surface-200"
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
      class="relative -m-2 flex h-11 w-11 cursor-pointer items-center justify-center rounded active:bg-surface-200"
    >
      <span class="iconify text-lg uil--bell"></span>
      {#if totalNotificationCount > 0}
        <UnreadDot class="absolute top-2 right-2" />
      {/if}
    </a>

    <!-- Connection lost indicator: only show when an authenticated instance has lost connection.
         Skip the origin instance if the user isn't authenticated (no WebSocket expected). -->
    {#if originStores?.currentUser.user && graphqlClientManager.originClient.showConnectionLostIcon}
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
