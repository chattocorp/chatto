<!-- @component Shows native alerts for new eligible occurrences, loading previews on demand. -->
<script lang="ts">
  import type { ServerStateStore } from '@chatto/client/server/store';
  import type { NotificationOccurrenceItem } from '@chatto/client/api/notifications';
  import { createNotificationAPI } from '@chatto/client/api/notifications';
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
  let generation = 0;

  function clear() {
    generation++;
    clearDesktopNotificationOwner(owner);
  }

  /** Check current eligibility both before loading previews and before delivery. */
  function eligibleArrivals(rows: NotificationOccurrenceItem[]) {
    const notifications = store.notifications;
    const handled = notifications.knownHandledNotificationIds(rows.map((row) => row.id));
    return eligibleNotificationAlerts(store.serverId, store, rows).filter(
      (row) =>
        row.unread &&
        row.targetSupported &&
        !handled.has(row.id) &&
        notifications.canDisplayOccurrence(row) &&
        (!row.expiresAt || Date.parse(row.expiresAt) > Date.now())
    );
  }

  /** Preview reads belong to this alert batch; failures retain generic delivery. */
  async function showArrivals(rows: NotificationOccurrenceItem[]) {
    const started = generation;
    const consent = desktopNotifications.consentRevision;
    let hydrated: NotificationOccurrenceItem[];
    try {
      hydrated = await store.connection
        .getAPI(createNotificationAPI)
        .batchGetNotificationOccurrences(
          rows.map((row) => row.id),
          { includeContent: true }
        );
    } catch {
      // A failed preview must not drop the alert or reuse stale display content.
      hydrated = rows.map((row) => ({ ...row, content: undefined }));
    }
    if (
      !active ||
      started !== generation ||
      !desktopNotifications.enabled ||
      consent !== desktopNotifications.consentRevision ||
      !store.isAuthenticated
    )
      return;
    const eligible = eligibleArrivals(hydrated);
    for (const row of eligible) show(row);
    if (eligible.length) playNotificationAlertSound(store.serverId);
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
      store.onUserDeleted(clear),
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
    // Retain routing across pagination without keeping display text or message previews.
    const target = {
      ...row,
      actor: null,
      content: undefined,
      room: row.room ? { id: row.room.id, name: '' } : null
    };
    void showDesktopNotification(
      id,
      row.content ?? { title: 'Chatto', body: m('settings.notifications.title') },
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
    const eligible = eligibleArrivals(fresh);
    if (eligible.length) {
      void showArrivals(eligible).catch(() =>
        console.error('Could not deliver desktop notifications.')
      );
    }
  });
</script>
