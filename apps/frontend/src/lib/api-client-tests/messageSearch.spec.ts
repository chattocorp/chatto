import { Timestamp } from '@bufbuild/protobuf';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { MessageSearchOrder, MessageSearchState } from '@chatto/api-types/api/v1/message_search_pb';
import { createMessageSearchAPI } from '$lib/api-client/messageSearch';
import { RoomKind } from '$lib/api-client/roomDirectory';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  getStatus: vi.fn(),
  searchMessages: vi.fn(),
  batchGetRooms: vi.fn(),
  batchGetUsers: vi.fn()
}));

vi.mock('@connectrpc/connect', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@connectrpc/connect')>();
  return { ...actual, createClient: mocks.createClient };
});

vi.mock('@connectrpc/connect-web', () => ({
  createConnectTransport: mocks.createConnectTransport
}));

describe('createMessageSearchAPI', () => {
  beforeEach(() => {
    Object.values(mocks).forEach((mock) => mock.mockReset());
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient
      .mockReturnValueOnce({ getStatus: mocks.getStatus, searchMessages: mocks.searchMessages })
      .mockReturnValueOnce({ batchGetRooms: mocks.batchGetRooms })
      .mockReturnValueOnce({ batchGetUsers: mocks.batchGetUsers });
  });

  function createAPI() {
    return createMessageSearchAPI({
      serverId: 'remote',
      baseUrl: 'https://chat.example/api/connect',
      bearerToken: 'secret'
    });
  }

  it('maps coarse provider status and retry timing', async () => {
    mocks.getStatus.mockResolvedValue({
      state: MessageSearchState.INDEXING,
      retryAfter: { seconds: 2n, nanos: 500_000_000 }
    });

    const status = await createAPI().getStatus();

    expect(status).toEqual({
      state: MessageSearchState.INDEXING,
      retryAfterMs: 2500
    });
    expect(mocks.getStatus).toHaveBeenCalledWith(
      {},
      {
        headers: { Authorization: 'Bearer secret' }
      }
    );
  });

  it('hydrates result actors and rooms while preserving provider order and cursor', async () => {
    mocks.searchMessages.mockResolvedValue({
      messages: [
        {
          id: 'message-2',
          roomId: 'room-2',
          actorId: 'user-2',
          body: 'second',
          createdAt: Timestamp.fromDate(new Date('2026-02-02T12:00:00Z')),
          threadRootEventId: 'root-1',
          attachments: [{ id: 'attachment-1' }]
        },
        {
          id: 'message-1',
          roomId: 'room-1',
          actorId: 'user-1',
          body: 'first',
          createdAt: Timestamp.fromDate(new Date('2026-01-01T12:00:00Z')),
          threadRootEventId: '',
          attachments: []
        }
      ],
      nextCursor: 'opaque-next'
    });
    mocks.batchGetRooms.mockResolvedValue({
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
    mocks.batchGetUsers.mockResolvedValue({
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

    expect(mocks.searchMessages).toHaveBeenCalledWith(
      {
        query: 'hello',
        roomId: 'room-2',
        authorId: 'user-2',
        order: MessageSearchOrder.NEWEST,
        pageSize: 50,
        cursor: ''
      },
      { headers: { Authorization: 'Bearer secret' } }
    );
    expect(mocks.batchGetRooms).toHaveBeenCalledWith(
      { roomIds: ['room-2', 'room-1'] },
      { headers: { Authorization: 'Bearer secret' } }
    );
    expect(response.nextCursor).toBe('opaque-next');
    expect(response.results).toMatchObject([
      {
        id: 'message-2',
        roomName: '',
        roomKind: RoomKind.DM,
        actor: { displayName: 'Two' },
        threadRootEventId: 'root-1',
        attachmentCount: 1
      },
      {
        id: 'message-1',
        roomName: 'general',
        roomKind: RoomKind.CHANNEL,
        actor: { displayName: 'One' },
        threadRootEventId: null,
        attachmentCount: 0
      }
    ]);
  });
});
