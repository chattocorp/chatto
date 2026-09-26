import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import type { Message } from '@chatto/api-types/api/v1/message_types_pb';
import { createChattoClient, minimumCursorHeaders, type ConnectAPIConfig } from './connect';
import { messageToTimelineEvent, timelineUsersForMessages } from './roomTimeline';
import type { TimelineEventView } from '$lib/render/timelineEvents';

/** One authoritative message, shared by timeline and collection consumers. */
export type MessageResource = { message: Message; timeline: TimelineEventView | null };

/** Bounded message reads for the server-owned realtime reconciliation queue. */
export function createMessageResourcesAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(MessageService, config);
  return {
    async read(roomId: string, eventIds: string[], minimumCursor?: string): Promise<MessageResource[]> {
      const response = await client.batchGetMessages(
        { roomId, eventIds },
        { headers: minimumCursorHeaders(minimumCursor), timeoutMs: 10_000 }
      );
      const users = await timelineUsersForMessages(config, response.messages, minimumCursor, true);
      return response.messages.map((message) => ({
        message,
        timeline: messageToTimelineEvent(message, users)
      }));
    }
  };
}
