<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import { createEventBusHandlerRegistrar } from '$lib/eventBus.svelte';
  import { notificationTarget } from '$lib/state/server/notifications.svelte';
  import { notificationLevelFromWire } from '$lib/state/server/notificationLevelWire';
  import type { ViewerData } from '$lib/state/server/permissions.svelte';
  import { wireEventBusManager } from '$lib/state/server/wireEventBus.svelte';
  import type { ViewerPermissionsView } from '$lib/pb/chatto/api/v1/chat_pb';
  import { appState } from '$lib/state/globals.svelte';
  import ServerIcon from './ServerIcon.svelte';
  import { useTabResumeCallback } from '$lib/hooks';

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
  // eslint-disable-next-line svelte/no-unused-svelte-ignore -- Svelte compiler warning, not ESLint
  // svelte-ignore state_referenced_locally - serverId is stable per component lifetime (keyed by server.id)
  const serverConnection = serverConnectionManager.getClient(serverId);
  const registeredServer = $derived(serverRegistry.getServer(serverId));

  // After the URL collapse (ADR-027), the active context is the deployment-wide
  // server named in the current URL segment.
  const isActiveServer = $derived(page.params.serverId === serverSegment);

  let displayName = $state('');
  let logoUrl = $state<string | null>(null);
  let loaded = $state(false);

  const iconServer = $derived.by(() => {
    const refreshedName = stores.serverInfo.name !== 'Chatto' ? stores.serverInfo.name : undefined;
    return {
      name: displayName || refreshedName || registeredServer?.name || stores.serverInfo.name,
      logoUrl: loaded ? logoUrl : (stores.serverInfo.iconUrl ?? registeredServer?.iconUrl)
    };
  });
  const iconDimmed = $derived(!loaded || serverConnection.showConnectionLostIcon);
  const iconTitle = $derived(
    iconDimmed ? `${iconServer.name} (connection unavailable)` : iconServer.name
  );

  // Single dispatcher for icon clicks — kind comes from serverIndicator()
  // so the two paths can't drift out of sync with what was rendered.
  function handleServerIndicatorClick(kind: 'notification' | 'unread') {
    if (kind === 'notification') return handleServerNotificationClick();
    return handleServerUnreadClick();
  }

  async function loadAll() {
    try {
      await Promise.all([loadViewerMetadata(), stores.rooms.refresh(), notificationStore.fetch()]);
    } catch (err) {
      console.error(`[server:${serverId}] failed to load sidebar data`, err);
    }
  }

  // Lightweight reload for server config changes (rename, logo, etc.).
  async function reloadServer() {
    try {
      await loadViewerMetadata();
    } catch (err) {
      console.error(`[server:${serverId}] failed to refresh sidebar server profile`, err);
    }
  }

  async function loadViewerMetadata() {
    const client = wireEventBusManager.getClient(serverId);
    if (!client) {
      throw new Error('wire client is not ready');
    }

    const response = await client.getViewer();
    const viewer = response.viewer;
    if (viewer?.permissions) {
      stores.setPermissions(viewerPermissionsFromWire(viewer.permissions));
    }
    const serverPref = viewer?.serverNotificationPreference;
    if (serverPref) {
      notificationLevelStore.setServerPreference(
        notificationLevelFromWire(serverPref.level),
        notificationLevelFromWire(serverPref.effectiveLevel)
      );
    }
    for (const pref of viewer?.roomNotificationPreferences ?? []) {
      notificationLevelStore.setRoomPreference(
        pref.roomId,
        notificationLevelFromWire(pref.level),
        notificationLevelFromWire(pref.effectiveLevel)
      );
    }

    if (response.serverProfile) {
      displayName = response.serverProfile.name;
      logoUrl = response.serverProfile.logoUrl || null;
      stores.serverInfo.name = response.serverProfile.name;
      stores.serverInfo.iconUrl = response.serverProfile.logoUrl || null;
      loaded = true;
    }
  }

  function viewerPermissionsFromWire(permissions: ViewerPermissionsView): ViewerData {
    return {
      canViewAdmin: permissions.canViewAdmin,
      canStartDMs: permissions.canStartDms,
      canAdminViewUsers: permissions.canAdminViewUsers,
      canAdminManageUsers: permissions.canAdminManageUsers,
      canAdminViewRoles: permissions.canAdminViewRoles,
      canAdminManageRoles: permissions.canAdminManageRoles,
      canAdminViewSystem: permissions.canAdminViewSystem,
      canAdminViewAudit: permissions.canAdminViewAudit
    };
  }

  // Load on mount and tab resume
  useTabResumeCallback(() => void loadAll());

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
        if (event.__typename === 'ServerUpdatedEvent') {
          reloadServer();
        }

        // Root message in any room on this server → mark that room
        // unread (unless the viewer authored it or is currently in it).
        if (event.__typename === 'MessagePostedEvent') {
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

          if (
            !isFromSelf &&
            !isViewingRoom &&
            !notificationLevelStore.isRoomMuted(eventRoomId)
          ) {
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

  // Handle click on icon notification dot. The icon's notification can come
  // from either a channel mention/reply or a DM message. Prefer channel
  // notifications when both are present.
  async function handleServerNotificationClick() {
    const notification =
      notificationStore.getSpaceNotification() ?? notificationStore.getDMNotification();
    if (!notification) return;

    const target = notificationTarget(notification);
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
      await stores.rooms.refresh();
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
  onIndicatorClick={handleServerIndicatorClick}
  title={iconTitle}
  dimmed={iconDimmed}
/>
