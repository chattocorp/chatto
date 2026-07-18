<script lang="ts">
  import { untrack, type Snippet } from 'svelte';
  import type { CurrentUser } from '$lib/auth/loadAuth';
  import AuthStatusNotice from '$lib/components/AuthStatusNotice.svelte';
  import NotificationSync from '$lib/components/NotificationSync.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import type { PresenceCache } from '$lib/state/presenceCache.svelte';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import { eventBusManager } from '$lib/state/server/eventBus.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { UserSettingsState } from '$lib/state/userSettings.svelte';
  import type { createUserProfileCache } from '$lib/state/userProfiles.svelte';
  import AuthenticatedChatProvider from './AuthenticatedChatProvider.svelte';

  let {
    user,
    userSettings,
    profileCache,
    presenceCache,
    children
  }: {
    user: CurrentUser;
    userSettings: UserSettingsState;
    profileCache: ReturnType<typeof createUserProfileCache>;
    presenceCache: PresenceCache;
    children: Snippet;
  } = $props();

  function realtimeRegistrations() {
    return serverRegistry.servers.flatMap((server) => {
      const store = serverRegistry.tryGetStore(server.id);
      return store?.isAuthenticated
        ? [
            {
              serverId: server.id,
              connection: serverConnectionManager.getClient(server.id),
              projectionSupported: store.serverInfo.supportsRealtimeProjection,
              sync: store.realtimeSync
            }
          ]
        : [];
    });
  }

  function synchronizeRealtimeTransports(
    registrations: ReturnType<typeof realtimeRegistrations>,
    activeServerId: string
  ) {
    eventBusManager.synchronizeAuthenticatedServers(registrations, activeServerId || null);
  }

  // Run synchronously so child route layouts can provide an already-registered
  // event bus during their own initialization.
  synchronizeRealtimeTransports(realtimeRegistrations(), getActiveServer());

  // Materialize the complete registration inputs as derived state. In
  // particular, a late discovery-capability update on a newly added remote
  // server must retrigger ownership even when no route or auth field changes.
  const registrations = $derived.by(realtimeRegistrations);
  const activeServerId = $derived(getActiveServer());

  $effect(() => {
    const nextRegistrations = registrations;
    const nextActiveServerId = activeServerId;

    // Transport synchronization reads and mutates reactive connection state.
    // Only the materialized registration inputs and active server should
    // retrigger ownership; tracking transport internals creates feedback loops.
    untrack(() => {
      synchronizeRealtimeTransports(nextRegistrations, nextActiveServerId);
    });
  });
</script>

<NotificationSync />
<AuthStatusNotice />

<AuthenticatedChatProvider {user} {userSettings} {profileCache} {presenceCache}>
  {@render children()}
</AuthenticatedChatProvider>
