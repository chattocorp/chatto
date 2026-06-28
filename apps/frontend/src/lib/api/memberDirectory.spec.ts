import { Timestamp } from '@bufbuild/protobuf';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { PresenceStatus } from '$lib/render/types';
import { PresenceStatus as APIPresenceStatus } from '$lib/pb/chatto/api/v1/presence_pb';
import { createMemberDirectoryAPI } from './memberDirectory';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  listServerMembers: vi.fn(),
  listRoomMembers: vi.fn()
}));

vi.mock('@connectrpc/connect', () => ({
  createClient: mocks.createClient
}));

vi.mock('@connectrpc/connect-web', () => ({
  createConnectTransport: mocks.createConnectTransport
}));

describe('createMemberDirectoryAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.listServerMembers.mockReset();
    mocks.listRoomMembers.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      listServerMembers: mocks.listServerMembers,
      listRoomMembers: mocks.listRoomMembers
    });
  });

  it('maps server member pages and sends bearer auth', async () => {
    mocks.listServerMembers.mockResolvedValue({
      members: [
        {
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
          },
          roles: ['everyone', 'admin'],
          createdAt: Timestamp.fromDate(new Date('2026-01-01T09:00:00Z'))
        }
      ],
      totalCount: 2,
      hasMore: true
    });

    const api = createMemberDirectoryAPI({
      baseUrl: 'https://remote.test/api/connect',
      bearerToken: 'token'
    });

    await expect(api.listServerMembers('ali', 10, 20)).resolves.toEqual({
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
    expect(mocks.listServerMembers).toHaveBeenCalledWith(
      { search: 'ali', limit: 10, offset: 20 },
      { headers: { Authorization: 'Bearer token' } }
    );
  });

  it('maps room member pages without auth headers', async () => {
    mocks.listRoomMembers.mockResolvedValue({
      members: [
        {
          id: 'U2',
          login: 'bob',
          displayName: 'Bob',
          deleted: false,
          presenceStatus: APIPresenceStatus.DO_NOT_DISTURB,
          roles: []
        }
      ],
      totalCount: 1,
      hasMore: false
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
      { roomId: 'room-1', search: 'bob', limit: 5, offset: 0 },
      { headers: undefined }
    );
  });

  it('maps offline and unspecified read statuses to offline', async () => {
    mocks.listServerMembers.mockResolvedValue({
      members: [
        {
          id: 'U3',
          login: 'carol',
          displayName: 'Carol',
          deleted: false,
          presenceStatus: APIPresenceStatus.OFFLINE,
          roles: []
        },
        {
          id: 'U4',
          login: 'dave',
          displayName: 'Dave',
          deleted: false,
          presenceStatus: APIPresenceStatus.UNSPECIFIED,
          roles: []
        }
      ],
      totalCount: 2,
      hasMore: false
    });

    const api = createMemberDirectoryAPI({ baseUrl: '/api/connect', bearerToken: null });

    await expect(api.listServerMembers()).resolves.toMatchObject({
      members: [
        { id: 'U3', presenceStatus: PresenceStatus.Offline },
        { id: 'U4', presenceStatus: PresenceStatus.Offline }
      ]
    });
  });
});
