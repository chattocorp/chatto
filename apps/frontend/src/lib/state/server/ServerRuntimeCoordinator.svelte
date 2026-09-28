<script lang="ts">
  import { untrack } from 'svelte';
  import { page } from '$app/state';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { eventBusManager } from './realtimeTransport.svelte';
  import { serverRegistry } from './registry.svelte';
  import { serverConnectionManager } from './serverConnection.svelte';
  import { startServerRecovery } from './serverRecovery';

  $effect(() => untrack(() => startServerRecovery(serverRegistry)));

  function realtimeRegistrations() {
    return serverRegistry.servers.flatMap((server) => {
      const store = serverRegistry.tryGetStore(server.id);
      return store?.isAuthenticated
        ? [
            {
              serverId: server.id,
              connection: serverConnectionManager.getClient(server.id),
              projectionSupported: store.serverInfo.isSupportedVersion,
              sync: store.realtimeSync,
              projectionHandler: store.realtimeProjectionHandler,
              completeProjectionCatchUp: (cursor: string) => store.completeRealtimeCatchUp(cursor),
              waitForProjectionReconciliation: () => store.waitForRealtimeReconciliation()
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
  const activeServerId = $derived(page.route.id?.startsWith('/chat') ? getActiveServer() : '');

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

  // Remote session termination is authoritative even when its server is not
  // the active route. Sign out that server. Reading each bus here subscribes
  // again when a server's bus starts after this effect first ran.
  $effect(() => {
    const remoteBuses = serverRegistry.servers
      .filter((server) => !serverRegistry.isOriginServer(server.id))
      .map((server) => ({ id: server.id, bus: eventBusManager.getBus(server.id) }));
    return untrack(() => {
      const disposers = remoteBuses.map(({ id, bus }) =>
        bus?.onSessionTerminated(() => {
          queueMicrotask(() => serverRegistry.clearServerAuthentication(id));
        })
      );
      return () => disposers.forEach((dispose) => dispose?.());
    });
  });
</script>
