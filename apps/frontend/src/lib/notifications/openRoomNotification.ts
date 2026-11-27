/**
 * Open the newest unread notification of one room at one attention level. A
 * room notification badge does this, so the viewer sees the activity that the
 * badge announced, not only the room.
 */

import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import type { NotificationAttentionLevel } from '@chatto/client/api/notifications';
import { notificationTarget } from '@chatto/client/server/notifications';
import { notificationPath } from '$lib/notificationPath';
import type { AppUiState } from '$lib/state/appUi.svelte';
import type { ServerScope } from '$lib/state/server/scope.svelte';
import { serverUi } from '$lib/state/server/serverUi';
import {
  navigateNotification,
  prepareUiForNotificationTarget
} from '$lib/notifications/notificationNavigationUi';

/**
 * Navigate to the source of a room's newest notification at `attentionLevel`
 * and highlight it. Without a loaded notification, open the notification list,
 * unless the server reports that the room has none.
 */
export async function openRoomNotification(
  scope: Pick<ServerScope, 'serverId' | 'store' | 'isCurrent'>,
  appUi: Pick<AppUiState, 'disableRoomCallWideFor'>,
  roomId: string,
  options: { isDM: boolean; attentionLevel: NotificationAttentionLevel }
): Promise<void> {
  const ui = serverUi(scope.store);
  const lookup = await ui.attention.resolveRoomNotification(roomId, options);
  if (!scope.isCurrent()) return;
  const notification = lookup.notification;

  if (!notification) {
    if (!lookup.ok || lookup.totalCount !== 0) {
      await goto(resolve('/chat/notifications'));
    }
    return;
  }

  const target = notificationTarget(notification);
  prepareUiForNotificationTarget(appUi, scope.serverId, target);
  if (target.eventId && target.roomId) {
    ui.pendingHighlights.set(target.roomId, target.threadRootId, target.eventId, notification.id);
  }

  const path = notificationPath(scope.serverId, notification);
  await navigateNotification(scope.store, notification, () => goto(path));
}
