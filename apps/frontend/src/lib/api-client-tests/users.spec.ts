import { describe, expect, it, vi, beforeEach } from 'vitest';
import { DirectoryMember as APIDirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { User as APIUser } from '@chatto/api-types/api/v1/users_pb';
import { createUserAPI, mapUserSummary } from '$lib/api-client/users';
import { createMemberDirectoryAPI } from '$lib/api-client/memberDirectory';
import { getUserStore, resetUserStoresForTests } from '$lib/state/server/users.svelte';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  batchGetUsers: vi.fn(),
  updateUserProfile: vi.fn(),
  uploadAvatar: vi.fn(),
  deleteAvatar: vi.fn()
}));

vi.mock('@connectrpc/connect', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@connectrpc/connect')>()),
  createClient: mocks.createClient
}));

vi.mock('@connectrpc/connect-web', () => ({
  createConnectTransport: mocks.createConnectTransport
}));

describe('createUserAPI', () => {
  it('shares a pending profile read with the room directory adapter', async () => {
    const config = {
      serverId: 'server',
      queryScope: 'session',
      baseUrl: '/api/connect',
      bearerToken: null
    };
    const userAPI = createUserAPI(config);
    const directoryAPI = createMemberDirectoryAPI(config);
    mocks.batchGetUsers.mockResolvedValue({
      users: [
        new APIDirectoryMember({
          user: { id: 'bot', login: 'bot', bot: { ownerUserId: 'owner' } }
        })
      ]
    });
    const [summaries, members] = await Promise.all([
      userAPI.batchGetUsers(['bot']),
      directoryAPI.batchGetUsers(['bot'])
    ]);
    expect(summaries[0].bot?.ownerUserId).toBe('owner');
    expect(members[0].isBot).toBe(true);
    expect(mocks.batchGetUsers).toHaveBeenCalledOnce();
  });
  beforeEach(() => {
    resetUserStoresForTests();
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.batchGetUsers.mockReset();
    mocks.updateUserProfile.mockReset();
    mocks.uploadAvatar.mockReset();
    mocks.deleteAvatar.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      batchGetUsers: mocks.batchGetUsers,
      updateUserProfile: mocks.updateUserProfile,
      uploadAvatar: mocks.uploadAvatar,
      deleteAvatar: mocks.deleteAvatar
    });
  });

  it('acknowledges an avatar command when its profile event arrives before the response', async () => {
    const store = getUserStore('server', 'session');
    mocks.deleteAvatar.mockImplementation(async () => {
      store.invalidate('U1');
      return { user: new APIUser({ id: 'U1', login: 'alice' }) };
    });
    const api = createUserAPI({
      serverId: 'server',
      queryScope: 'session',
      baseUrl: '/api/connect',
      bearerToken: null
    });
    await expect(api.deleteAvatar('U1')).resolves.toMatchObject({ id: 'U1', avatarUrl: null });
    expect(store.has('U1')).toBe(false);
  });

  it.each(['delete', 'clear'])(
    'rejects an avatar response after the profile privacy boundary %s',
    async (boundary) => {
      const store = getUserStore('server', 'session');
      mocks.deleteAvatar.mockImplementation(async () => {
        if (boundary === 'delete') store.delete('U1');
        else store.clear();
        return { user: new APIUser({ id: 'U1', login: 'alice' }) };
      });
      const api = createUserAPI({
        serverId: 'server',
        queryScope: 'session',
        baseUrl: '/api/connect',
        bearerToken: null
      });
      await expect(api.deleteAvatar('U1')).rejects.toThrow();
      expect(store.has('U1')).toBe(false);
    }
  );

  it('updates the profile of an explicit user with a sparse update mask', async () => {
    mocks.updateUserProfile.mockResolvedValue({
      user: new APIUser({
        id: 'B1',
        login: 'helper',
        displayName: 'Helper',
        bio: 'Answers questions.',
        bot: { ownerUserId: 'U1' }
      })
    });
    const api = createUserAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

    await expect(
      api.updateUserProfile('B1', { bio: 'Answers questions.', displayName: 'Helper' })
    ).resolves.toMatchObject({
      id: 'B1',
      displayName: 'Helper',
      bio: 'Answers questions.',
      bot: { ownerUserId: 'U1' }
    });
    expect(mocks.updateUserProfile).toHaveBeenCalledWith({
      userId: 'B1',
      bio: 'Answers questions.',
      displayName: 'Helper',
      updateMask: { paths: ['display_name', 'bio'] }
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
    expect(mocks.uploadAvatar).toHaveBeenCalledWith({
      userId: 'U1',
      image: {
        image: new Uint8Array([1, 2, 3]),
        filename: 'avatar.png',
        contentType: 'image/png'
      }
    });
    expect(mocks.deleteAvatar).toHaveBeenCalledWith({ userId: 'U1' });
  });

  it('loads user summaries in batches', async () => {
    mocks.batchGetUsers.mockResolvedValue({
      users: [
        new APIDirectoryMember({
          user: new APIUser({
            id: 'U1',
            login: 'alice',
            displayName: 'Alice',
            deleted: false,
            bot: { ownerUserId: 'owner' },
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
        bot: { ownerUserId: 'owner' },
        avatarUrl: 'https://cdn/avatar.webp',
        bio: null,
        timezone: null
      }
    ]);

    expect(mocks.createConnectTransport).toHaveBeenCalledWith(
      expect.objectContaining({
        baseUrl: 'https://remote.test/api/connect',
        useBinaryFormat: true
      })
    );
    expect(mocks.batchGetUsers).toHaveBeenCalledWith(
      { userIds: ['U1', 'U2'] },
      { headers: undefined }
    );
  });

  it('maps a bot owner identity without treating it as a management grant', () => {
    expect(
      mapUserSummary(new APIUser({ id: 'bot', bot: { ownerUserId: 'owner' } })).bot?.ownerUserId
    ).toBe('owner');
  });

  it('uses bot metadata presence as the bot marker', () => {
    expect(mapUserSummary(new APIUser({ id: 'human' })).isBot).toBe(false);
    expect(mapUserSummary(new APIUser({ id: 'bot', bot: {} })).isBot).toBe(true);
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
