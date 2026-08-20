<!--
@component

Handles real-time notification synchronization across all authenticated instances
and installed-app badge updates.

**Responsibilities:**
- Listens for live notification transitions attached to authoritative projection replacements
- Plays the user's selected sound for non-silent creations
- Reconciles the installed-app badge from authoritative unread occurrence counts

Include this component once in the application root so signed-out pages also clear stale badges.
-->
<script lang="ts">
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { eventBusManager } from '$lib/state/server/eventBus.svelte';
  import { userPreferences } from '$lib/state/userPreferences.svelte';
  import { playNotificationSound } from '$lib/audio/notificationSounds';
  import {
    listenForAppBadgeRefresh,
    updateAppBadge,
    type AppBadgeIntent
  } from '$lib/notifications/appBadge';
  import Deadline from '$lib/lifecycle/Deadline.svelte';
  import Interval from '$lib/lifecycle/Interval.svelte';
  import type { ProjectionHandler } from '$lib/eventBus.svelte';

  const reconciliationIntervalMs = 60_000;
  const rememberedSoundEvents = 256;

  // Subscribe to notification events on all authenticated instance buses.
  // Uses the event bus manager directly (not Svelte context) to handle all instances.
  $effect(() => {
    const cleanups: (() => void)[] = [];

    for (const instance of serverRegistry.servers) {
      const stores = serverRegistry.getStore(instance.id);
      if (!stores.isAuthenticated) continue;

      const bus = eventBusManager.getBus(instance.id);
      if (!bus) continue;
      const soundedEventIds: string[] = [];

      const handler: ProjectionHandler = (event) => {
        for (const operation of event.operations) {
          if (operation.operation.case !== 'notificationOccurrencesReplace') continue;
          if (
            event.id &&
            operation.operation.value.playNotificationSound &&
            !soundedEventIds.includes(event.id)
          ) {
            soundedEventIds.push(event.id);
            if (soundedEventIds.length > rememberedSoundEvents) {
              soundedEventIds.shift();
            }
            playNotificationSound(
              userPreferences.notificationSound,
              userPreferences.notificationSoundFilters
            );
          }
        }
      };

      bus.projectionHandlers.add(handler);
      cleanups.push(() => bus.projectionHandlers.delete(handler));
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
