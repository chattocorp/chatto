import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import type { PinnedMessage } from '@chatto/api-types/api/v1/rooms_pb';
import { createChattoClient, type ConnectAPIConfig, minimumCursorHeaders } from './connect';
import { timelineUsersForMessages } from './roomTimeline';

export type PinnedMessagesPage = {
  items: PinnedMessage[];
  totalCount: number;
  hasMore: boolean;
  latestPinMarker: string;
};

export function createPinnedMessagesAPI(config: ConnectAPIConfig) {
  const rooms = createChattoClient(RoomService, config);
  return {
    async list(roomId: string, limit: number, offset: number, minimumCursor?: string): Promise<PinnedMessagesPage> {
      const response = await rooms.listPinnedMessages(
        { roomId, page: { limit, offset } },
        {
          headers: minimumCursorHeaders(minimumCursor),
          ...(minimumCursor ? { timeoutMs: 10_000 } : {})
        }
      );
      await timelineUsersForMessages(
        config,
        response.pinnedMessages.flatMap((item) => (item.message ? [item.message] : [])),
        minimumCursor
      );
      return {
        items: response.pinnedMessages,
        totalCount: Number(response.page?.totalCount ?? response.pinnedMessages.length),
        hasMore: response.page?.hasMore ?? false,
        latestPinMarker: response.latestPinMarker
      };
    },
    async create(roomId: string, messageEventId: string): Promise<PinnedMessage | null> {
      const response = await rooms.createPinnedMessage({ roomId, messageEventId });
      return response.pinnedMessage ?? null;
    },
    async remove(roomId: string, messageEventId: string): Promise<void> {
      await rooms.deletePinnedMessage({ roomId, messageEventId });
    }
  };
}

export type PinnedMessagesAPI = ReturnType<typeof createPinnedMessagesAPI>;
