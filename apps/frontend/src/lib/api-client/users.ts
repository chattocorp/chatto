import {
  authHeaders,
  createChattoClient,
  REALTIME_MINIMUM_CURSOR_HEADER,
  type ConnectAPIConfig
} from './connect.js';
import { getUserStore } from '$lib/state/server/users.svelte';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { DirectoryMember as APIDirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import type { User } from '@chatto/api-types/api/v1/users_pb';

const REALTIME_RESOURCE_TIMEOUT_MS = 10_000;

export { mapUserSummary, mapOptionalUserSummary, type UserSummary } from './userSummary.js';
import { mapOptionalUserSummary, mapUserSummary, type UserSummary } from './userSummary.js';

export type UserAPIConfig = ConnectAPIConfig;

export function createUserAPI(config: UserAPIConfig) {
  const client = createChattoClient(UserService, config);
  const headers = () => authHeaders(config);
  const store = config.serverId ? getUserStore(config.serverId, config.queryScope) : undefined;
  const updateProfile = async (read: () => Promise<User>): Promise<UserSummary> => {
    if (!store) return mapUserSummary(await read());
    const members = await store.readSnapshot(async () => {
      const user = await read();
      return [new APIDirectoryMember({ ...store.get(user.id), user })];
    });
    return mapUserSummary(requiredUser(members[0]?.user));
  };

  return {
    async batchGetUsers(userIds: string[], minimumCursor?: string): Promise<UserSummary[]> {
      let requestHeaders: HeadersInit | undefined = headers();
      if (minimumCursor) {
        const boundedHeaders = new Headers(requestHeaders);
        boundedHeaders.set(REALTIME_MINIMUM_CURSOR_HEADER, minimumCursor);
        requestHeaders = boundedHeaders;
      }
      const read = async (ids: string[]) => (await client.batchGetUsers(
        { userIds: ids },
        {
          headers: requestHeaders,
          ...(minimumCursor ? { timeoutMs: REALTIME_RESOURCE_TIMEOUT_MS } : {})
        }
      )).users;
      const members = store ? await store.resolve(userIds, read, minimumCursor) : await read(userIds);
      return members.flatMap((member) => {
        const summary = member.user;
        return summary ? [mapUserSummary(summary)] : [];
      });
    },
    async uploadAvatar(userId: string, file: File): Promise<UserSummary> {
      return updateProfile(async () => {
        const response = await client.uploadAvatar(
        {
          userId,
          image: {
            image: new Uint8Array(await file.arrayBuffer()),
            filename: file.name,
            contentType: file.type
          }
        },
        { headers: headers() }
      );
        return requiredUser(response.user);
      });
    },
    async deleteAvatar(userId: string): Promise<UserSummary> {
      return updateProfile(async () => {
        const response = await client.deleteAvatar({ userId }, { headers: headers() });
        return requiredUser(response.user);
      });
    }
  };
}

export type UserAPI = ReturnType<typeof createUserAPI>;

export function mapDirectoryMemberUserSummary(member: APIDirectoryMember): UserSummary | null {
  return mapOptionalUserSummary(member.user);
}

function requiredUser(user: Parameters<typeof mapUserSummary>[0] | undefined) {
  if (!user) throw new Error('avatar response did not include a user');
  return user;
}
