<!--
@component

Handles real-time notification synchronization across all authenticated instances
and PWA badge updates.

**Responsibilities:**
- Listens for live notification transitions attached to authoritative projection replacements
- Plays the user's selected sound for non-silent creations
- Updates PWA dock badge based on aggregated pending-notification count

Include this component once in the chat layout (unconditionally).
-->
<script lang="ts">
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { eventBusManager } from '$lib/state/server/eventBus.svelte';
  import { userPreferences } from '$lib/state/userPreferences.svelte';
  import { playNotificationSound } from '$lib/audio/notificationSounds';
  import {
    updateBadge,
    clearBadge,
    syncServiceWorkerNotificationBadgeState,
    type AppBadgeIntent
  } from '$lib/notifications/appBadge';
  import type { ProjectionHandler } from '$lib/eventBus.svelte';
  import { RealtimeProjectionNotificationAction } from '@chatto/api-types/realtime/v1/realtime_pb';
  import { NotificationItemKind } from '$lib/api-client/notifications';

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

  let badgeState = $derived.by((): { intent: AppBadgeIntent; allStoresLoaded: boolean } => {
    let dmCount = 0;
    let canUseNumericDmCount = true;
    let hasNotification = false;
    let allStoresLoaded = true;

    for (const instance of serverRegistry.servers) {
      const stores = serverRegistry.getStore(instance.id);
      if (!stores.isAuthenticated) continue;
      if (!stores.notifications.hasLoaded) allStoresLoaded = false;

      const notifications = stores.notifications.notifications;
      const notificationTotal = stores.notifications.unreadNotificationCount;
      dmCount += notifications.filter((n) => n.kind === NotificationItemKind.DirectMessage).length;
      if (notificationTotal > notifications.length) {
        canUseNumericDmCount = false;
      }
      if (notifications.length > 0 || notificationTotal > 0) {
        hasNotification = true;
      }
    }

    if (dmCount > 0 && canUseNumericDmCount) {
      return { intent: { kind: 'count', count: dmCount }, allStoresLoaded };
    }
    if (hasNotification) return { intent: { kind: 'flag' }, allStoresLoaded };
    return { intent: { kind: 'clear' }, allStoresLoaded };
  });

  // Update PWA dock badge based on pending notifications only. Plain unread
  // rooms stay in-app so users can choose notification levels for important rooms.
  $effect(() => {
    if (badgeState.intent.kind === 'clear' && !badgeState.allStoresLoaded) return;

    syncServiceWorkerNotificationBadgeState(badgeState.intent);

    if (badgeState.intent.kind !== 'clear') {
      updateBadge(badgeState.intent);
    } else {
      clearBadge();
    }
  });
</script>
