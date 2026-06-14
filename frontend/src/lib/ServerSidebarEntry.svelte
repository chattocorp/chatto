<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { graphqlClientManager } from '$lib/state/server/graphqlClient.svelte';
  import { createEventBusHandlerRegistrar } from '$lib/eventBus.svelte';
  import { graphql } from './gql';
  import { notificationTarget } from '$lib/state/server/notifications.svelte';
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
  const gqlClient = graphqlClientManager.getClient(serverId);
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
  const iconDimmed = $derived(!loaded || gqlClient.showConnectionLostIcon);
  const iconTitle = $derived(
    iconDimmed ? `${iconServer.name} (connection unavailable)` : iconServer.name
  );

  // Single dispatcher for icon clicks — kind comes from serverIndicator()
  // so the two paths can't drift out of sync with what was rendered.
  function handleServerIndicatorClick(kind: 'notification' | 'unread') {
    if (kind === 'notification') return handleServerNotificationClick();
    return handleServerUnreadClick();
  }

  // Get the GraphQL client for this server
  function getClient() {
    return gqlClient.client;
  }

  // Single combined query for server icon, unread status, notification prefs, and viewer permissions.
  const ServerSidebarEntryInitQuery = graphql(`
    query ServerSidebarEntryInit {
      server {
        profile {
          name
          logoUrl
        }
        viewerHasUnreadRooms
        viewerNotificationPreference {
          level
          effectiveLevel
        }
        rooms(type: DM) {
          id
          hasUnread
          viewerNotificationPreference {
            level
            effectiveLevel
          }
        }
      }
      viewer {
        user {
          roomNotificationPreferences {
            roomId
            level
            effectiveLevel
          }
        }
        canViewAdmin
        canStartDMs
        canAdminViewUsers
        canAdminManageUsers
        canAdminViewRoles
        canAdminManageRoles
        canAdminViewSystem
        canAdminViewAudit
      }
    }
  `);

  async function loadAll() {
    try {
      const client = getClient();

      const [initResult] = await Promise.all([
        client.query(ServerSidebarEntryInitQuery, {}).toPromise(),
        notificationStore.fetch()
      ]);

      if (initResult.error) {
        console.error(`[server:${serverId}] failed to load sidebar icon data`, initResult.error);
        return;
      }

      if (!initResult.data) return;

      const { server, viewer } = initResult.data;

      if (viewer) {
        stores.setPermissions(viewer);
        // Populate room-level notification preferences first.
        for (const pref of viewer.user.roomNotificationPreferences) {
          notificationLevelStore.setRoomPreference(pref.roomId, pref.level, pref.effectiveLevel);
        }
      }

      if (server) {
        // Populate server-level notification preference and unread state.
        const pref = server.viewerNotificationPreference;
        if (pref) {
          notificationLevelStore.setServerPreference(pref.level, pref.effectiveLevel);
        }
        roomUnreadStore.clear();
        roomUnreadStore.setServerHasUnread(server.viewerHasUnreadRooms);

        // Populate DM unread status and notification preferences. Channel
        // and DM rooms now share the same per-room unread map.
        for (const room of server.rooms) {
          const roomPref = room.viewerNotificationPreference;
          if (roomPref) {
            notificationLevelStore.setRoomPreference(room.id, roomPref.level, roomPref.effectiveLevel);
          }
          if (room.hasUnread) {
            roomUnreadStore.setRoomUnread(room.id, true);
          }
        }
      }

      if (server) {
        displayName = server.profile.name;
        logoUrl = server.profile.logoUrl ?? null;
        loaded = true;
      }
    } catch (err) {
      console.error(`[server:${serverId}] failed to load sidebar icon data`, err);
    }
  }

  // Lightweight reload for server config changes (rename, logo, etc.).
  async function reloadServer() {
    const client = getClient();
    const result = await client
      .query(
        graphql(`
          query ServerSidebarEntryIconRefresh {
            server {
              profile {
                name
                logoUrl
              }
            }
          }
        `),
        {}
      )
      .toPromise();

    if (result.data?.server) {
      displayName = result.data.server.profile.name;
      logoUrl = result.data.server.profile.logoUrl ?? null;
    }
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

  // Query to fetch rooms with unread status on demand (sentinel-only server flag).
  const FirstUnreadRoomQuery = graphql(`
    query FirstUnreadRoom {
      server {
        rooms(type: CHANNEL) {
          id
          hasUnread
        }
      }
    }
  `);

  // Handle click on icon unread dot. Channel and DM unreads both flow through
  // this server icon.
  async function handleServerUnreadClick() {
    let roomId = roomUnreadStore.getFirstUnreadRoomId();

    if (!roomId) {
      const client = getClient();
      const result = await client.query(FirstUnreadRoomQuery, {}).toPromise();

      const rooms = result.data?.server?.rooms;
      if (rooms) {
        roomUnreadStore.initRooms(
          rooms.map((r: { id: string; hasUnread: boolean }) => ({ id: r.id, hasUnread: r.hasUnread }))
        );
        roomId = rooms.find((r: { hasUnread: boolean }) => r.hasUnread)?.id ?? null;
      }
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
