<!--
@component

Handles real-time notification synchronization across all authenticated instances
and installed-app badge updates.

**Responsibilities:**
- Listens for live notification transitions attached to authoritative projection replacements
- Plays the user's selected sound for non-silent creations
- Shows an exact DM count or a flag for other/incompletely loaded pending notifications

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
  import { NotificationReason } from '$lib/api-client/notifications';
  import type { ProjectionHandler } from '$lib/eventBus.svelte';
  import { RealtimeProjectionNotificationAction } from '@chatto/api-types/realtime/v1/realtime_pb';

  // Subscribe to notification events on all authenticated instance buses.
  // Uses the event bus manager directly (not Svelte context) to handle all instances.
  $effect(() => {
    const cleanups: (() => void)[] = [];

    for (const instance of serverRegistry.servers) {
      const stores = serverRegistry.getStore(instance.id);
      if (!stores.isAuthenticated) continue;

      const bus = eventBusManager.getBus(instance.id);
      if (!bus) continue;

      const handler: ProjectionHandler = (event) => {
        for (const operation of event.operations) {
          if (operation.operation.case !== 'notificationsReplace') continue;
          const change = operation.operation.value.change;
          if (change?.action === RealtimeProjectionNotificationAction.CREATED && !change.silent) {
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
    let dmCount = 0;
    let hasNotification = false;
    let allStoresLoaded = true;
    let hasCompleteNotificationPages = true;

    for (const instance of serverRegistry.servers) {
      const stores = serverRegistry.getStore(instance.id);
      if (!stores.isAuthenticated) continue;

      const unreadGroups = stores.notifications.groups.filter((group) => group.unread);
      const notificationTotal = stores.notifications.unreadNotificationCount;
      dmCount += unreadGroups.filter((group) =>
        group.reasons.includes(NotificationReason.DIRECT_MESSAGE)
      ).length;
      if (notificationTotal > 0) hasNotification = true;
      if (!stores.notifications.hasLoaded) {
        allStoresLoaded = false;
        hasCompleteNotificationPages = false;
      } else if (notificationTotal !== unreadGroups.length) {
        hasCompleteNotificationPages = false;
      }
    }

    if (dmCount > 0 && hasCompleteNotificationPages) return { kind: 'count', count: dmCount };
    if (hasNotification) return { kind: 'flag' };
    if (!allStoresLoaded) return null;
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

  // KV expiry may not produce a watcher mutation. Refresh each authoritative
  // Inbox at its next server-provided expiry boundary so long-lived tabs do
  // not retain stale groups or badge counts.
  $effect(() => {
    const timers: ReturnType<typeof setTimeout>[] = [];
    for (const instance of serverRegistry.servers) {
      const stores = serverRegistry.getStore(instance.id);
      if (!stores.isAuthenticated || !stores.notifications.nextInboxExpiryAt) continue;
      const boundary = new Date(stores.notifications.nextInboxExpiryAt).getTime() + 50;
      const schedule = () => {
        const remaining = boundary - Date.now();
        if (remaining <= 0) {
          void stores.notifications.fetch();
          return;
        }
        timers.push(setTimeout(schedule, Math.min(remaining, 2_147_483_647)));
      };
      schedule();
    }
    return () => {
      for (const timer of timers) clearTimeout(timer);
    };
  });
</script>
