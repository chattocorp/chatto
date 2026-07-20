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
  import * as m from '$lib/i18n/messages';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import NavigationContextMenu from '$lib/components/menus/NavigationContextMenu.svelte';
  import {
    contextMenuTrigger,
    type ContextMenuTriggerDetails
  } from '$lib/ui/contextMenuTrigger.svelte';
  import { markNavigationServerAsRead } from '$lib/navigation/readActions';

  let {
    serverId,
    currentUserId: _currentUserId
  }: { serverId: string; currentUserId?: string } = $props();

  const serverSegment = $derived(serverIdToSegment(serverId));

  // Get this server's stores
  // eslint-disable-next-line svelte/no-unused-svelte-ignore -- Svelte compiler warning, not ESLint
  // svelte-ignore state_referenced_locally - serverId is stable per component lifetime (keyed by server.id)
  const stores = serverRegistry.getStore(serverId);
  const notificationStore = stores.notifications;
  const roomUnreadStore = stores.roomUnread;
  const appUi = getAppUiState();
  // eslint-disable-next-line svelte/no-unused-svelte-ignore -- Svelte compiler warning, not ESLint
  // svelte-ignore state_referenced_locally - serverId is stable per component lifetime (keyed by server.id)
  const serverConnection = serverConnectionManager.getClient(serverId);
  const registeredServer = $derived(serverRegistry.getServer(serverId));

  // After the URL collapse (ADR-027), the active context is the deployment-wide
  // server named in the current URL segment.
  const isActiveServer = $derived(page.params.serverId === serverSegment);

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
  const compatibility = $derived(stores.serverInfo.compatibility);
  const compatibilityMessage = $derived.by(() => {
    switch (compatibility.reason) {
      case 'missing-recommended-capabilities':
        return m['chat.server_gutter.compatibility_degraded']();
      case 'server-too-old':
        return m['chat.server_gutter.compatibility_server_too_old']();
      case 'web-client-too-old':
        return m['chat.server_gutter.compatibility_client_too_old']();
      case 'missing-required-capabilities':
        return m['chat.server_gutter.compatibility_unsupported']();
      case 'legacy-server':
        return m['chat.server_gutter.compatibility_unknown']();
      default:
        return null;
    }
  });
  const compatibilityWarning = $derived(
    compatibility.status === 'degraded' || compatibility.status === 'unsupported'
  );
  const iconDimmed = $derived(!loaded || serverConnection.showConnectionLostIcon || needsReauth);
  const iconTitle = $derived(
    needsReauth
      ? m['ui.auth_status.sidebar_reauth']({ server: iconServer.name })
      : compatibilityWarning && compatibilityMessage
        ? `${iconServer.name} — ${compatibilityMessage}`
        : iconDimmed
          ? `${iconServer.name} (connection unavailable)`
          : iconServer.name
  );
  let contextMenu = $state<ContextMenuTriggerDetails | null>(null);
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

  function handleRemoveServer(): void {
    closeContextMenu();
    pushState('', {
      modal: {
        type: 'removeServer',
        serverId,
        spaceName: iconServer.name
      }
    });
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
      notificationStore.getSpaceNotification() ?? notificationStore.getDMNotification();
    if (!notification) {
      await goto(resolve('/chat/notifications'));
      return;
    }

    const target = notificationTarget(notification);
    prepareUiForNotificationTarget(appUi, serverId, target);
    if (target.eventId && target.roomId) {
      stores.pendingHighlights.set(target.roomId, target.threadRootId, target.eventId);
    }
    void notificationStore.dismiss(notification.id);

    const path = notificationStore.getCleanPath(serverId, notification);
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- path from getCleanPath() is already resolved
    await goto(path);
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
  href={resolve('/chat/[serverId]', { serverId: serverSegment })}
  selected={isActiveServer}
  indicator={stores.serverIndicator()}
  notificationCount={notificationStore.unreadNotificationCount}
  onIndicatorClick={handleServerIndicatorClick}
  contextMenuTrigger={serverContextMenuTrigger}
  title={iconTitle}
  dimmed={iconDimmed}
  {compatibilityWarning}
/>

{#if contextMenu}
  <ContextMenu
    position={contextMenu.position}
    presentation={contextMenu.presentation}
    ariaLabel={m['room_list.server_actions']({ server: iconServer.name })}
    class="w-80 max-w-[calc(100vw-1rem)]"
    onclose={closeContextMenu}
  >
    <div
      class="menu-section px-3 py-2 text-sm"
      role="presentation"
      data-testid="server-compatibility-section"
    >
      <div class="text-muted">
        {stores.serverInfo.version
          ? m['chat.server_gutter.version']({ version: stores.serverInfo.version })
          : m['chat.server_gutter.version_unknown']()}
      </div>
      {#if compatibilityMessage}
        <div
          class={[
            'mt-1 flex items-start gap-1.5 whitespace-normal',
            compatibilityWarning ? 'text-warning' : 'text-muted'
          ]}
          data-testid="server-compatibility-message"
        >
          {#if compatibilityWarning}
            <span class="iconify mt-0.5 shrink-0 uil--exclamation-circle" aria-hidden="true"></span>
          {/if}
          <span>{compatibilityMessage}</span>
        </div>
      {/if}
    </div>
    <NavigationContextMenu
      kind="server"
      canMarkRead={roomUnreadStore.hasAnyUnread || notificationStore.unreadNotificationCount > 0}
      onMarkRead={handleMarkServerRead}
      onLeave={handleRemoveServer}
    />
  </ContextMenu>
{/if}
