import { MessageSearchService } from '@chatto/api-types/api/v1/message_search_connect';
import { MessageSearchOrder, MessageSearchState } from '@chatto/api-types/api/v1/message_search_pb';
import { authHeaders, createChattoClient, handleAuthError, type ConnectAPIConfig } from './connect';
import { createRoomDirectoryAPI } from './roomDirectory';
import { createUserAPI, type UserSummary } from './users';

export { MessageSearchOrder, MessageSearchState };

export type MessageSearchStatus = {
  state: MessageSearchState;
  retryAfterMs: number | null;
};

export type MessageSearchResult = {
  id: string;
  roomId: string;
  roomName: string | null;
  actorId: string;
  actor: UserSummary | null;
  body: string;
  createdAt: string;
  threadRootEventId: string | null;
  attachmentCount: number;
};

export type MessageSearchPage = {
  results: MessageSearchResult[];
  nextCursor: string | null;
};

export type MessageSearchInput = {
  query: string;
  roomId?: string;
  authorId?: string;
  order: MessageSearchOrder;
  cursor?: string | null;
};

export function createMessageSearchAPI(config: ConnectAPIConfig) {
  const search = createChattoClient(MessageSearchService, config);
  const rooms = createRoomDirectoryAPI(config);
  const users = createUserAPI(config);
  const headers = () => authHeaders(config);

  return {
    async getStatus(): Promise<MessageSearchStatus> {
      try {
        const response = await search.getStatus({}, { headers: headers() });
        return {
          state: response.state,
          retryAfterMs: response.retryAfter
            ? Number(response.retryAfter.seconds) * 1_000 + response.retryAfter.nanos / 1_000_000
            : null
        };
      } catch (error) {
        return handleAuthError(config, error);
      }
    },

    async searchMessages(input: MessageSearchInput): Promise<MessageSearchPage> {
      try {
        const response = await search.searchMessages(
          {
            query: input.query,
            roomId: input.roomId,
            authorId: input.authorId,
            order: input.order,
            pageSize: 20,
            cursor: input.cursor ?? ''
          },
          { headers: headers() }
        );

        const roomIds = [...new Set(response.messages.map((message) => message.roomId))];
        const actorIds = [
          ...new Set(response.messages.map((message) => message.actorId).filter(Boolean))
        ];
        const [roomRows, userRows] = await Promise.all([
          rooms.batchGetRooms(roomIds).catch(() => []),
          users.batchGetUsers(actorIds).catch(() => [])
        ]);
        const roomNames = new Map(roomRows.map((room) => [room.id, room.name]));
        const actors = new Map(userRows.map((user) => [user.id, user]));

        return {
          results: response.messages.map((message) => ({
            id: message.id,
            roomId: message.roomId,
            roomName: roomNames.get(message.roomId) ?? null,
            actorId: message.actorId,
            actor: actors.get(message.actorId) ?? null,
            body: message.body ?? '',
            createdAt: message.createdAt?.toDate().toISOString() ?? '',
            threadRootEventId: message.threadRootEventId || null,
            attachmentCount: message.attachments.length
          })),
          nextCursor: response.nextCursor || null
        };
      } catch (error) {
        return handleAuthError(config, error);
      }
    }
  };
}

export type MessageSearchAPI = ReturnType<typeof createMessageSearchAPI>;
