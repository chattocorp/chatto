import {
  authHeaders,
  Code,
  ConnectError,
  createChattoClient,
  type ConnectAPIConfig
} from './connect.js';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import type { DirectoryMember as APIDirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import {
  mapUserPresenceView,
  mapUserSummary,
  type UserPresenceView,
  type UserSummary
} from './userSummary.js';
export { presenceStatusOrOffline as apiPresenceStatus } from './enumDefaults.js';

export type MemberDirectoryAPIConfig = ConnectAPIConfig;

export type DirectoryMember = UserSummary & UserPresenceView & {
  roles: string[];
  createdAt: string | null;
};

export type MemberDirectoryPage = {
  members: DirectoryMember[];
  totalCount: number;
  hasMore: boolean;
};

export function createMemberDirectoryAPI(config: MemberDirectoryAPIConfig) {
  const users = createChattoClient(UserService, config);
  const rooms = createChattoClient(RoomService, config);
  const headers = () => authHeaders(config);

  return {
    async listUsers(
      search = '',
      limit = 20,
      offset = 0,
      options: { signal?: AbortSignal } = {}
    ): Promise<MemberDirectoryPage> {
      const response = await users.listUsers(
        { search, page: { limit, offset } },
        {
          headers: headers(),
          ...(options.signal ? { signal: options.signal } : {})
        }
      );
      return {
        members: response.users.map(mapDirectoryMember),
        totalCount: Number(response.page?.totalCount ?? 0),
        hasMore: response.page?.hasMore ?? false
      };
    },

    async getUser(userId: string): Promise<DirectoryMember | null> {
      try {
        const response = await users.getUser(
          { target: { case: 'userId', value: userId } },
          { headers: headers() }
        );
        return response.user ? mapDirectoryMember(response.user) : null;
      } catch (err) {
        if (err instanceof ConnectError && err.code === Code.NotFound) {
          return null;
        }
        throw err;
      }
    },

    async getUserByLogin(login: string): Promise<DirectoryMember | null> {
      try {
        const response = await users.getUser(
          { target: { case: 'login', value: login } },
          { headers: headers() }
        );
        return response.user ? mapDirectoryMember(response.user) : null;
      } catch (err) {
        if (err instanceof ConnectError && err.code === Code.NotFound) {
          return null;
        }
        throw err;
      }
    },

    async batchGetUsers(userIds: string[]): Promise<DirectoryMember[]> {
      const response = await users.batchGetUsers({ userIds }, { headers: headers() });
      return response.users.map(mapDirectoryMember);
    },

    async listRoomMembers(
      roomId: string,
      search = '',
      limit = 250,
      offset = 0,
      options: { signal?: AbortSignal } = {}
    ): Promise<MemberDirectoryPage> {
      const response = await rooms.listMembers(
        { roomId, search, page: { limit, offset } },
        {
          headers: headers(),
          ...(options.signal ? { signal: options.signal } : {})
        }
      );
      return {
        members: response.members.map(mapDirectoryMember),
        totalCount: Number(response.page?.totalCount ?? 0),
        hasMore: response.page?.hasMore ?? false
      };
    },

    async getRoomMember(roomId: string, userId: string): Promise<DirectoryMember | null> {
      try {
        const response = await rooms.getMember({ roomId, userId }, { headers: headers() });
        return response.member ? mapDirectoryMember(response.member) : null;
      } catch (err) {
        if (err instanceof ConnectError && err.code === Code.NotFound) {
          return null;
        }
        throw err;
      }
    },

    async batchGetRoomMembers(
      roomId: string,
      userIds: string[],
      options: { signal?: AbortSignal } = {}
    ): Promise<DirectoryMember[]> {
      const response = await rooms.batchGetMembers(
        { roomId, userIds },
        {
          headers: headers(),
          ...(options.signal ? { signal: options.signal } : {})
        }
      );
      return response.members.map(mapDirectoryMember);
    }
  };
}

export type MemberDirectoryAPI = ReturnType<typeof createMemberDirectoryAPI>;

export function mapDirectoryMember(member: APIDirectoryMember): DirectoryMember {
  const user = member.user;
  // The directory contract renders a blank, offline member when the response
  // omits the user instead of dropping the row, and never leaves `isBot`
  // unset.
  const summary: UserSummary = user
    ? { ...mapUserSummary(user), isBot: user.isBot ?? false }
    : { id: '', login: '', displayName: '', deleted: false, isBot: false, avatarUrl: null };
  return {
    ...summary,
    ...mapUserPresenceView(user),
    roles: [...member.roles],
    createdAt: member.createdAt?.toDate().toISOString() ?? null
  };
}
