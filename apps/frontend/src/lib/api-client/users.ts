import { authHeaders, createChattoClient } from './connect.js';
import { UserService } from '@chatto/api-types/api/v1/member_directory_connect';
import type { DirectoryMember as APIDirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';

export { mapUserSummary, mapOptionalUserSummary, type UserSummary } from './userSummary.js';
import { mapOptionalUserSummary, mapUserSummary, type UserSummary } from './userSummary.js';

export type UserAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export function createUserAPI(config: UserAPIConfig) {
  const client = createChattoClient(UserService, config);
  const headers = () => authHeaders(config);

  return {
    async batchGetUsers(userIds: string[]): Promise<UserSummary[]> {
      const response = await client.batchGetUsers({ userIds }, { headers: headers() });
      return response.users.flatMap((member) => {
        const summary = member.user;
        return summary ? [mapUserSummary(summary)] : [];
      });
    }
  };
}

export type UserAPI = ReturnType<typeof createUserAPI>;

export function mapDirectoryMemberUserSummary(member: APIDirectoryMember): UserSummary | null {
  return mapOptionalUserSummary(member.user);
}
