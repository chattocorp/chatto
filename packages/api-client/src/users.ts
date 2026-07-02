import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { UserDirectoryService } from "@chatto/api-types/api/v1/users_connect";
import type {
  User as APIUser,
  UserProfile as APIUserProfile,
} from "@chatto/api-types/api/v1/users_pb";

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
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true,
  });
  const client = createClient(UserDirectoryService, transport);
  const headers = () =>
    config.bearerToken
      ? { Authorization: `Bearer ${config.bearerToken}` }
      : undefined;

  return {
    async batchGetUsers(userIds: string[]): Promise<UserSummary[]> {
      const response = await client.batchGetUsers(
        { userIds },
        { headers: headers() },
      );
      return response.users.flatMap((profile) => {
        const summary = profile.user;
        return summary ? [mapUserSummary(summary)] : [];
      });
    },
  };
}

export type UserAPI = ReturnType<typeof createUserAPI>;

export function mapUserProfileSummary(profile: APIUserProfile): UserSummary | null {
  return profile.user ? mapUserSummary(profile.user) : null;
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
