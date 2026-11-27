import { resolve } from '$app/paths';
import type { ResolvedPathname } from '$app/types';
import type { NotificationOccurrenceItem } from '@chatto/client/api/notifications';
import { notificationTarget } from '@chatto/client/server/notifications';
import { serverIdToSegment } from '$lib/navigation';
import { conversationPath } from '$lib/navigation/conversationPath';

/**
 * Build a clean (no `?highlight=`) destination path for a notification.
 * Use this with `PendingHighlightStore.set()` to deliver the highlight
 * intent without polluting the URL.
 */
export function notificationPath(
  serverId: string,
  notification: NotificationOccurrenceItem
): ResolvedPathname {
  const serverSegment = serverIdToSegment(serverId);
  const target = notificationTarget(notification);

  if (!target.roomId) {
    return resolve('/chat/[serverId]', { serverId: serverSegment });
  }
  return conversationPath(serverId, target.roomId, {
    threadId: target.isDM ? null : target.threadRootId
  });
}
