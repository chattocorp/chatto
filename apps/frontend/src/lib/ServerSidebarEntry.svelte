<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import { createEventBusHandlerRegistrar } from '$lib/eventBus.svelte';
  import { isMessagePostedEvent, RoomEventKind, roomEventKind } from '$lib/render/eventKinds';
  import { getAuthenticatedServerState } from '$lib/api-client/serverState';
  import { getViewerStateViaConnect } from '$lib/api-client/viewer';
  import {
    createRoomDirectoryAPI,
    RoomDirectoryScope,
    RoomKind
  } from '$lib/api-client/roomDirectory';
  import { notificationTarget } from '$lib/state/server/notifications.svelte';
  import { prepareUiForNotificationTarget } from '$lib/notifications/notificationNavigationUi';
  import { getAppUiState } from '$lib/state/appUi.svelte';
  import { appState } from '$lib/state/globals.svelte';
  import ServerIcon from './ServerIcon.svelte';
  import { onMount } from 'svelte';
  import * as m from '$lib/i18n/messages';

  let {
    serverId,
    currentUserId
  }: {
    serverId: string;
    currentUserId?: string;
  } = $props();

  const serverSegment = $derived(serverIdToSegment(serverId));

  // Get this server's stores
  // eslint-disable-next-line svelte/no-unused-svelte-ignore -- Svelte compiler warning, not ESLint
  // svelte-ignore state_referenced_locally - serverId is stable per component lifetime (keyed by server.id)
  const stores = serverRegistry.getStore(serverId);
  const notificationStore = stores.notifications;
  const roomUnreadStore = stores.roomUnread;
  const notificationLevelStore = stores.notificationLevels;
  const appUi = getAppUiState();
  // eslint-disable-next-line svelte/no-unused-svelte-ignore -- Svelte compiler warning, not ESLint
  // svelte-ignore state_referenced_locally - serverId is stable per component lifetime (keyed by server.id)
  const serverConnection = serverConnectionManager.getClient(serverId);
  const registeredServer = $derived(serverRegistry.getServer(serverId));

  function connectAPIConfig() {
    return {
      serverId,
      baseUrl: serverConnection.connectBaseUrl,
      bearerToken: serverConnection.bearerToken
    };
  }

  function roomDirectoryAPI() {
    return createRoomDirectoryAPI(connectAPIConfig());
  }

  // After the URL collapse (ADR-027), the active context is the deployment-wide
  // server named in the current URL segment.
  const isActiveServer = $derived(page.params.serverId === serverSegment);

  let displayName = $state('');
  let logoUrl = $state<string | null>(null);
  let privateDataLoaded = $state(false);
  const loaded = $derived(!stores.isAuthenticated || privateDataLoaded);

  const iconServer = $derived.by(() => {
    const refreshedName = stores.serverInfo.name !== 'Chatto' ? stores.serverInfo.name : undefined;
    return {
      name: displayName || refreshedName || registeredServer?.name || stores.serverInfo.name,
      logoUrl:
        stores.isAuthenticated && privateDataLoaded
          ? logoUrl
          : (stores.serverInfo.iconUrl ?? registeredServer?.iconUrl)
    };
  });
  const needsReauth = $derived(registeredServer?.reauthRequiredAt != null);
  const iconDimmed = $derived(!loaded || serverConnection.showConnectionLostIcon || needsReauth);
  const iconTitle = $derived(
    needsReauth
      ? m['ui.auth_status.sidebar_reauth']({ server: iconServer.name })
      : iconDimmed
        ? `${iconServer.name} (connection unavailable)`
        : iconServer.name
  );

  // Single dispatcher for icon clicks — kind comes from serverIndicator()
  // so the two paths can't drift out of sync with what was rendered.
  function handleServerIndicatorClick(kind: 'notification' | 'unread') {
    if (kind === 'notification') return handleServerNotificationClick();
    return handleServerUnreadClick();
  }

  async function loadAll() {
    const unreadSnapshotRevision = roomUnreadStore.captureSnapshotRevision();
    try {
      const [serverState, viewer, rooms] = await Promise.all([
        getAuthenticatedServerState(connectAPIConfig()),
        getViewerStateViaConnect(connectAPIConfig()),
        roomDirectoryAPI().listRooms(RoomDirectoryScope.ALL),
        notificationStore.fetch()
      ]);

      stores.setPermissions(viewer);
      // Populate room-level notification preferences first.
      for (const pref of viewer.roomNotificationPreferences) {
        notificationLevelStore.setRoomPreference(pref.roomId, pref.level, pref.effectiveLevel);
      }

      const pref = viewer.serverNotificationPreference;
      notificationLevelStore.setServerPreference(pref.level, pref.effectiveLevel);
      const hasUnreadChannel = rooms.some(
        (room) => room.kind === RoomKind.CHANNEL && room.hasUnread
      );
      roomUnreadStore.initRooms(
        rooms,
        serverState.viewerHasUnreadRooms && !hasUnreadChannel,
        unreadSnapshotRevision
      );

      displayName = serverState.name;
      logoUrl = serverState.logoUrl;
      privateDataLoaded = true;
    } catch (err) {
      console.error(`[server:${serverId}] failed to load sidebar icon data`, err);
    }
  }

  // Lightweight reload for server config changes (rename, logo, etc.).
  async function reloadServer() {
    if (registeredServer?.reauthRequiredAt != null) return;
    try {
      const serverState = await getAuthenticatedServerState(connectAPIConfig());
      displayName = serverState.name;
      logoUrl = serverState.logoUrl;
    } catch (err) {
      console.warn(`[server:${serverId}] failed to refresh sidebar icon`, err);
    }
  }

  onMount(() => {
    if (stores.isAuthenticated) void loadAll();
  });

  // Subscribe to server events. Use $effect (not onMount) so that if the
  // event bus isn't started yet on first run — possible when this component
  // mounts before the parent layout's startBus effect for this server —
  // the effect re-runs once the bus comes online (getBus is a reactive read
  // on a SvelteMap). Without this, cross-server unread bookkeeping is
  // silently dropped and unread dots never light up for remote servers.
  $effect(() => {
    const registrar = createEventBusHandlerRegistrar(serverId);
    if (!registrar) return;

    const cleanups: (() => void)[] = [];

    cleanups.push(
      registrar.onEvent((serverEvent) => {
        const actorId = serverEvent.actorId;
        const event = serverEvent.event;
        if (!event) return;

        // Reload the icon when server config (name/logo) changes.
        if (roomEventKind(event) === RoomEventKind.ServerUpdated) {
          reloadServer();
        }

        // Root message in any room on this server → mark that room
        // unread (unless the viewer authored it or is currently in it).
        if (isMessagePostedEvent(event)) {
          if (event.threadRootEventId) return; // root messages only
          const eventRoomId = event.roomId;
          const isFromSelf = actorId === currentUserId;

          // The viewer is "in" a room when the URL matches AND they're
          // actually present (window focused + tab visible). A URL-only
          // match while the tab is hidden should still mark the room as
          // unread so the dot lights up when they return.
          const isViewingRoom =
            page.params.serverId === serverSegment &&
            page.params.roomId === eventRoomId &&
            appState.isPresent;

          if (!isFromSelf && !isViewingRoom && !notificationLevelStore.isRoomMuted(eventRoomId)) {
            roomUnreadStore.setRoomUnread(eventRoomId, true);
          }
        }
      })
    );

    cleanups.push(
      registrar.onRoomMarkedAsRead(({ roomId }) => {
        roomUnreadStore.setRoomUnread(roomId, false);
      })
    );

    cleanups.push(
      registrar.onNotificationLevelChanged(({ roomId, level, effectiveLevel }) => {
        if (roomId) {
          notificationLevelStore.setRoomPreference(roomId, level, effectiveLevel);
          if (notificationLevelStore.isRoomMuted(roomId)) {
            roomUnreadStore.setRoomUnread(roomId, false);
          }
        } else {
          notificationLevelStore.setServerPreference(level, effectiveLevel);
          if (notificationLevelStore.isServerMuted()) {
            roomUnreadStore.setServerHasUnread(false);
          }
        }
      })
    );

    return () => {
      for (const cleanup of cleanups) cleanup();
    };
  });

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
      const unreadSnapshotRevision = roomUnreadStore.captureSnapshotRevision();
      const rooms = await roomDirectoryAPI().listRooms(RoomDirectoryScope.CHANNELS);
      roomUnreadStore.updateRooms(rooms, unreadSnapshotRevision);
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
  title={iconTitle}
  dimmed={iconDimmed}
/>
