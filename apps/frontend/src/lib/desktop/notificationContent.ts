/** Native alert text from current local data. Never creates stores or requests content. */
import {
  NotificationSignalKind,
  type NotificationOccurrenceItem
} from '@chatto/client/api/notifications';
import type { ServerStateStore } from '@chatto/client/server/store';
import { formatAccountName } from '@chatto/client/timeline/accountName';
import { isMessagePostedEvent } from '@chatto/client/timeline/timelineEvents';
import { m } from '$lib/i18n/messages';
import { getReactionDisplay } from '$lib/emoji';

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
  return {
    title: activitySummary(row),
    body: [room ? `#${room}` : '', preview].filter(Boolean).join('\n')
  };
}

/** Reuse the notification list's translated activity labels for plain-text OS alerts. */
function activitySummary(row: NotificationOccurrenceItem): string {
  const actor =
    row.actor && !row.actor.deleted
      ? formatAccountName(row.actor.displayName || row.actor.login, row.actor)
      : m('common.deleted_user');
  switch (row.signalKind) {
    case NotificationSignalKind.ROOM_INVITATION:
      return m('chat.notifications.summary.room_invitation', { actor });
    case NotificationSignalKind.DIRECT_MESSAGE:
      return m('chat.notifications.summary.direct_message', { actor });
    case NotificationSignalKind.REPLY:
      return m('chat.notifications.summary.reply', { actor });
    case NotificationSignalKind.DIRECT_MENTION:
    case NotificationSignalKind.ROLE_MENTION:
    case NotificationSignalKind.HERE:
    case NotificationSignalKind.ALL:
      return m('chat.notifications.summary.mention', { actor });
    case NotificationSignalKind.FOLLOWED_THREAD:
      return m('chat.notifications.summary.followed_thread', { actor });
    case NotificationSignalKind.ROOM_MESSAGE:
    case NotificationSignalKind.FOLLOWED_ROOM:
      return m('chat.notifications.summary.new_message', { actor });
    case NotificationSignalKind.REACTION:
      return row.reactionEmoji
        ? m('chat.notifications.summary.reaction', {
            actor,
            emoji: getReactionDisplay(row.reactionEmoji)
          })
        : m('chat.notifications.summary.activity', { actor });
    default:
      return m('chat.notifications.activity');
  }
}
