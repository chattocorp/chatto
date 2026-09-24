const CHAT_ROOM_ROUTE_IDS = new Set([
  '/chat/[serverId]/[roomId]',
  '/chat/[serverId]/[roomId]/[threadId]',
  '/chat/[serverId]/[roomId]/[threadId]/m/[messageId]'
]);

/** Only these routes can render from bounded saved text before viewer verification. */
export function supportsSavedViewRoute(routeId: string | null): boolean {
  return routeId === '/chat/[serverId]/[roomId]' || routeId === '/chat/[serverId]/overview';
}

export function chatRoomIdFromRoute(
  routeId: string | null,
  roomId: string | undefined
): string | null {
  if (!routeId || !CHAT_ROOM_ROUTE_IDS.has(routeId)) return null;
  return roomId || null;
}
