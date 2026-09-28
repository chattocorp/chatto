import { resolve } from '$app/paths';
import type { NotificationOccurrenceItem } from '@chatto/client/api/notifications';
import { notificationTarget } from '@chatto/client/server/notifications';
import { serverIdToSegment } from '$lib/navigation';

/**
 * Build a clean (no `?highlight=`) destination path for a notification.
 * Use this with `PendingHighlightStore.set()` to deliver the highlight
 * intent without polluting the URL.
 */
export function notificationPath(
  serverId: string,
  notification: NotificationOccurrenceItem
): string {
  const serverSegment = serverIdToSegment(serverId);
  const target = notificationTarget(notification);

  if (!target.roomId) {
    return resolve('/chat/[serverId]', { serverId: serverSegment });
  }
  // DMs are rooms on the server, so they use the standard room URL.
  if (target.threadRootId && !target.isDM) {
    return resolve('/chat/[serverId]/[roomId]/[threadId]', {
      serverId: serverSegment,
      roomId: target.roomId,
      threadId: target.threadRootId
    });
  }
  return resolve('/chat/[serverId]/[roomId]', {
    serverId: serverSegment,
    roomId: target.roomId
  });
}
