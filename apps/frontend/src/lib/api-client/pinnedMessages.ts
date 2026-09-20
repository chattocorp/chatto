import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import type { PinnedMessage } from '@chatto/api-types/api/v1/rooms_pb';
import { authHeaders, createChattoClient, handleAuthError, REALTIME_MINIMUM_CURSOR_HEADER, type ConnectAPIConfig } from './connect';
import { timelineUsersForMessages } from './roomTimeline';

export type PinnedMessagesPage = {
  items: PinnedMessage[];
  totalCount: number;
  hasMore: boolean;
  latestPinMarker: string;
};

export function createPinnedMessagesAPI(config: ConnectAPIConfig) {
  const rooms = createChattoClient(RoomService, config);
  const headers = () => authHeaders(config);
  return {
    async list(roomId: string, limit: number, offset: number, minimumCursor?: string): Promise<PinnedMessagesPage> {
      try {
        const requestHeaders = new Headers(headers());
        if (minimumCursor) requestHeaders.set(REALTIME_MINIMUM_CURSOR_HEADER, minimumCursor);
        const response = await rooms.listPinnedMessages(
          { roomId, page: { limit, offset } },
          { headers: requestHeaders, ...(minimumCursor ? { timeoutMs: 10_000 } : {}) }
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
