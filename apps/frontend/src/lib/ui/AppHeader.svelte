<script lang="ts">
  import KeyboardShortcut from '$lib/components/KeyboardShortcut.svelte';
  import {
    bindingText,
    shortcutPreferences,
    shortcutProps
  } from '$lib/state/shortcutPreferences.svelte';
  import { page } from '$app/state';
  import { gutterEnabled as configuredGutterEnabled } from '$lib/frontendConfig';
  import { pushState } from '$app/navigation';
  import { serverUi } from '$lib/state/server/serverUi';
  import { resolve } from '$app/paths';
  import { serverRegistry, serverConnectionManager } from '$lib/client';
  import { firstAuthenticatedServerId } from '$lib/serverCatalogue';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import { version } from '$app/environment';
  import { sidebarNav, quickSwitcher } from '$lib/state/globals.svelte';
  import { m } from '$lib/i18n/messages';
  import UnreadDot from '$lib/ui/UnreadDot.svelte';
  import MotdContent from '$lib/ui/MotdContent.svelte';
  import { SERVER_SETTINGS_ROOT_ROUTE } from '$lib/navigation/settingsRoutes';
  import { desktopUpdates, type DesktopUpdates } from '$lib/desktop/updates.svelte';
  import type { Snippet } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import type { ServerStateStore } from '@chatto/client/server/store';

  let {
    updates = desktopUpdates,
    gutterEnabled = configuredGutterEnabled,
    callControls,
    enhanceMotdLink
  }: {
    updates?: DesktopUpdates;
    /** Whether the hosting frontend offers multi-server navigation. */
    gutterEnabled?: boolean;
    /** App-owned controls for the connected call, independent of navigation. */
    callControls?: Snippet<[ServerStateStore]>;
    /** App-owned link enhancement for the MOTD preview, such as Chatto link pills. */
    enhanceMotdLink?: Attachment<HTMLAnchorElement>;
  } = $props();

  // The MOTD and the connection warning follow the active server. Without a
  // server in the URL, the active server is the origin.
  const motd = $derived(serverRegistry.tryGetStore(getActiveServer())?.serverInfo.motd);
  // A server without a signed-in user has no connection to warn about.
  const connectionWarning = $derived.by(() => {
    const serverId = getActiveServer();
    if (!serverRegistry.getServer(serverId)) return null;
    if (!serverRegistry.tryGetStore(serverId)?.currentUser.user) return null;
    const connection = serverConnectionManager.getClient(serverId);
    if (!connection.showConnectionLostIcon) return null;
    return {
      urgent: connection.showConnectionLostBanner,
      // API requests can fail while the realtime connection is open.
      label:
        connection.status === 'connected'
          ? m('chat.server_gutter.connection_unavailable')
          : m('ui.realtime_paused')
    };
  });
  // Call controls follow their owner, even when navigation selects another server.
  const activeCallStore = $derived.by(() => {
    for (const server of serverRegistry.servers) {
      const store = serverRegistry.tryGetStore(server.id);
      if (store && serverUi(store).voiceCall.connected) return store;
    }
    return null;
  });

  /** A server's notification counts without viewed ones; zero without a store. */
  function attentionCounts(serverId: string) {
    const store = serverRegistry.tryGetStore(serverId);
    return store
      ? serverUi(store).attention.counts
      : { unreadNotificationCount: 0, importantUnreadNotificationCount: 0 };
  }

  // Aggregate exact notification counts across all servers.
  const totalNotificationCount = $derived(
    serverRegistry.servers.reduce(
      (sum, instance) => sum + attentionCounts(instance.id).unreadNotificationCount,
      0
    )
  );
  const totalImportantNotificationCount = $derived(
    serverRegistry.servers.reduce(
      (sum, instance) => sum + attentionCounts(instance.id).importantUnreadNotificationCount,
      0
    )
  );

  const showChatNavigation = $derived(
    gutterEnabled || serverRegistry.isAuthenticated(getActiveServer())
  );
  // Help and app preferences have their own sidebars, including before sign-in.
  const showSidebarToggle = $derived(
    showChatNavigation ||
      page.route.id?.startsWith('/help') ||
      page.route.id?.startsWith('/chat/preferences')
  );
  const hasInstances = $derived(serverRegistry.servers.length > 0);
  const notificationsServerId = $derived.by(() => {
    const activeServerId = getActiveServer();
    return activeServerId && serverRegistry.isAuthenticated(activeServerId) ? activeServerId : null;
  });
  let settingsLink: HTMLAnchorElement;
  const preferencesServerId = $derived.by(() => {
    const activeServerId = getActiveServer();
    if (activeServerId && serverRegistry.isAuthenticated(activeServerId)) return activeServerId;
    return firstAuthenticatedServerId();
  });

  function showAboutChatto() {
    pushState('', { modal: { type: 'aboutChatto' } });
  }
</script>

<!-- WebKit extends the solid background of a sticky header into its top system bar. -->
<header
  class="app-header @container sticky top-0 flex keyboard-hide-mobile h-[var(--app-header-height)] shrink-0 items-center justify-between gap-1 overflow-x-auto bg-surface px-2 text-muted sm:gap-3 desktop-presentation:text-sm"
