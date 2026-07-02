import { authHeaders, createChattoClient, handleAuthError } from "./connect.js";
import { RoomService } from "@chatto/api-types/api/v1/rooms_connect";
import { ThreadService } from "@chatto/api-types/api/v1/threads_connect";

export type ConnectAPIConfig = {
  serverId?: string;
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export type MarkRoomAsReadResult = {
  lastReadAt: string | null;
  previousLastReadAt: string | null;
};

export type MarkThreadAsReadResult = {
  previousReadAt: string | null;
};

export function createReadStateAPI(config: ConnectAPIConfig) {
  const rooms = createChattoClient(RoomService, config);
  const threads = createChattoClient(ThreadService, config);
  const headers = () => authHeaders(config);
  return {
    async markRoomAsRead(input: {
      roomId: string;
      upToEventId?: string;
    }): Promise<MarkRoomAsReadResult> {
      try {
        const response = await rooms.markRoomAsRead(
          {
            roomId: input.roomId,
            upToEventId: input.upToEventId ?? "",
          },
          { headers: headers() },
        );
        return {
          lastReadAt: response.lastReadAt?.toDate().toISOString() ?? null,
          previousLastReadAt:
            response.previousLastReadAt?.toDate().toISOString() ?? null,
        };
      } catch (err) {
        return handleAuthError(config, err);
      }
    },

    async markThreadAsRead(input: {
      roomId: string;
      threadRootEventId: string;
      upToEventId?: string;
    }): Promise<MarkThreadAsReadResult> {
      try {
        const response = await threads.markThreadAsRead(
          {
            roomId: input.roomId,
            threadRootEventId: input.threadRootEventId,
            upToEventId: input.upToEventId ?? "",
          },
          { headers: headers() },
        );
        return {
          previousReadAt:
            response.previousReadAt?.toDate().toISOString() ?? null,
        };
      } catch (err) {
        return handleAuthError(config, err);
      }
    },
  };
}
