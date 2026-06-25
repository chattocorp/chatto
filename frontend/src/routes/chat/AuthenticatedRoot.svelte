<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { CurrentUser } from '$lib/auth/loadAuth';
  import NotificationSync from '$lib/components/NotificationSync.svelte';
  import { createPresenceCache } from '$lib/state/presenceCache.svelte';
  import { graphqlClientManager } from '$lib/state/server/graphqlClient.svelte';
  import { eventBusManager } from '$lib/state/server/eventBus.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { UserSettingsState } from '$lib/state/userSettings.svelte';
  import { createUserProfileCache } from '$lib/state/userProfiles.svelte';
  import AuthenticatedChatProvider from './AuthenticatedChatProvider.svelte';

  let {
    user,
    userSettings,
    children
  }: {
    user: CurrentUser;
    userSettings: UserSettingsState;
    children: Snippet;
  } = $props();

  const profileCache = createUserProfileCache();
  const presenceCache = createPresenceCache();

  function startAuthenticatedBuses() {
    for (const server of serverRegistry.servers) {
      const store = serverRegistry.tryGetStore(server.id);
      if (store?.isAuthenticated) {
        eventBusManager.startBus(
          server.id,
          graphqlClientManager.getClient(server.id)
        );
      }
    }
  }

  // Run synchronously so child route layouts can provide an already-started
  // event bus during their own initialization.
  startAuthenticatedBuses();

  $effect(() => {
    startAuthenticatedBuses();
  });
</script>

<NotificationSync />

<AuthenticatedChatProvider
  {user}
  {userSettings}
  {profileCache}
  {presenceCache}
>
  {@render children()}
</AuthenticatedChatProvider>
