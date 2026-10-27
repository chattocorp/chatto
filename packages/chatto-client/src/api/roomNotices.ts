/** Member-only notice reads and version-checked commands. */
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import { RoomNotice } from '@chatto/api-types/api/v1/rooms_pb';
import { createChattoClient, minimumCursorHeaders, type ConnectAPIConfig } from './connect.js';

/** Maximum notice Markdown length in Unicode code points. */
export const MAX_ROOM_NOTICE_LENGTH = 4000;

/** Creates a stateless facade. Errors, including stale-write conflicts, reach the caller. */
export function createRoomNoticesAPI(config: ConnectAPIConfig) {
  const rooms = createChattoClient(RoomService, config);
  return {
    async get(roomId: string, options: { signal?: AbortSignal; minimumCursor?: string } = {}) {
      const response = await rooms.getNotice(
        { roomId },
        {
          signal: options.signal,
          headers: minimumCursorHeaders(options.minimumCursor)
        }
      );
      return response.notice ?? new RoomNotice();
    },
    async publish(
      roomId: string,
      body: string,
      expectedVersion: string,
      republish: boolean,
      options: { signal?: AbortSignal } = {}
    ) {
      const response = await rooms.publishNotice(
        { roomId, body, expectedVersion, republish },
        options
      );
      return response.notice ?? new RoomNotice();
    },
    async clear(roomId: string, expectedVersion: string, options: { signal?: AbortSignal } = {}) {
      const response = await rooms.clearNotice({ roomId, expectedVersion }, options);
      return response.notice ?? new RoomNotice();
    }
  };
}

export type RoomNoticesAPI = ReturnType<typeof createRoomNoticesAPI>;
