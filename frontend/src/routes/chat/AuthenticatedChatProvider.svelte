<script lang="ts">
  import { onDestroy, type Snippet } from 'svelte';
  import type { CurrentUser } from '$lib/auth/loadAuth';
  import { PresenceStatus } from '$lib/gql/graphql';
  import type { PresenceCache } from '$lib/state/presenceCache.svelte';
  import type { UserSettingsState } from '$lib/state/userSettings.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { graphqlClientManager } from '$lib/state/server/graphqlClient.svelte';
  import { provideEventBus } from '$lib/eventBus.svelte';
  import { eventBusManager } from '$lib/state/server/eventBus.svelte';
  import {
    useUserProfileUpdate,
    useUserSettingsUpdate,
    useSessionTerminated
  } from '$lib/hooks';
  import { initSessionChannel } from '$lib/auth/sessionChannel';
  import { initPresenceTracking } from '$lib/presenceTracking';
  import ReturnUrlHandler from '$lib/components/ReturnUrlHandler.svelte';
  import PushNotificationPrompt from '$lib/components/PushNotificationPrompt.svelte';
  import PushNotificationSetup from '$lib/components/PushNotificationSetup.svelte';
  import WelcomeBanner from '$lib/components/WelcomeBanner.svelte';

  let {
    user,
    userSettings,
    profileCache,
    presenceCache,
    children
  }: {
    user: CurrentUser;
    userSettings: UserSettingsState;
    profileCache: { update: (userId: string, displayName: string, avatarUrl: string | null, login: string) => void };
    presenceCache: PresenceCache;
    children: Snippet;
  } = $props();

  // Populate the origin server's CurrentUserState from the load function
  // data. Cookie-auth stores stay loading until this provider mounts, so
  // route guards cannot observe a transient "not loading, no user" gap.
  const originServer = serverRegistry.originServer;
  if (!originServer) {
    throw new Error(
      'AuthenticatedChatProvider mounted without a registered origin instance — guard the parent {#if} on serverRegistry.originServer.'
    );
  }
  const currentUserState = serverRegistry.getStore(originServer.id).currentUser;
  // svelte-ignore state_referenced_locally
  currentUserState.user = { ...user, presenceStatus: PresenceStatus.Online };
  currentUserState.loading = false;
  onDestroy(() => {
    if (currentUserState.user?.id === user.id) {
      currentUserState.user = undefined;
      currentUserState.loading = false;
    }
  });
  // svelte-ignore state_referenced_locally
  presenceCache.update(user.id, PresenceStatus.Online);

  // Initialize user settings from the user's settings data
  // svelte-ignore state_referenced_locally
  userSettings.updateFromData(user.settings);

  // Start (idempotent) and expose the origin server's event bus via Svelte
  // context so the on* hooks below can use it. Root +layout.svelte's $effect
  // also starts buses for every authenticated server, but the user state may
  // not have flipped to authenticated at root-init time — starting it here
  // unconditionally guarantees the bus exists by the time the context is
  // set, so consumer handlers register against the right bus rather than a
  // dropped no-op.
  const originServerId = serverRegistry.originServer?.id;
  if (originServerId) {
    const originClient = graphqlClientManager.originClient;
    eventBusManager.startBus(originServerId, originClient);
    provideEventBus(() => originServerId);

    // Subscribe to profile update events and populate the cache
    useUserProfileUpdate((update) => {
      profileCache.update(update.userId, update.displayName, update.avatarUrl, update.login);
    });

    // Subscribe to settings update events for multi-tab sync
    useUserSettingsUpdate((update) => {
      userSettings.timezone = update.timezone;
      userSettings.timeFormat = update.timeFormat;
    });

    // Handle session terminated events from server (logout from another tab/device, admin boot)
    useSessionTerminated((reason) => {
      console.log('Session terminated by server:', reason);
      serverRegistry.handleAuthenticationRequired(originServerId);
    });

    // Handle logout from another tab in the same browser (instant, no server round-trip)
    $effect(() => initSessionChannel(() => serverRegistry.handleAuthenticationRequired(originServerId)));

  }

  // Initialize presence tracking (idle detection → AWAY, active → ONLINE).
  // This works across all instances, not just origin.
  initPresenceTracking(
    () =>
      serverRegistry.servers.map(
        (i) => graphqlClientManager.getClient(i.id).client
      ),
    (status) => {
      if (currentUserState.user) {
        presenceCache.update(currentUserState.user.id, status);
      }
    }
  );
</script>

<ReturnUrlHandler />
<PushNotificationSetup />
<PushNotificationPrompt userId={user.id} />
<WelcomeBanner />

{@render children()}
