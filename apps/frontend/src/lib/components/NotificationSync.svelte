<!--
@component

Handles real-time notification synchronization across all authenticated instances
and PWA badge updates.

**Responsibilities:**
- Listens for new notifications on all instance event buses and plays the user's selected sound
- Syncs notification dismissals from other devices
- Updates PWA dock badge based on aggregated notification count and unread state

Include this component once in the chat layout (unconditionally).
-->
<script lang="ts">
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { eventBusManager } from '$lib/state/server/eventBus.svelte';
  import { userPreferences } from '$lib/state/userPreferences.svelte';
  import { playNotificationSound } from '$lib/audio/notificationSounds';
  import {
    updateBadge,
    setFlagBadge,
    clearBadge,
    syncServiceWorkerUnreadBadgeState
  } from '$lib/notifications/appBadge';
  import type { EventEnvelope, EventHandler } from '$lib/eventBus.svelte';
  import { RoomEventKind, roomEventKind } from '$lib/render/eventKinds';

  function notificationCreatedEvent(event: EventEnvelope['event']): { silent?: boolean } | null {
    if (!event || !('silent' in event)) return null;
    return { silent: event.silent === true };
  }

  function notificationDismissedEvent(
    event: EventEnvelope['event']
  ): { notificationId: string } | null {
    if (!event || !('notificationId' in event) || typeof event.notificationId !== 'string') {
      return null;
    }
    return { notificationId: event.notificationId };
  }

  // Subscribe to notification events on all authenticated instance buses.
  // Uses the event bus manager directly (not Svelte context) to handle all instances.
  $effect(() => {
    const cleanups: (() => void)[] = [];

    for (const instance of serverRegistry.servers) {
      const stores = serverRegistry.getStore(instance.id);
      if (!stores.isAuthenticated) continue;

      const bus = eventBusManager.getBus(instance.id);
      if (!bus) continue;

      const notificationStore = stores.notifications;

      const handler: EventHandler = (event) => {
        if (!event.event) return;

        switch (roomEventKind(event.event)) {
          case RoomEventKind.NotificationCreated: {
            const notification = notificationCreatedEvent(event.event);
            if (!notification) break;
            void Promise.allSettled([
              notificationStore.addNotification(),
              stores.rooms.refreshNotificationCounts()
            ]);
            if (!notification.silent) {
              playNotificationSound(
                userPreferences.notificationSound,
                userPreferences.notificationSoundFilters
              );
            }
            break;
          }
          case RoomEventKind.NotificationDismissed: {
            const notification = notificationDismissedEvent(event.event);
            if (!notification) break;
            const roomId = notificationStore.removeNotification(notification.notificationId);
            if (roomId) {
              void stores.rooms.refreshNotificationCounts();
            } else if (!notificationStore.consumeLocalDismissal(notification.notificationId)) {
              void Promise.allSettled([
                notificationStore.fetch(),
                stores.rooms.refreshNotificationCounts()
              ]);
            }
            break;
          }
        }
      };

      bus.handlers.add(handler);
      cleanups.push(() => bus.handlers.delete(handler));
    }

    return () => {
      for (const fn of cleanups) fn();
    };
  });

  // Aggregate notification count and unread state across all authenticated instances.
  let totalNotificationCount = $derived(
    serverRegistry.servers.reduce((sum, instance) => {
      const stores = serverRegistry.getStore(instance.id);
      if (!stores.isAuthenticated) return sum;
      return (
        sum + Math.max(stores.notifications.unreadNotificationCount, stores.notifications.count)
      );
    }, 0)
  );

  let hasAnyUnread = $derived(
    serverRegistry.servers.some((instance) => {
      const stores = serverRegistry.getStore(instance.id);
      if (!stores.isAuthenticated) return false;
      return stores.roomUnread.hasAnyUnread;
    })
  );

  // Update PWA dock badge based on aggregated state
  $effect(() => {
    syncServiceWorkerUnreadBadgeState(hasAnyUnread);

    if (totalNotificationCount > 0) {
      updateBadge(totalNotificationCount);
    } else if (hasAnyUnread) {
      setFlagBadge();
    } else {
      clearBadge();
    }
  });
</script>
