/** Shared local alert policy and audio. Callers choose live hints or polled arrivals as their source. */
import type { ServerStateStore } from '@chatto/client/server/store';
import {
  NotificationAttentionLevel,
  type NotificationOccurrenceItem
} from '@chatto/client/api/notifications';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { presencePreferences } from '$lib/state/server/presencePreference';
import { serverUi } from '$lib/state/server/serverUi';
import { getServerNotificationPreferences } from '$lib/state/serverNotificationPreferences.svelte';
import { playNotificationSound } from '$lib/audio/notificationSounds';

/** DND is checked at arrival and again after a pending resource read. */
export function notificationAlertsMuted(serverId: string, store: ServerStateStore): boolean {
  return Boolean(
    store.viewerId &&
    presencePreferences.get({ serverId, userId: store.viewerId }).status ===
      PresenceStatus.DO_NOT_DISTURB
  );
}

/** Important unread activity outside a currently read surface may interrupt the user. */
export function eligibleNotificationAlerts(
  serverId: string,
  store: ServerStateStore,
  rows: readonly NotificationOccurrenceItem[]
): NotificationOccurrenceItem[] {
  if (!store.isAuthenticated || notificationAlertsMuted(serverId, store)) return [];
  return rows.filter(
    (row) =>
      row.attentionLevel === NotificationAttentionLevel.IMPORTANT &&
      serverUi(store).attention.needsAttention(row)
  );
}

/** Play the originating server's selected sound once for a completed eligible batch. */
export function playNotificationAlertSound(serverId: string): void {
  const preferences = getServerNotificationPreferences(serverId);
  playNotificationSound(preferences.notificationSound, preferences.notificationSoundFilters);
}
