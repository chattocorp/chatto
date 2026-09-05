<!--
@component

Handles real-time notification synchronization across all authenticated instances
and installed-app badge updates.

**Responsibilities:**
- Listens for live notification creation hints
- Plays the user's selected sound for eligible in-app notification creations
- Reconciles the installed-app badge from authoritative unread occurrence counts

Include this component once in the application root so signed-out pages also clear stale badges.
-->
<script lang="ts">
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { eventBusManager } from '$lib/state/server/eventBus.svelte';
  import { getServerNotificationPreferences } from '$lib/state/serverNotificationPreferences.svelte';
  import { playNotificationSound } from '$lib/audio/notificationSounds';
  import {
    listenForAppBadgeRefresh,
    updateAppBadge,
    type AppBadgeIntent
  } from '$lib/notifications/appBadge';
  import Deadline from '$lib/lifecycle/Deadline.svelte';
  import Interval from '$lib/lifecycle/Interval.svelte';
  import PushNotificationSync from './PushNotificationSync.svelte';
  import type { ProjectionHandler } from '$lib/eventBus.svelte';
  import { presencePreference } from '$lib/state/presencePreference.svelte';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';

  const reconciliationIntervalMs = 60_000;
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
      const notificationPreferences = getServerNotificationPreferences(instance.id);
      const viewer = stores.currentUser?.user;
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
        const hasUnreadCreation = stores.notifications.occurrences.some(
          (row) => pendingCreations.includes(row.id) && row.unread
        );
        pendingCreations.length = 0;
        checkingSound = false;
        if (
          !readSucceeded ||
          !active ||
          !stores.isAuthenticated ||
          stores.currentUser?.user?.id !== viewer?.id ||
          presencePreference.effectiveStatus === PresenceStatus.DO_NOT_DISTURB ||
          !hasUnreadCreation
        )
          return;
        playNotificationSound(
          notificationPreferences.notificationSound,
          notificationPreferences.notificationSoundFilters
        );
      }

      const handler: ProjectionHandler = (event) => {
        const semantic = event.event?.event;
        if (semantic?.case !== 'notificationOccurrencesChanged') return;
        const notificationId = semantic.value.createdNotificationId;
        if (!notificationId || handledNotificationIds.includes(notificationId)) return;
        // Remember suppressed hints too. A duplicate must not sound after DND ends.
        handledNotificationIds.push(notificationId);
        if (handledNotificationIds.length > rememberedNotificationIds)
          handledNotificationIds.shift();
        if (presencePreference.effectiveStatus === PresenceStatus.DO_NOT_DISTURB) return;
        pendingCreations.push(notificationId);
        if (pendingCreations.length > rememberedNotificationIds) pendingCreations.shift();
        // Several causes can describe one activity. Play once per completed batch.
        if (!checkingSound) void soundForCreations();
      };

      bus.projectionHandlers.add(handler);
      cleanups.push(() => {
        active = false;
        bus.projectionHandlers.delete(handler);
      });
    }

    return () => {
      for (const fn of cleanups) fn();
    };
  });

  function appBadgeIntent(): AppBadgeIntent | null {
    let unreadOccurrenceCount = 0;

    for (const instance of serverRegistry.servers) {
      const stores = serverRegistry.getStore(instance.id);
      if (!stores.isAuthenticated) continue;
      if (!stores.notifications.hasLoaded) return null;
      unreadOccurrenceCount += stores.notifications.unreadNotificationCount;
    }

    if (unreadOccurrenceCount > 0) return { kind: 'count', count: unreadOccurrenceCount };
    return { kind: 'clear' };
  }

  function syncAppBadge() {
    const intent = appBadgeIntent();
    if (intent) void updateAppBadge(intent);
  }

  // Synchronize the external OS badge directly from authoritative notification stores.
  // Avoid clearing an existing badge until every authenticated store has loaded.
  $effect(syncAppBadge);

  // Declarative Web Push may apply an origin-only count without changing a store.
  // Reassert the existing aggregate when the worker reports a regular push.
  $effect(() => {
    return listenForAppBadgeRefresh(syncAppBadge);
  });
</script>

{#each serverRegistry.servers as instance (instance.id)}
  {@const stores = serverRegistry.getStore(instance.id)}
  {#if stores.isAuthenticated}
    {#if stores.currentUser?.user?.id}
      {#key stores.currentUser.user.id}
        <PushNotificationSync
          serverUrl={instance.url}
          recipientId={stores.currentUser.user.id}
          notifications={stores.notifications}
        />
      {/key}
    {/if}
    <!-- Core NATS invalidations are latency hints; the notification stream is authoritative. -->
    <Interval
      milliseconds={reconciliationIntervalMs}
      ontick={() => void stores.notifications.reconcile()}
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
