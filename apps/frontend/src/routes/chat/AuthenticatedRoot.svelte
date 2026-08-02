<script lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { onDestroy, untrack, type Snippet } from 'svelte';
  import { mapDirectoryMember } from '$lib/api-client/memberDirectory';
  import { createPresenceAPI } from '$lib/api-client/presence';
  import { viewerResponseToState } from '$lib/api-client/viewer';
  import { clearCachedUser, type CurrentUser } from '$lib/auth/loadAuth';
  import { resumeReturnNavigation } from '$lib/auth/returnNavigation';
  import { hardRedirectAfterSignOut, isExplicitSignOutRedirectInProgress } from '$lib/auth/signOut';
  import { initSessionChannel } from '$lib/auth/sessionChannel';
  import AuthStatusNotice from '$lib/components/AuthStatusNotice.svelte';
  import PushNotificationPrompt from '$lib/components/PushNotificationPrompt.svelte';
  import PushNotificationSetup from '$lib/components/PushNotificationSetup.svelte';
  import WelcomeBanner from '$lib/components/WelcomeBanner.svelte';
  import { useProjectionEvent, useSessionTerminated } from '$lib/hooks/useEvent.svelte';
  import { initPresenceTracking } from '$lib/presenceTracking';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import {
    updateAuthenticatedCurrentUserPresenceEntries,
    type PresenceCache
  } from '$lib/state/presenceCache.svelte';
  import { presencePreference } from '$lib/state/presencePreference.svelte';
  import { eventBusManager } from '$lib/state/server/eventBus.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import {
    scheduleCustomStatusExpiry,
    type createUserProfileCache
  } from '$lib/state/userProfiles.svelte';

  let {
    user,
    profileCache,
    presenceCache,
    children
  }: {
    user: CurrentUser;
    profileCache: ReturnType<typeof createUserProfileCache>;
    presenceCache: PresenceCache;
    children: Snippet;
  } = $props();

  const originServer = serverRegistry.originServer;
  if (!originServer) {
    throw new Error(
      'AuthenticatedRoot mounted without a registered origin instance — guard the parent {#if} on serverRegistry.originServer.'
    );
  }

  const originServerId = originServer.id;
  const currentUserState = serverRegistry.getStore(originServerId).currentUser;

  // Populate the origin viewer before reconciling realtime registrations so
  // the single coordinator pass creates its bus before consumers subscribe.
  // svelte-ignore state_referenced_locally
  currentUserState.user = { ...user, presenceStatus: PresenceStatus.ONLINE };
  currentUserState.loading = false;
  // svelte-ignore state_referenced_locally
  presenceCache.update({ serverId: originServerId, userId: user.id }, PresenceStatus.ONLINE);
  void resumeReturnNavigation();

  onDestroy(() => {
    if (currentUserState.user?.id === user.id) {
      currentUserState.user = undefined;
      currentUserState.loading = false;
    }
  });

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

  // Run synchronously so projection/session consumers below always find the
  // origin bus during their own initialization.
  eventBusManager.synchronizeAuthenticatedServers(
    realtimeRegistrations(),
    getActiveServer() || null
  );

  // Materialize the complete registration inputs as derived state. In
  // particular, late discovery metadata on a newly added remote server must
  // retrigger ownership even when no route or auth field changes.
  const registrations = $derived.by(realtimeRegistrations);
  const activeServerId = $derived(getActiveServer());

  $effect(() => {
    const nextRegistrations = registrations;
    const nextActiveServerId = activeServerId;

    // Transport synchronization reads and mutates reactive connection state.
    // Only the materialized registration inputs and active server should
    // retrigger ownership; tracking transport internals creates feedback loops.
    untrack(() => {
      eventBusManager.synchronizeAuthenticatedServers(
        nextRegistrations,
        nextActiveServerId || null
      );
    });
  });

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

  function clearTerminatedOriginSession() {
    clearCachedUser();
    serverRegistry.clearServerAuthentication(originServerId);
    hardRedirectAfterSignOut('/');
  }

  // Keep origin-global profile caches synchronized with the same projection
  // operations that own each server-scoped store.
  useProjectionEvent(
    (event) => {
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
        } else if (operation.operation.case === 'userRemove') {
          profileCache.remove(operation.operation.value.userId);
        }
      }
    },
    () => originServerId
  );

  // Handle session terminated events from server (logout from another tab/device, admin boot).
  useSessionTerminated(
    (reason) => {
      console.log('Session terminated by server:', reason);
      if (isExplicitSignOutRedirectInProgress()) return;
      clearTerminatedOriginSession();
    },
    () => originServerId
  );

  // Handle logout from another tab in the same browser (instant, no server round-trip).
  $effect(() =>
    initSessionChannel(() => {
      if (isExplicitSignOutRedirectInProgress()) return;
      clearTerminatedOriginSession();
    })
  );

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

  // Initialize presence tracking (idle detection → AWAY, active → ONLINE).
  // This works across all instances, not just origin.
  const stopPresenceTracking = initPresenceTracking(
    () =>
      serverRegistry.servers
        .filter((server) => serverRegistry.tryGetStore(server.id)?.isAuthenticated)
        .map((server) => serverConnectionManager.getClient(server.id).getAPI(createPresenceAPI)),
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
</script>

<AuthStatusNotice />
<PushNotificationSetup />
<PushNotificationPrompt userId={user.id} />
<WelcomeBanner />

{@render children()}
