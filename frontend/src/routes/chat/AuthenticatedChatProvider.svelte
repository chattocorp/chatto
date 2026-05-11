<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { CurrentUser } from '$lib/auth/loadAuth';
  import type { PresenceCache } from '$lib/state/presenceCache.svelte';
  import type { UserSettingsState } from '$lib/state/userSettings.svelte';
  import { setCurrentUser } from '$lib/auth/currentUser.svelte';
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
    userSettings,
    profileCache,
    presenceCache,
    children
  }: {
    user: CurrentUser;
    userSettings: UserSettingsState;
    profileCache: { update: (userId: string, displayName: string, avatarUrl: string, login: string) => void };
    presenceCache: PresenceCache;
    children: Snippet;
  } = $props();

  // Populate the current user state from the load function data.
  //
  // The registry is the single source of truth for CurrentUserState — child
  // routes (chat/[instanceId]/+layout.svelte) read it via
  // `instanceRegistry.tryGetStore(...).currentUser`. Parents may not have
  // resolved the origin instance at *their* script init time, so we look it
  // up here ourselves rather than accepting it as a prop. Without this, a
  // prop snapshotted before origin registration would be a *different*
  // CurrentUserState object from the registry's, and writing `.user` to it
  // would have no effect on the auth guard's view of the world (#184).
  //
  // The parent's `{#if data.user && instanceRegistry.originInstance}` guard
  // ensures the origin store exists by the time this script runs. Auth-failure
  // and session-validation handlers are wired on the GraphQLClient by
  // `InstanceStateStore`'s constructor, so no further setup is needed here.
  const originInstance = instanceRegistry.originInstance;
  if (!originInstance) {
    throw new Error(
      'AuthenticatedChatProvider mounted without a registered origin instance — guard the parent {#if} on instanceRegistry.originInstance.'
    );
  }
  const currentUserState = instanceRegistry.getStore(originInstance.id).currentUser;
  // svelte-ignore state_referenced_locally
  currentUserState.user = user;
  currentUserState.loading = false;

  // Override the root layout's context (which holds a fallback CurrentUserState
  // constructed at root-layout init time, before origin was registered) with
  // the registry's. Components inside the authenticated tree read this via
  // getCurrentUser() and would otherwise see an empty user — even though we
  // just populated the registry's.
  setCurrentUser(currentUserState);

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
      if (event.event.__typename === 'ServerConfigUpdatedEvent') {
        const config = event.event;
        instanceRegistry.getStore(originInstanceId).instance.updateConfig({
          serverName: config.serverName,
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
