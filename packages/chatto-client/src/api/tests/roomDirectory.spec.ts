import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { RoomDirectoryScope } from '@chatto/api-types/api/v1/room_directory_pb';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { createRoomDirectoryAPI } from '../roomDirectory.js';
import { RoomDirectoryService } from '@chatto/api-types/api/v1/room_directory_connect';
import { fakeServer, mockService, receivedRequest } from '../../testing/fakeServer.js';
import { RoomThreadingMode } from '../../util/roomThreading.js';

const Permission = {
  Attach: 'message.attach',
  BanMember: 'room.remove-member',
  CreateRoom: 'room.create',
  EchoMessage: 'message.echo',
  JoinRoom: 'room.join',
  ManageMessage: 'message.manage',
  ManageRoom: 'room.manage',
  PostInThread: 'message.post-in-thread',
  PostMessage: 'message.post',
  ReadInteractions: 'message.read-interactions',
  ReadMessages: 'message.read',
  React: 'message.react'
} as const;

const mocks = mockService(RoomDirectoryService);

function directoryAPI() {
  return createRoomDirectoryAPI(
    fakeServer((router) => router.service(RoomDirectoryService, mocks))
  );
}

describe('createRoomDirectoryAPI', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('collects all directory pages with the same scope and cancels with the caller signal', async () => {
    mocks.listRooms
      .mockReturnValueOnce({ rooms: [{ room: { id: 'a', name: 'A' } }], page: { hasMore: true } })
      .mockReturnValueOnce({
        rooms: [{ room: { id: 'b', name: 'B' } }],
        page: { hasMore: false }
      });
    const api = directoryAPI();
    const rooms = await api.listRooms(RoomDirectoryScope.ALL);
    expect(rooms.map((room) => room.id)).toEqual(['a', 'b']);
    expect(mocks.listRooms.mock.calls.map(([request]) => request)).toMatchObject([
      { scope: RoomDirectoryScope.ALL, page: { limit: 100, offset: 0 } },
      { scope: RoomDirectoryScope.ALL, page: { limit: 100, offset: 1 } }
    ]);
    await expect(
      api.listRooms(RoomDirectoryScope.ALL, { signal: AbortSignal.abort() })
    ).rejects.toMatchObject({ code: Code.Canceled });
  });

  it('lists rooms for a scope and maps room state', async () => {
    mocks.listRooms.mockReturnValue({
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
          viewerState: roomViewerState({
            isMember: true,
            hasUnread: true,
            [Permission.JoinRoom]: false
          })
        },
        {
          room: {
            id: 'room-2',
            name: 'random',
            kind: RoomKind.DM,
            archived: true,
            universal: false
          },
          viewerState: roomViewerState({
            isMember: true,
            hasUnread: false,
            [Permission.JoinRoom]: true
          })
        },
        {}
      ]
    });

    const api = directoryAPI();
    const rooms = await api.listRooms(RoomDirectoryScope.DMS);

    expect(receivedRequest(mocks.listRooms)).toMatchObject({
      scope: RoomDirectoryScope.DMS,
      page: { limit: 100, offset: 0 }
    });
    expect(rooms).toEqual([
      {
        id: 'room-1',
        name: 'general',
        description: 'Lobby channel',
        kind: RoomKind.CHANNEL,
        archived: false,
        isUniversal: true,
        slowModeSeconds: 0,
        threadingMode: RoomThreadingMode.ENABLED,
        slowModeNextPostAt: null,
        isMember: true,
        hasUnread: true,
        canReadMessages: null,
        canJoinRoom: false,
        canManageRoom: false
      },
      {
        id: 'room-2',
        name: 'random',
        description: null,
        kind: RoomKind.DM,
        archived: true,
        isUniversal: false,
        slowModeSeconds: 0,
        threadingMode: RoomThreadingMode.ENABLED,
        slowModeNextPostAt: null,
        isMember: true,
        hasUnread: false,
        canReadMessages: null,
        canJoinRoom: true,
        canManageRoom: false
      }
    ]);
  });

  it('gets one room and maps viewer permissions', async () => {
    mocks.getRoom.mockReturnValue({
      room: {
        room: {
          id: 'room-1',
          name: 'general',
          description: 'Lobby channel',
          kind: RoomKind.CHANNEL,
          archived: false,
          universal: true
        },
        viewerState: roomViewerState({
          isMember: true,
          hasUnread: true,
          [Permission.JoinRoom]: false,
          [Permission.PostMessage]: true,
          [Permission.ReadMessages]: true,
          [Permission.PostInThread]: true,
          [Permission.Attach]: false,
          [Permission.React]: true,
          [Permission.EchoMessage]: true,
          [Permission.ManageMessage]: false,
          [Permission.ManageRoom]: true,
          [Permission.BanMember]: false
        })
      }
    });

    const api = directoryAPI();
    const room = await api.getRoom('room-1');

    expect(receivedRequest(mocks.getRoom)).toMatchObject({ roomId: 'room-1' });
    expect(room).toEqual({
      id: 'room-1',
      name: 'general',
      description: 'Lobby channel',
      hasLimitedMessageAccess: false,
      kind: RoomKind.CHANNEL,
      archived: false,
      isUniversal: true,
      slowModeSeconds: 0,
      threadingMode: RoomThreadingMode.ENABLED,
      slowModeNextPostAt: null,
      isMember: true,
      hasUnread: true,
      canReadMessages: true,
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

  it('admits a room when interaction-scoped reads are enabled', async () => {
    mocks.getRoom.mockReturnValue({
      room: {
        room: {
          id: 'room-interactions',
          name: 'bot-work',
          kind: RoomKind.CHANNEL
        },
        viewerState: roomViewerState({
          isMember: true,
          hasUnread: false,
          [Permission.ReadMessages]: false,
          [Permission.ReadInteractions]: true
        })
      }
    });

    const api = directoryAPI();

    await expect(api.getRoom('room-interactions')).resolves.toMatchObject({
      id: 'room-interactions',
      canReadMessages: true
    });
  });

  it('returns null when a room is not visible', async () => {
    mocks.getRoom.mockImplementation(() => {
      throw new ConnectError('not found', Code.NotFound);
    });

    const api = directoryAPI();

    await expect(api.getRoom('hidden-room')).resolves.toBeNull();
  });

  it('preserves permission denied on singular room reads', async () => {
    mocks.getRoom.mockImplementation(() => {
      throw new ConnectError('permission denied', Code.PermissionDenied);
    });

    const api = directoryAPI();

    await expect(api.getRoom('hidden-room')).rejects.toMatchObject({
      code: Code.PermissionDenied
    });
  });

  it('batch gets rooms and maps viewer permissions', async () => {
    mocks.batchGetRooms.mockReturnValue({
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
          viewerState: roomViewerState({
            isMember: true,
            hasUnread: false,
            [Permission.JoinRoom]: false,
            [Permission.PostMessage]: true,
            [Permission.PostInThread]: false,
            [Permission.Attach]: true,
            [Permission.React]: true,
            [Permission.EchoMessage]: false,
            [Permission.ManageMessage]: false,
            [Permission.ManageRoom]: false,
            [Permission.BanMember]: false
          })
        }
      ]
    });

    const api = directoryAPI();

    await expect(api.batchGetRooms(['room-1', 'missing'])).resolves.toMatchObject([
      {
        id: 'room-1',
        canPostMessage: true,
        canAttach: true
      }
    ]);
    expect(receivedRequest(mocks.batchGetRooms)).toMatchObject({ roomIds: ['room-1', 'missing'] });
  });

  it('lists room groups and maps mixed sidebar items', async () => {
    mocks.listRoomGroups.mockReturnValue({
      groups: [
        {
          id: 'g1',
          name: 'Lobby',
          viewerState: groupViewerState(true),
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
            },
            {
              item: {
                case: 'room',
                value: { room: { id: 'random', name: 'random', kind: RoomKind.CHANNEL } }
              }
            }
          ]
        }
      ]
    });

    const api = directoryAPI();
    const groups = await api.listRoomGroups();

    expect(mocks.listRoomGroups).toHaveBeenCalledOnce();
    expect(groups).toEqual([
      {
        id: 'g1',
        name: 'Lobby',
        canCreateRoom: true,
        canManageGroup: false,
        roomIds: ['general', 'random'],
        items: [
          {
            id: 'link:docs',
            type: 'link',
            link: { id: 'docs', label: 'Docs', url: 'https://example.com/docs' }
          },
          {
            id: 'room:general',
            type: 'room',
            roomId: 'general',
            room: expect.objectContaining({ id: 'general', name: 'general' })
          },
          {
            id: 'room:random',
            type: 'room',
            roomId: 'random',
            room: expect.objectContaining({ id: 'random', name: 'random' })
          }
        ]
      }
    ]);
  });

  it('returns empty item order when no ordered sidebar items are present', async () => {
    mocks.listRoomGroups.mockReturnValue({
      groups: [
        {
          id: 'g1',
          name: 'Lobby',
          viewerState: groupViewerState(false),
          items: []
        }
      ]
    });

    const api = directoryAPI();

    await expect(api.listRoomGroups()).resolves.toMatchObject([
      {
        id: 'g1',
        canCreateRoom: false,
        roomIds: [],
        items: []
      }
    ]);
    expect(mocks.listRoomGroups).toHaveBeenCalledOnce();
  });

  it('gets and batch gets room groups', async () => {
    const group = {
      id: 'g1',
      name: 'Lobby',
      viewerState: groupViewerState(true),
      items: [
        {
          item: {
            case: 'room' as const,
            value: { room: { id: 'general', name: 'general', kind: RoomKind.CHANNEL } }
          }
        }
      ]
    };
    mocks.getRoomGroup.mockReturnValue({ group });
    mocks.batchGetRoomGroups.mockReturnValue({ groups: [group] });

    const api = directoryAPI();

    await expect(api.getRoomGroup('g1')).resolves.toMatchObject({
      id: 'g1',
      canCreateRoom: true,
      roomIds: ['general']
    });
    await expect(api.batchGetRoomGroups(['g1', 'missing'])).resolves.toMatchObject([
      {
        id: 'g1',
        canCreateRoom: true,
        roomIds: ['general']
      }
    ]);

    expect(receivedRequest(mocks.getRoomGroup)).toMatchObject({ groupId: 'g1' });
    expect(receivedRequest(mocks.batchGetRoomGroups)).toMatchObject({
      groupIds: ['g1', 'missing']
    });
  });

  it('returns null when a room group is missing', async () => {
    mocks.getRoomGroup.mockImplementation(() => {
      throw new ConnectError('not found', Code.NotFound);
    });

    const api = directoryAPI();

    await expect(api.getRoomGroup('missing-group')).resolves.toBeNull();
  });

  it('propagates Connect errors', async () => {
    mocks.listRooms.mockImplementation(() => {
      throw new ConnectError('authentication required', Code.Unauthenticated);
    });

    const api = directoryAPI();

    await expect(api.listRooms(RoomDirectoryScope.CHANNELS)).rejects.toMatchObject({
      code: Code.Unauthenticated,
      rawMessage: 'authentication required'
    });
    expect(receivedRequest(mocks.listRooms)).toMatchObject({
      scope: RoomDirectoryScope.CHANNELS,
      page: { limit: 100, offset: 0 }
    });
  });
});

function roomViewerState(
  input: Record<string, boolean> & { isMember: boolean; hasUnread: boolean }
) {
  const { isMember, hasUnread, ...permissions } = input;
  return {
    isMember,
    hasUnread,
    permissions: Object.entries(permissions).map(([permission, granted]) => ({
      permission,
      granted
    }))
  };
}

function groupViewerState(canCreateRoom: boolean, canManageGroup = false) {
  return {
    permissions: [
      {
        permission: Permission.CreateRoom,
        granted: canCreateRoom
      },
      {
        permission: Permission.ManageRoom,
        granted: canManageGroup
      }
    ]
  };
}
