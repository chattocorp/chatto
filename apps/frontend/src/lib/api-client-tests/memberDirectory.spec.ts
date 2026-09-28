import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { Timestamp } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { PresenceStatus as APIPresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { createMemberDirectoryAPI } from '$lib/api-client/memberDirectory';
import { REALTIME_MINIMUM_CURSOR_HEADER, type ConnectAPIConfig } from '$lib/api-client/connect';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { fakeServer, mockService, receivedContext, receivedRequest } from '$lib/test-utils';

const users = mockService(UserService);
const rooms = mockService(RoomService);

function config(extra: Partial<ConnectAPIConfig> = {}) {
  return fakeServer(
    (router) => router.service(UserService, users).service(RoomService, rooms),
    extra
  );
}

function directoryAPI() {
  return createMemberDirectoryAPI(config());
}

describe('createMemberDirectoryAPI', () => {
  it('requests a bounded presence-filtered page without changing ordinary list defaults', async () => {
    rooms.listMembers.mockReturnValue({
      userIds: [],
      page: { totalCount: 0n, hasMore: false }
    });
    const api = directoryAPI();
    await api.listOnlineRoomMembers('room', PresenceStatus.AWAY, 250, 0);
    expect(receivedRequest(rooms.listMembers)).toMatchObject({
      roomId: 'room',
      search: '',
      page: { limit: 250, offset: 0 },
      presenceStatuses: [PresenceStatus.AWAY]
    });
    expect(receivedContext(rooms.listMembers)?.timeoutMs()).toBeGreaterThan(9_000);
  });
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('maps user pages', async () => {
    users.listUsers.mockReturnValue({
      users: [
        {
          user: {
            id: 'U1',
            login: 'alice',
            displayName: 'Alice',
            deleted: false,
            bot: { ownerUserId: 'owner' },
            avatarUrl: 'https://cdn/avatar.webp',
            presenceStatus: APIPresenceStatus.AWAY,
            customStatus: {
              emoji: ':seedling:',
              text: 'Focus',
              expiresAt: Timestamp.fromDate(new Date('2026-06-01T12:00:00Z'))
            }
          },
          roles: ['everyone', 'admin'],
          createdAt: Timestamp.fromDate(new Date('2026-01-01T09:00:00Z'))
        }
      ],
      page: { totalCount: 2n, hasMore: true }
    });

    const api = directoryAPI();

    await expect(api.listUsers('ali', 10, 20)).resolves.toEqual({
      members: [
        {
          id: 'U1',
          login: 'alice',
          displayName: 'Alice',
          deleted: false,
          isBot: true,
          bot: { ownerUserId: 'owner' },
          avatarUrl: 'https://cdn/avatar.webp',
          bio: null,
          timezone: null,
          presenceStatus: PresenceStatus.AWAY,
          customStatus: {
            emoji: ':seedling:',
            text: 'Focus',
            expiresAt: '2026-06-01T12:00:00.000Z'
          },
          roles: ['everyone', 'admin'],
          createdAt: '2026-01-01T09:00:00.000Z'
        }
      ],
      totalCount: 2,
      hasMore: true
    });

    expect(receivedRequest(users.listUsers)).toMatchObject({
      search: 'ali',
      page: { limit: 10, offset: 20 }
    });
    await expect(
      api.listUsers('ali', 10, 20, { signal: AbortSignal.abort() })
    ).rejects.toMatchObject({
      code: Code.Canceled
    });
  });

  it('gets and batch gets users', async () => {
    const member = {
      user: {
        id: 'U1',
        login: 'alice',
        displayName: 'Alice',
        deleted: false,
        presenceStatus: APIPresenceStatus.ONLINE
      },
      roles: ['everyone']
    };
    users.getUser.mockReturnValue({ user: member });
    users.batchGetUsers.mockReturnValue({ users: [member] });

    const api = directoryAPI();

    await expect(api.getUser('U1')).resolves.toMatchObject({
      id: 'U1',
      presenceStatus: PresenceStatus.ONLINE
    });
    await expect(api.getUserByLogin('alice')).resolves.toMatchObject({
      id: 'U1',
      presenceStatus: PresenceStatus.ONLINE
    });
    await expect(api.batchGetUsers(['U1', 'missing'])).resolves.toMatchObject([{ id: 'U1' }]);

    expect(receivedRequest(users.getUser, 0)).toMatchObject({
      target: { case: 'userId', value: 'U1' }
    });
    expect(receivedRequest(users.getUser, 1)).toMatchObject({
      target: { case: 'login', value: 'alice' }
    });
    expect(receivedRequest(users.batchGetUsers)).toMatchObject({ userIds: ['U1', 'missing'] });
  });

  it('maps room member pages', async () => {
    rooms.listMembers.mockReturnValue({
      userIds: ['U2'],
      page: { totalCount: 1n, hasMore: false }
    });
    users.batchGetUsers.mockReturnValue({
      users: [
        {
          user: {
            id: 'U2',
            login: 'bob',
            displayName: 'Bob',
            deleted: false,
            presenceStatus: APIPresenceStatus.DO_NOT_DISTURB
          },
          roles: []
        }
      ]
    });

    const api = directoryAPI();

    await expect(api.listRoomMembers('room-1', 'bob', 5, 0)).resolves.toEqual({
      members: [
        {
          id: 'U2',
          login: 'bob',
          displayName: 'Bob',
          deleted: false,
          isBot: false,
          avatarUrl: null,
          bio: null,
          timezone: null,
          presenceStatus: PresenceStatus.DO_NOT_DISTURB,
          customStatus: null,
          roles: [],
          createdAt: null
        }
      ],
      memberIds: ['U2'],
      totalCount: 1,
      consumedCount: 1,
      hasMore: false
    });

    expect(receivedRequest(rooms.listMembers)).toMatchObject({
      roomId: 'room-1',
      search: 'bob',
      page: { limit: 5, offset: 0 }
    });
  });

  it('keeps room member IDs when profile hydration returns no users', async () => {
    rooms.listMembers.mockReturnValue({
      userIds: ['U2'],
      page: { totalCount: 1n, hasMore: false }
    });
    users.batchGetUsers.mockReturnValue({ users: [] });
    const api = directoryAPI();

    await expect(api.listRoomMembers('room-1')).resolves.toEqual({
      members: [],
      memberIds: ['U2'],
      consumedCount: 1,
      totalCount: 1,
      hasMore: false
    });
  });

  it('waits for the join cursor when hydrating IDs from a room member page', async () => {
    const member = {
      user: { id: 'U2', login: 'newcomer', displayName: 'Newcomer' },
      roles: []
    };
    rooms.listMembers.mockReturnValue({
      userIds: ['U2'],
      page: { totalCount: 1n, hasMore: false }
    });
    users.batchGetUsers.mockImplementation((_request, context) => ({
      users:
        context.requestHeader.get(REALTIME_MINIMUM_CURSOR_HEADER) === 'join-cursor' ? [member] : []
    }));
    const api = createMemberDirectoryAPI(
      config({ serverId: 'cursor-test', queryScope: 'cursor-test' })
    );

    const page = await api.listRoomMembers('room-1', '', 250, 0, {
      minimumCursor: 'join-cursor'
    });

    expect(page.members.map((user) => user.id)).toEqual(['U2']);
    for (const handler of [rooms.listMembers, users.batchGetUsers]) {
      expect(receivedContext(handler)?.requestHeader.get(REALTIME_MINIMUM_CURSOR_HEADER)).toBe(
        'join-cursor'
      );
    }
  });

  it('defaults room member pages to 250 members', async () => {
    rooms.listMembers.mockReturnValue({
      userIds: [],
      page: { totalCount: 0n, hasMore: false }
    });

    const api = directoryAPI();

    await api.listRoomMembers('room-1');

    expect(receivedRequest(rooms.listMembers)).toMatchObject({
      roomId: 'room-1',
      search: '',
      page: { limit: 250, offset: 0 }
    });
  });

  it('gets and batch gets room members', async () => {
    const member = {
      user: {
        id: 'U2',
        login: 'bob',
        displayName: 'Bob',
        deleted: false,
        presenceStatus: APIPresenceStatus.OFFLINE
      },
      roles: []
    };
    rooms.getMember.mockReturnValue({ member });
    rooms.batchGetMembers.mockReturnValue({ members: [member] });

    const api = directoryAPI();

    await expect(api.getRoomMember('room-1', 'U2')).resolves.toMatchObject({ id: 'U2' });
    await expect(api.batchGetRoomMembers('room-1', ['U2', 'missing'])).resolves.toMatchObject([
      { id: 'U2' }
    ]);
    await expect(
      api.batchGetRoomMembers('room-1', ['U2'], { signal: AbortSignal.abort() })
    ).rejects.toMatchObject({ code: Code.Canceled });

    expect(receivedRequest(rooms.getMember)).toMatchObject({ roomId: 'room-1', userId: 'U2' });
    expect(receivedRequest(rooms.batchGetMembers)).toMatchObject({
      roomId: 'room-1',
      userIds: ['U2', 'missing']
    });
  });

  it('passes cancellation through when listing room members', async () => {
    await expect(
      directoryAPI().listRoomMembers('room-1', '', 20, 40, { signal: AbortSignal.abort() })
    ).rejects.toMatchObject({ code: Code.Canceled });
  });

  it('returns null when singular member lookups are missing', async () => {
    const notFound = () => {
      throw new ConnectError('missing', Code.NotFound);
    };
    users.getUser.mockImplementationOnce(notFound);
    rooms.getMember.mockImplementationOnce(notFound);

    const api = directoryAPI();

    await expect(api.getUser('missing')).resolves.toBeNull();
    await expect(api.getRoomMember('room-1', 'U2')).resolves.toBeNull();
  });

  it('preserves permission denied on singular room member reads', async () => {
    rooms.getMember.mockImplementationOnce(() => {
      throw new ConnectError('denied', Code.PermissionDenied);
    });

    const api = directoryAPI();

    await expect(api.getRoomMember('room-1', 'U2')).rejects.toMatchObject({
      code: Code.PermissionDenied
    });
  });

  it('maps offline and unspecified read statuses to offline', async () => {
    users.listUsers.mockReturnValue({
      users: [
        {
          user: {
            id: 'U3',
            login: 'carol',
            displayName: 'Carol',
            deleted: false,
            presenceStatus: APIPresenceStatus.OFFLINE
          },
          roles: []
        },
        {
          user: {
            id: 'U4',
            login: 'dave',
            displayName: 'Dave',
            deleted: false,
            presenceStatus: APIPresenceStatus.UNSPECIFIED
          },
          roles: []
        }
      ],
      page: { totalCount: 2n, hasMore: false }
    });

    const api = directoryAPI();

    await expect(api.listUsers()).resolves.toMatchObject({
      members: [
        { id: 'U3', presenceStatus: PresenceStatus.OFFLINE },
        { id: 'U4', presenceStatus: PresenceStatus.OFFLINE }
      ]
    });
  });
});
