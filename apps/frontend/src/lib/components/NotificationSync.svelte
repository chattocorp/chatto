<!--
@component

Handles real-time notification synchronization across all authenticated instances
and installed-app badge updates.

**Responsibilities:**
- Listens for live notification creation hints
- Plays the user's selected sound for eligible in-app notification creations
- Reconciles the unnumbered installed-app badge from important unread notifications

Include this component once in the application root so signed-out pages also clear stale badges.
-->
<script lang="ts">
  import { serverRegistry, eventBusManager } from '$lib/client';
  import { serverUi } from '$lib/state/server/serverUi';
  import {
    eligibleNotificationAlerts,
    notificationAlertsMuted,
    playNotificationAlertSound
  } from '$lib/notifications/notificationAlerts';
  import {
    listenForAppBadgeRefresh,
    updateAppBadge,
    type AppBadgeIntent
  } from '$lib/notifications/appBadge';
  import Deadline from '$lib/lifecycle/Deadline.svelte';
  import Interval from '$lib/lifecycle/Interval.svelte';
  import PushNotificationSync from './PushNotificationSync.svelte';
  import { NOTIFICATION_RECONCILIATION_INTERVAL_MS } from '@chatto/client/server/notifications';
  import type { ProjectionHandler } from '@chatto/client/realtime/eventBus';
  import {
    desktopNotifications,
    hasDesktopNotifications,
    listenForDesktopNotificationClicks
  } from '$lib/desktop/notifications.svelte';
  import IncomingCallAlert from './voice/IncomingCallAlert.svelte';
  import DesktopNotificationSync from './DesktopNotificationSync.svelte';

  $effect(() => listenForDesktopNotificationClicks());

  type CallLink = { serverId: string; roomId: string; callId: string };
  type CallAlert = CallLink & { occurrenceId?: string; accountId: string; fromLink: boolean };
  let {
    callLink = null,
    onCloseCallLink
  }: { callLink?: CallLink | null; onCloseCallLink?: () => void } = $props();
  let liveCalls = $state.raw<(CallAlert & { occurrenceId: string })[]>([]);
  const callAlerts = $derived.by(() => {
    if (!callLink) return liveCalls;
    const link = callLink;
    const store = serverRegistry.tryGetStore(link.serverId);
    if (!store?.isAuthenticated || !store.accountId) return liveCalls;
    const same = (alert: CallAlert) =>
      alert.serverId === link.serverId &&
      alert.accountId === store.accountId &&
      alert.callId === link.callId;
    return [
      ...liveCalls.filter((alert) => !same(alert)),
      { ...liveCalls.find(same), ...link, accountId: store.accountId, fromLink: true }
    ];
  });

  function closeCall(alert: CallAlert) {
    if (
      liveCalls.some(
        (item) =>
          item.serverId === alert.serverId &&
          item.accountId === alert.accountId &&
          item.callId === alert.callId
      )
    )
      liveCalls = liveCalls.filter(
        (item) =>
          item.serverId !== alert.serverId ||
          item.accountId !== alert.accountId ||
          item.callId !== alert.callId
      );
    if (alert.fromLink) onCloseCallLink?.();
  }

  const rememberedNotificationIds = 256;

  // Subscribe to notification events on all authenticated instance buses.
  // Uses the event bus manager directly (not Svelte context) to handle all instances.
  $effect(() => {
    const cleanups: (() => void)[] = [];

    for (const instance of serverRegistry.servers) {
      const stores = serverRegistry.getStore(instance.id);
      if (!stores.isAuthenticated) continue;

      const bus = eventBusManager.getBus(instance.id);
      if (!bus) continue;
      const handledNotificationIds: string[] = [];
      const accountId = stores.accountId;
      let active = true;
      let checkingSound = false;
      const pendingCreations: string[] = [];

      async function soundForCreations() {
        checkingSound = true;
        let readSucceeded = false;
        try {
          // The store handler starts the resource read before this handler runs.
          // Do not infer unread state from a live hint or a stale retained row.
          readSucceeded = await stores.waitForRealtimeResourceRefresh('notifications');
        } catch {
          // A snapshot reset can cancel this read.
        }
        // Ambient notifications keep their badges but never trigger local audio.
        const eligible = eligibleNotificationAlerts(
          instance.id,
          stores,
          stores.notifications.occurrences.filter((row) => pendingCreations.includes(row.id))
        );
        pendingCreations.length = 0;
        checkingSound = false;
        if (
          !readSucceeded ||
          !active ||
          !stores.isAuthenticated ||
          stores.accountId !== accountId ||
          eligible.length === 0
        )
          return;
        const calls = eligible.flatMap((row) =>
          row.call && row.room && accountId
            ? [
                {
                  serverId: instance.id,
                  accountId,
                  roomId: row.room.id,
                  callId: row.call.id,
                  occurrenceId: row.id,
                  fromLink: false
                }
              ]
            : []
        );
        if (calls.length) {
          // A call hint can arrive before its call-start event. A current read
          // also suppresses late hints for a call that has ended.
          const refreshed = await stores.refreshNotificationCalls().catch(() => false);
          if (
            !active ||
            !stores.isAuthenticated ||
            stores.accountId !== accountId ||
            notificationAlertsMuted(instance.id, stores)
          )
            return;
          const ui = serverUi(stores);
          const current =
            refreshed && stores.realtimeSync.phase === 'ready'
              ? calls.filter(
                  (call) =>
                    !liveCalls.some(
                      (live) => live.serverId === call.serverId && live.callId === call.callId
                    ) &&
                    ui.voiceCall.permissionsFor(call.roomId).join &&
                    ui.activeCallRooms.getCallId(call.roomId) === call.callId &&
                    !ui.activeCallRooms.getParticipantCallPresence(call.roomId, call.accountId)
                )
              : [];
          if (current.length)
            liveCalls = [...liveCalls, ...current].slice(-rememberedNotificationIds);
          else if (eligible.length === calls.length) return;
        }
        // Desktop alerts own their sound for both live and polled updates.
        if (hasDesktopNotifications() && desktopNotifications.enabled) return;
        playNotificationAlertSound(instance.id);
      }

      const handler: ProjectionHandler = (event) => {
        if (event.reset) {
          pendingCreations.length = 0;
          liveCalls = liveCalls.filter((call) => call.serverId !== instance.id);
        }
        const semantic = event.event?.event;
        if (semantic?.case !== 'notificationOccurrencesChanged') return;
        const notificationId = semantic.value.createdNotificationId;
        if (!notificationId || handledNotificationIds.includes(notificationId)) return;
        // Remember suppressed hints too. A duplicate must not sound after DND ends.
        handledNotificationIds.push(notificationId);
        if (handledNotificationIds.length > rememberedNotificationIds)
          handledNotificationIds.shift();
        if (notificationAlertsMuted(instance.id, stores)) return;
        pendingCreations.push(notificationId);
        if (pendingCreations.length > rememberedNotificationIds) pendingCreations.shift();
        // Several causes can describe one activity. Play once per completed batch.
        if (!checkingSound) void soundForCreations();
      };

      const unsubscribe = bus.subscribe(handler);
      cleanups.push(() => {
        active = false;
        liveCalls = liveCalls.filter((call) => call.serverId !== instance.id);
        unsubscribe();
      });
    }

    return () => {
      for (const fn of cleanups) fn();
    };
  });

  function appBadgeIntent(): AppBadgeIntent | null {
    let importantUnreadCount = 0;

    for (const instance of serverRegistry.servers) {
      const stores = serverRegistry.getStore(instance.id);
      if (!stores.isAuthenticated) continue;
      if (!stores.notifications.hasLoaded) return null;
      importantUnreadCount += serverUi(stores).attention.counts.importantUnreadNotificationCount;
    }

    if (importantUnreadCount > 0) return { kind: 'flag' };
    return { kind: 'clear' };
  }

  function syncAppBadge() {
    const intent = appBadgeIntent();
    if (intent) void updateAppBadge(intent);
  }

  // Synchronize the external OS badge directly from authoritative notification stores.
  // Avoid clearing an existing badge until every authenticated store has loaded.
  $effect(syncAppBadge);

  // A push can set a flag after its notification was read or removed.
  // Reassert current important attention when the worker reports a regular push.
  $effect(() => {
    return listenForAppBadgeRefresh(syncAppBadge);
  });
