import { createChattoClient, type ConnectAPIConfig } from './connect.js';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import { ThreadService } from '@chatto/api-types/api/v1/threads_connect';

export type MarkRoomAsReadResult = {
  lastReadAt: string | null;
  previousLastReadAt: string | null;
};

export type MarkThreadAsReadResult = {
  lastReadAt: string | null;
  previousLastReadAt: string | null;
};

export function createReadStateAPI(config: ConnectAPIConfig) {
  const rooms = createChattoClient(RoomService, config);
  const threads = createChattoClient(ThreadService, config);
  return {
    async markRoomAsRead(
      input: {
        roomId: string;
        upToEventId?: string;
      },
      options: { signal?: AbortSignal } = {}
    ): Promise<MarkRoomAsReadResult> {
      const response = await rooms.markRoomAsRead(
        {
          roomId: input.roomId,
          upToEventId: input.upToEventId ?? ''
        },
        { signal: options.signal }
      );
      return {
        lastReadAt: response.lastReadAt?.toDate().toISOString() ?? null,
        previousLastReadAt: response.previousLastReadAt?.toDate().toISOString() ?? null
      };
    },

    async markThreadAsRead(
      input: {
        roomId: string;
        threadRootEventId: string;
        upToEventId?: string;
      },
      options: { signal?: AbortSignal } = {}
    ): Promise<MarkThreadAsReadResult> {
      const response = await threads.markThreadAsRead(
        {
          roomId: input.roomId,
          threadRootEventId: input.threadRootEventId,
          upToEventId: input.upToEventId ?? ''
        },
        { signal: options.signal }
      );
      return {
        lastReadAt: response.lastReadAt?.toDate().toISOString() ?? null,
        previousLastReadAt: response.previousLastReadAt?.toDate().toISOString() ?? null
      };
    }
  };
}
