/** Native alert text from current local data. Never creates stores or requests content. */
import {
  NotificationSignalKind,
  type NotificationOccurrenceItem
} from '@chatto/client/api/notifications';
import type { ServerStateStore } from '@chatto/client/server/store';
import { formatAccountName } from '@chatto/client/timeline/accountName';
import { isMessagePostedEvent } from '@chatto/client/timeline/timelineEvents';
import { getReactionDisplay } from '$lib/emoji';
import { notificationActivitySummary } from '$lib/notifications/activitySummary';

/** Format the activity immediately, adding message text only if it is already loaded. */
export function desktopNotificationContent(
  store: ServerStateStore,
  row: NotificationOccurrenceItem
): { title: string; body: string } {
  const room = row.signalKind === NotificationSignalKind.DIRECT_MESSAGE ? '' : row.room?.name;
  let preview = '';
  if (row.room && row.eventId && store.notifications.canDisplayOccurrence(row)) {
    const event = store.knownMessage(row.room.id, row.eventId);
    if (
      event &&
      isMessagePostedEvent(event.event) &&
      !event.event.deletedAt &&
      event.actorResolution !== 'deleted'
    ) {
      preview = event.event.body ?? '';
    }
  }
  const actor =
    row.actor && !row.actor.deleted
      ? formatAccountName(row.actor.displayName || row.actor.login, row.actor)
      : undefined;
  return {
    title: notificationActivitySummary(row.signalKind, actor, {
      emoji: row.reactionEmoji ? getReactionDisplay(row.reactionEmoji) : ''
    }),
    body: [room ? `#${room}` : '', preview].filter(Boolean).join('\n')
  };
}
