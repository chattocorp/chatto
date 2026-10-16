import type { ServerStateStore } from '@chatto/client/server/store';
import { userConversationRoomId } from './conversationPath';

const CHAT_ROOM_ROUTE_IDS = new Set([
  '/chat/[serverId]/[roomId]',
  '/chat/[serverId]/[roomId]/[threadId]',
  '/chat/[serverId]/[roomId]/[threadId]/m/[messageId]'
]);

export function chatRoomIdFromRoute(
  routeId: string | null,
  roomId: string | undefined,
  userId?: string,
  store?: ServerStateStore
): string | null {
  if (routeId?.startsWith('/chat/[serverId]/user/[userId]')) {
    return userConversationRoomId(store, userId);
  }
  if (!routeId || !CHAT_ROOM_ROUTE_IDS.has(routeId)) return null;
  return roomId || null;
}
