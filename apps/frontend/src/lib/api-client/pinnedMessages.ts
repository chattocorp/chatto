import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import type { PinnedMessage } from '@chatto/api-types/api/v1/rooms_pb';
import { authHeaders, createChattoClient, handleAuthError, type ConnectAPIConfig } from './connect';

export type PinnedMessagesPage = {
  items: PinnedMessage[];
  activeMessageEventIds: string[];
  totalCount: number;
  hasMore: boolean;
};

export function createPinnedMessagesAPI(config: ConnectAPIConfig) {
  const rooms = createChattoClient(RoomService, config);
  const headers = () => authHeaders(config);
  return {
    async list(roomId: string, limit: number, offset: number): Promise<PinnedMessagesPage> {
      try {
        const response = await rooms.listPinnedMessages(
          { roomId, page: { limit, offset } },
          { headers: headers() }
        );
        return {
          items: response.pinnedMessages,
          activeMessageEventIds: response.activeMessageEventIds,
          totalCount: Number(response.page?.totalCount ?? response.pinnedMessages.length),
          hasMore: response.page?.hasMore ?? false
        };
      } catch (error) {
        return handleAuthError(config, error);
      }
    },
    async create(roomId: string, messageEventId: string): Promise<PinnedMessage | null> {
      try {
        const response = await rooms.createPinnedMessage(
          { roomId, messageEventId },
          { headers: headers() }
        );
        return response.pinnedMessage ?? null;
      } catch (error) {
        return handleAuthError(config, error);
      }
    },
    async remove(roomId: string, messageEventId: string): Promise<void> {
      try {
        await rooms.deletePinnedMessage({ roomId, messageEventId }, { headers: headers() });
      } catch (error) {
        return handleAuthError(config, error);
      }
    }
  };
}

export type PinnedMessagesAPI = ReturnType<typeof createPinnedMessagesAPI>;
