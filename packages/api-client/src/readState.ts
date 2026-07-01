import { notifyAuthenticationRequired } from "./hooks.js";
import { Code, ConnectError, createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
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
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true,
  });
  const rooms = createClient(RoomService, transport);
  const threads = createClient(ThreadService, transport);
  const headers = () =>
    config.bearerToken
      ? { Authorization: `Bearer ${config.bearerToken}` }
      : undefined;

  async function handleAuthError(err: unknown): Promise<never> {
    if (
      err instanceof ConnectError &&
      err.code === Code.Unauthenticated &&
      config.serverId
    ) {
      notifyAuthenticationRequired(
        config.serverId,
        config.onAuthenticationRequired,
      );
    }
    throw err;
  }

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
        return handleAuthError(err);
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
        return handleAuthError(err);
      }
    },
  };
}
