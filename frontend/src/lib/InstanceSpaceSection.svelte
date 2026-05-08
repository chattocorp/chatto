<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { instanceIdToSegment } from '$lib/navigation';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { graphqlClientManager } from '$lib/state/instance/graphqlClient.svelte';
  import type { ViewerData } from '$lib/state/instance/permissions.svelte';
  import { createInstanceEventBusHandlerRegistrar } from '$lib/instanceEventBus.svelte';
  import { graphql, useFragment } from './gql';
  import { SpaceIconSpaceFragmentDoc, type SpaceIconSpaceFragment } from './gql/graphql';
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
  // exposes a single user-facing primary space, that space is the active one.
  const isActiveInstance = $derived(page.params.instanceId === instanceSegment);
  const activeSpaceId = $derived(isActiveInstance ? stores.instance.primarySpaceId : undefined);

  let spaces = $state(new Array<SpaceIconSpaceFragment>());

  // Single dispatcher for space-icon clicks — kind comes from spaceIndicator()
  // so the two paths can't drift out of sync with what was rendered.
  function handleSpaceIndicatorClick(spaceId: string, kind: 'notification' | 'unread') {
    if (kind === 'notification') return handleSpaceNotificationClick(spaceId);
    return handleSpaceUnreadClick(spaceId);
  }

  // Get the GraphQL client for this instance
  function getClient() {
    return graphqlClientManager.getClient(instanceId).client;
  }

  // Single combined query for space list, unread status, notification prefs, and viewer permissions
  const SpaceListInitQuery = graphql(`
    query SpaceListInit {
      me {
        spaces {
          ...SpaceIconSpace
          viewerHasUnreadRooms
          viewerNotificationPreference {
            level
            effectiveLevel
          }
        }
        roomNotificationPreferences {
          spaceId
          roomId
          level
          effectiveLevel
        }
      }
      dmSpace: space(id: "DM") {
        rooms {
          id
          hasUnread
          viewerNotificationPreference {
            level
            effectiveLevel
          }
        }
      }
      viewer {
        canViewAdmin
        canListSpaces
        canViewDMs
        canWriteDMs
        canAdminViewUsers
        canAdminManageUsers
        canAdminViewSpaces
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
      client.query(SpaceListInitQuery, {}).toPromise(),
      notificationStore.fetch()
    ]);

    if (!initResult.data) return;

    const { me, dmSpace, viewer } = initResult.data;

    // Store viewer permissions on the per-instance store (all instances)
    if (viewer) {
      stores.setPermissions(viewer);
      // Also update the context-based permissions (origin instance, for layouts)
      if (onPermissionsLoaded) {
        onPermissionsLoaded(viewer);
      }
    }

    if (me) {
      // Populate room-level notification preferences first
      for (const pref of me.roomNotificationPreferences) {
        notificationLevelStore.setRoomPreference(
          pref.spaceId,
          pref.roomId,
          pref.level,
          pref.effectiveLevel
        );
      }

      // Populate space-level notification preferences and unread state
      roomUnreadStore.clear();
      for (const space of me.spaces) {
        const spaceData = useFragment(SpaceIconSpaceFragmentDoc, space);
        const pref = space.viewerNotificationPreference;
        if (pref) {
          notificationLevelStore.setSpacePreference(spaceData.id, pref.level, pref.effectiveLevel);
        }
        roomUnreadStore.setSpaceHasUnread(spaceData.id, space.viewerHasUnreadRooms);
      }

      // Set spaces for sidebar icons
      spaces = me.spaces.map((s) => useFragment(SpaceIconSpaceFragmentDoc, s));
    }

    // Populate DM unread status and notification preferences
    if (dmSpace) {
      for (const room of dmSpace.rooms) {
        const pref = room.viewerNotificationPreference;
        if (pref) {
          notificationLevelStore.setRoomPreference(
            DM_SPACE_ID,
            room.id,
            pref.level,
            pref.effectiveLevel
          );
        }
      }
      roomUnreadStore.initSpaceRooms(
        DM_SPACE_ID,
        dmSpace.rooms.map((r) => ({ id: r.id, hasUnread: r.hasUnread }))
      );
    }
  }

  // Lightweight reload for membership changes (space join/leave/update)
  async function reloadSpaces() {
    const client = getClient();
    const result = await client
      .query(
        graphql(`
          query GetAllSpaces {
            me {
              spaces {
                ...SpaceIconSpace
              }
            }
          }
        `),
        {}
      )
      .toPromise();

    if (result.data) {
      spaces =
        result.data.me?.spaces.map((s) =>
          useFragment(SpaceIconSpaceFragmentDoc, s)
        ) || [];
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

    // Subscribe to instance events for space membership changes and new messages
    cleanups.push(
      registrar.onInstanceEvent((instanceEvent) => {
        const actorId = instanceEvent.actorId;
        const event = instanceEvent.event;
        if (!event) return;

        // Reload spaces when membership changes or a space is updated
        if (
          event.__typename === 'UserJoinedSpaceEvent' ||
          event.__typename === 'UserLeftSpaceEvent' ||
          event.__typename === 'SpaceUpdatedEvent'
        ) {
          reloadSpaces();
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

    // Handle room marked as read events (multi-tab/multi-device sync)
    cleanups.push(
      registrar.onRoomMarkedAsRead(({ spaceId, roomId }) => {
        roomUnreadStore.setRoomUnread(spaceId, roomId, false);
      })
    );

    // Handle notification level changes (multi-tab sync)
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

  // Handle click on space notification dot. For the primary space the
  // indicator can be sourced from EITHER a channel mention/reply (which
  // notificationStore.getSpaceNotification surfaces) OR a DM message
  // (DM notifications have no spaceId, so they need a separate accessor).
  // Prefer channel notifications when both are present.
  async function handleSpaceNotificationClick(spaceId: string) {
    const isPrimary = spaceId === stores.instance.primarySpaceId;
    const notification =
      notificationStore.getSpaceNotification(spaceId) ??
      (isPrimary ? notificationStore.getDMNotification() : undefined);
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

  // Query to fetch rooms with unread status on demand (for sentinel-only spaces)
  const FirstUnreadRoomQuery = graphql(`
    query FirstUnreadRoom($spaceId: ID!) {
      space(id: $spaceId) {
        rooms {
          id
          hasUnread
        }
      }
    }
  `);

  // Handle click on space unread dot. The primary space surfaces both
  // channel and DM unreads (#330 phase 3) — fall back to the DM-space
  // unread map if no channel unread is found, so the icon's behaviour
  // matches what its dot is reporting.
  async function handleSpaceUnreadClick(spaceId: string) {
    const isPrimary = spaceId === stores.instance.primarySpaceId;
    let roomId = roomUnreadStore.getFirstUnreadRoomId(spaceId);

    if (!roomId) {
      const client = getClient();
      const result = await client.query(FirstUnreadRoomQuery, { spaceId }).toPromise();

      const rooms = result.data?.space?.rooms;
      if (rooms) {
        roomUnreadStore.initSpaceRooms(
          spaceId,
          rooms.map((r) => ({ id: r.id, hasUnread: r.hasUnread }))
        );
        roomId = rooms.find((r) => r.hasUnread)?.id ?? null;
      }
    }

    if (!roomId && isPrimary) {
      roomId = roomUnreadStore.getFirstUnreadRoomId(DM_SPACE_ID);
    }

    if (roomId) {
      await goto(resolve('/chat/[instanceId]/(chrome)/[roomId]', { instanceId: instanceSegment, roomId }));
    } else {
      await goto(resolve('/chat/[instanceId]', { instanceId: instanceSegment }));
    }
  }
</script>

<!-- Space icons for this instance -->
{#each spaces as space (space.id)}
  <SpaceIcon
    {space}
    href={resolve('/chat/[instanceId]', { instanceId: instanceSegment })}
    selected={space.id === activeSpaceId}
    indicator={stores.spaceIndicator(space.id)}
    onIndicatorClick={(kind) => handleSpaceIndicatorClick(space.id, kind)}
  />
{/each}
