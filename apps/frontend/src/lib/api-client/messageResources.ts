import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import type { Message } from '@chatto/api-types/api/v1/message_types_pb';
import {
  authHeaders, createChattoClient, handleAuthError, REALTIME_MINIMUM_CURSOR_HEADER
} from './connect';
import {
  messageToTimelineEvent, timelineUsersForMessages, type RoomTimelineAPIConfig
} from './roomTimeline';
import type { TimelineEventView } from '$lib/render/timelineEvents';

/** One authoritative message, shared by timeline and collection consumers. */
export type MessageResource = { message: Message; timeline: TimelineEventView | null };

/** Bounded message reads for the server-owned realtime reconciliation queue. */
export function createMessageResourcesAPI(config: RoomTimelineAPIConfig) {
  const client = createChattoClient(MessageService, config);
  return {
    async read(roomId: string, eventIds: string[], minimumCursor?: string): Promise<MessageResource[]> {
      const headers = new Headers(authHeaders(config));
      if (minimumCursor) headers.set(REALTIME_MINIMUM_CURSOR_HEADER, minimumCursor);
      try {
        const response = await client.batchGetMessages(
          { roomId, eventIds }, { headers, timeoutMs: 10_000 }
        );
        const users = await timelineUsersForMessages(config, response.messages, minimumCursor, true);
        return response.messages.map((message) => ({
          message, timeline: messageToTimelineEvent(message, users)
        }));
      } catch (error) {
        return handleAuthError(config, error);
      }
    }
  };
}
