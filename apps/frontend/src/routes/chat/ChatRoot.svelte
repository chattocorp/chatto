<script lang="ts">
  import { onDestroy, untrack, type Snippet } from 'svelte';
  import { resolve } from '$app/paths';
  import { createPresenceAPI } from '$lib/api-client/presence';
  import { createAccountAPI } from '$lib/api-client/account';
  import { beginOriginReauthentication } from '$lib/auth/reauth';
  import { resumeReturnNavigation } from '$lib/auth/returnNavigation';
  import { hardRedirectAfterSignOut, isExplicitSignOutRedirectInProgress } from '$lib/auth/signOut';
  import { initSessionChannel } from '$lib/auth/sessionChannel';
  import AuthStatusNotice from '$lib/components/AuthStatusNotice.svelte';
  import PushNotificationSetup from '$lib/components/PushNotificationSetup.svelte';
  import ScreenWakeLock from '$lib/components/ScreenWakeLock.svelte';
  import WelcomeBanner from '$lib/components/WelcomeBanner.svelte';
  import { eventBusManager } from '$lib/state/server/eventBus.svelte';
  import { initPresenceTracking } from '$lib/presenceTracking';
  import { serverIdToSegment } from '$lib/navigation';
  import { createDeviceTimezoneReportTracker, deviceTimezone } from '$lib/utils/deviceTimezone';
  import { idleState } from '$lib/state/idle.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import { scheduleCustomStatusExpiry } from '$lib/utils/customStatusExpiry';

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
      const remainingServerId = serverRegistry.firstAuthenticatedServerId(serverId);
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

  $effect(() => presenceTracking.sync());

  // Report this device's time zone once per (server, user) when the viewer has
  // no explicit override. Explicitly chosen zones are never overwritten, so
  // reconnects and other devices cannot clobber a deliberate setting.
  const timezoneReports = createDeviceTimezoneReportTracker();
  $effect(() => {
    const zone = deviceTimezone();
    if (!zone) return;
    for (const server of serverRegistry.servers) {
      const store = serverRegistry.tryGetStore(server.id);
      const user = store?.currentUser.user;
      if (!store || !user || !store.isAuthenticated) continue;
      const userId = store.accountId;
      if (user.settings?.shareTimezone === undefined) continue;
      const key = `${server.id}:${userId}`;
      if (!timezoneReports.begin(key)) continue;
      // Empty means the user explicitly selected browser default. Only an
      // absent preference permits automatic device reporting.
      if (user.settings?.timezone != null) {
        continue;
      }
      const api = serverConnectionManager.getClient(server.id).getAPI(createAccountAPI);
      void api
        .updateSettings({ timezone: zone })
        .then((settings) => {
          store.currentUser.update(userId, (user) => {
            if (!user.settings) return { settings };
            if (user.settings.timezone != null) return null;
            return { settings: { ...user.settings, timezone: settings.timezone ?? null } };
          });
        })
        .catch(() => {
          // Reporting is best-effort; the absence simply keeps the profile
          // zone empty. A later relevant store change can retry the report.
          timezoneReports.allowRetry(key);
        });
    }
  });
</script>

<AuthStatusNotice />
{#if idleState.isInAnyCall}
  <ScreenWakeLock />
{/if}
<PushNotificationSetup />
{#if verifiedOriginUserId}
  <WelcomeBanner />
{/if}

{@render children()}
