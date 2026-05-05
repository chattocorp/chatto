<script lang="ts">
  import { createSpaceEventBus, startSpaceSubscription } from '$lib/spaceEventBus.svelte';
  import { usePresenceChange, useReconnectCallback, useSpaceEvent } from '$lib/hooks';
  import { useConnection } from '$lib/state/instance/connection.svelte';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { getInstancePermissions } from '$lib/state/instance/permissions.svelte';
  import { getPresenceCache } from '$lib/state/presenceCache.svelte';
  import { SpaceRoomsStore, setSpaceRoomsStore } from '$lib/state/space';
  import { DM_SPACE_ID } from '$lib/constants';
  import { untrack, type Snippet } from 'svelte';

  let { spaceId, children }: { spaceId: string; children: Snippet } = $props();

  // Create event bus context synchronously
  const spaceEventBus = createSpaceEventBus();

  // Capture presence cache during init (context must be read synchronously)
  const presenceCache = getPresenceCache();

  const connection = useConnection();
  const stores = instanceRegistry.getStore(getActiveInstance()());
  const instancePerms = getInstancePermissions();

  // One SpaceRoomsStore per <SpaceEventProvider>: the parent layout's
  // {#key spaceId} wraps this component, so the initial spaceId is the
  // only value this instance will ever see. Sidebar and pages share this
  // single source of truth.
  const spaceRoomsStore = new SpaceRoomsStore(
    connection().client,
    untrack(() => spaceId),
    stores.notificationLevels,
    stores.roomUnread
  );
  setSpaceRoomsStore(spaceRoomsStore);

  // Start space event subscriptions (messages, room events, reactions, presence).
  // The primary space carries channels and the hidden DM space carries DM rooms
  // (#330 phase 3). Both feed into the same bus so RoomEventsPane / RoomList /
  // SpaceRoomsStore see events regardless of which underlying space they come
  // from. Skip the DM subscription if this provider is itself rooted at the DM
  // space (avoids double-subscribing) or if the viewer has no dm.view (the
  // backend would reject the subscription, looping the WebSocket on retries
  // and never letting the page settle).
  // Explicitly track reconnectCount so the subscriptions restart after WebSocket
  // reconnections — don't rely solely on graphql-ws to re-subscribe, which can
  // silently fail if the subscription was in an intermediate state during the drop.
  $effect(() => {
    const conn = connection();
    void conn.reconnectCount;
    const canDM = instancePerms.current.loaded
      ? instancePerms.current.canViewDMs
      : true; // optimistic until permissions load
    const cleanups: (() => void)[] = [];
    cleanups.push(startSpaceSubscription(spaceEventBus, conn.client, spaceId));
    if (spaceId !== DM_SPACE_ID && canDM) {
      cleanups.push(startSpaceSubscription(spaceEventBus, conn.client, DM_SPACE_ID));
    }
    return () => cleanups.forEach((c) => c());
  });

  // Clear presence cache after WebSocket reconnection
  useReconnectCallback(() => {
    console.log('WebSocket reconnected, clearing presence cache');
    presenceCache.clear();
  });

  // Populate global presence cache from space events so that any UserAvatar
  // (including newly-mounted ones like popovers) sees the latest presence.
  usePresenceChange((userId, status) => {
    presenceCache.update(userId, status);
  });

  // Forward space events to the rooms store (refreshes on membership / room
  // metadata changes). Done here once instead of in every consumer.
  useSpaceEvent((event) => spaceRoomsStore.ingestSpaceEvent(event));
</script>

<div data-testid="space-subscription-active" class="hidden"></div>
{@render children()}
