import { segmentToServerId } from '$lib/navigation';
import type { AppUiState } from '$lib/state/appUi.svelte';

type NotificationUiController = Pick<AppUiState, 'disableRoomCallWideFor' | 'requestSidebarReveal'>;

export function notificationRoomTargetFromPathname(
  pathname: string
): { serverId: string; roomId: string } | null {
  const [, chatSegment, serverSegment, roomSegment] = pathname.split('/');
  if (chatSegment !== 'chat' || !serverSegment || !roomSegment) return null;

  const decodedServerSegment = decodePathSegment(serverSegment);
  const roomId = decodePathSegment(roomSegment);
  if (!decodedServerSegment || !roomId) return null;

  const serverId = segmentToServerId(decodedServerSegment);
  if (!serverId) return null;

  return { serverId, roomId };
}

/** Prepare room UI before following a push notification URL. */
export function prepareUiForNotificationPath(
  appUi: NotificationUiController,
  pathname: string
): void {
  const target = notificationRoomTargetFromPathname(pathname);
  if (target) prepareUiForNotificationTarget(appUi, target.serverId, target);
}

/** Prepare room UI before opening an in-app notification target. */
export function prepareUiForNotificationTarget(
  appUi: NotificationUiController,
  serverId: string,
  target: { roomId: string | null }
): void {
  if (!target.roomId) return;
  appUi.disableRoomCallWideFor(serverId, target.roomId);
  appUi.requestSidebarReveal(serverId, target.roomId);
}

function decodePathSegment(segment: string): string | null {
  try {
    return decodeURIComponent(segment);
  } catch {
    return null;
  }
}
