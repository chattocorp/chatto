<!-- @component Shows native alerts from the existing notification store. Owns no transport or timer. -->
<script lang="ts">
  import type { ServerStateStore } from '@chatto/client/server/store';
  import type { NotificationOccurrenceItem } from '@chatto/client/api/notifications';
  import { notificationTarget } from '@chatto/client/server/notifications';
  import { goto } from '$app/navigation';
  import { m } from '$lib/i18n/messages';
  import { serverUi } from '$lib/state/server/serverUi';
  import {
    eligibleNotificationAlerts,
    playNotificationAlertSound
  } from '$lib/notifications/notificationAlerts';
  import { notificationPath } from '$lib/notificationPath';
  import {
    navigateNotification,
    prepareUiForNotificationTarget
  } from '$lib/notifications/notificationNavigationUi';
  import { getAppUiState } from '$lib/state/appUi.svelte';
  import { NotificationArrivals } from '$lib/desktop/notificationArrivals';
  import {
    desktopNotifications,
    showDesktopNotification,
    clearDesktopNotificationOwner,
    reconcileDesktopNotifications
  } from '$lib/desktop/notifications.svelte';

  const { store }: { store: ServerStateStore } = $props();
  const appUi = getAppUiState();
  const arrivals = new NotificationArrivals();
  const owner = {};
  let active = true;

  function clear() {
    clearDesktopNotificationOwner(owner);
  }

  // Store listeners own privacy cleanup for this account-scoped component.
  $effect(() => {
    active = true;
    const reset = () => {
      clear();
      arrivals.reset();
    };
    const stops = [
      store.onReset((event) => {
        if (event.privacy) reset();
      }),
      store.onRoomAccessLost(clear),
      store.onPermissionsChanged(clear),
      store.onSessionEnded(reset)
    ];
    return () => {
      active = false;
      clear();
      for (const stop of stops) stop();
    };
  });

  async function open(retained: NotificationOccurrenceItem) {
    if (!active || !store.isAuthenticated) return;
    const row = store.notifications.occurrences.find((row) => row.id === retained.id) ?? retained;
    if (
      !row.targetSupported ||
      !store.notifications.canDisplayOccurrence(row) ||
      (row.expiresAt && Date.parse(row.expiresAt) <= Date.now())
    )
      return;
    const target = notificationTarget(row);
    prepareUiForNotificationTarget(appUi, store.serverId, target);
    if (target.roomId && target.eventId)
      serverUi(store).pendingHighlights.set(
        target.roomId,
        target.threadRootId,
        target.eventId,
        row.id
      );
    await navigateNotification(store, row, () => goto(notificationPath(store.serverId, row)));
  }

  function show(row: NotificationOccurrenceItem) {
    const id = crypto.randomUUID();
    // Retain routing fields across pagination, without copying actor or room display text.
    const target = { ...row, actor: null, room: row.room ? { id: row.room.id, name: '' } : null };
    void showDesktopNotification(
      id,
      m('settings.notifications.title'),
      () => {
        void open(target).catch(() => console.error('Could not open the notification.'));
      },
      { owner, occurrenceId: row.id, expiresAt: row.expiresAt }
    ).catch(() => console.error('Could not show a desktop notification.'));
  }

  // Showing OS alerts is an external side effect. Both polls and live reads update this store.
  $effect(() => {
    const notifications = store.notifications;
    if (
      !store.isAuthenticated ||
      !notifications.hasLoaded ||
      notifications.loading ||
      notifications.error
    )
      return;
    const rows = notifications.occurrences;
    const fresh = arrivals.read(rows);
    void notifications.pushRevision;
    reconcileDesktopNotifications(owner, notifications);
    if (!desktopNotifications.enabled) return;
    const eligible = eligibleNotificationAlerts(store.serverId, store, fresh).filter(
      (row) => row.targetSupported
    );
    for (const row of eligible) show(row);
    if (eligible.length) {
      playNotificationAlertSound(store.serverId);
    }
  });
</script>
