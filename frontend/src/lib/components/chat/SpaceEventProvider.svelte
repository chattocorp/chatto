<script lang="ts">
  import { createSpaceEventBus, startSpaceSubscription } from '$lib/spaceEventBus.svelte';
  import { usePresenceChange, useReconnectCallback, useSpaceEvent } from '$lib/hooks';
  import { useConnection } from '$lib/state/instance/connection.svelte';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { getPresenceCache } from '$lib/state/presenceCache.svelte';
  import { SpaceRoomsStore, setSpaceRoomsStore } from '$lib/state/space';
  import { untrack, type Snippet } from 'svelte';

  let { spaceId, children }: { spaceId: string; children: Snippet } = $props();

  // Create event bus context synchronously
  const spaceEventBus = createSpaceEventBus();

  // Capture presence cache during init (context must be read synchronously)
  const presenceCache = getPresenceCache();

  const connection = useConnection();
  const stores = instanceRegistry.getStore(getActiveInstance()());

  // One SpaceRoomsStore per <SpaceEventProvider>: the parent layout's
  // {#key data.spaceId} wraps this component, so the initial spaceId is the
  // only value this instance will ever see. Sidebar and pages share this
  // single source of truth.
  const spaceRoomsStore = new SpaceRoomsStore(
    connection().client,
    untrack(() => spaceId),
    stores.notificationLevels,
    stores.roomUnread
  );
  setSpaceRoomsStore(spaceRoomsStore);

  // Start space event subscription (messages, room events, reactions, presence).
  // Explicitly track reconnectCount so the subscription restarts after WebSocket
  // reconnections — don't rely solely on graphql-ws to re-subscribe, which can
  // silently fail if the subscription was in an intermediate state during the drop.
  $effect(() => {
    const conn = connection();
    void conn.reconnectCount;
    return startSpaceSubscription(spaceEventBus, conn.client, spaceId);
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
