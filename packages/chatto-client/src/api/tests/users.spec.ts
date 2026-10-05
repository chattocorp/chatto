import { describe, expect, it, beforeEach, vi } from 'vitest';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { DirectoryMember as APIDirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { User as APIUser } from '@chatto/api-types/api/v1/users_pb';
import { createUserAPI, mapUserSummary } from '../users.js';
import { createMemberDirectoryAPI } from '../memberDirectory.js';
import { getUserStore, resetUserStoresForTests } from '../../server/users.js';
import { fakeServer, mockService, receivedRequest } from '../../testing/fakeServer.js';

const mocks = mockService(UserService);

/** An API config for the `server` session whose requests reach the mocked user service. */
function config() {
  return fakeServer((router) => router.service(UserService, mocks), {
    serverId: 'server',
    queryScope: 'session'
  });
}

describe('createUserAPI', () => {
  it('shares a pending profile read with the room directory adapter', async () => {
    const userAPI = createUserAPI(config());
    const directoryAPI = createMemberDirectoryAPI(config());
    mocks.batchGetUsers.mockReturnValue({
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
    vi.resetAllMocks();
  });

  it('acknowledges an avatar command when its profile event arrives before the response', async () => {
    const store = getUserStore('server', 'session');
    mocks.deleteAvatar.mockImplementation(async () => {
      store.invalidate('U1');
      return { user: new APIUser({ id: 'U1', login: 'alice' }) };
    });
    const api = createUserAPI(config());
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
      const api = createUserAPI(config());
      await expect(api.deleteAvatar('U1')).rejects.toThrow();
      expect(store.has('U1')).toBe(false);
    }
  );

  it('updates the profile of an explicit user with a sparse update mask', async () => {
    mocks.updateUserProfile.mockReturnValue({
      user: new APIUser({
        id: 'B1',
        login: 'helper',
        displayName: 'Helper',
        bio: 'Answers questions.',
        bot: { ownerUserId: 'U1' }
      })
    });
    const api = createUserAPI(config());

    await expect(
      api.updateUserProfile('B1', { bio: 'Answers questions.', displayName: 'Helper' })
    ).resolves.toMatchObject({
      id: 'B1',
      displayName: 'Helper',
      bio: 'Answers questions.',
      bot: { ownerUserId: 'U1' }
    });
    expect(receivedRequest(mocks.updateUserProfile)).toMatchObject({
      userId: 'B1',
      bio: 'Answers questions.',
      displayName: 'Helper',
      updateMask: { paths: ['display_name', 'bio'] }
    });
  });

  it('uploads and deletes an avatar for an explicit user', async () => {
    mocks.uploadAvatar.mockReturnValue({
      user: new APIUser({
        id: 'U1',
        login: 'alice',
        displayName: 'Alice',
        avatarUrl: 'https://cdn/new-avatar.webp'
      })
    });
    mocks.deleteAvatar.mockReturnValue({
      user: new APIUser({ id: 'U1', login: 'alice', displayName: 'Alice' })
    });
    const api = createUserAPI(config());
    const file = new File([new Uint8Array([1, 2, 3])], 'avatar.png', { type: 'image/png' });

    await expect(api.uploadAvatar('U1', file)).resolves.toMatchObject({
      id: 'U1',
      avatarUrl: 'https://cdn/new-avatar.webp'
    });
    await expect(api.deleteAvatar('U1')).resolves.toMatchObject({ id: 'U1', avatarUrl: null });
    expect(receivedRequest(mocks.uploadAvatar)).toMatchObject({
      userId: 'U1',
      image: {
        image: new Uint8Array([1, 2, 3]),
        filename: 'avatar.png',
        contentType: 'image/png'
      }
    });
    expect(receivedRequest(mocks.deleteAvatar)).toMatchObject({ userId: 'U1' });
  });

  it('loads user summaries in batches', async () => {
    mocks.batchGetUsers.mockReturnValue({
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

    const api = createUserAPI(config());

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

    expect(receivedRequest(mocks.batchGetUsers)).toMatchObject({ userIds: ['U1', 'U2'] });
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

  it('updates profiles without a server store and rejects answers without a user', async () => {
    const api = createUserAPI(fakeServer((router) => router.service(UserService, mocks)));
    mocks.deleteAvatar.mockReturnValue({ user: { id: 'u1', login: 'ada' } });
    await expect(api.deleteAvatar('u1')).resolves.toMatchObject({ id: 'u1', login: 'ada' });
    mocks.deleteAvatar.mockReturnValue({});
    await expect(api.deleteAvatar('u1')).rejects.toThrow('did not include a user');
  });
});
