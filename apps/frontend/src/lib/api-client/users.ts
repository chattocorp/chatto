import { authHeaders, createChattoClient } from "./connect.js";
import { UserService } from "@chatto/api-types/api/v1/member_directory_connect";
import type { DirectoryMember as APIDirectoryMember } from "@chatto/api-types/api/v1/member_directory_pb";
import type { User as APIUser } from "@chatto/api-types/api/v1/users_pb";

export type UserAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export type UserSummary = {
  id: string;
  login: string;
  displayName: string;
  deleted: boolean;
  avatarUrl: string | null;
};

export function createUserAPI(config: UserAPIConfig) {
  const client = createChattoClient(UserService, config);
  const headers = () => authHeaders(config);

  return {
    async batchGetUsers(userIds: string[]): Promise<UserSummary[]> {
      const response = await client.batchGetUsers(
        { userIds },
        { headers: headers() },
      );
      return response.users.flatMap((member) => {
        const summary = member.user;
        return summary ? [mapUserSummary(summary)] : [];
      });
    },
  };
}

export type UserAPI = ReturnType<typeof createUserAPI>;

export function mapDirectoryMemberUserSummary(
  member: APIDirectoryMember,
): UserSummary | null {
  return member.user ? mapUserSummary(member.user) : null;
}

export function mapUserSummary(user: APIUser): UserSummary {
  return {
    id: user.id,
    login: user.login,
    displayName: user.displayName,
    deleted: user.deleted,
    avatarUrl: user.avatarUrl || null,
  };
}