</script>

{#each serverRegistry.servers as instance (instance.id)}
  {@const stores = serverRegistry.getStore(instance.id)}
  {#if stores.isAuthenticated}
    {#if stores.accountId}
      {#key stores.accountId}
        {#if hasDesktopNotifications()}
          <DesktopNotificationSync
            store={stores}
            liveCallOccurrenceIds={liveCalls
              .filter(
                (call) => call.serverId === instance.id && call.accountId === stores.accountId
              )
              .map((call) => call.occurrenceId)}
          />
        {/if}
        <PushNotificationSync
          serverUrl={instance.url}
          recipientId={stores.accountId}
          notifications={stores.notifications}
        />
      {/key}
    {/if}
    <!-- Core NATS invalidations are latency hints; the notification stream is authoritative.
    The store skips the read while realtime keeps the list current. -->
    <Interval
      milliseconds={NOTIFICATION_RECONCILIATION_INTERVAL_MS}
      ontick={() => void stores.reconcileNotifications()}
    />

    <!-- Stream expiry has no per-message projection callback. -->
    {#if stores.notifications.nextExpiryAt}
      <Deadline
        at={stores.notifications.nextExpiryAt}
        offsetMilliseconds={50}
        onreached={() => void stores.notifications.fetch()}
      />
    {/if}
  {/if}
{/each}

{#if callAlerts.length}
  <div
    class="pointer-events-none fixed inset-x-3 top-[calc(env(safe-area-inset-top,0px)+1rem)] z-50 flex flex-col items-end gap-2"
    role="status"
  >
    {#each callAlerts as alert ([alert.serverId, alert.accountId, alert.callId].join('/'))}
      {@const store = serverRegistry.getStore(alert.serverId)}
      {#if store.isAuthenticated && store.accountId === alert.accountId}
        <IncomingCallAlert
          {store}
          roomId={alert.roomId}
          callId={alert.callId}
          occurrenceId={alert.occurrenceId}
          fromLink={alert.fromLink}
          onclose={() => closeCall(alert)}
        />
      {/if}
    {/each}
  </div>
{/if}
