import { beforeEach, describe, expect, it, vi } from 'vitest';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { User } from '@chatto/api-types/api/v1/users_pb';
import { createRealtimeResourceAPI } from '$lib/api-client/realtimeResources';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  getServerProfile: vi.fn(),
  getMotd: vi.fn(),
  getRuntimeConfig: vi.fn(),
  getViewer: vi.fn(),
  listRooms: vi.fn(),
  listRoomGroups: vi.fn(),
  listNotificationOccurrences: vi.fn(),
  listActiveCalls: vi.fn(),
  batchGetUsers: vi.fn(),
  listUsers: vi.fn()
}));

vi.mock('@connectrpc/connect', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@connectrpc/connect')>();
  return { ...actual, createClient: mocks.createClient };
});

vi.mock('@connectrpc/connect-web', () => ({
  createConnectTransport: mocks.createConnectTransport
}));

describe('createRealtimeResourceAPI', () => {
  beforeEach(() => {
    for (const mock of Object.values(mocks)) mock.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      getServerProfile: mocks.getServerProfile,
      getMotd: mocks.getMotd,
      getRuntimeConfig: mocks.getRuntimeConfig,
      getViewer: mocks.getViewer,
      listRooms: mocks.listRooms,
      listRoomGroups: mocks.listRoomGroups,
      listNotificationOccurrences: mocks.listNotificationOccurrences,
      listActiveCalls: mocks.listActiveCalls,
      batchGetUsers: mocks.batchGetUsers,
      listUsers: mocks.listUsers
    });
    mocks.getServerProfile.mockResolvedValue({ profile: { name: 'Boundary Server' } });
    mocks.getMotd.mockResolvedValue({ motd: 'Hello' });
    mocks.getRuntimeConfig.mockResolvedValue({});
    mocks.getViewer.mockResolvedValue({});
    mocks.listRooms.mockResolvedValue({ rooms: [] });
    mocks.listRoomGroups.mockResolvedValue({ groups: [] });
    mocks.listNotificationOccurrences.mockResolvedValue({ occurrences: [] });
    mocks.listActiveCalls.mockResolvedValue({ calls: [] });
    mocks.batchGetUsers.mockImplementation(({ userIds }: { userIds: string[] }) =>
      Promise.resolve({
        users: userIds.map((userId) => new DirectoryMember({ user: new User({ id: userId }) }))
      })
    );
  });

  it('binds every bootstrap resource read to the opaque minimum cursor', async () => {
    const api = createRealtimeResourceAPI({
      baseUrl: 'https://chat.example.test/api/connect',
      bearerToken: 'access-token'
    });
    const cursor = 'opaque-E';

    await Promise.all(
      [
        'server',
        'serverState',
        'viewer',
        'rooms',
        'roomGroups',
        'notifications',
        'activeCalls'
      ].map((family) => api.read(family as Parameters<typeof api.read>[0], cursor))
    );

    for (const call of [
      mocks.getServerProfile,
      mocks.getMotd,
      mocks.getRuntimeConfig,
      mocks.getViewer,
      mocks.listRooms,
      mocks.listRoomGroups,
      mocks.listNotificationOccurrences,
      mocks.listActiveCalls
    ]) {
      const options = call.mock.calls[0]?.at(-1) as
        { headers?: Headers; timeoutMs?: number } | undefined;
      expect(options?.headers?.get('Authorization')).toBe('Bearer access-token');
      expect(options?.headers?.get('Chatto-Realtime-Minimum-Cursor')).toBe(cursor);
      expect(options?.timeoutMs).toBe(10_000);
    }
    expect(mocks.listUsers).not.toHaveBeenCalled();
  });

  it('hydrates only requested users in bounded merge batches', async () => {
    const api = createRealtimeResourceAPI({
      baseUrl: 'https://chat.example.test/api/connect',
      bearerToken: null
    });
    const userIds = Array.from({ length: 101 }, (_, index) => `user-${index}`);

    const [update] = await api.readUsers([...userIds, 'user-0'], 'opaque-E');

    expect(mocks.batchGetUsers).toHaveBeenCalledTimes(2);
    expect(mocks.batchGetUsers.mock.calls[0]?.[0].userIds).toHaveLength(100);
    expect(mocks.batchGetUsers.mock.calls[1]?.[0].userIds).toEqual(['user-100']);
    expect(mocks.batchGetUsers.mock.calls[0]?.[1].timeoutMs).toBe(10_000);
    expect(update.replace).toBe(false);
    expect(update.resource.case).toBe('users');
    if (update.resource.case !== 'users') throw new Error('expected users resource');
    expect(update.resource.value.users).toHaveLength(101);
    expect(mocks.listUsers).not.toHaveBeenCalled();
  });
});
