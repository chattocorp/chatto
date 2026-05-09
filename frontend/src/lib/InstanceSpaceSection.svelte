<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { instanceIdToSegment } from '$lib/navigation';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { graphqlClientManager } from '$lib/state/instance/graphqlClient.svelte';
  import type { ViewerData } from '$lib/state/instance/permissions.svelte';
  import { createInstanceEventBusHandlerRegistrar } from '$lib/instanceEventBus.svelte';
  import { graphql } from './gql';
  import { notificationTarget } from '$lib/state/instance/notifications.svelte';
  import SpaceIcon from './SpaceIcon.svelte';
  import { useTabResumeCallback } from '$lib/hooks';

  const DM_SPACE_ID = 'DM';

  let {
    instanceId,
    currentUserId,
    onPermissionsLoaded
  }: {
    instanceId: string;
    currentUserId?: string;
    /** Callback to update instance permissions when the combined query completes (home instance only). */
    onPermissionsLoaded?: (viewer: ViewerData) => void;
  } = $props();

  const instanceSegment = $derived(instanceIdToSegment(instanceId));

  // Get this instance's stores
  // eslint-disable-next-line svelte/no-unused-svelte-ignore -- Svelte compiler warning, not ESLint
  // svelte-ignore state_referenced_locally - instanceId is stable per component lifetime (keyed by instance.id)
  const stores = instanceRegistry.getStore(instanceId);
  const notificationStore = stores.notifications;
  const roomUnreadStore = stores.roomUnread;
  const notificationLevelStore = stores.notificationLevels;

  // After the URL collapse (ADR-027), "this instance is active" simply means
  // the URL's instance segment matches this one — and since each instance
  // is now a single deployment-wide server, that's the active context.
  const isActiveInstance = $derived(page.params.instanceId === instanceSegment);
  const primarySpaceId = $derived(stores.instance.primarySpaceId);
  const activeSpaceId = $derived(isActiveInstance ? primarySpaceId : undefined);

  let displayName = $state('');
  let logoUrl = $state<string | null>(null);
  let loaded = $state(false);

  // Single dispatcher for icon clicks — kind comes from spaceIndicator()
  // so the two paths can't drift out of sync with what was rendered.
  function handleSpaceIndicatorClick(spaceId: string, kind: 'notification' | 'unread') {
    if (kind === 'notification') return handleSpaceNotificationClick(spaceId);
    return handleSpaceUnreadClick(spaceId);
  }

  // Get the GraphQL client for this instance
  function getClient() {
    return graphqlClientManager.getClient(instanceId).client;
  }

  // Single combined query for instance icon, unread status, notification prefs, and viewer permissions.
  const InstanceInitQuery = graphql(`
    query InstanceInit {
      instance {
        primarySpaceId
        config {
          instanceName
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
      me {
        roomNotificationPreferences {
          spaceId
          roomId
          level
          effectiveLevel
        }
      }
      viewer {
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

    const { instance, me, viewer } = initResult.data;

    // Store viewer permissions on the per-instance store (all instances)
    if (viewer) {
      stores.setPermissions(viewer);
      // Also update the context-based permissions (origin instance, for layouts)
      if (onPermissionsLoaded) {
        onPermissionsLoaded(viewer);
      }
    }

    if (me) {
      // Populate room-level notification preferences first.
      for (const pref of me.roomNotificationPreferences) {
        notificationLevelStore.setRoomPreference(
          pref.spaceId,
          pref.roomId,
          pref.level,
          pref.effectiveLevel
        );
      }
    }

    if (instance && instance.primarySpaceId) {
      const spaceId = instance.primarySpaceId;
      // Populate instance-level notification preference and unread state.
      const pref = instance.viewerNotificationPreference;
      if (pref) {
        notificationLevelStore.setSpacePreference(spaceId, pref.level, pref.effectiveLevel);
      }
      roomUnreadStore.clear();
      roomUnreadStore.setSpaceHasUnread(spaceId, instance.viewerHasUnreadRooms);

      // Populate DM unread status and notification preferences.
      for (const room of instance.rooms) {
        const roomPref = room.viewerNotificationPreference;
        if (roomPref) {
          notificationLevelStore.setRoomPreference(
            DM_SPACE_ID,
            room.id,
            roomPref.level,
            roomPref.effectiveLevel
          );
        }
      }
      roomUnreadStore.initSpaceRooms(
        DM_SPACE_ID,
        instance.rooms.map((r) => ({ id: r.id, hasUnread: r.hasUnread }))
      );
    }

    if (instance) {
      displayName = instance.config.instanceName;
      logoUrl = instance.config.logoUrl ?? null;
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
            instance {
              config {
                instanceName
                logoUrl(width: 96, height: 96)
              }
            }
          }
        `),
        {}
      )
      .toPromise();

    if (result.data?.instance) {
      displayName = result.data.instance.config.instanceName;
      logoUrl = result.data.instance.config.logoUrl ?? null;
    }
  }

  // Load on mount and tab resume
  useTabResumeCallback(() => loadAll());

  // Subscribe to instance events. Use $effect (not onMount) so that if the
  // event bus isn't started yet on first run — possible when this component
  // mounts before the parent layout's startBus effect for this instance —
  // the effect re-runs once the bus comes online (getBus is a reactive read
  // on a SvelteMap). Without this, e.g. cross-instance NewMessageInSpaceEvent
  // is silently dropped and unread dots never light up for remote spaces.
  $effect(() => {
    const registrar = createInstanceEventBusHandlerRegistrar(instanceId);
    if (!registrar) return;

    const cleanups: (() => void)[] = [];

    cleanups.push(
      registrar.onInstanceEvent((instanceEvent) => {
        const actorId = instanceEvent.actorId;
        const event = instanceEvent.event;
        if (!event) return;

        // Reload the icon when instance config (name/logo) changes.
        if (event.__typename === 'SpaceUpdatedEvent') {
          reloadInstance();
        }

        // New message in space - mark that specific room as unread
        if (event.__typename === 'NewMessageInSpaceEvent') {
          const eventSpaceId = event.spaceId;
          const eventRoomId = event.roomId;
          const isFromSelf = actorId === currentUserId;

          // Per ADR-027 the URL no longer carries spaceId, and DM rooms now
          // share the channel URL shape (#330 phase 3) — the viewer is "in"
          // a room when the URL's roomId matches and they're on this
          // instance's segment.
          const isViewingRoom =
            page.params.instanceId === instanceSegment &&
            page.params.roomId === eventRoomId;

          if (
            !isFromSelf &&
            !isViewingRoom &&
            !notificationLevelStore.isRoomMuted(eventSpaceId, eventRoomId)
          ) {
            roomUnreadStore.setRoomUnread(eventSpaceId, eventRoomId, true);
          }
        }
      })
    );

    cleanups.push(
      registrar.onRoomMarkedAsRead(({ spaceId, roomId }) => {
        roomUnreadStore.setRoomUnread(spaceId, roomId, false);
      })
    );

    cleanups.push(
      registrar.onNotificationLevelChanged(({ spaceId, roomId, level, effectiveLevel }) => {
        if (roomId) {
          notificationLevelStore.setRoomPreference(spaceId, roomId, level, effectiveLevel);
          if (notificationLevelStore.isRoomMuted(spaceId, roomId)) {
            roomUnreadStore.setRoomUnread(spaceId, roomId, false);
          }
        } else {
          notificationLevelStore.setSpacePreference(spaceId, level, effectiveLevel);
          if (notificationLevelStore.isSpaceMuted(spaceId)) {
            roomUnreadStore.setSpaceHasUnread(spaceId, false);
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
  async function handleSpaceNotificationClick(spaceId: string) {
    const notification =
      notificationStore.getSpaceNotification(spaceId) ?? notificationStore.getDMNotification();
    if (!notification) return;

    const target = notificationTarget(notification);
    if (target.eventId && target.spaceId && target.roomId) {
      stores.pendingHighlights.set(target.spaceId, target.roomId, target.threadRootId, target.eventId);
    }
    void notificationStore.dismiss(notification.id);

    const path = notificationStore.getCleanPath(instanceId, notification);
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- path from getCleanPath() is already resolved
    await goto(path);
  }

  // Query to fetch rooms with unread status on demand (sentinel-only spaces).
  const FirstUnreadRoomQuery = graphql(`
    query FirstUnreadRoom {
      instance {
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
  async function handleSpaceUnreadClick(spaceId: string) {
    let roomId = roomUnreadStore.getFirstUnreadRoomId(spaceId);

    if (!roomId) {
      const client = getClient();
      const result = await client.query(FirstUnreadRoomQuery, {}).toPromise();

      const rooms = result.data?.instance?.rooms;
      if (rooms) {
        roomUnreadStore.initSpaceRooms(
          spaceId,
          rooms.map((r) => ({ id: r.id, hasUnread: r.hasUnread }))
        );
        roomId = rooms.find((r) => r.hasUnread)?.id ?? null;
      }
    }

    if (!roomId) {
      roomId = roomUnreadStore.getFirstUnreadRoomId(DM_SPACE_ID);
    }

    if (roomId) {
      await goto(resolve('/chat/[instanceId]/(chrome)/[roomId]', { instanceId: instanceSegment, roomId }));
    } else {
      await goto(resolve('/chat/[instanceId]', { instanceId: instanceSegment }));
    }
  }
</script>

<!-- One icon per instance (server = instance post-#330). -->
{#if loaded && primarySpaceId}
  <SpaceIcon
    space={{ name: displayName, logoUrl }}
    href={resolve('/chat/[instanceId]', { instanceId: instanceSegment })}
    selected={primarySpaceId === activeSpaceId}
    indicator={stores.spaceIndicator(primarySpaceId)}
    onIndicatorClick={(kind) => handleSpaceIndicatorClick(primarySpaceId, kind)}
  />
{/if}
