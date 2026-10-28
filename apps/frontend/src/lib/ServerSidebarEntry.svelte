<script lang="ts">
  import { conversationPath } from '$lib/navigation/conversationPath';
  import { page } from '$app/state';
  import { serverUi } from '$lib/state/server/serverUi';
  import { goto, pushState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { serverRegistry, serverConnectionManager } from '$lib/client';
  import { notificationTarget } from '@chatto/client/server/notifications';
  import { serverDisplayName } from '@chatto/client/server/state';
  import { MINIMUM_CONNECTABLE_SERVER_VERSION } from '@chatto/client/server/compatibility';
  import { serverHost } from '$lib/serverUrl';
  import {
    prepareUiForNotificationTarget,
    navigateNotification
  } from '$lib/notifications/notificationNavigationUi';
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
  import { markNavigationServerAsRead } from '$lib/navigation/readActions';
  import { beginOriginReauthentication } from '$lib/auth/reauth';
  import { isRemoteSignInPending, startRemoteSignIn } from '$lib/auth/remoteSignIn.svelte';
  import { hardRedirectAfterSignOut } from '$lib/auth/signOutRedirect';
  import { clientAccount } from '$lib/state/clientAccount';
  import { toast } from '$lib/ui/toast';
  import { copyToClipboard } from '$lib/utils/copyToClipboard';
  import { notificationPath } from '$lib/notificationPath';
  import { serverGutterOrder } from '$lib/state/serverGutterOrder.svelte';
  import type { Attachment } from 'svelte/attachments';
  import { tick } from 'svelte';

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
  const servers = $derived(serverRegistry.servers);
  const originId = $derived(servers.find((server) => serverRegistry.isOriginServer(server.id))?.id);
  const serverIds = $derived(servers.map((server) => server.id));
  const orderedServerIds = $derived(serverGutterOrder.ordered(serverIds, originId));
  const orderIndex = $derived(orderedServerIds.indexOf(serverId));
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
   * A connection problem or compatibility warning, or null. The gutter icon has two
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
          if (compatibility.status === 'limited') return m('chat.compatibility.limited');
          return m('chat.server_gutter.compatibility_server_too_old', {
            version: MINIMUM_CONNECTABLE_SERVER_VERSION
          });
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
      stores.serverInfo.canConnect &&
      !serverConnection.showConnectionLostIcon
  );
  let contextMenu = $state<ContextMenuTriggerDetails | null>(null);
  let signingOut = $state(false);
  let serverLink: HTMLAnchorElement | null = null;
  let keyboardMenu = false;
  /** Move keyboard focus into the menu after its popover reaches the top layer. */
  const focusKeyboardMenu: Attachment<HTMLElement> = (node) => {
    if (!keyboardMenu) return;
    const frame = requestAnimationFrame(() =>
      node
        .closest('[role="menu"]')
        ?.querySelector<HTMLElement>('a[href], button:not(:disabled)')
        ?.focus()
    );
    return () => cancelAnimationFrame(frame);
  };
  /** Native pointer menus plus an explicit keyboard path on every platform. */
  const serverContextMenuTrigger: Attachment<HTMLElement> = (node) => {
    serverLink = node.querySelector('a');
    const detach = contextMenuTrigger((details) => {
      keyboardMenu = false;
      contextMenu = details;
    })(node);
    const keydown = (event: KeyboardEvent) => {
      if (event.key !== 'ContextMenu' && !(event.shiftKey && event.key === 'F10')) return;
      event.preventDefault();
      event.stopPropagation();
      keyboardMenu = true;
      const rect = node.getBoundingClientRect();
      contextMenu = { position: { x: rect.right, y: rect.bottom }, presentation: 'auto' };
    };
    node.addEventListener('keydown', keydown);
    return () => {
      node.removeEventListener('keydown', keydown);
      detach?.();
      serverLink = null;
    };
  };

  function closeContextMenu(): void {
    const returnFocus = keyboardMenu;
    keyboardMenu = false;
    contextMenu = null;
    if (returnFocus) void tick().then(() => serverLink?.focus({ preventScroll: true }));
  }

  /** Move within the server list without changing navigation or authentication. */
  function moveServer(direction: -1 | 1): void {
    closeContextMenu();
    serverGutterOrder.move(serverId, direction, serverIds, originId);
    void tick().then(() => serverLink?.focus({ preventScroll: true }));
  }

  function handleMarkServerRead(): void {
    closeContextMenu();
    void markNavigationServerAsRead(serverId);
  }

  async function handleCopyServerHostname(): Promise<void> {
    const hostname = host;
    closeContextMenu();
    if (!hostname) return;

    await copyToClipboard(hostname);
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
    closeContextMenu();
    signingOut = true;
    try {
      const navigation = await clientAccount.signOutCurrentServer(serverId);
      if (!navigation) return;
      const href = navigation.serverId
        ? resolve('/chat/[serverId]', { serverId: serverIdToSegment(navigation.serverId) })
        : resolve('/');
      if (navigation.kind === 'hard') {
        hardRedirectAfterSignOut(href);
      } else {
        await goto(href);
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
    await navigateNotification(stores, notification, () => goto(resolve(path as '/')));
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
      await goto(conversationPath(serverId, roomId));
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
      {@attach focusKeyboardMenu}
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

    <MenuSection>
      <MenuItem
        icon="icon-[uil--info-circle]"
        href={resolve('/chat/[serverId]/info', { serverId: serverSegment })}
        onclick={closeContextMenu}
      >
        {m('chat.server_info.title')}
      </MenuItem>
      {#if host}
        <MenuItem
          icon="icon-[uil--copy]"
          onclick={() => void handleCopyServerHostname()}
          dataTestid="copy-server-hostname"
        >
          {m('room_list.copy_server_hostname')}
        </MenuItem>
      {/if}
      {#if serverActionsAvailable}
        <MenuItem
          icon="icon-[uil--check-circle]"
          onclick={handleMarkServerRead}
          disabled={!roomUnreadStore.hasAnyUnread &&
            notificationStore.unreadNotificationCount === 0}
        >
          {m('room_list.mark_as_read')}
        </MenuItem>
      {/if}
    </MenuSection>
    {#if serverIds.length > 1}
      <MenuSection>
        <MenuItem
          icon="icon-[uil--arrow-up]"
          onclick={() => moveServer(-1)}
          disabled={orderIndex <= 0}
          dataTestid="move-server-up"
        >
          {m('chat.server_gutter.move_up')}
        </MenuItem>
        <MenuItem
          icon="icon-[uil--arrow-down]"
          onclick={() => moveServer(1)}
          disabled={orderIndex < 0 || orderIndex >= orderedServerIds.length - 1}
          dataTestid="move-server-down"
        >
          {m('chat.server_gutter.move_down')}
        </MenuItem>
      </MenuSection>
    {/if}
    {#if recoveryNeeded || signInRequired || stores.isAuthenticated || !serverRegistry.isOriginServer(serverId)}
      <MenuSection>
        {#if recoveryNeeded}
          <MenuItem
            icon="icon-[uil--redo]"
            mirrorIconInRtl
            onclick={() => {
              closeContextMenu();
              void serverRegistry.recoverServer(serverId);
            }}>{m('common.retry')}</MenuItem
          >
        {/if}
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
    {/if}
  </ContextMenu>
{/if}
