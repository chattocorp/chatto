<!--
@component

Handles real-time notification synchronization across all authenticated instances
and PWA badge updates.

**Responsibilities:**
- Listens for new notifications on all instance event buses and plays the user's selected sound
- Syncs notification dismissals from other devices
- Syncs room-level mention indicator clears from other devices
- Updates PWA dock badge based on aggregated notification count and unread state

Include this component once in the chat layout (unconditionally).
-->
<script lang="ts">
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { eventBusManager } from '$lib/state/server/eventBus.svelte';
  import { userPreferences } from '$lib/state/userPreferences.svelte';
  import { playNotificationSound } from '$lib/audio/notificationSounds';
  import { updateBadge, setFlagBadge, clearBadge } from '$lib/notifications/appBadge';
  import type { EventHandler } from '$lib/eventBus.svelte';

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

        if (event.event.__typename === 'NotificationCreatedEvent') {
          notificationStore.addNotification();
          playNotificationSound(userPreferences.notificationSound);
        }

        if (event.event.__typename === 'NotificationDismissedEvent') {
          notificationStore.removeNotification(event.event.notificationId);
        }

        // The server-side mention KV flag was cleared (because the user
        // dismissed a mention notification or marked the room as read on any
        // device). Drop the per-room `hasMention` flag here so the orange dot
        // disappears across every tab and every connected server — not just
        // the active one. RoomList's local markRead effect handles the
        // active-server-active-room case redundantly; this is the
        // cross-tab/cross-server path.
        if (event.event.__typename === 'MentionStatusClearedEvent') {
          stores.rooms.clearMention(event.event.mscRoomId);
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
      return sum + stores.notifications.count;
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
    if (totalNotificationCount > 0) {
      updateBadge(totalNotificationCount);
    } else if (hasAnyUnread) {
      setFlagBadge();
    } else {
      clearBadge();
    }
  });
</script>
