/** Canonical user and room destinations. Room IDs remain the API identity. */
import { resolve } from '$app/paths';
import type { ResolvedPathname } from '$app/types';
import { RoomKind } from '@chatto/client/api/roomDirectory';
import { serverRegistry } from '$lib/client';
import { serverIdToSegment } from '$lib/navigation';

/** The authoritative participant data needed to select a conversation URL. */
export type ConversationStore = {
  readonly accountId: string | null;
  readonly projection: {
    readonly rooms: ReadonlyMap<
      string,
      {
        readonly room?: { readonly kind?: RoomKind };
        readonly memberUserIds: readonly string[];
      }
    >;
  };
};

/** Select a user only from complete participant IDs, including deleted accounts. */
export function conversationUserId(
  store: ConversationStore | undefined,
  roomId: string
): string | null {
  const entry = store?.projection.rooms.get(roomId);
  const viewerId = store?.accountId;
  if (!entry || entry.room?.kind !== RoomKind.DM || !viewerId) return null;
  const ids = entry.memberUserIds;
  if (!ids.includes(viewerId) || ids.length > 2 || ids.length === 0) return null;
  return ids.find((id) => id !== viewerId) ?? viewerId;
}

/** Find an already projected conversation without creating one. */
export function userConversationRoomId(
  store: ConversationStore | undefined,
  userId: string | undefined
): string | null {
  if (!store || !userId) return null;
  for (const [roomId] of store.projection.rooms) {
    if (conversationUserId(store, roomId) === userId) return roomId;
  }
  return null;
}

/** Optional destination within a conversation. */
export type ConversationTarget = {
  threadId?: string | null;
  messageId?: string | null;
};

/** Append the shared thread/message suffix to a resolved conversation base. */
function withTarget(
  base: ResolvedPathname,
  { threadId, messageId }: ConversationTarget
): ResolvedPathname {
  return `${base}${threadId ? `/${encodeURIComponent(threadId)}` : ''}${messageId ? `/m/${encodeURIComponent(messageId)}` : ''}` as ResolvedPathname;
}

/** User-page path, optionally targeting a thread or message. */
export function userPath(
  serverId: string,
  userId: string,
  target: ConversationTarget = {}
): ResolvedPathname {
  const base = resolve('/chat/[serverId]/user/[userId]', {
    serverId: serverIdToSegment(serverId),
    userId
  });
  return withTarget(base, target);
}

/** Use a user page for supported DMs, or a room route until metadata is available. */
export function conversationPath(
  serverId: string,
  roomId: string,
  {
    store = serverRegistry.tryGetStore(serverId),
    ...target
  }: ConversationTarget & {
    store?: ConversationStore;
  } = {}
): ResolvedPathname {
  const userId = conversationUserId(store, roomId);
  if (userId) return userPath(serverId, userId, target);
  const base = resolve('/chat/[serverId]/[roomId]', {
    serverId: serverIdToSegment(serverId),
    roomId
  });
  return withTarget(base, target);
}