>
  <!-- Leading: global navigation, notifications, and client-wide actions -->
  <div class="flex shrink-0 items-center gap-3">
    {#if showSidebarToggle}
      <!-- Sidebar toggle - 44px tap target for mobile accessibility -->
      <button
        type="button"
        class="app-header-icon"
        onclick={() => sidebarNav.toggle()}
        aria-label={m('ui.toggle_sidebar')}
        aria-expanded={sidebarNav.isOpen}
        title={m('ui.toggle_sidebar')}
      >
        <span aria-hidden="true" class="iconify icon-[uil--bars] text-xl"></span>
      </button>
    {/if}

    {#if showChatNavigation && hasInstances}
      <!-- Notification bell - 44px tap target for mobile accessibility -->
      <a
        href={notificationsServerId
          ? resolve('/chat/[serverId]/notifications', {
              serverId: serverIdToSegment(notificationsServerId)
            })
          : resolve('/chat/notifications')}
        aria-label={m('ui.notifications')}
        title={m('ui.notifications')}
        class="app-header-icon relative"
      >
        <span aria-hidden="true" class="iconify icon-[uil--bell] text-lg"></span>
        {#if totalNotificationCount > 0}
          <UnreadDot
            color={totalImportantNotificationCount > 0 ? 'warning' : 'ambient'}
            class="absolute end-2 top-2"
            testid="notifications-unread-dot"
          />
        {/if}
      </a>

      <!-- Quick switcher trigger -->
      <button
        type="button"
        class="app-header-icon"
        onclick={() => quickSwitcher.open()}
        aria-label={m('ui.open_quick_switcher')}
        title={[m('ui.open_quick_switcher'), bindingText(shortcutPreferences.bindings.switcher)]
          .filter(Boolean)
          .join(' · ')}
      >
        <span aria-hidden="true" class="iconify icon-[uil--apps] text-lg"></span>
      </button>
    {/if}

    <KeyboardShortcut
      {...shortcutProps(shortcutPreferences.bindings.settings)}
      description={m('settings.app_preferences.title')}
      ontrigger={() => settingsLink.click()}
    />
    <a
      bind:this={settingsLink}
      href={preferencesServerId
        ? resolve(SERVER_SETTINGS_ROOT_ROUTE, {
            serverId: serverIdToSegment(preferencesServerId)
          })
        : resolve('/chat/preferences')}
      class="app-header-icon"
      aria-label={m('settings.app_preferences.title')}
      title={m('settings.app_preferences.title')}
    >
      <span class="iconify icon-[uil--setting] text-lg" aria-hidden="true"></span>
    </a>

    <!-- Connection warning of the active server: a failed realtime attempt or
         API requests that fail on the network. -->
    {#if connectionWarning}
      <span
        class={[
          'iconify icon-[uil--wifi-slash] text-lg',
          connectionWarning.urgent ? 'text-warning' : 'animate-pulse'
        ]}
        role="img"
        aria-label={connectionWarning.label}
        title={connectionWarning.label}
      ></span>
    {/if}
  </div>

  {#if activeCallStore && callControls}
    <div class="shrink-0">
      {#key activeCallStore}
        {@render callControls(activeCallStore)}
      {/key}
    </div>
  {/if}

  <!-- MOTD -->
  {#if motd}
    <MotdContent
      {motd}
      enhanceLink={enhanceMotdLink}
      onclick={() => pushState('', { modal: { type: 'motd', motd } })}
    />
  {:else}
    <span class={['flex-1', activeCallStore && callControls && 'max-sm:hidden']}></span>
  {/if}

  <!-- Client version, About, and Help actions -->
  <div class="flex shrink-0 items-center gap-3">
    {#if updates.status?.status === 'ready'}
      <a
        href={resolve('/chat/preferences/desktop')}
        class="app-header-icon text-action"
        title={m('settings.desktop.ready_link')}
        aria-label={m('settings.desktop.ready_link')}
      >
        <span class="iconify icon-[uil--download-alt] text-lg" aria-hidden="true"></span>
      </a>
    {/if}
    {#if version}
      <!-- Wide viewports have room to show the client version next to the About action. -->
      <span class="hidden text-sm tabular-nums md:inline" data-testid="app-header-version"
        >v{version}</span
      >
      <button
        type="button"
        class="app-header-icon"
        onclick={showAboutChatto}
        title={m('ui.tooltip.about', { subject: 'Chatto' })}
        aria-label={m('ui.tooltip.about', { subject: 'Chatto' })}
      >
        <span class="iconify icon-[uil--info-circle] text-lg" aria-hidden="true"></span>
      </button>
    {/if}
    <a
      href={resolve('/help')}
      class="app-header-icon"
      aria-label={m('ui.help')}
      title={m('ui.help')}
    >
      <span class="iconify icon-[uil--question-circle] text-lg" aria-hidden="true"></span>
    </a>
  </div>
</header>

<style>
  /* Keep the surface full-width while content stays clear of native window buttons.
     Overlay coordinates use the viewport, not the padded shell width.
     The custom properties also let stories model the host-provided safe area. */
  .app-header {
    padding-left: calc(0.5rem + var(--app-header-titlebar-x, env(titlebar-area-x, 0px)));
    padding-right: calc(
      0.5rem + 100vw - var(--app-header-titlebar-x, env(titlebar-area-x, 0px)) -
        var(--app-header-titlebar-width, env(titlebar-area-width, 100vw))
    );
    /* Electron window dragging excludes the interactive elements below. */
    -webkit-app-region: drag;
  }
  .app-header :global(a),
  .app-header :global(button) {
    -webkit-app-region: no-drag;
  }
</style>
