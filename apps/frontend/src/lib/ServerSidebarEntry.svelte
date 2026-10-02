<script lang="ts">
  import { page } from '$app/state';
  import { serverUi } from '$lib/state/server/serverUi';
  import { goto, pushState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { serverRegistry, serverConnectionManager } from '$lib/client';
  import { notificationTarget } from '@chatto/client/server/notifications';
  import { serverDisplayName } from '@chatto/client/server/state';
  import { serverHost } from '@chatto/client/util/serverUrl';
  import { prepareUiForNotificationTarget } from '$lib/notifications/notificationNavigationUi';
  import { getAppUiState } from '$lib/state/appUi.svelte';
  import ServerIcon from './ServerIcon.svelte';
  import { m } from '$lib/i18n/messages';
  import {
    ContextMenu,
    MenuItem,
    MenuSection,
    contextMenuTrigger,
    type ContextMenuTriggerDetails
  } from '$lib/ui';
  import NavigationContextMenu from '$lib/components/menus/NavigationContextMenu.svelte';
  import { markNavigationServerAsRead } from '$lib/navigation/readActions';
  import { beginOriginReauthentication } from '$lib/auth/reauth';
  import { isRemoteSignInPending, startRemoteSignIn } from '$lib/auth/remoteSignIn.svelte';
  import { hardRedirectAfterSignOut } from '$lib/auth/signOutRedirect';
  import { clientAccount } from '$lib/state/clientAccount';
  import { toast } from '$lib/ui/toast';
  import { notificationPath } from '$lib/notificationPath';

  let { serverId }: { serverId: string } = $props();

  const serverSegment = $derived(serverIdToSegment(serverId));

  // Get this server's stores
  // eslint-disable-next-line svelte/no-unused-svelte-ignore -- Svelte compiler warning, not ESLint
  // svelte-ignore state_referenced_locally - serverId is stable per component lifetime (keyed by server.id)
  const stores = serverRegistry.getStore(serverId);
  const notificationStore = stores.notifications;
  const roomUnreadStore = serverUi(stores).roomUnread;
  const appUi = getAppUiState();
  // eslint-disable-next-line svelte/no-unused-svelte-ignore -- Svelte compiler warning, not ESLint
  // svelte-ignore state_referenced_locally - serverId is stable per component lifetime (keyed by server.id)
  const serverConnection = serverConnectionManager.getClient(serverId);
  const registeredServer = $derived(serverRegistry.getServer(serverId));
  const host = $derived(registeredServer ? serverHost(registeredServer.url) : null);

  // After the URL collapse (ADR-027), the active context is the deployment-wide
  // server named in the current URL segment.
  // Setup belongs to the origin server, while the client remains multi-server.
  const setupRequired = $derived(
    serverRegistry.isOriginServer(serverId) && page.data.serverInfo?.setupRequired === true
  );
  const isActiveServer = $derived(
    page.params.serverId === serverSegment || (setupRequired && page.route.id === '/setup')
  );

  const privateDataLoaded = $derived(stores.projection?.viewer != null);

  const iconServer = $derived.by(() => {
    return {
      name: serverDisplayName(stores.serverInfo, registeredServer?.name),
      logoUrl:
        stores.isAuthenticated && privateDataLoaded
          ? stores.serverInfo.iconUrl
          : (stores.serverInfo.iconUrl ?? registeredServer?.iconUrl)
    };
  });
  const needsReauth = $derived(registeredServer?.reauthRequiredAt != null);
  const needsSignIn = $derived(
    !setupRequired && !stores.isAuthenticated && (!registeredServer?.token || needsReauth)
  );
  const signInRequired = $derived(!setupRequired && (needsSignIn || needsReauth));
  const compatibility = $derived(stores.serverInfo.compatibility);
  const serverUnavailable = $derived(compatibility.status === 'unreachable');

  /**
   * Why the client cannot use this server, or null. The gutter icon has two
   * states: normal, or a warning when this is set. Discovery and connection
   * attempts in progress are not problems; only a failed attempt is.
   */
  const problem = $derived.by((): string | null => {
    if (signInRequired) return m('ui.auth_status.sidebar_reauth', { server: iconServer.name });
    if (!stores.serverInfo.loading) {
      switch (compatibility.reason) {
        case 'unreachable':
          return m('chat.server_gutter.unreachable');
        case 'server-too-old':
          return m('chat.server_gutter.compatibility_server_too_old');
        case 'server-version-unknown':
          return m('chat.server_gutter.compatibility_unknown');
      }
    }
    if (serverConnection.showConnectionLostIcon) {
      return m('chat.server_gutter.connection_unavailable');
    }
    return null;
  });
  const iconTitle = $derived.by(() => {
    if (!problem) return iconServer.name;
    // The sign-in message already names the server.
    if (signInRequired) return problem;
    return `${iconServer.name} — ${problem}`;
  });
  const recoveryNeeded = $derived(serverUnavailable || serverRegistry.needsRecovery(serverId));
  const serverActionsAvailable = $derived(
    stores.isAuthenticated &&
      privateDataLoaded &&
      compatibility.status === 'supported' &&
      !serverConnection.showConnectionLostIcon
  );
  let contextMenu = $state<ContextMenuTriggerDetails | null>(null);
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
    const hostname = host;
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

  function handleSignIn(): void {
    const server = registeredServer;
    if (!server) return;
    closeContextMenu();
    if (serverRegistry.isOriginServer(serverId)) {
      beginOriginReauthentication(resolve('/chat/[serverId]', { serverId: serverSegment }));
      return;
    }
    void startRemoteSignIn(server);
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

  // Selecting a server with a problem opens it, and the server view explains
  // the problem. Recovery runs in the background and never opens sign-in.
  function handleServerClick(): void {
    if (recoveryNeeded) void serverRegistry.recoverServer(serverId);
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
      serverUi(stores).attention.getNonDMNotification() ??
      serverUi(stores).attention.getDMNotification();
    if (!notification || !notification.targetSupported) {
      await goto(resolve('/chat/notifications'));
      return;
    }

    const target = notificationTarget(notification);
    prepareUiForNotificationTarget(appUi, serverId, target);
    if (target.eventId && target.roomId) {
      serverUi(stores).pendingHighlights.set(
        target.roomId,
        target.threadRootId,
        target.eventId,
        notification.id
      );
    }

    const path = notificationPath(serverId, notification);
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
  href={setupRequired
    ? resolve('/setup')
    : resolve('/chat/[serverId]', { serverId: serverSegment })}
  selected={isActiveServer}
  indicator={serverUi(stores).serverIndicator()}
  notificationCount={serverUi(stores).attention.counts.unreadNotificationCount}
  importantNotificationCount={serverUi(stores).attention.counts.importantUnreadNotificationCount}
  onclick={handleServerClick}
  onIndicatorClick={handleServerIndicatorClick}
  contextMenuTrigger={serverContextMenuTrigger}
  title={iconTitle}
  warning={problem !== null}
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
      {#if host}
        <div
          class="mt-0.5 truncate text-muted"
          title={registeredServer?.url}
          data-testid="server-hostname"
        >
          {host}
        </div>
      {/if}
      {#if stores.serverInfo.version}
        <div class="mt-1 text-muted">
          {m('chat.server_gutter.version', { version: stores.serverInfo.version })}
        </div>
      {/if}
      {#if problem}
        <div
          class="mt-1 flex items-start gap-1.5 whitespace-normal text-warning"
          data-testid="server-problem-message"
        >
          <span class="iconify mt-0.5 icon-[uil--exclamation-circle] shrink-0" aria-hidden="true"
          ></span>
          <span>{problem}</span>
        </div>
      {/if}
    </div>

    {#if recoveryNeeded}
      <MenuItem
        onclick={() => {
          closeContextMenu();
          void serverRegistry.recoverServer(serverId);
        }}
      >
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
            mirrorIconInRtl
            onclick={handleSignIn}
            disabled={isRemoteSignInPending(serverId)}
            dataTestid="server-log-in"
          >
            {m('chat.server_gutter.log_in')}
          </MenuItem>
        {:else}
          <MenuItem
            icon="icon-[uil--sign-out-alt]"
            mirrorIconInRtl
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
    {#if host}
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
