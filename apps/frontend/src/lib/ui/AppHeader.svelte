<script lang="ts">
  import { pushState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import { version } from '$app/environment';
  import { sidebarNav, quickSwitcher } from '$lib/state/globals.svelte';
  import { m } from '$lib/i18n/messages';
  import UnreadDot from '$lib/ui/UnreadDot.svelte';
  import MotdContent from '$lib/ui/MotdContent.svelte';

  // MOTD follows the active server; the connection-lost icon below stays
  // bound to the origin store since it reflects the SPA host's own connection.
  const motd = $derived(serverRegistry.tryGetStore(getActiveServer())?.serverInfo.motd);
  const originStore = $derived(serverRegistry.tryGetStore(serverRegistry.originServer?.id ?? ''));

  // Aggregate exact notification counts across all servers.
  const totalNotificationCount = $derived(
    serverRegistry.servers.reduce(
      (sum, instance) =>
        sum + (serverRegistry.tryGetStore(instance.id)?.notifications.unreadNotificationCount ?? 0),
      0
    )
  );
  const totalImportantNotificationCount = $derived(
    serverRegistry.servers.reduce(
      (sum, instance) =>
        sum +
        (serverRegistry.tryGetStore(instance.id)?.notifications.importantUnreadNotificationCount ?? 0),
      0
    )
  );

  // Show sign-out button when any server is registered
  const hasInstances = $derived(serverRegistry.servers.length > 0);
  const preferencesServerId = $derived.by(() => {
    const activeServerId = getActiveServer();
    if (activeServerId && serverRegistry.isAuthenticated(activeServerId)) return activeServerId;
    return serverRegistry.firstAuthenticatedServerId();
  });
  function handleSignOut() {
    pushState('', { modal: { type: 'logout' } });
  }

  function showAboutChatto() {
    pushState('', { modal: { type: 'aboutChatto' } });
  }
</script>

<header class="app-header flex items-center justify-between gap-2 p-2 text-muted md:text-sm">
  <!-- Leading: global navigation, notifications, and client-wide actions -->
  <div class="flex items-center gap-3">
    <!-- Hamburger - 44px tap target for mobile accessibility -->
    <button
      type="button"
      class="app-header-icon"
      onclick={() => sidebarNav.toggle()}
      aria-label={m('ui.toggle_sidebar')}
      aria-expanded={sidebarNav.isOpen}
      title={m('ui.toggle_sidebar')}
    >
      <span class="iconify icon-[uil--bars] text-xl"></span>
    </button>

    {#if hasInstances}
      <!-- Notification bell - 44px tap target for mobile accessibility -->
      <a
        href={resolve('/chat/notifications')}
        aria-label={m('ui.notifications')}
        title={m('ui.notifications')}
        class="relative app-header-icon"
      >
        <span class="iconify icon-[uil--bell] text-lg"></span>
        {#if totalNotificationCount > 0}
          <UnreadDot
            color={totalImportantNotificationCount > 0 ? 'warning' : 'ambient'}
            class="absolute end-2 top-2"
            testid="notifications-unread-dot"
          />
        {/if}
      </a>
    {/if}

    <!-- Quick switcher trigger -->
    {#if hasInstances}
      <button
        type="button"
        class="app-header-icon"
        onclick={() => quickSwitcher.open()}
        aria-label={m('ui.open_quick_switcher')}
        title={m('ui.quick_switcher_shortcut')}
      >
        <span class="iconify icon-[uil--apps] text-lg"></span>
      </button>
    {/if}

    <a
      href={preferencesServerId
        ? resolve('/chat/[serverId]/settings/appearance', {
            serverId: serverIdToSegment(preferencesServerId)
          })
        : resolve('/chat/preferences')}
      class="app-header-icon"
      aria-label={m('settings.app_preferences.title')}
      title={m('settings.app_preferences.title')}
    >
      <span class="iconify icon-[uil--setting] text-lg" aria-hidden="true"></span>
    </a>

    <!-- Connection lost indicator: only show when an authenticated server has lost connection.
         Skip the origin server if the user isn't authenticated (no WebSocket expected). -->
    {#if originStore?.currentUser.user && serverConnectionManager.originClient.showConnectionLostIcon}
      <span
        class={[
          'iconify icon-[uil--wifi-slash] text-lg',
          serverConnectionManager.originClient.showConnectionLostBanner
            ? 'text-warning'
            : 'animate-pulse'
        ]}
        title={m('ui.realtime_paused')}
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
      <button
        type="button"
        class="min-h-10 cursor-pointer rounded px-2 text-muted transition-colors hover:bg-surface-emphasized hover:text-text focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-action"
        onclick={showAboutChatto}
        title={m('ui.tooltip.about', { subject: 'Chatto' })}
        aria-label={m('ui.tooltip.about', { subject: 'Chatto' })}
      >
        v{version}
      </button>
    {/if}

    {#if hasInstances}
      <button
        type="button"
        class="iconify icon-[uil--signout] cursor-pointer hover:text-text"
        onclick={handleSignOut}
        title={m('ui.sign_out')}
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
