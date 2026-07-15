<script lang="ts">
  import { onDestroy, type Snippet } from 'svelte';
  import type { CurrentUser } from '$lib/auth/loadAuth';
  import { PresenceStatus } from '$lib/render/types';
  import {
    updateAuthenticatedCurrentUserPresenceEntries,
    type PresenceCache
  } from '$lib/state/presenceCache.svelte';
  import { presencePreference } from '$lib/state/presencePreference.svelte';
  import type { UserSettingsState } from '$lib/state/userSettings.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import { provideEventBus } from '$lib/eventBus.svelte';
  import { eventBusManager } from '$lib/state/server/eventBus.svelte';
  import {
    useUserProfileUpdate,
    useUserCustomStatusUpdate,
    useUserSettingsUpdate,
    useSessionTerminated
  } from '$lib/hooks';
  import {
    scheduleCustomStatusExpiry,
    type CustomUserStatus
  } from '$lib/state/userProfiles.svelte';
  import { clearCachedUser } from '$lib/auth/loadAuth';
  import { hardRedirectAfterSignOut, isExplicitSignOutRedirectInProgress } from '$lib/auth/signOut';
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
    profileCache: {
      update: (
        userId: string,
        displayName: string,
        avatarUrl: string | null,
        login: string,
        customStatus?: CustomUserStatus | null
      ) => void;
      updateStatus: (userId: string, customStatus: CustomUserStatus | null) => void;
    };
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
  presenceCache.update({ serverId: originServer.id, userId: user.id }, PresenceStatus.Online);

  // Initialize user settings from the user's settings data
  // svelte-ignore state_referenced_locally
  userSettings.updateFromData(user.settings);

  $effect(() => {
    const status = currentUserState.user?.customStatus;
    const currentUserId = currentUserState.user?.id;
    if (!status?.expiresAt || !currentUserId) return;

    return scheduleCustomStatusExpiry(status, () => {
      if (
        currentUserState.user?.id === currentUserId &&
        currentUserState.user.customStatus?.expiresAt === status.expiresAt
      ) {
        currentUserState.user = {
          ...currentUserState.user,
          customStatus: null
        };
        profileCache.updateStatus(currentUserId, null);
      }
    });
  });

  // Start (idempotent) and expose the origin server's event bus via Svelte
  // context so the on* hooks below can use it. Root +layout.svelte's $effect
  // also starts buses for every authenticated server, but the user state may
  // not have flipped to authenticated at root-init time — starting it here
  // unconditionally guarantees the bus exists by the time the context is
  // set, so consumer handlers register against the right bus rather than a
  // dropped no-op.
  const originServerId = serverRegistry.originServer?.id;
  if (originServerId) {
    const authenticatedOriginServerId = originServerId;
    const originClient = serverConnectionManager.originClient;
    eventBusManager.startBus(authenticatedOriginServerId, originClient);
    provideEventBus(() => authenticatedOriginServerId);

    function clearTerminatedOriginSession() {
      clearCachedUser();
      serverRegistry.clearServerAuthentication(authenticatedOriginServerId);
      hardRedirectAfterSignOut('/');
    }

    // Subscribe to profile update events and populate the cache
    useUserProfileUpdate((update) => {
      profileCache.update(
        update.userId,
        update.displayName,
        update.avatarUrl,
        update.login
      );
      if (currentUserState.user?.id === update.userId) {
        currentUserState.user = {
          ...currentUserState.user,
          displayName: update.displayName,
          avatarUrl: update.avatarUrl,
          login: update.login
        };
      }
    });

    useUserCustomStatusUpdate((update) => {
      profileCache.updateStatus(update.userId, update.customStatus);
      if (currentUserState.user?.id === update.userId) {
        currentUserState.user = {
          ...currentUserState.user,
          customStatus: update.customStatus
        };
      }
    });

    // Subscribe to settings update events for multi-tab sync
    useUserSettingsUpdate((update) => {
      userSettings.timezone = update.timezone;
      userSettings.timeFormat = update.timeFormat;
    });

    // Handle session terminated events from server (logout from another tab/device, admin boot)
    useSessionTerminated((reason) => {
      console.log('Session terminated by server:', reason);
      if (isExplicitSignOutRedirectInProgress()) return;
      clearTerminatedOriginSession();
    });

    // Handle logout from another tab in the same browser (instant, no server round-trip)
    $effect(() =>
      initSessionChannel(() => {
        if (isExplicitSignOutRedirectInProgress()) return;
        clearTerminatedOriginSession();
      })
    );

  }

  // Initialize presence tracking (idle detection → AWAY, active → ONLINE).
  // This works across all instances, not just origin.
  const stopPresenceTracking = initPresenceTracking(
    () =>
      serverRegistry.servers
        .filter((server) => serverRegistry.tryGetStore(server.id)?.isAuthenticated)
        .map((server) => {
          const client = serverConnectionManager.getClient(server.id);
          return {
            serverId: server.id,
            baseUrl: client.connectBaseUrl,
            bearerToken: client.bearerToken
          };
        }),
    (status) => {
      updateAuthenticatedCurrentUserPresenceEntries(
        presenceCache,
        currentUserPresenceStores(),
        status
      );
    },
    {
      onPauseLiveEvents: () => {
        eventBusManager.pauseAll();
      },
      onResumeLiveEvents: () => {
        eventBusManager.resumeAll();
        for (const server of serverRegistry.servers) {
          if (serverRegistry.tryGetStore(server.id)?.isAuthenticated) {
            eventBusManager.startBus(server.id, serverConnectionManager.getClient(server.id));
          }
        }
      }
    }
  );
  onDestroy(stopPresenceTracking);

  $effect(() => {
    updateAuthenticatedCurrentUserPresenceEntries(
      presenceCache,
      currentUserPresenceStores(),
      presencePreference.effectiveStatus
    );
  });

  function currentUserPresenceStores() {
    return serverRegistry.servers.map((server) => {
      const store = serverRegistry.tryGetStore(server.id);
      return store
        ? {
            serverId: server.id,
            isAuthenticated: store.isAuthenticated,
            currentUser: store.currentUser
          }
        : null;
    });
  }
</script>

<ReturnUrlHandler />
<PushNotificationSetup />
<PushNotificationPrompt userId={user.id} />
<WelcomeBanner />

{@render children()}
