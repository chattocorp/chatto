<script lang="ts">
  import { onDestroy, untrack, type Snippet } from 'svelte';
  import { resolve } from '$app/paths';
  import { createPresenceAPI } from '$lib/api-client/presence';
  import { createAccountAPI } from '$lib/api-client/account';
  import { clearCachedUser } from '$lib/auth/loadAuth';
  import { resumeReturnNavigation } from '$lib/auth/returnNavigation';
  import { hardRedirectAfterSignOut, isExplicitSignOutRedirectInProgress } from '$lib/auth/signOut';
  import { initSessionChannel } from '$lib/auth/sessionChannel';
  import AuthStatusNotice from '$lib/components/AuthStatusNotice.svelte';
  import PushNotificationSetup from '$lib/components/PushNotificationSetup.svelte';
  import ScreenWakeLock from '$lib/components/ScreenWakeLock.svelte';
  import WelcomeBanner from '$lib/components/WelcomeBanner.svelte';
  import { onSessionTerminated } from '$lib/eventBus.svelte';
  import { initPresenceTracking } from '$lib/presenceTracking';
  import { serverIdToSegment } from '$lib/navigation';
  import { createDeviceTimezoneReportTracker, deviceTimezone } from '$lib/utils/deviceTimezone';
  import type { PresenceCache } from '$lib/state/presenceCache.svelte';
  import { presencePreferences } from '$lib/state/server/presencePreference.svelte';
  import { idleState } from '$lib/state/idle.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import { scheduleCustomStatusExpiry } from '$lib/utils/customStatusExpiry';

  let {
    presenceCache,
    children
  }: {
    presenceCache: PresenceCache;
    children: Snippet;
  } = $props();

  // The route can keep data.user = null after a saved view starts. Follow the
  // registry's verified identity so session effects start without a route load.
  const rootPresenceCache = untrack(() => presenceCache);
  const originServerId = $derived(serverRegistry.originServer?.id ?? null);
  const verifiedOriginUserId = $derived.by(() => {
    const store = originServerId ? serverRegistry.tryGetStore(originServerId) : undefined;
    const currentUser = store?.currentUser;
    const userId = currentUser?.user?.id;
    return userId && currentUser?.verifiedUserId === userId && store?.isAuthenticated
      ? userId
      : null;
  });

  $effect(() => {
    if (!originServerId || !verifiedOriginUserId) return;
    void resumeReturnNavigation();
  });

  $effect(() => {
    const serverId = originServerId;
    const userId = verifiedOriginUserId;
    if (!serverId || !userId) return;

    function clearTerminatedOriginSession() {
      if (!serverId || !userId) return;
      const current = serverRegistry.tryGetStore(serverId)?.currentUser;
      if (current?.user?.id !== userId || current.verifiedUserId !== userId) return;
      clearCachedUser();
      serverRegistry.clearServerAuthentication(serverId);
      const remainingServerId = serverRegistry.firstAuthenticatedServerId(serverId);
      hardRedirectAfterSignOut(
        remainingServerId
          ? resolve('/chat/[serverId]', { serverId: serverIdToSegment(remainingServerId) })
          : '/'
      );
    }

    const stopTermination = onSessionTerminated(serverId, (reason) => {
      console.warn('Session terminated by server:', reason);
      if (isExplicitSignOutRedirectInProgress()) return;
      clearTerminatedOriginSession();
    });
    const stopChannel = initSessionChannel(() => {
      if (isExplicitSignOutRedirectInProgress()) return;
      clearTerminatedOriginSession();
    });
    return () => {
      stopTermination();
      stopChannel();
    };
  });

  $effect(() => {
    const serverId = originServerId;
    const userId = verifiedOriginUserId;
    if (!serverId || !userId) return;
    const currentUser = serverRegistry.tryGetStore(serverId)?.currentUser;
    const status = currentUser?.user?.customStatus;
    if (!status?.expiresAt) return;

    return scheduleCustomStatusExpiry(status, () => {
      if (
        currentUser?.user?.id === userId &&
        currentUser.user.customStatus?.expiresAt === status.expiresAt
      ) {
        currentUser.user = { ...currentUser.user, customStatus: null };
      }
    });
  });

  function presenceReporters() {
    return serverRegistry.servers.flatMap((server) => {
      const store = serverRegistry.tryGetStore(server.id);
      const userId = store?.currentUser.user?.id;
      if (!store?.isAuthenticated || !userId) return [];
      const api = serverConnectionManager.getClient(server.id).getAPI(createPresenceAPI);
      return [{ serverId: server.id, userId, ...api }];
    });
  }

  const presenceTracking = initPresenceTracking(presenceReporters);
  onDestroy(() => presenceTracking.stop());

  $effect(() => presenceTracking.sync());

  $effect(() => {
    for (const scope of presenceReporters()) {
      rootPresenceCache.update(
        { serverId: scope.serverId, userId: scope.userId },
        presencePreferences.get(scope).status
      );
    }
  });

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
      if (user.settings?.shareTimezone === undefined) continue;
      const key = `${server.id}:${user.id}`;
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
          const currentUser = store.currentUser;
          if (currentUser.user?.id !== user.id) return;
          if (!currentUser.user.settings) {
            currentUser.user = { ...currentUser.user, settings };
          } else if (currentUser.user.settings.timezone == null) {
            currentUser.user = {
              ...currentUser.user,
              settings: { ...currentUser.user.settings, timezone: settings.timezone ?? null }
            };
          }
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
