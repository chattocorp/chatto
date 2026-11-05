import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { NotificationService } from '@chatto/api-types/api/v1/notifications_connect';
import { RoomDirectoryService } from '@chatto/api-types/api/v1/room_directory_connect';
import { ServerService } from '@chatto/api-types/api/v1/server_state_connect';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { ViewerService } from '@chatto/api-types/api/v1/viewer_connect';
import { VoiceCallService } from '@chatto/api-types/api/v1/voice_calls_connect';
import { createRealtimeResourceAPI } from '../realtimeResources.js';
import { RoomDirectoryScope } from '@chatto/api-types/api/v1/room_directory_pb';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import {
  fakeServer,
  mockService,
  receivedContext,
  receivedRequest
} from '../../testing/fakeServer.js';

const server = mockService(ServerService);
const viewer = mockService(ViewerService);
const users = mockService(UserService);
const rooms = mockService(RoomDirectoryService);
const notifications = mockService(NotificationService);
const calls = mockService(VoiceCallService);

function realtimeAPI(bearerToken: string | null = null) {
  return createRealtimeResourceAPI(
    fakeServer(
      (router) =>
        router
          .service(ServerService, server)
          .service(ViewerService, viewer)
          .service(UserService, users)
          .service(RoomDirectoryService, rooms)
          .service(NotificationService, notifications)
          .service(VoiceCallService, calls),
      { bearerToken }
    )
  );
}

