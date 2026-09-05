import {
  authHeaders,
  createChattoClient,
  REALTIME_MINIMUM_CURSOR_HEADER,
  type ConnectAPIConfig
} from './connect';
import { NotificationService } from '@chatto/api-types/api/v1/notifications_connect';
import { RoomDirectoryService } from '@chatto/api-types/api/v1/room_directory_connect';
import { ServerService } from '@chatto/api-types/api/v1/server_state_connect';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { ViewerService } from '@chatto/api-types/api/v1/viewer_connect';
import { VoiceCallService } from '@chatto/api-types/api/v1/voice_calls_connect';
import type { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import type {
  ListRoomGroupsResponse,
  ListRoomsResponse
} from '@chatto/api-types/api/v1/room_directory_pb';
import type { ServerPublicProfile } from '@chatto/api-types/api/v1/server_pb';
import type {
  GetMotdResponse,
  GetRuntimeConfigResponse
} from '@chatto/api-types/api/v1/server_state_pb';
import type { GetViewerResponse } from '@chatto/api-types/api/v1/viewer_pb';
import type { ListActiveCallsResponse } from '@chatto/api-types/api/v1/voice_calls_pb';
import type { ListNotificationOccurrencesResponse } from '@chatto/api-types/api/v1/notifications_pb';

const REALTIME_RESOURCE_TIMEOUT_MS = 10_000;
const USER_BATCH_SIZE = 100;

export type RealtimeResourceFamily =
  'server' | 'serverState' | 'viewer' | 'rooms' | 'roomGroups' | 'notifications' | 'activeCalls';

export type RealtimeResource =
  | { case: 'server'; value: ServerPublicProfile }
  | { case: 'motd'; value: GetMotdResponse }
  | { case: 'runtimeConfig'; value: GetRuntimeConfigResponse }
  | { case: 'viewer'; value: GetViewerResponse }
  | { case: 'users'; value: { users: DirectoryMember[] } }
  | { case: 'rooms'; value: ListRoomsResponse }
  | { case: 'roomGroups'; value: ListRoomGroupsResponse }
  | { case: 'notifications'; value: ListNotificationOccurrencesResponse }
  | { case: 'activeCalls'; value: ListActiveCallsResponse };

/** One canonical ConnectRPC resource response applied to local server state. */
export class RealtimeResourceUpdate {
  readonly resource: RealtimeResource;
  /** Replace the complete resource family instead of merging returned rows. */
  readonly replace: boolean;

  constructor(init: { resource: RealtimeResource; replace?: boolean }) {
    this.resource = init.resource;
    this.replace = init.replace ?? true;
  }
}

/** Reads canonical public resources used by the retained server projection. */
export function createRealtimeResourceAPI(config: ConnectAPIConfig) {
  const server = createChattoClient(ServerService, config);
  const viewer = createChattoClient(ViewerService, config);
  const users = createChattoClient(UserService, config);
  const rooms = createChattoClient(RoomDirectoryService, config);
  const notifications = createChattoClient(NotificationService, config);
  const calls = createChattoClient(VoiceCallService, config);

  const headers = (minimumCursor?: string): Headers => {
    const result = new Headers(authHeaders(config));
    if (minimumCursor) result.set(REALTIME_MINIMUM_CURSOR_HEADER, minimumCursor);
    return result;
  };
  const options = (minimumCursor?: string) => ({
    headers: headers(minimumCursor),
    timeoutMs: REALTIME_RESOURCE_TIMEOUT_MS
  });

  const read = async (
    family: RealtimeResourceFamily,
    minimumCursor?: string
  ): Promise<RealtimeResourceUpdate[]> => {
    switch (family) {
      case 'server': {
        const response = await server.getServerProfile({}, options(minimumCursor));
        return response.profile
          ? [new RealtimeResourceUpdate({ resource: { case: 'server', value: response.profile } })]
          : [];
      }
      case 'serverState': {
        const [motd, runtime] = await Promise.all([
          server.getMotd({}, options(minimumCursor)),
          server.getRuntimeConfig({}, options(minimumCursor))
        ]);
        return [
          new RealtimeResourceUpdate({ resource: { case: 'motd', value: motd } }),
          new RealtimeResourceUpdate({ resource: { case: 'runtimeConfig', value: runtime } })
        ];
      }
      case 'viewer': {
        const response = await viewer.getViewer({}, options(minimumCursor));
        return [new RealtimeResourceUpdate({ resource: { case: 'viewer', value: response } })];
      }
      case 'rooms': {
        const response = await rooms.listRooms({}, options(minimumCursor));
        return [new RealtimeResourceUpdate({ resource: { case: 'rooms', value: response } })];
      }
      case 'roomGroups': {
        const response = await rooms.listRoomGroups({}, options(minimumCursor));
        return [new RealtimeResourceUpdate({ resource: { case: 'roomGroups', value: response } })];
      }
      case 'notifications': {
        const response = await notifications.listNotificationOccurrences(
          { page: { limit: 50 } },
          options(minimumCursor)
        );
        return [
          new RealtimeResourceUpdate({ resource: { case: 'notifications', value: response } })
        ];
      }
      case 'activeCalls': {
        const response = await calls.listActiveCalls({}, options(minimumCursor));
        return [new RealtimeResourceUpdate({ resource: { case: 'activeCalls', value: response } })];
      }
    }
  };

  const readUsers = async (
    userIds: Iterable<string>,
    minimumCursor?: string
  ): Promise<RealtimeResourceUpdate[]> => {
    const uniqueIds = [...new Set(userIds)].filter(Boolean);
    if (uniqueIds.length === 0) return [];
    const entries: DirectoryMember[] = [];
    for (let offset = 0; offset < uniqueIds.length; offset += USER_BATCH_SIZE) {
      const response = await users.batchGetUsers(
        { userIds: uniqueIds.slice(offset, offset + USER_BATCH_SIZE) },
        options(minimumCursor)
      );
      entries.push(...response.users);
    }
    return [
      new RealtimeResourceUpdate({
        resource: { case: 'users', value: { users: entries } },
        replace: false
      })
    ];
  };

  return { read, readUsers };
}

export type RealtimeResourceAPI = ReturnType<typeof createRealtimeResourceAPI>;
