import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { configureApiClientHooks } from '@chatto/api-client/hooks';
import { RoomDirectoryScope } from '@chatto/api-types/api/v1/room_directory_pb';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { createRoomDirectoryAPI } from '@chatto/api-client/roomDirectory';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  listRooms: vi.fn(),
  getRoom: vi.fn(),
  listRoomGroups: vi.fn(),
  handleAuthenticationRequired: vi.fn()
}));

vi.mock('@connectrpc/connect', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@connectrpc/connect')>();
  return {
    ...actual,
    createClient: mocks.createClient
  };
});

vi.mock('@connectrpc/connect-web', () => ({
  createConnectTransport: mocks.createConnectTransport
}));

describe('createRoomDirectoryAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.listRooms.mockReset();
    mocks.getRoom.mockReset();
    mocks.listRoomGroups.mockReset();
    mocks.handleAuthenticationRequired.mockReset();

    configureApiClientHooks({ onAuthenticationRequired: mocks.handleAuthenticationRequired });
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      listRooms: mocks.listRooms,
      getRoom: mocks.getRoom,
      listRoomGroups: mocks.listRoomGroups
    });
  });

  it('lists rooms for a scope with bearer auth and maps room state', async () => {
    mocks.listRooms.mockResolvedValue({
      rooms: [
        {
          room: {
            id: 'room-1',
            name: 'general',
            description: 'Lobby channel',
            kind: RoomKind.CHANNEL,
            archived: false,
            universal: true
          },
          viewerState: { isMember: true, hasUnread: true, canJoinRoom: false }
        },
        {
          room: {
            id: 'room-2',
            name: 'random',
            kind: RoomKind.DM,
            archived: true,
            universal: false
          },
          viewerState: { isMember: true, hasUnread: false, canJoinRoom: true }
        },
        { viewerState: { hasUnread: true } }
      ]
    });

    const api = createRoomDirectoryAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.com/api/connect',
      bearerToken: 'token'
    });
    const rooms = await api.listRooms(RoomDirectoryScope.DMS);

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://remote.example.com/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.listRooms).toHaveBeenCalledWith(
      { scope: RoomDirectoryScope.DMS },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(rooms).toEqual([
      {
        id: 'room-1',
        name: 'general',
        description: 'Lobby channel',
        kind: RoomKind.CHANNEL,
        archived: false,
        isUniversal: true,
        isMember: true,
        hasUnread: true,
        canJoinRoom: false
      },
      {
        id: 'room-2',
        name: 'random',
        description: null,
        kind: RoomKind.DM,
        archived: true,
        isUniversal: false,
        isMember: true,
        hasUnread: false,
        canJoinRoom: true
      }
    ]);
  });

  it('gets one room and maps viewer permissions', async () => {
    mocks.getRoom.mockResolvedValue({
      room: {
        room: {
          id: 'room-1',
          name: 'general',
          description: 'Lobby channel',
          kind: RoomKind.CHANNEL,
          archived: false,
          universal: true
        },
        viewerState: {
          isMember: true,
          hasUnread: true,
          canJoinRoom: false,
          canPostMessage: true,
          canPostInThread: true,
          canAttach: false,
          canReact: true,
          canEchoMessage: true,
          canManageOthersMessage: false,
          canManageRoom: true,
          canBanRoomMembers: false
        }
      }
    });

    const api = createRoomDirectoryAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.com/api/connect',
      bearerToken: 'token'
    });
    const room = await api.getRoom('room-1');

    expect(mocks.getRoom).toHaveBeenCalledWith(
      { roomId: 'room-1' },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(room).toEqual({
      id: 'room-1',
      name: 'general',
      description: 'Lobby channel',
      kind: RoomKind.CHANNEL,
      archived: false,
      isUniversal: true,
      isMember: true,
      hasUnread: true,
      canJoinRoom: false,
      canPostMessage: true,
      canPostInThread: true,
      canAttach: false,
      canReact: true,
      canEchoMessage: true,
      canManageOthersMessage: false,
      canManageRoom: true,
      canBanRoomMembers: false
    });
  });

  it('returns null when a room is not visible', async () => {
    mocks.getRoom.mockRejectedValue(new ConnectError('not found', Code.NotFound));

    const api = createRoomDirectoryAPI({
      serverId: 'remote',
      baseUrl: '/api/connect',
      bearerToken: null
    });

    await expect(api.getRoom('hidden-room')).resolves.toBeNull();
    expect(mocks.handleAuthenticationRequired).not.toHaveBeenCalled();
  });

  it('lists room groups and maps mixed sidebar items', async () => {
    mocks.listRoomGroups.mockResolvedValue({
      groups: [
        {
          id: 'g1',
          name: 'Lobby',
          rooms: [
            { room: { id: 'general', name: 'general', kind: RoomKind.CHANNEL } },
            { room: { id: 'general', name: 'general', kind: RoomKind.CHANNEL } },
            { room: { id: 'random', name: 'random', kind: RoomKind.CHANNEL } }
          ],
          items: [
            {
              item: {
                case: 'sidebarLink',
                value: { id: 'docs', label: 'Docs', url: 'https://example.com/docs' }
              }
            },
            {
              item: {
                case: 'room',
                value: { room: { id: 'general', name: 'general', kind: RoomKind.CHANNEL } }
              }
            }
          ]
        }
      ]
    });

    const api = createRoomDirectoryAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.com/api/connect',
      bearerToken: 'token'
    });
    const groups = await api.listRoomGroups();

    expect(mocks.listRoomGroups).toHaveBeenCalledWith(
      {},
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(groups).toEqual([
      {
        id: 'g1',
        name: 'Lobby',
        roomIds: ['general', 'random'],
        items: [
          {
            id: 'link:docs',
            type: 'link',
            link: { id: 'docs', label: 'Docs', url: 'https://example.com/docs' }
          },
          { id: 'room:general', type: 'room', roomId: 'general' }
        ]
      }
    ]);
  });

  it('falls back to group rooms when no ordered sidebar items are present', async () => {
    mocks.listRoomGroups.mockResolvedValue({
      groups: [
        {
          id: 'g1',
          name: 'Lobby',
          rooms: [
            { room: { id: 'general', name: 'general', kind: RoomKind.CHANNEL } },
            { room: { id: 'random', name: 'random', kind: RoomKind.CHANNEL } }
          ],
          items: []
        }
      ]
    });

    const api = createRoomDirectoryAPI({
      baseUrl: '/api/connect',
      bearerToken: null
    });

    await expect(api.listRoomGroups()).resolves.toMatchObject([
      {
        id: 'g1',
        roomIds: ['general', 'random'],
        items: [
          { id: 'room:general', type: 'room', roomId: 'general' },
          { id: 'room:random', type: 'room', roomId: 'random' }
        ]
      }
    ]);
  });

  it('routes unauthenticated errors through the server registry', async () => {
    const err = new ConnectError('authentication required', Code.Unauthenticated);
    mocks.listRooms.mockRejectedValue(err);

    const api = createRoomDirectoryAPI({
      serverId: 'remote',
      baseUrl: '/api/connect',
      bearerToken: null
    });

    await expect(api.listRooms(RoomDirectoryScope.CHANNELS)).rejects.toBe(err);
    expect(mocks.listRooms).toHaveBeenCalledWith(
      { scope: RoomDirectoryScope.CHANNELS },
      { headers: undefined }
    );
    expect(mocks.handleAuthenticationRequired).toHaveBeenCalledWith('remote');
  });
});
