<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { CurrentUser } from '$lib/auth/loadAuth';
  import AuthStatusNotice from '$lib/components/AuthStatusNotice.svelte';
  import NotificationSync from '$lib/components/NotificationSync.svelte';
  import { shouldPauseLiveEventsForStoredPresence } from '$lib/presenceTracking';
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

  function synchronizeRealtimeTransports() {
    if (shouldPauseLiveEventsForStoredPresence()) {
      eventBusManager.pauseAll();
      return;
    }

    eventBusManager.resumeAll();
    eventBusManager.synchronizeAuthenticatedServers(
      serverRegistry.servers.flatMap((server) => {
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
      }),
      getActiveServer() || null
    );
  }

  // Run synchronously so child route layouts can provide an already-registered
  // event bus during their own initialization.
  synchronizeRealtimeTransports();

  $effect(() => {
    synchronizeRealtimeTransports();
  });
</script>

<NotificationSync />
<AuthStatusNotice />

<AuthenticatedChatProvider {user} {userSettings} {profileCache} {presenceCache}>
  {@render children()}
</AuthenticatedChatProvider>
