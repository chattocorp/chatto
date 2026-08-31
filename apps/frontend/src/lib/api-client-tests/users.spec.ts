import { describe, expect, it, vi, beforeEach } from 'vitest';
import { DirectoryMember as APIDirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { User as APIUser } from '@chatto/api-types/api/v1/users_pb';
import { createUserAPI, mapUserSummary } from '$lib/api-client/users';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  batchGetUsers: vi.fn(),
  uploadAvatar: vi.fn(),
  deleteAvatar: vi.fn()
}));

vi.mock('@connectrpc/connect', () => ({
  createClient: mocks.createClient
}));

vi.mock('@connectrpc/connect-web', () => ({
  createConnectTransport: mocks.createConnectTransport
}));

describe('createUserAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.batchGetUsers.mockReset();
    mocks.uploadAvatar.mockReset();
    mocks.deleteAvatar.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      batchGetUsers: mocks.batchGetUsers,
      uploadAvatar: mocks.uploadAvatar,
      deleteAvatar: mocks.deleteAvatar
    });
  });

  it('uploads and deletes an avatar for an explicit user', async () => {
    mocks.uploadAvatar.mockResolvedValue({
      user: new APIUser({
        id: 'U1',
        login: 'alice',
        displayName: 'Alice',
        avatarUrl: 'https://cdn/new-avatar.webp'
      })
    });
    mocks.deleteAvatar.mockResolvedValue({
      user: new APIUser({ id: 'U1', login: 'alice', displayName: 'Alice' })
    });
    const api = createUserAPI({ baseUrl: '/api/connect', bearerToken: 'token' });
    const file = new File([new Uint8Array([1, 2, 3])], 'avatar.png', { type: 'image/png' });

    await expect(api.uploadAvatar('U1', file)).resolves.toMatchObject({
      id: 'U1',
      avatarUrl: 'https://cdn/new-avatar.webp'
    });
    await expect(api.deleteAvatar('U1')).resolves.toMatchObject({ id: 'U1', avatarUrl: null });
    expect(mocks.uploadAvatar).toHaveBeenCalledWith(
      {
        userId: 'U1',
        image: {
          image: new Uint8Array([1, 2, 3]),
          filename: 'avatar.png',
          contentType: 'image/png'
        }
      },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(mocks.deleteAvatar).toHaveBeenCalledWith(
      { userId: 'U1' },
      { headers: { Authorization: 'Bearer token' } }
    );
  });

  it('loads user summaries in batches and sends bearer auth', async () => {
    mocks.batchGetUsers.mockResolvedValue({
      users: [
        new APIDirectoryMember({
          user: new APIUser({
            id: 'U1',
            login: 'alice',
            displayName: 'Alice',
            deleted: false,
            isBot: true,
            avatarUrl: 'https://cdn/avatar.webp'
          })
        })
      ]
    });

    const api = createUserAPI({
      baseUrl: 'https://remote.test/api/connect',
      bearerToken: 'token'
    });

    await expect(api.batchGetUsers(['U1', 'U2'])).resolves.toEqual([
      {
        id: 'U1',
        login: 'alice',
        displayName: 'Alice',
        deleted: false,
        isBot: true,
        avatarUrl: 'https://cdn/avatar.webp',
        bio: null,
        timezone: null
      }
    ]);

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://remote.test/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.batchGetUsers).toHaveBeenCalledWith(
      { userIds: ['U1', 'U2'] },
      { headers: { Authorization: 'Bearer token' } }
    );
  });

  it('maps missing avatar URLs to null', () => {
    expect(
      mapUserSummary(
        new APIUser({
          id: 'U2',
          login: 'bob',
          displayName: 'Bob',
          deleted: false,
          avatarUrl: ''
        })
      )
    ).toEqual({
      id: 'U2',
      login: 'bob',
      displayName: 'Bob',
      deleted: false,
      isBot: false,
      avatarUrl: null,
      bio: null,
      timezone: null
    });
  });
});
