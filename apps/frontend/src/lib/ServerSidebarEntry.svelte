<script lang="ts">
  import { conversationPath } from '$lib/navigation/conversationPath';
  import { page } from '$app/state';
  import { serverUi } from '$lib/state/server/serverUi';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { serverRegistry } from '$lib/client';
  import { notificationTarget } from '@chatto/client/server/notifications';
  import {
    prepareUiForNotificationTarget,
    navigateNotification
  } from '$lib/notifications/notificationNavigationUi';
  import { getAppUiState } from '$lib/state/appUi.svelte';
  import ServerIcon from './ServerIcon.svelte';
  import ServerActionsMenu from '$lib/components/ServerActionsMenu.svelte';
  import { m } from '$lib/i18n/messages';
  import {
    MenuItem,
    MenuSection,
    contextMenuTrigger,
    type ContextMenuTriggerDetails
  } from '$lib/ui';
  import { notificationPath } from '$lib/notificationPath';
  import { serverGutterOrder } from '$lib/state/serverGutterOrder.svelte';
  import { serverStatus } from '$lib/state/server/serverStatus.svelte';
  import type { Attachment } from 'svelte/attachments';
  import { tick } from 'svelte';

  let { serverId }: { serverId: string } = $props();

  const serverSegment = $derived(serverIdToSegment(serverId));

  // eslint-disable-next-line svelte/no-unused-svelte-ignore -- Svelte compiler warning, not ESLint
  // svelte-ignore state_referenced_locally - serverId is stable per component lifetime (keyed by server.id)
  const status = serverStatus(serverId);
  const stores = status.stores;
  const roomUnreadStore = serverUi(stores).roomUnread;
  const appUi = getAppUiState();
  const servers = $derived(serverRegistry.servers);
  const originId = $derived(servers.find((server) => serverRegistry.isOriginServer(server.id))?.id);
  const serverIds = $derived(servers.map((server) => server.id));
  const orderedServerIds = $derived(serverGutterOrder.ordered(serverIds, originId));
  const orderIndex = $derived(orderedServerIds.indexOf(serverId));

  // After the URL collapse (ADR-027), the active context is the deployment-wide
  // server named in the current URL segment.
  const setupRequired = $derived(status.setupRequired);
  const isActiveServer = $derived(
    page.params.serverId === serverSegment || (setupRequired && page.route.id === '/setup')
  );

  const iconServer = $derived({
    name: status.name,
    logoUrl:
      stores.isAuthenticated && status.privateDataLoaded
        ? stores.serverInfo.iconUrl
        : (stores.serverInfo.iconUrl ?? status.registeredServer?.iconUrl)
  });
  const signInRequired = $derived(status.signInRequired);

  /**
   * A connection problem or compatibility warning, or null. The gutter icon has two
   * states: normal, or a warning when this is set.
   */
  const problem = $derived(status.problem);
  const iconTitle = $derived.by(() => {
    if (!problem) return iconServer.name;
    // The sign-in message already names the server.
    if (signInRequired) return problem;
    return `${iconServer.name} — ${problem}`;
  });
  let contextMenu = $state<ContextMenuTriggerDetails | null>(null);
  let serverLink: HTMLAnchorElement | null = null;
  let keyboardMenu = $state(false);
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

  // Selecting a server with a problem opens it, and the server view explains
  // the problem. Recovery runs in the background and never opens sign-in.
  function handleServerClick(): void {
    if (status.recoveryNeeded) void serverRegistry.recoverServer(serverId);
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
  <ServerActionsMenu
    {status}
    position={contextMenu.position}
    presentation={contextMenu.presentation}
    focusFirstItem={keyboardMenu}
    onclose={closeContextMenu}
  >
    {#snippet ordering()}
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
    {/snippet}
  </ServerActionsMenu>
{/if}
