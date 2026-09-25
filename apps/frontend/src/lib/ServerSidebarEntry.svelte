<script lang="ts">
  import { page } from '$app/state';
  import { goto, pushState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import { notificationTarget } from '$lib/state/server/notifications.svelte';
  import { prepareUiForNotificationTarget } from '$lib/notifications/notificationNavigationUi';
  import { getAppUiState } from '$lib/state/appUi.svelte';
  import ServerIcon from './ServerIcon.svelte';
  import { m } from '$lib/i18n/messages';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import MenuItem from '$lib/ui/MenuItem.svelte';
  import MenuSection from '$lib/ui/MenuSection.svelte';
  import NavigationContextMenu from '$lib/components/menus/NavigationContextMenu.svelte';
  import {
    contextMenuTrigger,
    type ContextMenuTriggerDetails
  } from '$lib/ui/contextMenuTrigger.svelte';
  import { markNavigationServerAsRead } from '$lib/navigation/readActions';
  import { beginOriginReauthentication, startRemoteReauthentication } from '$lib/auth/reauth';
  import { hardRedirectAfterSignOut } from '$lib/auth/signOut';
  import { clientAccount } from '$lib/state/clientAccount';
  import { toast } from '$lib/ui/toast';
  import { onMount } from 'svelte';
  import { loadSavedView } from '$lib/storage/savedViews';

  let { serverId, currentUserId: _currentUserId }: { serverId: string; currentUserId?: string } =
    $props();

  const serverSegment = $derived(serverIdToSegment(serverId));

  // Get this server's stores
  // eslint-disable-next-line svelte/no-unused-svelte-ignore -- Svelte compiler warning, not ESLint
  // svelte-ignore state_referenced_locally - serverId is stable per component lifetime (keyed by server.id)
  const stores = serverRegistry.getStore(serverId);
  onMount(() => {
    const userId = serverRegistry.getServer(serverId)?.userId ?? null;
    void loadSavedView(serverId, userId).then((view) => {
      if (view && serverRegistry.getServer(serverId)?.userId === userId) stores.restoreSavedView(view);
    });
  });
  const notificationStore = stores.notifications;
  const roomUnreadStore = stores.roomUnread;
  const appUi = getAppUiState();
  // eslint-disable-next-line svelte/no-unused-svelte-ignore -- Svelte compiler warning, not ESLint
  // svelte-ignore state_referenced_locally - serverId is stable per component lifetime (keyed by server.id)
  const serverConnection = serverConnectionManager.getClient(serverId);
  const registeredServer = $derived(serverRegistry.getServer(serverId));
  const serverHost = $derived.by(() => {
    if (!registeredServer) return null;
    try {
      return new URL(registeredServer.url).host;
    } catch {
      return registeredServer.url;
    }
  });

  // After the URL collapse (ADR-027), the active context is the deployment-wide
  // server named in the current URL segment.
  // Setup belongs to the origin server, while the client remains multi-server.
  const setupRequired = $derived(
    serverRegistry.isOriginServer(serverId) && page.data.serverInfo?.setupRequired === true
  );
  const isActiveServer = $derived(
    page.params.serverId === serverSegment ||
      (setupRequired && page.route.id === '/setup')
  );

  const privateDataLoaded = $derived(stores.projection?.viewer != null);
  const loaded = $derived(!stores.isAuthenticated || privateDataLoaded);

  const iconServer = $derived.by(() => {
    const refreshedName = stores.serverInfo.name !== 'Chatto' ? stores.serverInfo.name : undefined;
    return {
      name: refreshedName || registeredServer?.name || stores.serverInfo.name,
      logoUrl:
        stores.isAuthenticated && privateDataLoaded
          ? stores.serverInfo.iconUrl
          : (stores.serverInfo.iconUrl ?? registeredServer?.iconUrl)
    };
  });
  const needsReauth = $derived(registeredServer?.reauthRequiredAt != null);
  const needsSignIn = $derived(
    !setupRequired && !stores.isAuthenticated &&
    (!registeredServer?.token || needsReauth)
  );
  const signInRequired = $derived(!setupRequired && (needsSignIn || needsReauth));
  const compatibility = $derived(stores.serverInfo.compatibility);
  const awaitingDiscovery = $derived(stores.networkStartupDeferred || stores.serverInfo.loading);
  const compatibilityMessage = $derived.by(() => {
    if (awaitingDiscovery) return null;
    switch (compatibility.reason) {
      case 'server-too-old':
        return m('chat.server_gutter.compatibility_server_too_old');
      case 'server-version-unknown':
        return m('chat.server_gutter.compatibility_unknown');
      case 'unreachable':
        return m('chat.server_gutter.unreachable');
      default:
        return null;
    }
  });
  const compatibilityWarning = $derived(
    !awaitingDiscovery && compatibility.status !== 'supported'
  );
  const gutterWarning = $derived(compatibilityWarning || serverConnection.showConnectionLostIcon);
  const serverUnavailable = $derived(compatibility.status === 'unreachable');
  const connectionWarningMessage = $derived(
    serverConnection.showConnectionLostIcon && !serverUnavailable
      ? m('chat.server_gutter.connection_unavailable')
      : null
  );
  const recoveryNeeded = $derived(serverUnavailable || serverRegistry.needsRecovery(serverId));
  const serverActionsAvailable = $derived(
    stores.isAuthenticated &&
      privateDataLoaded &&
      compatibility.status === 'supported' &&
      !serverConnection.showConnectionLostIcon
  );
  const iconDimmed = $derived(
    !privateDataLoaded && (signInRequired || !loaded || serverConnection.showConnectionLostIcon)
  );
  const iconTitle = $derived(
    signInRequired
      ? m('ui.auth_status.sidebar_reauth', { server: iconServer.name })
      : compatibilityWarning && compatibilityMessage
        ? `${iconServer.name} — ${compatibilityMessage}`
        : connectionWarningMessage
          ? `${iconServer.name} — ${connectionWarningMessage}`
          : iconServer.name
  );
  let contextMenu = $state<ContextMenuTriggerDetails | null>(null);
  let signingIn = $state(false);
  let signingOut = $state(false);
  const serverContextMenuTrigger = contextMenuTrigger((details) => {
    contextMenu = details;
  });

  function closeContextMenu(): void {
    contextMenu = null;
  }

  function handleMarkServerRead(): void {
    closeContextMenu();
    void markNavigationServerAsRead(serverId);
  }

  async function handleCopyServerHostname(): Promise<void> {
    const hostname = serverHost;
    closeContextMenu();
    if (!hostname) return;

    try {
      await navigator.clipboard.writeText(hostname);
      toast.success(m('common.copied_to_clipboard'));
    } catch {
      toast.error(m('common.error.generic'));
    }
  }

  function handleRemoveServer(): void {
    if (serverRegistry.isOriginServer(serverId)) return;
    closeContextMenu();
    pushState('', {
      modal: {
        type: 'removeServer',
        serverId,
        spaceName: iconServer.name
      }
    });
  }

  async function handleSignIn(): Promise<void> {
    const server = registeredServer;
    if (signingIn || !server) return;
    closeContextMenu();
    signingIn = true;
    if (serverRegistry.isOriginServer(serverId)) {
      try {
        beginOriginReauthentication(resolve('/chat/[serverId]', { serverId: serverSegment }));
      } finally {
        signingIn = false;
      }
      return;
    }
    try {
      await startRemoteReauthentication(server);
    } catch {
      toast.error(m('add_server.start_failed'));
    } finally {
      signingIn = false;
    }
  }

  async function handleSignOut(): Promise<void> {
    if (signingOut || !stores.isAuthenticated) return;
    const wasActive = isActiveServer;
    closeContextMenu();
    signingOut = true;
    try {
      const navigation = await clientAccount.signOutCurrentServer(serverId);
      if (!navigation) return;
      if (navigation.kind === 'hard') {
        const href = wasActive
          ? navigation.serverId
            ? resolve('/chat/[serverId]', { serverId: serverIdToSegment(navigation.serverId) })
            : resolve('/')
          : window.location.pathname + window.location.search + window.location.hash;
        hardRedirectAfterSignOut(href);
      } else if (wasActive) {
        await goto(
          navigation.serverId
            ? resolve('/chat/[serverId]', { serverId: serverIdToSegment(navigation.serverId) })
            : resolve('/')
        );
      }
    } catch {
      toast.error(m('common.error.network'));
    } finally {
      signingOut = false;
    }
  }

  async function handleServerClick(event: MouseEvent): Promise<void> {
    if (signInRequired) {
      event.preventDefault();
      const icon = event.currentTarget;
      if (icon instanceof HTMLElement) {
        const bounds = icon.getBoundingClientRect();
        contextMenu = { position: { x: bounds.right, y: bounds.top }, presentation: 'auto' };
      }
      return;
    }
    if (recoveryNeeded && !stores.savedView) {
      event.preventDefault();
      await serverRegistry.recoverServer(serverId);
      if (stores.isAuthenticated && stores.serverInfo.compatibility.status === 'supported') {
        await goto(resolve('/chat/[serverId]', { serverId: serverSegment }));
      }
      return;
    }
  }

  // Single dispatcher for icon clicks — kind comes from serverIndicator()
  // so the two paths can't drift out of sync with what was rendered.
  function handleServerIndicatorClick(kind: 'notification' | 'unread') {
    if (kind === 'notification') return handleServerNotificationClick();
    return handleServerUnreadClick();
  }

  // Handle click on icon notification badge. The icon's notification can come
  // from either a channel mention/reply or a DM message. Prefer channel
  // notifications when both are present.
  async function handleServerNotificationClick() {
    const notification =
      notificationStore.getNonDMNotification() ?? notificationStore.getDMNotification();
    if (!notification || !notification.targetSupported) {
      await goto(resolve('/chat/notifications'));
      return;
    }

    const target = notificationTarget(notification);
    prepareUiForNotificationTarget(appUi, serverId, target);
    if (target.eventId && target.roomId) {
      stores.pendingHighlights.set(
        target.roomId,
        target.threadRootId,
        target.eventId,
        notification.id
      );
    }

    const path = notificationStore.getCleanPath(serverId, notification);
    await goto(resolve(path as '/'));
  }

  // Handle click on icon unread dot. Channel and DM unreads both flow through
  // this server icon.
  async function handleServerUnreadClick() {
    let roomId = roomUnreadStore.getFirstUnreadRoomId();

    if (!roomId) {
      roomUnreadStore.resolveUnknownUnread();
      roomId = roomUnreadStore.getFirstUnreadRoomId();
    }

    if (roomId) {
      await goto(resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId }));
    } else {
      await goto(resolve('/chat/[serverId]', { serverId: serverSegment }));
    }
  }
