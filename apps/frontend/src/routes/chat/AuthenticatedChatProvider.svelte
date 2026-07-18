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
  import { useProjectionEvent, useSessionTerminated } from '$lib/hooks';
  import { mapDirectoryMember } from '$lib/api-client/memberDirectory';
  import { viewerResponseToState } from '$lib/api-client/viewer';
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
      remove: (userId: string) => void;
      clear: () => void;
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

  // Register and expose the origin server's stable bus surface. The root
  // coordinator decides whether its transport is live, polling, or dormant.
  const originServerId = serverRegistry.originServer?.id;
  if (originServerId) {
    const authenticatedOriginServerId = originServerId;
    const originClient = serverConnectionManager.originClient;
    eventBusManager.ensureBus(
      authenticatedOriginServerId,
      originClient,
      serverRegistry.getStore(authenticatedOriginServerId).serverInfo.supportsRealtimeProjection,
      serverRegistry.getStore(authenticatedOriginServerId).realtimeSync
    );
    provideEventBus(() => authenticatedOriginServerId);

    function clearTerminatedOriginSession() {
      clearCachedUser();
      serverRegistry.clearServerAuthentication(authenticatedOriginServerId);
      hardRedirectAfterSignOut('/');
    }

    // Keep origin-global profile/settings caches synchronized with the same
    // projection operations that own each server-scoped store.
    useProjectionEvent((event) => {
      for (const operation of event.operations) {
        if (operation.operation.case === 'reset') {
          profileCache.clear();
        } else if (operation.operation.case === 'userUpsert') {
          const member = mapDirectoryMember(operation.operation.value);
          if (!member.id) continue;
          profileCache.update(
            member.id,
            member.displayName,
            member.avatarUrl,
            member.login,
            member.customStatus
          );
        } else if (operation.operation.case === 'viewerUpsert') {
          const viewer = viewerResponseToState(operation.operation.value);
          currentUserState.user = viewer.user;
          profileCache.update(
            viewer.user.id,
            viewer.user.displayName,
            viewer.user.avatarUrl ?? null,
            viewer.user.login,
            viewer.user.customStatus ?? null
          );
          userSettings.updateFromData(viewer.user.settings);
        } else if (operation.operation.case === 'userRemove') {
          profileCache.remove(operation.operation.value.userId);
        }
      }
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
