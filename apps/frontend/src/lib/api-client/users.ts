import {
  authHeaders,
  createChattoClient,
  REALTIME_MINIMUM_CURSOR_HEADER,
  type ConnectAPIConfig
} from './connect.js';
import { mapDirectoryMember } from './memberDirectory';
import { primeRegisteredDirectoryUsers } from '$lib/query/cacheRegistry';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import type { DirectoryMember as APIDirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';

const REALTIME_RESOURCE_TIMEOUT_MS = 10_000;

export { mapUserSummary, mapOptionalUserSummary, type UserSummary } from './userSummary.js';
import { mapOptionalUserSummary, mapUserSummary, type UserSummary } from './userSummary.js';

export type UserAPIConfig = ConnectAPIConfig;

export function createUserAPI(config: UserAPIConfig) {
  const client = createChattoClient(UserService, config);
  const headers = () => authHeaders(config);

  return {
    async batchGetUsers(userIds: string[], minimumCursor?: string): Promise<UserSummary[]> {
      let requestHeaders: HeadersInit | undefined = headers();
      if (minimumCursor) {
        const boundedHeaders = new Headers(requestHeaders);
        boundedHeaders.set(REALTIME_MINIMUM_CURSOR_HEADER, minimumCursor);
        requestHeaders = boundedHeaders;
      }
      const response = await client.batchGetUsers(
        { userIds },
        {
          headers: requestHeaders,
          ...(minimumCursor ? { timeoutMs: REALTIME_RESOURCE_TIMEOUT_MS } : {})
        }
      );
      if (config.serverId && config.queryScope) {
        primeRegisteredDirectoryUsers(
          config.serverId,
          config.queryScope,
          response.users.map(mapDirectoryMember),
          false
        );
      }
      return response.users.flatMap((member) => {
        const summary = member.user;
        return summary ? [mapUserSummary(summary)] : [];
      });
    },
    async uploadAvatar(userId: string, file: File): Promise<UserSummary> {
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
      return mapUserSummary(requiredUser(response.user));
    },
    async deleteAvatar(userId: string): Promise<UserSummary> {
      const response = await client.deleteAvatar({ userId }, { headers: headers() });
      return mapUserSummary(requiredUser(response.user));
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
