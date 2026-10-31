/** Shared translated activity sentences for the notification list and native alerts. */
import { NotificationSignalKind } from '@chatto/client/api/notifications';
import { m } from '$lib/i18n/messages';

/**
 * The caller supplies a plain account name or a rich account token, and rendered
 * reaction emoji. Grouping and account presentation remain with the caller.
 */
export function notificationActivitySummary(
  kind: NotificationSignalKind | undefined,
  actor?: string,
  reaction?: { emoji: string; multipleActors?: boolean }
): string {
  if (kind === NotificationSignalKind.REACTION) {
    const name = actor ?? m('common.deleted_user');
    if (reaction?.emoji) {
      const values = { actor: name, emoji: reaction.emoji };
      return reaction.multipleActors
        ? m('chat.notifications.summary.reaction_group', values)
        : m('chat.notifications.summary.reaction', values);
    }
    return m('chat.notifications.summary.activity', { actor: name });
  }
  if (!actor) return m('chat.notifications.activity');
  switch (kind) {
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
    default:
      return m('chat.notifications.summary.activity', { actor });
  }
}
