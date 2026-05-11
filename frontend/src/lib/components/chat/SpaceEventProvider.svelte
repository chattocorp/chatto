<script lang="ts">
  import { provideServerEventBus } from '$lib/serverEventBus.svelte';
  import {
    usePresenceChange,
    useReconnectCallback,
    useRoomLayoutUpdated,
    useServerEvent
  } from '$lib/hooks';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getPresenceCache } from '$lib/state/presenceCache.svelte';
  import { SpaceRoomsStore, setSpaceRoomsStore } from '$lib/state/space';
  import type { Snippet } from 'svelte';

  let { children }: { children: Snippet } = $props();

  // The myServerEvents subscription was started by the registry when this
  // server got connected; here we just expose its bus via Svelte context so
  // descendant components can register handlers without going through the
  // manager directly.
  const getServerId = getActiveServer();
  provideServerEventBus(getServerId());

  // Capture presence cache during init (context must be read synchronously)
  const presenceCache = getPresenceCache();

  const connection = useConnection();
  const stores = serverRegistry.getStore(getServerId());

  // One SpaceRoomsStore per <SpaceEventProvider>: post-PR(b) the API has
  // a single server, so the store no longer carries a spaceId — the
  // sidebar and chat pages share this single source of truth for the
  // user's joined-room set.
  const spaceRoomsStore = new SpaceRoomsStore(
    connection().client,
    stores.notificationLevels,
    stores.roomUnread
  );
  setSpaceRoomsStore(spaceRoomsStore);

  // Clear presence cache after WebSocket reconnection
  useReconnectCallback(() => {
    console.log('WebSocket reconnected, clearing presence cache');
    presenceCache.clear();
  });

  // Populate global presence cache from server events so that any UserAvatar
  // (including newly-mounted ones like popovers) sees the latest presence.
  usePresenceChange((userId, status) => {
    presenceCache.update(userId, status);
  });

  // Forward room-scoped events to the rooms store (refreshes on membership
  // / room metadata changes). Done here once instead of in every consumer.
  useServerEvent((event) => spaceRoomsStore.ingestServerEvent(event));

  // Refetch on RoomLayoutUpdatedEvent regardless of which UI surface is
  // mounted — the admin saving from /server-admin/rooms used to miss this
  // event because RoomList (the only listener) was unmounted while the
  // chrome sidebar showed the admin nav. Wiring it here guarantees a
  // refresh as long as we're inside the chat tree.
  useRoomLayoutUpdated(() => {
    void spaceRoomsStore.refresh();
  });
</script>

<div data-testid="space-subscription-active" class="hidden"></div>
{@render children()}
