<script lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { onDestroy, untrack, type Snippet } from 'svelte';
  import { resolve } from '$app/paths';
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
  import ScreenWakeLock from '$lib/components/ScreenWakeLock.svelte';
  import WelcomeBanner from '$lib/components/WelcomeBanner.svelte';
  import { useProjectionEvent, useSessionTerminated } from '$lib/hooks/useEvent.svelte';
  import { initPresenceTracking } from '$lib/presenceTracking';
  import { serverIdToSegment } from '$lib/navigation';
  import { getPushRegistrationTargets } from '$lib/notifications/pushNotifications';
  import {
    updateAuthenticatedCurrentUserPresenceEntries,
    type PresenceCache
  } from '$lib/state/presenceCache.svelte';
  import { presencePreference } from '$lib/state/presencePreference.svelte';
  import { idleState } from '$lib/state/idle.svelte';
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
    user?: CurrentUser | null;
    profileCache: ReturnType<typeof createUserProfileCache>;
    presenceCache: PresenceCache;
    children: Snippet;
  } = $props();

  // The chat layout keys this root by origin server/viewer identity. The
  // application-root runtime coordinator has already installed this viewer
  // and created its event bus before the chat subtree initializes.
  const originUser = untrack(() => user);
  const rootProfileCache = untrack(() => profileCache);
  const rootPresenceCache = untrack(() => presenceCache);
  const originServer = serverRegistry.originServer;
  const originServerId = originServer?.id ?? null;
  const currentUserState = originServerId
    ? serverRegistry.getStore(originServerId).currentUser
    : null;
  const originSession =
    originUser && originServerId && currentUserState
      ? { user: originUser, serverId: originServerId, currentUser: currentUserState }
      : null;
  const pushPromptTarget = $derived(getPushRegistrationTargets()[0] ?? null);

  if (originSession) {
    rootPresenceCache.update(
      { serverId: originSession.serverId, userId: originSession.user.id },
      PresenceStatus.ONLINE
    );
    void resumeReturnNavigation();
  }

  if (originSession) {
    const session = originSession;

    $effect(() => {
      const status = session.currentUser.user?.customStatus;
      const currentUserId = session.currentUser.user?.id;
      if (!status?.expiresAt || !currentUserId) return;

      return scheduleCustomStatusExpiry(status, () => {
        if (
          session.currentUser.user?.id === currentUserId &&
          session.currentUser.user.customStatus?.expiresAt === status.expiresAt
        ) {
          session.currentUser.user = {
            ...session.currentUser.user,
            customStatus: null
          };
          rootProfileCache.updateStatus(currentUserId, null);
        }
      });
    });

    function clearTerminatedOriginSession() {
      clearCachedUser();
      serverRegistry.clearServerAuthentication(session.serverId);
      const remainingServerId = serverRegistry.firstAuthenticatedServerId(session.serverId);
      hardRedirectAfterSignOut(
        remainingServerId
          ? resolve('/chat/[serverId]', { serverId: serverIdToSegment(remainingServerId) })
          : '/'
      );
    }

    // Keep origin-global profile caches synchronized with the same projection
    // operations that own each server-scoped store.
    useProjectionEvent(
      (event) => {
        for (const operation of event.operations) {
          if (operation.operation.case === 'reset') {
            rootProfileCache.clear();
          } else if (operation.operation.case === 'userUpsert') {
            const member = mapDirectoryMember(operation.operation.value);
            if (!member.id) continue;
            rootProfileCache.update(
              member.id,
              member.displayName,
              member.avatarUrl,
              member.login,
              member.customStatus
            );
          } else if (operation.operation.case === 'viewerUpsert') {
            const viewer = viewerResponseToState(operation.operation.value);
            session.currentUser.user = viewer.user;
            rootProfileCache.update(
              viewer.user.id,
              viewer.user.displayName,
              viewer.user.avatarUrl ?? null,
              viewer.user.login,
              viewer.user.customStatus ?? null
            );
          } else if (operation.operation.case === 'userRemove') {
            rootProfileCache.remove(operation.operation.value.userId);
          }
        }
      },
      () => session.serverId
    );

    // Handle session terminated events from server (logout from another tab/device, admin boot).
    useSessionTerminated(
      (reason) => {
        console.log('Session terminated by server:', reason);
        if (isExplicitSignOutRedirectInProgress()) return;
        clearTerminatedOriginSession();
      },
      () => session.serverId
    );

    // Handle logout from another tab in the same browser (instant, no server round-trip).
    $effect(() =>
      initSessionChannel(() => {
        if (isExplicitSignOutRedirectInProgress()) return;
        clearTerminatedOriginSession();
      })
    );
  }

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
        rootPresenceCache,
        currentUserPresenceStores(),
        status
      );
    }
  );
  onDestroy(stopPresenceTracking);

  $effect(() => {
    updateAuthenticatedCurrentUserPresenceEntries(
      rootPresenceCache,
      currentUserPresenceStores(),
      presencePreference.effectiveStatus
    );
  });
</script>

<AuthStatusNotice />
{#if idleState.isInAnyCall}
  <ScreenWakeLock />
{/if}
<PushNotificationSetup />
{#if pushPromptTarget}
  {#key `${pushPromptTarget.serverId}:${pushPromptTarget.userId}`}
    <PushNotificationPrompt {...pushPromptTarget} />
  {/key}
{/if}
{#if originSession}
  <WelcomeBanner />
{/if}

{@render children()}
