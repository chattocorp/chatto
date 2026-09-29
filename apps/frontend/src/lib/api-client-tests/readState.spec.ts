import { Timestamp } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createReadStateAPI } from '$lib/api-client/readState';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import { ThreadService } from '@chatto/api-types/api/v1/threads_connect';
import { fakeServer, mockService, receivedRequest } from '$lib/test-utils';

const rooms = mockService(RoomService);
const threads = mockService(ThreadService);

function readStateAPI() {
  return createReadStateAPI(
    fakeServer((router) => router.service(RoomService, rooms).service(ThreadService, threads))
  );
}

describe('createReadStateAPI', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('marks a room read and converts timestamp fields', async () => {
    rooms.markRoomAsRead.mockReturnValue({
      lastReadAt: Timestamp.fromDate(new Date('2026-06-01T12:00:00Z')),
      previousLastReadAt: Timestamp.fromDate(new Date('2026-06-01T11:00:00Z'))
    });

    const result = await readStateAPI().markRoomAsRead({
      roomId: 'room-1',
      upToEventId: 'event-2'
    });

    expect(receivedRequest(rooms.markRoomAsRead)).toMatchObject({
      roomId: 'room-1',
      upToEventId: 'event-2'
    });
    expect(result).toEqual({
      lastReadAt: '2026-06-01T12:00:00.000Z',
      previousLastReadAt: '2026-06-01T11:00:00.000Z'
    });
  });

  it('marks a thread read up to the latest event', async () => {
    threads.markThreadAsRead.mockReturnValue({
      lastReadAt: Timestamp.fromDate(new Date('2026-06-01T12:00:00Z')),
      previousLastReadAt: Timestamp.fromDate(new Date('2026-06-01T10:00:00Z'))
    });

    const result = await readStateAPI().markThreadAsRead({
      roomId: 'room-1',
      threadRootEventId: 'root-1'
    });

    expect(receivedRequest(threads.markThreadAsRead)).toMatchObject({
      roomId: 'room-1',
      threadRootEventId: 'root-1',
      upToEventId: ''
    });
    expect(result).toEqual({
      lastReadAt: '2026-06-01T12:00:00.000Z',
      previousLastReadAt: '2026-06-01T10:00:00.000Z'
    });
  });

  it('propagates Connect errors', async () => {
    rooms.markRoomAsRead.mockImplementation(() => {
      throw new ConnectError('authentication required', Code.Unauthenticated);
    });

    await expect(readStateAPI().markRoomAsRead({ roomId: 'room-1' })).rejects.toMatchObject({
      code: Code.Unauthenticated,
      rawMessage: 'authentication required'
    });
  });
});