</script>

<!-- One icon per connected server. -->
<ServerIcon
  server={iconServer}
  href={setupRequired ? resolve('/setup') : resolve('/chat/[serverId]', { serverId: serverSegment })}
  selected={isActiveServer}
  indicator={stores.serverIndicator()}
  notificationCount={notificationStore.attention.unreadNotificationCount}
  importantNotificationCount={notificationStore.attention.importantUnreadNotificationCount}
  onclick={handleServerClick}
  onIndicatorClick={handleServerIndicatorClick}
  contextMenuTrigger={serverContextMenuTrigger}
  title={iconTitle}
  dimmed={iconDimmed}
  {signInRequired}
  compatibilityWarning={gutterWarning}
/>

{#if contextMenu}
  <ContextMenu
    position={contextMenu.position}
    presentation={contextMenu.presentation}
    ariaLabel={m('room_list.server_actions', { server: iconServer.name })}
    class="w-80 max-w-[calc(100vw-1rem)]"
    onclose={closeContextMenu}
  >
    <div
      class="menu-section px-3 py-2 text-sm"
      role="presentation"
      data-testid="server-compatibility-section"
    >
      <div class="truncate font-medium text-text" data-testid="server-name">
        {iconServer.name}
      </div>
      {#if serverHost}
        <div
          class="mt-0.5 truncate text-muted"
          title={registeredServer?.url}
          data-testid="server-hostname"
        >
          {serverHost}
        </div>
      {/if}
      {#if !awaitingDiscovery}
        <div class="mt-1 flex items-center gap-1.5 text-muted">
          {#if serverUnavailable}
            <span class="iconify icon-[uil--wifi-slash] shrink-0 text-warning" aria-hidden="true"
            ></span>
            <span class="text-warning">{m('chat.server_gutter.unreachable')}</span>
          {:else}
            <span>
              {stores.serverInfo.version
                ? m('chat.server_gutter.version', { version: stores.serverInfo.version })
                : m('chat.server_gutter.version_unknown')}
            </span>
          {/if}
        </div>
      {/if}
      {#if signInRequired}
        <div
          class="mt-1 flex items-start gap-1.5 whitespace-normal text-warning"
          data-testid="server-sign-in-message"
        >
          <span class="iconify mt-0.5 icon-[uil--exclamation-circle] shrink-0" aria-hidden="true"
          ></span>
          <span>{m('ui.auth_status.sidebar_reauth', { server: iconServer.name })}</span>
        </div>
      {/if}
      {#if compatibilityMessage && !serverUnavailable}
        <div
          class={[
            'mt-1 flex items-start gap-1.5 whitespace-normal',
            compatibilityWarning ? 'text-warning' : 'text-muted'
          ]}
          data-testid="server-compatibility-message"
        >
          {#if compatibilityWarning}
            <span class="iconify mt-0.5 icon-[uil--exclamation-circle] shrink-0" aria-hidden="true"
            ></span>
          {/if}
          <span>{compatibilityMessage}</span>
        </div>
      {/if}
      {#if connectionWarningMessage}
        <div
          class="mt-1 flex items-start gap-1.5 whitespace-normal text-warning"
          data-testid="server-connection-message"
        >
          <span class="iconify mt-0.5 icon-[uil--exclamation-circle] shrink-0" aria-hidden="true"
          ></span>
          <span>{connectionWarningMessage}</span>
        </div>
      {/if}
    </div>

    {#if recoveryNeeded}
      <MenuItem onclick={() => { closeContextMenu(); void serverRegistry.recoverServer(serverId); }}>
        {m('common.retry')}
      </MenuItem>
    {/if}
    <NavigationContextMenu
      kind="server"
      showMarkRead={serverActionsAvailable}
      canMarkRead={roomUnreadStore.hasAnyUnread || notificationStore.unreadNotificationCount > 0}
      canLeave={false}
      onMarkRead={handleMarkServerRead}
      onLeave={handleRemoveServer}
    />
    <MenuSection>
      {#if signInRequired || stores.isAuthenticated}
        {#if signInRequired}
          <MenuItem
            icon="icon-[uil--sign-in-alt]"
            onclick={() => void handleSignIn()}
            disabled={signingIn}
            dataTestid="server-log-in"
          >
            {m('chat.server_gutter.log_in')}
          </MenuItem>
        {:else}
          <MenuItem
            icon="icon-[uil--sign-out-alt]"
            onclick={() => void handleSignOut()}
            disabled={signingOut}
            dataTestid="server-sign-out"
          >
            {m('chat.server_gutter.sign_out')}
          </MenuItem>
        {/if}
      {/if}
      {#if !serverRegistry.isOriginServer(serverId)}
        <MenuItem icon="icon-[uil--minus-circle]" tone="danger" onclick={handleRemoveServer}>
          {m('room_list.remove_server')}
        </MenuItem>
      {/if}
    </MenuSection>
    {#if serverHost}
      <MenuSection>
        <MenuItem
          icon="icon-[uil--copy]"
          onclick={() => void handleCopyServerHostname()}
          dataTestid="copy-server-hostname"
        >
          {m('room_list.copy_server_hostname')}
        </MenuItem>
      </MenuSection>
    {/if}
  </ContextMenu>
{/if}
