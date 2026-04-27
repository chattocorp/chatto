<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { CurrentUser } from '$lib/auth/loadAuth';
  import type { CurrentUserState } from '$lib/auth/currentUser.svelte';
  import type { PresenceCache } from '$lib/state/presenceCache.svelte';
  import type { UserSettingsState } from '$lib/state/userSettings.svelte';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { graphqlClientManager } from '$lib/state/instance/graphqlClient.svelte';
  import { initInstanceEventBus } from '$lib/instanceEventBus.svelte';
  import {
    useInstanceEvent,
    useUserProfileUpdate,
    useUserSettingsUpdate,
    useSessionTerminated
  } from '$lib/hooks';
  import { initSessionChannel } from '$lib/auth/sessionChannel';
  import { initPresenceTracking } from '$lib/presenceTracking';
  import ReturnUrlHandler from '$lib/components/ReturnUrlHandler.svelte';
  import PushNotificationSetup from '$lib/components/PushNotificationSetup.svelte';
  import WelcomeBanner from '$lib/components/WelcomeBanner.svelte';

  let {
    user,
    currentUserState,
    userSettings,
    profileCache,
    presenceCache,
    children
  }: {
    user: CurrentUser;
    currentUserState: CurrentUserState;
    userSettings: UserSettingsState;
    profileCache: { update: (userId: string, displayName: string, avatarUrl: string, login: string) => void };
    presenceCache: PresenceCache;
    children: Snippet;
  } = $props();

  // Populate the current user state from the load function data.
  // The store's GraphQLClient already has its auth handlers wired (in
  // InstanceStateStore's constructor) so a stale-cookie 'not authenticated'
  // response will trigger the redirect/clear flow without further setup here.
  // svelte-ignore state_referenced_locally
  currentUserState.user = user;
  // svelte-ignore state_referenced_locally
  currentUserState.loading = false;

  // Initialize user settings from the user's settings data
  // svelte-ignore state_referenced_locally
  userSettings.updateFromData(user.settings);

  // Initialize event bus for the origin instance and set its context
  // (so child components can use the context-based on* hooks).
  // Token-authenticated instance buses are managed at the layout level (unconditionally).
  // All origin-instance event bus features are guarded — the origin may not be registered
  // (e.g., user disconnected it, or the SPA is served statically).
  const originInstanceId = instanceRegistry.originInstance?.id;
  if (originInstanceId) {
    const originClient = graphqlClientManager.originClient;
    initInstanceEventBus(originClient.client, originInstanceId);

    // Subscribe to profile update events and populate the cache
    useUserProfileUpdate((update) => {
      profileCache.update(update.userId, update.displayName, update.avatarUrl, update.login);
    });

    // Subscribe to settings update events for multi-tab sync
    useUserSettingsUpdate((update) => {
      userSettings.timezone = update.timezone || null;
      userSettings.timeFormat = update.timeFormat;
    });

    // Handle session terminated events from server (logout from another tab/device, admin boot)
    useSessionTerminated((reason) => {
      console.log('Session terminated by server:', reason);
      currentUserState.handleAuthFailure();
    });

    // Handle logout from another tab in the same browser (instant, no server round-trip)
    $effect(() => initSessionChannel(() => currentUserState.handleAuthFailure()));

    // Listen for instance config updates (for page title, MOTD, welcome message, etc.)
    useInstanceEvent((event) => {
      if (!event.event) return;
      if (event.event.__typename === 'InstanceConfigUpdatedEvent') {
        const config = event.event;
        instanceRegistry.getStore(originInstanceId).instance.updateConfig({
          instanceName: config.instanceName,
          motd: config.motd ?? null,
          welcomeMessage: config.welcomeMessage ?? null
        });
      }
    });
  }

  // Initialize presence tracking (idle detection → AWAY, active → ONLINE).
  // This works across all instances, not just origin.
  initPresenceTracking(
    () =>
      instanceRegistry.instances.map(
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
<WelcomeBanner />

{@render children()}