describe('createRealtimeResourceAPI', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    server.getServerProfile.mockReturnValue({ profile: { name: 'Boundary Server' } });
    server.getMotd.mockReturnValue({ motd: 'Hello' });
    server.getRuntimeConfig.mockReturnValue({});
    viewer.getViewer.mockReturnValue({});
    rooms.listRooms.mockReturnValue({ rooms: [] });
    rooms.listRoomGroups.mockReturnValue({ groups: [] });
    notifications.listNotificationOccurrences.mockReturnValue({ occurrences: [] });
    calls.listActiveCalls.mockReturnValue({ calls: [] });
    users.batchGetUsers.mockImplementation(({ userIds }) => ({
      users: userIds.map((userId) => ({ user: { id: userId } }))
    }));
  });

  it('binds every bootstrap resource read to the opaque minimum cursor', async () => {
    const api = realtimeAPI('access-token');
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

    for (const handler of [
      server.getServerProfile,
      server.getMotd,
      server.getRuntimeConfig,
      viewer.getViewer,
      rooms.listRooms,
      rooms.listRoomGroups,
      notifications.listNotificationOccurrences,
      calls.listActiveCalls
    ]) {
      const context = receivedContext(handler);
      expect(context?.requestHeader.get('Chatto-Realtime-Minimum-Cursor')).toBe(cursor);
      expect(context?.requestHeader.get('Authorization')).toBe('Bearer access-token');
      expect(context?.timeoutMs()).toBeGreaterThan(9_000);
    }
    expect(users.listUsers).not.toHaveBeenCalled();
  });

  it.each([
    { family: 'channelRooms', scope: RoomDirectoryScope.CHANNELS, kind: RoomKind.CHANNEL },
    { family: 'directMessages', scope: RoomDirectoryScope.DMS, kind: RoomKind.DM }
  ] as const)(
    'collects every $family page independently at the minimum cursor',
    async ({ family, scope, kind }) => {
      rooms.listRooms
        .mockReturnValueOnce({ rooms: [{ room: { id: 'a', kind } }], page: { hasMore: true } })
        .mockReturnValueOnce({ rooms: [{ room: { id: 'b', kind } }], page: { hasMore: false } });
      const [update] = await realtimeAPI().read(family, 'cursor');
      expect(update.replace).toBe(true);
      expect(update.resource.case).toBe('rooms');
      if (update.resource.case !== 'rooms') throw new Error('expected rooms');
      expect(update.resource.scope).toBe(scope);
      expect(update.resource.value.rooms.map((entry) => entry.room?.id)).toEqual(['a', 'b']);
      expect(rooms.listRooms.mock.calls.map(([request]) => request.page)).toMatchObject([
        { limit: 100, offset: 0 },
        { limit: 100, offset: 1 }
      ]);
      for (const [, context] of rooms.listRooms.mock.calls) {
        expect(context.requestHeader.get('Chatto-Realtime-Minimum-Cursor')).toBe('cursor');
      }
      expect(rooms.listRooms.mock.calls.every(([request]) => request.scope === scope)).toBe(true);
    }
  );

  it.each(['channelRooms', 'directMessages'] as const)(
    'rejects a failed later %s page instead of returning a partial replacement',
    async (family) => {
      rooms.listRooms
        .mockReturnValueOnce({ rooms: [{ room: { id: 'a' } }], page: { hasMore: true } })
        .mockImplementationOnce(() => {
          throw new ConnectError('second page failed', Code.Unavailable);
        });
      await expect(realtimeAPI().read(family)).rejects.toMatchObject({
        code: Code.Unavailable,
        rawMessage: 'second page failed'
      });
    }
  );

  it('keeps DM collection independent when the channel list changes between pages', async () => {
    const channels = Array.from({ length: 100 }, (_, index) => ({
      room: { id: `channel-${index}`, kind: RoomKind.CHANNEL }
    }));
    const dm = { room: { id: 'dm', kind: RoomKind.DM }, hasMessageHistory: true };
    const requests: Array<{ scope: RoomDirectoryScope; offset: number }> = [];
    rooms.listRooms.mockImplementation(({ scope, page }) => {
      requests.push({ scope, offset: page?.offset ?? 0 });
      if (scope === RoomDirectoryScope.DMS) return { rooms: [dm] };
      return { rooms: channels, page: { hasMore: false } };
    });
    const api = realtimeAPI();
    const [channelUpdate] = await api.read('channelRooms');
    channels.shift();
    const [dmUpdate] = await api.read('directMessages');
    expect(
      channelUpdate.resource.case === 'rooms' && channelUpdate.resource.value.rooms
    ).toHaveLength(100);
    expect(dmUpdate.resource.case === 'rooms' && dmUpdate.resource.value.rooms).toMatchObject([dm]);
    expect(requests).toEqual([
      { scope: RoomDirectoryScope.CHANNELS, offset: 0 },
      { scope: RoomDirectoryScope.DMS, offset: 0 }
    ]);
  });

  it.each([
    { family: 'channelRooms', kind: RoomKind.DM },
    { family: 'directMessages', kind: RoomKind.CHANNEL }
  ] as const)(
    'does not publish the other kind from a mis-scoped $family response',
    async ({ family, kind }) => {
      rooms.listRooms.mockReturnValue({ rooms: [{ room: { id: 'other', kind } }] });
      const [update] = await realtimeAPI().read(family);
      expect(update.resource.case === 'rooms' && update.resource.value.rooms).toEqual([]);
    }
  );

  it('rejects an empty continuation instead of looping', async () => {
    rooms.listRooms.mockReturnValue({ rooms: [], page: { hasMore: true } });
    await expect(realtimeAPI().read('rooms')).rejects.toThrow('empty continuation');
    expect(rooms.listRooms).toHaveBeenCalledTimes(1);
  });

  it('hydrates only requested users in bounded merge batches', async () => {
    const userIds = Array.from({ length: 101 }, (_, index) => `user-${index}`);

    const [update] = await realtimeAPI().readUsers([...userIds, 'user-0'], 'opaque-E');

    expect(users.batchGetUsers).toHaveBeenCalledTimes(2);
    expect(receivedRequest(users.batchGetUsers, 0)?.userIds).toHaveLength(100);
    expect(receivedRequest(users.batchGetUsers, 1)?.userIds).toEqual(['user-100']);
    expect(receivedContext(users.batchGetUsers)?.timeoutMs()).toBeGreaterThan(9_000);
    expect(update.replace).toBe(false);
    expect(update.resource.case).toBe('users');
    if (update.resource.case !== 'users') throw new Error('expected users resource');
    expect(update.resource.value.users).toHaveLength(101);
    expect(users.listUsers).not.toHaveBeenCalled();
  });
});
