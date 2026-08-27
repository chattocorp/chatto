<script lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { onDestroy, untrack } from 'svelte';
  import { page } from '$app/state';
  import type { CurrentUser } from '$lib/auth/loadAuth';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { eventBusManager } from './eventBus.svelte';
  import { serverRegistry } from './registry.svelte';
  import { serverConnectionManager } from './serverConnection.svelte';

  let { user }: { user?: CurrentUser | null } = $props();

  // The root layout keys this coordinator by origin viewer identity, so the
  // optional origin viewer is stable for this component lifetime.
  const originUser = untrack(() => user);
  const originServerId = serverRegistry.originServer?.id ?? null;
  const originCurrentUser = originServerId
    ? serverRegistry.getStore(originServerId).currentUser
    : null;

  if (originUser && originCurrentUser) {
    // Install the root-load viewer before creating buses so synchronous
    // consumers always find the origin server's transport registration.
    originCurrentUser.user = {
      ...originUser,
      presenceStatus: PresenceStatus.ONLINE
    };
    originCurrentUser.loading = false;

    onDestroy(() => {
      if (originCurrentUser.user?.id === originUser.id) {
        originCurrentUser.user = undefined;
        originCurrentUser.loading = false;
      }
    });
  }

  function realtimeRegistrations() {
    return serverRegistry.servers.flatMap((server) => {
      const store = serverRegistry.tryGetStore(server.id);
      return store?.isAuthenticated
        ? [
            {
              serverId: server.id,
              connection: serverConnectionManager.getClient(server.id),
              projectionSupported: store.serverInfo.supportsRealtimeProjection,
              sync: store.realtimeSync,
              projectionHandler: store.realtimeProjectionHandler
            }
          ]
        : [];
    });
  }

  // Late session restoration and discovery metadata must both retrigger
  // ownership, including while the app remains on the welcome/login route.
  // The registration carries each store's canonical reducer, allowing the bus
  // to install it before a newly opened socket can deliver its first snapshot.
  const registrations = $derived.by(realtimeRegistrations);
  const activeServerId = $derived(
    page.route.id?.startsWith('/chat') ? getActiveServer() : ''
  );

  $effect(() => {
    const nextRegistrations = registrations;
    const nextActiveServerId = activeServerId;

    // Synchronization mutates connection state. Track only the materialized
    // registration inputs and URL-active server to avoid feedback loops.
    untrack(() => {
      eventBusManager.synchronizeAuthenticatedServers(
        nextRegistrations,
        nextActiveServerId || null
      );
    });
  });
</script>
