import { authHeaders, createChattoClient, type ConnectAPIConfig } from './connect';
import { NotificationService } from '@chatto/api-types/api/v1/notifications_connect';
import { RoomDirectoryService } from '@chatto/api-types/api/v1/room_directory_connect';
import { ServerService } from '@chatto/api-types/api/v1/server_state_connect';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { ViewerService } from '@chatto/api-types/api/v1/viewer_connect';
import { VoiceCallService } from '@chatto/api-types/api/v1/voice_calls_connect';
import { ServerDiscoveryService } from '@chatto/api-types/chatto/discovery/v1/server_connect';
import { ServerSnapshotChunk } from '@chatto/api-types/api/v1/server_snapshot_pb';
import { ListUsersResponse } from '@chatto/api-types/api/v1/user_service_pb';

export type RealtimeResourceFamily =
  | 'server'
  | 'serverState'
  | 'viewer'
  | 'users'
  | 'rooms'
  | 'roomGroups'
  | 'notifications'
  | 'activeCalls';

/** Reads canonical public resources after a semantic realtime invalidation. */
export function createRealtimeResourceAPI(config: ConnectAPIConfig) {
  const discovery = createChattoClient(ServerDiscoveryService, config);
  const server = createChattoClient(ServerService, config);
  const viewer = createChattoClient(ViewerService, config);
  const users = createChattoClient(UserService, config);
  const rooms = createChattoClient(RoomDirectoryService, config);
  const notifications = createChattoClient(NotificationService, config);
  const calls = createChattoClient(VoiceCallService, config);
  const headers = () => authHeaders(config);

  return {
    async read(family: RealtimeResourceFamily): Promise<ServerSnapshotChunk[]> {
      switch (family) {
        case 'server': {
          const response = await discovery.getServer({});
          return response.profile
            ? [new ServerSnapshotChunk({ resource: { case: 'server', value: response.profile } })]
            : [];
        }
        case 'serverState': {
          const [motd, runtime] = await Promise.all([
            server.getMotd({}, { headers: headers() }),
            server.getRuntimeConfig({}, { headers: headers() })
          ]);
          return [
            new ServerSnapshotChunk({ resource: { case: 'motd', value: motd } }),
            new ServerSnapshotChunk({ resource: { case: 'runtimeConfig', value: runtime } })
          ];
        }
        case 'viewer': {
          const response = await viewer.getViewer({}, { headers: headers() });
          return [new ServerSnapshotChunk({ resource: { case: 'viewer', value: response } })];
        }
        case 'users': {
          const entries = [];
          for (let offset = 0; ; offset += 500) {
            const response = await users.listUsers(
              { page: { limit: 500, offset } },
              { headers: headers() }
            );
            entries.push(...response.users);
            if (!response.page?.hasMore || response.users.length === 0) break;
          }
          return [
            new ServerSnapshotChunk({
              resource: { case: 'users', value: new ListUsersResponse({ users: entries }) }
            })
          ];
        }
        case 'rooms': {
          const response = await rooms.listRooms({}, { headers: headers() });
          return [new ServerSnapshotChunk({ resource: { case: 'rooms', value: response } })];
        }
        case 'roomGroups': {
          const response = await rooms.listRoomGroups({}, { headers: headers() });
          return [new ServerSnapshotChunk({ resource: { case: 'roomGroups', value: response } })];
        }
        case 'notifications': {
          const response = await notifications.listNotificationOccurrences(
            { page: { limit: 50 } },
            { headers: headers() }
          );
          return [
            new ServerSnapshotChunk({ resource: { case: 'notifications', value: response } })
          ];
        }
        case 'activeCalls': {
          const response = await calls.listActiveCalls({}, { headers: headers() });
          return [new ServerSnapshotChunk({ resource: { case: 'activeCalls', value: response } })];
        }
      }
    }
  };
}

export type RealtimeResourceAPI = ReturnType<typeof createRealtimeResourceAPI>;
