import { describe, expect, it, vi, beforeEach } from 'vitest';
import { User as APIUser } from '$lib/pb/chatto/api/v1/users_pb';
import { createUserAPI, mapUserSummary } from './users';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  batchGetUsers: vi.fn()
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
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      batchGetUsers: mocks.batchGetUsers
    });
  });

  it('loads user summaries in batches and sends bearer auth', async () => {
    mocks.batchGetUsers.mockResolvedValue({
      users: [
        {
          id: 'U1',
          login: 'alice',
          displayName: 'Alice',
          deleted: false,
          avatarUrl: 'https://cdn/avatar.webp'
        }
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
        avatarUrl: 'https://cdn/avatar.webp'
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
      mapUserSummary(new APIUser({
        id: 'U2',
        login: 'bob',
        displayName: 'Bob',
        deleted: false,
        avatarUrl: ''
      }))
    ).toEqual({
      id: 'U2',
      login: 'bob',
      displayName: 'Bob',
      deleted: false,
      avatarUrl: null
    });
  });
});
