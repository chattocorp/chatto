<!--
@component

Handles real-time notification synchronization across all authenticated instances
and installed-app badge updates.

**Responsibilities:**
- Listens for live notification transitions attached to authoritative projection replacements
- Plays the user's selected sound for non-silent creations
- Updates the installed app badge from the aggregate pending-notification count

Include this component once in the chat layout (unconditionally).
-->
<script lang="ts">
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { eventBusManager } from '$lib/state/server/eventBus.svelte';
  import { userPreferences } from '$lib/state/userPreferences.svelte';
  import { playNotificationSound } from '$lib/audio/notificationSounds';
  import { listenForAppBadgeRefresh, updateAppBadge } from '$lib/notifications/appBadge';
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

  function syncAppBadge() {
    let notificationCount = 0;
    let allStoresLoaded = true;

    for (const instance of serverRegistry.servers) {
      const stores = serverRegistry.getStore(instance.id);
      if (!stores.isAuthenticated) continue;

      notificationCount += stores.notifications.unreadNotificationCount;
      if (!stores.notifications.hasLoaded) allStoresLoaded = false;
    }

    if (notificationCount === 0 && !allStoresLoaded) return;
    void updateAppBadge(notificationCount);
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
