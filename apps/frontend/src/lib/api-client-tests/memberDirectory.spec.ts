import { Timestamp } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { PresenceStatus } from '$lib/api-client/renderTypes';
import { PresenceStatus as APIPresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { createMemberDirectoryAPI } from '$lib/api-client/memberDirectory';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  listUsers: vi.fn(),
  getUser: vi.fn(),
  batchGetUsers: vi.fn(),
  listRoomMembers: vi.fn(),
  getRoomMember: vi.fn(),
  batchGetRoomMembers: vi.fn()
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

describe('createMemberDirectoryAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.listUsers.mockReset();
    mocks.getUser.mockReset();
    mocks.batchGetUsers.mockReset();
    mocks.listRoomMembers.mockReset();
    mocks.getRoomMember.mockReset();
    mocks.batchGetRoomMembers.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient
      .mockReturnValueOnce({
        listUsers: mocks.listUsers,
        getUser: mocks.getUser,
        batchGetUsers: mocks.batchGetUsers
      })
      .mockReturnValueOnce({
        listMembers: mocks.listRoomMembers,
        getMember: mocks.getRoomMember,
        batchGetMembers: mocks.batchGetRoomMembers
      });
  });

  it('maps user pages and sends bearer auth', async () => {
    mocks.listUsers.mockResolvedValue({
      users: [
        {
          user: {
            id: 'U1',
            login: 'alice',
            displayName: 'Alice',
            deleted: false,
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

    const api = createMemberDirectoryAPI({
      baseUrl: 'https://remote.test/api/connect',
      bearerToken: 'token'
    });

    await expect(api.listUsers('ali', 10, 20)).resolves.toEqual({
      members: [
        {
          id: 'U1',
          login: 'alice',
          displayName: 'Alice',
          deleted: false,
          avatarUrl: 'https://cdn/avatar.webp',
          presenceStatus: PresenceStatus.Away,
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

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://remote.test/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.listUsers).toHaveBeenCalledWith(
      { search: 'ali', page: { limit: 10, offset: 20 } },
      { headers: { Authorization: 'Bearer token' } }
    );
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
    mocks.getUser.mockResolvedValue({ user: member });
    mocks.batchGetUsers.mockResolvedValue({ users: [member] });

    const api = createMemberDirectoryAPI({
      baseUrl: 'https://remote.test/api/connect',
      bearerToken: 'token'
    });

    await expect(api.getUser('U1')).resolves.toMatchObject({
      id: 'U1',
      presenceStatus: PresenceStatus.Online
    });
    await expect(api.getUserByLogin('alice')).resolves.toMatchObject({
      id: 'U1',
      presenceStatus: PresenceStatus.Online
    });
    await expect(api.batchGetUsers(['U1', 'missing'])).resolves.toMatchObject([
      { id: 'U1' }
    ]);

    expect(mocks.getUser).toHaveBeenNthCalledWith(
      1,
      { target: { case: 'userId', value: 'U1' } },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(mocks.getUser).toHaveBeenNthCalledWith(
      2,
      { target: { case: 'login', value: 'alice' } },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(mocks.batchGetUsers).toHaveBeenCalledWith(
      { userIds: ['U1', 'missing'] },
      { headers: { Authorization: 'Bearer token' } }
    );
  });

  it('maps room member pages without auth headers', async () => {
    mocks.listRoomMembers.mockResolvedValue({
      members: [
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
      ],
      page: { totalCount: 1n, hasMore: false }
    });

    const api = createMemberDirectoryAPI({ baseUrl: '/api/connect', bearerToken: null });

    await expect(api.listRoomMembers('room-1', 'bob', 5, 0)).resolves.toEqual({
      members: [
        {
          id: 'U2',
          login: 'bob',
          displayName: 'Bob',
          deleted: false,
          avatarUrl: null,
          presenceStatus: PresenceStatus.DoNotDisturb,
          customStatus: null,
          roles: [],
          createdAt: null
        }
      ],
      totalCount: 1,
      hasMore: false
    });

    expect(mocks.listRoomMembers).toHaveBeenCalledWith(
      { roomId: 'room-1', search: 'bob', page: { limit: 5, offset: 0 } },
      { headers: undefined }
    );
  });

  it('defaults room member pages to 250 members', async () => {
    mocks.listRoomMembers.mockResolvedValue({
      members: [],
      page: { totalCount: 0n, hasMore: false }
    });

    const api = createMemberDirectoryAPI({ baseUrl: '/api/connect', bearerToken: null });

    await api.listRoomMembers('room-1');

    expect(mocks.listRoomMembers).toHaveBeenCalledWith(
      { roomId: 'room-1', search: '', page: { limit: 250, offset: 0 } },
      { headers: undefined }
    );
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
    mocks.getRoomMember.mockResolvedValue({ member });
    mocks.batchGetRoomMembers.mockResolvedValue({ members: [member] });

    const api = createMemberDirectoryAPI({ baseUrl: '/api/connect', bearerToken: null });

    await expect(api.getRoomMember('room-1', 'U2')).resolves.toMatchObject({ id: 'U2' });
    await expect(api.batchGetRoomMembers('room-1', ['U2', 'missing'])).resolves.toMatchObject([
      { id: 'U2' }
    ]);

    expect(mocks.getRoomMember).toHaveBeenCalledWith(
      { roomId: 'room-1', userId: 'U2' },
      { headers: undefined }
    );
    expect(mocks.batchGetRoomMembers).toHaveBeenCalledWith(
      { roomId: 'room-1', userIds: ['U2', 'missing'] },
      { headers: undefined }
    );
  });

  it('returns null when singular member lookups are missing', async () => {
    mocks.getUser.mockRejectedValueOnce(new ConnectError('missing', Code.NotFound));
    mocks.getRoomMember.mockRejectedValueOnce(new ConnectError('missing', Code.NotFound));

    const api = createMemberDirectoryAPI({ baseUrl: '/api/connect', bearerToken: null });

    await expect(api.getUser('missing')).resolves.toBeNull();
    await expect(api.getRoomMember('room-1', 'U2')).resolves.toBeNull();
  });

  it('preserves permission denied on singular room member reads', async () => {
    const err = new ConnectError('denied', Code.PermissionDenied);
    mocks.getRoomMember.mockRejectedValueOnce(err);

    const api = createMemberDirectoryAPI({ baseUrl: '/api/connect', bearerToken: null });

    await expect(api.getRoomMember('room-1', 'U2')).rejects.toBe(err);
  });

  it('maps offline and unspecified read statuses to offline', async () => {
    mocks.listUsers.mockResolvedValue({
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

    const api = createMemberDirectoryAPI({ baseUrl: '/api/connect', bearerToken: null });

    await expect(api.listUsers()).resolves.toMatchObject({
      members: [
        { id: 'U3', presenceStatus: PresenceStatus.Offline },
        { id: 'U4', presenceStatus: PresenceStatus.Offline }
      ]
    });
  });
});
