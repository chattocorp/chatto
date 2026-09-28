import { Timestamp } from '@bufbuild/protobuf';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { MessageSearchService } from '@chatto/api-types/api/v1/message_search_connect';
import { RoomDirectoryService } from '@chatto/api-types/api/v1/room_directory_connect';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { MessageSearchOrder, MessageSearchState } from '@chatto/api-types/api/v1/message_search_pb';
import { createMessageSearchAPI } from '$lib/api-client/messageSearch';
import { RoomKind } from '$lib/api-client/roomDirectory';
import { fakeServer, mockService, receivedRequest } from '$lib/test-utils';

const search = mockService(MessageSearchService);
const rooms = mockService(RoomDirectoryService);
const users = mockService(UserService);

function createAPI() {
  return createMessageSearchAPI(
    fakeServer((router) =>
      router
        .service(MessageSearchService, search)
        .service(RoomDirectoryService, rooms)
        .service(UserService, users)
    )
  );
}

describe('createMessageSearchAPI', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('maps coarse provider status and retry timing', async () => {
    search.getStatus.mockReturnValue({
      state: MessageSearchState.INDEXING,
      retryAfter: { seconds: 2n, nanos: 500_000_000 }
    });

    const status = await createAPI().getStatus();

    expect(status).toEqual({
      state: MessageSearchState.INDEXING,
      retryAfterMs: 2500
    });
    expect(search.getStatus).toHaveBeenCalledOnce();
  });

  it('hydrates result actors and rooms while preserving provider order and cursor', async () => {
    search.searchMessages.mockReturnValue({
      results: [
        {
          relevanceScore: 9.5,
          message: {
            id: 'message-2',
            roomId: 'room-2',
            actorId: 'user-2',
            body: 'second',
            createdAt: Timestamp.fromDate(new Date('2026-02-02T12:00:00Z')),
            threadRootEventId: 'root-1',
            attachments: [{ id: 'attachment-1' }]
          }
        },
        {
          relevanceScore: 4.25,
          message: {
            id: 'message-1',
            roomId: 'room-1',
            actorId: 'user-1',
            body: 'first',
            createdAt: Timestamp.fromDate(new Date('2026-01-01T12:00:00Z')),
            threadRootEventId: '',
            attachments: []
          }
        }
      ],
      nextCursor: 'opaque-next'
    });
    rooms.batchGetRooms.mockReturnValue({
      rooms: [
        {
          room: { id: 'room-1', name: 'general', kind: RoomKind.CHANNEL },
          viewerState: { permissions: [] }
        },
        {
          room: { id: 'room-2', name: '', kind: RoomKind.DM },
          viewerState: { permissions: [] }
        }
      ]
    });
    users.batchGetUsers.mockReturnValue({
      users: [
        { user: { id: 'user-1', login: 'one', displayName: 'One', deleted: false } },
        { user: { id: 'user-2', login: 'two', displayName: 'Two', deleted: false } }
      ]
    });

    const response = await createAPI().searchMessages({
      query: 'hello',
      roomId: 'room-2',
      authorId: 'user-2',
      order: MessageSearchOrder.NEWEST
    });

    expect(receivedRequest(search.searchMessages)).toMatchObject({
      query: 'hello',
      roomId: 'room-2',
      authorId: 'user-2',
      order: MessageSearchOrder.NEWEST,
      pageSize: 50,
      cursor: ''
    });
    expect(receivedRequest(rooms.batchGetRooms)).toMatchObject({ roomIds: ['room-2', 'room-1'] });
    expect(response.nextCursor).toBe('opaque-next');
    expect(response.results).toMatchObject([
      {
        id: 'message-2',
        roomName: '',
        roomKind: RoomKind.DM,
        actor: { displayName: 'Two' },
        threadRootEventId: 'root-1',
        attachmentCount: 1,
        relevanceScore: 9.5
      },
      {
        id: 'message-1',
        roomName: 'general',
        roomKind: RoomKind.CHANNEL,
        actor: { displayName: 'One' },
        threadRootEventId: null,
        attachmentCount: 0,
        relevanceScore: 4.25
      }
    ]);
  });
});
