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
  import SpaceIcon from './SpaceIcon.svelte';
  import { useTabResumeCallback } from '$lib/hooks';

  let {
    serverId,
    currentUserId
  }: {
    serverId: string;
    currentUserId?: string;
  } = $props();

  const instanceSegment = $derived(serverIdToSegment(serverId));

  // Get this instance's stores
  // eslint-disable-next-line svelte/no-unused-svelte-ignore -- Svelte compiler warning, not ESLint
  // svelte-ignore state_referenced_locally - serverId is stable per component lifetime (keyed by instance.id)
  const stores = serverRegistry.getStore(serverId);
  const notificationStore = stores.notifications;
  const roomUnreadStore = stores.roomUnread;
  const notificationLevelStore = stores.notificationLevels;

  // After the URL collapse (ADR-027), "this instance is active" simply means
  // the URL's instance segment matches this one — and since each instance
  // is now a single deployment-wide server, that's the active context.
  const isActiveInstance = $derived(page.params.serverId === instanceSegment);

  let displayName = $state('');
  let logoUrl = $state<string | null>(null);
  let loaded = $state(false);

  // Single dispatcher for icon clicks — kind comes from spaceIndicator()
  // so the two paths can't drift out of sync with what was rendered.
  function handleSpaceIndicatorClick(kind: 'notification' | 'unread') {
    if (kind === 'notification') return handleSpaceNotificationClick();
    return handleSpaceUnreadClick();
  }

  // Get the GraphQL client for this instance
  function getClient() {
    return graphqlClientManager.getClient(serverId).client;
  }

  // Single combined query for instance icon, unread status, notification prefs, and viewer permissions.
  const InstanceInitQuery = graphql(`
    query InstanceInit {
      server {
        config {
          serverName
          logoUrl(width: 96, height: 96)
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
        canViewDMs
        canWriteDMs
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
    const client = getClient();

    const [initResult] = await Promise.all([
      client.query(InstanceInitQuery, {}).toPromise(),
      notificationStore.fetch()
    ]);

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
      displayName = server.config.serverName;
      logoUrl = server.config.logoUrl ?? null;
      loaded = true;
    }
  }

  // Lightweight reload for instance config changes (rename, logo, etc.).
  async function reloadInstance() {
    const client = getClient();
    const result = await client
      .query(
        graphql(`
          query InstanceIconRefresh {
            server {
              config {
                serverName
                logoUrl(width: 96, height: 96)
              }
            }
          }
        `),
        {}
      )
      .toPromise();

    if (result.data?.server) {
      displayName = result.data.server.config.serverName;
      logoUrl = result.data.server.config.logoUrl ?? null;
    }
  }

  // Load on mount and tab resume
  useTabResumeCallback(() => loadAll());

  // Subscribe to instance events. Use $effect (not onMount) so that if the
  // event bus isn't started yet on first run — possible when this component
  // mounts before the parent layout's startBus effect for this instance —
  // the effect re-runs once the bus comes online (getBus is a reactive read
  // on a SvelteMap). Without this, cross-instance unread bookkeeping is
  // silently dropped and unread dots never light up for remote servers.
  $effect(() => {
    const registrar = createEventBusHandlerRegistrar(serverId);
    if (!registrar) return;

    const cleanups: (() => void)[] = [];

    cleanups.push(
      registrar.onEvent((instanceEvent) => {
        const actorId = instanceEvent.actorId;
        const event = instanceEvent.event;
        if (!event) return;

        // Reload the icon when instance config (name/logo) changes.
        if (event.__typename === 'ServerUpdatedEvent') {
          reloadInstance();
        }

        // Root message in any room on this server → mark that room
        // unread (unless the viewer authored it or is currently in it).
        if (event.__typename === 'MessagePostedEvent') {
          if (event.inThread) return; // root messages only
          const eventRoomId = event.roomId;
          const isFromSelf = actorId === currentUserId;

          // The viewer is "in" a room when the URL's roomId matches and they're
          // on this instance's segment.
          const isViewingRoom =
            page.params.serverId === instanceSegment &&
            page.params.roomId === eventRoomId;

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
  // from either a channel mention/reply (notificationStore.getSpaceNotification)
  // or a DM message (notificationStore.getDMNotification). Prefer channel
  // notifications when both are present.
  async function handleSpaceNotificationClick() {
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

  // Query to fetch rooms with unread status on demand (sentinel-only spaces).
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
  // this instance icon — fall back to DM-space unread map if no channel unread
  // is found.
  async function handleSpaceUnreadClick() {
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
      await goto(resolve('/chat/[serverId]/(chrome)/[roomId]', { serverId: instanceSegment, roomId }));
    } else {
      await goto(resolve('/chat/[serverId]', { serverId: instanceSegment }));
    }
  }
</script>

<!-- One icon per instance (server = instance post-#330). -->
{#if loaded}
  <SpaceIcon
    space={{ name: displayName, logoUrl }}
    href={resolve('/chat/[serverId]', { serverId: instanceSegment })}
    selected={isActiveInstance}
    indicator={stores.spaceIndicator()}
    onIndicatorClick={handleSpaceIndicatorClick}
  />
{/if}
