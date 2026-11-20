<script lang="ts">
  import { onDestroy, untrack, type Snippet } from 'svelte';
  import { resolve } from '$app/paths';
  import { createPresenceAPI } from '@chatto/client/api/presence';
  import { beginOriginReauthentication } from '$lib/auth/reauth';
  import { resumeReturnNavigation } from '$lib/auth/returnNavigation';
  import { isExplicitSignOutRedirectInProgress } from '@chatto/client/auth/signOut';
  import { hardRedirectAfterSignOut } from '$lib/auth/signOutRedirect';
  import { initSessionChannel } from '$lib/auth/sessionChannel';
  import AuthStatusNotice from '$lib/components/AuthStatusNotice.svelte';
  import ScreenWakeLock from '$lib/components/ScreenWakeLock.svelte';
  import WelcomeBanner from '$lib/components/WelcomeBanner.svelte';
  import { eventBusManager, serverRegistry, serverConnectionManager } from '$lib/client';
  import { firstAuthenticatedServerId } from '$lib/serverCatalogue';
  import {
    initPresenceTracking,
    refreshPresencePreference
  } from '$lib/state/server/presenceTracking';
  import { serverIdToSegment } from '$lib/navigation';
  import { idleState } from '$lib/state/idle.svelte';
  import { scheduleCustomStatusExpiry } from '@chatto/client/util/customStatusExpiry';

  let {
    children
  }: {
    children: Snippet;
  } = $props();

  // Follow the registry's verified identity instead of the root load's user, so
  // session effects start when verification ends without a new route load.
  const originServerId = $derived(serverRegistry.originServer?.id ?? null);
  const verifiedOriginUserId = $derived.by(() => {
    const store = originServerId ? serverRegistry.tryGetStore(originServerId) : undefined;
    const currentUser = store?.currentUser;
    const userId = store?.accountId;
    return userId && currentUser?.verifiedUserId === userId && store?.isAuthenticated
      ? userId
      : null;
  });

  $effect(() => {
    if (!originServerId || !verifiedOriginUserId) return;
    void resumeReturnNavigation();
  });

  // A rejected origin viewer with nothing loaded to read goes to sign-in and
  // returns to the current page afterwards.
  $effect(() => {
    if (!serverRegistry.originSignInRequired || isExplicitSignOutRedirectInProgress()) return;
    untrack(() => beginOriginReauthentication());
  });

  $effect(() => {
    const serverId = originServerId;
    const userId = verifiedOriginUserId;
    if (!serverId || !userId) return;

    function clearTerminatedOriginSession() {
      if (!serverId || !userId) return;
      const store = serverRegistry.tryGetStore(serverId);
      if (store?.accountId !== userId || store.currentUser.verifiedUserId !== userId) return;
      serverRegistry.clearServerAuthentication(serverId);
      const remainingServerId = firstAuthenticatedServerId(serverId);
      hardRedirectAfterSignOut(
        remainingServerId
          ? resolve('/chat/[serverId]', { serverId: serverIdToSegment(remainingServerId) })
          : '/'
      );
    }

    const stopTermination = eventBusManager.getBus(serverId)?.onSessionTerminated((reason) => {
      console.warn('Session terminated by server:', reason);
      if (isExplicitSignOutRedirectInProgress()) return;
      clearTerminatedOriginSession();
    });
    const stopChannel = initSessionChannel(() => {
      if (isExplicitSignOutRedirectInProgress()) return;
      clearTerminatedOriginSession();
    });
    return () => {
      stopTermination?.();
      stopChannel();
    };
  });

  $effect(() => {
    const serverId = originServerId;
    const userId = verifiedOriginUserId;
    if (!serverId || !userId) return;
    const store = serverRegistry.tryGetStore(serverId);
    const currentUser = store?.currentUser;
    const status = currentUser?.user?.customStatus;
    if (!status?.expiresAt) return;

    return scheduleCustomStatusExpiry(status, () => {
      currentUser?.update(userId, (user) =>
        user.customStatus?.expiresAt === status.expiresAt ? { customStatus: null } : null
      );
    });
  });

  function presenceReporters() {
    return serverRegistry.servers.flatMap((server) => {
      const store = serverRegistry.tryGetStore(server.id);
      const userId = store?.accountId;
      if (!store?.isAuthenticated || !userId) return [];
      const api = serverConnectionManager.getClient(server.id).getAPI(createPresenceAPI);
      return [{ serverId: server.id, userId, ...api }];
    });
  }

  const presenceTracking = initPresenceTracking(presenceReporters);
  onDestroy(() => presenceTracking.stop());
  // Another device changed the viewer's presence choice on a server: read it again.
  onDestroy(
    serverRegistry.watchStores((store) =>
      store.onUpdate((update) => {
        if (update.event?.event.case !== 'viewerPresencePreferenceChanged') return;
        if (store.accountId) {
          refreshPresencePreference({ serverId: store.serverId, userId: store.accountId });
        }
      })
    )
  );

  $effect(() => presenceTracking.sync());

  // Web Push registration is not needed for the first paint. Loading it on
  // demand keeps it out of every chat route's initial bundle.
  const pushSubscriptionRefresh = import('$lib/components/PushSubscriptionRefresh.svelte');
</script>

<AuthStatusNotice />
{#if idleState.isInAnyCall}
  <ScreenWakeLock />
{/if}
{#await pushSubscriptionRefresh then { default: PushSubscriptionRefresh }}
  <PushSubscriptionRefresh />
{/await}
{#if verifiedOriginUserId}
  <WelcomeBanner />
{/if}

{@render children()}
