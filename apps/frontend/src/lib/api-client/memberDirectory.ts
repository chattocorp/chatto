import {
  Code,
  ConnectError,
  createChattoClient,
  type ConnectAPIConfig,
  minimumCursorHeaders
} from './connect.js';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import type { DirectoryMember as APIDirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { mapDirectoryMember, type DirectoryMember } from './directoryMemberView';
export { mapDirectoryMember, type DirectoryMember } from './directoryMemberView';
import { getUserStore } from '$lib/state/server/users.svelte';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
export { presenceStatusOrOffline as apiPresenceStatus } from './enumDefaults.js';


export type MemberDirectoryPage = {
  members: DirectoryMember[];
  /** Authorized membership IDs in page order, including profiles not yet available. */
  memberIds?: string[];
  totalCount: number;
  hasMore: boolean;
  /** Number of membership IDs consumed, including unavailable profiles. */
  consumedCount?: number;
};

export function createMemberDirectoryAPI(config: ConnectAPIConfig) {
  const users = createChattoClient(UserService, config);
  const rooms = createChattoClient(RoomService, config);
  const store = config.serverId ? getUserStore(config.serverId, config.queryScope) : undefined;
  const readProfiles = async (read: () => Promise<APIDirectoryMember[]>) =>
    (store ? await store.readSnapshot(read) : await read()).map(mapDirectoryMember);
  const batchUsers = async (userIds: string[], minimumCursor?: string): Promise<APIDirectoryMember[]> => {
    const response = await users.batchGetUsers(
      { userIds },
      {
        headers: minimumCursorHeaders(minimumCursor),
        ...(minimumCursor ? { timeoutMs: 10_000 } : {})
      }
    );
    return response.users;
  };
  const loadUsers =
    store
      ? async (ids: string[], minimumCursor?: string) =>
          (await store.resolve(ids, batchUsers, minimumCursor)).map(mapDirectoryMember)
      : async (ids: string[], minimumCursor?: string) => {
          const members: DirectoryMember[] = [];
          for (let offset = 0; offset < ids.length; offset += 100) {
            members.push(...(await batchUsers(ids.slice(offset, offset + 100), minimumCursor)).map(mapDirectoryMember));
          }
          return members;
        };

  return {
    async listUsers(
      search = '',
      limit = 20,
      offset = 0,
      options: { signal?: AbortSignal } = {}
    ): Promise<MemberDirectoryPage> {
      let response!: Awaited<ReturnType<typeof users.listUsers>>;
      const members = await readProfiles(async () => {
        response = await users.listUsers(
          { search, page: { limit, offset } },
          { ...(options.signal ? { signal: options.signal } : {}) }
        );
        return response.users;
      });
      return {
        members,
        totalCount: Number(response.page?.totalCount ?? 0),
        hasMore: response.page?.hasMore ?? false
      };
    },

    async getUser(userId: string): Promise<DirectoryMember | null> {
      try {
        const members = await readProfiles(async () => {
          const response = await users.getUser({ target: { case: 'userId', value: userId } });
          return response.user ? [response.user] : [];
        });
        return members[0] ?? null;
      } catch (err) {
        if (err instanceof ConnectError && err.code === Code.NotFound) {
          return null;
        }
        throw err;
      }
    },

    async getUserByLogin(login: string): Promise<DirectoryMember | null> {
      try {
        const members = await readProfiles(async () => {
          const response = await users.getUser({ target: { case: 'login', value: login } });
          return response.user ? [response.user] : [];
        });
        return members[0] ?? null;
      } catch (err) {
        if (err instanceof ConnectError && err.code === Code.NotFound) {
          return null;
        }
        throw err;
      }
    },

    async batchGetUsers(userIds: string[]): Promise<DirectoryMember[]> {
      return loadUsers(userIds);
    },

    async listRoomMembers(
      roomId: string,
      search = '',
      limit = 250,
      offset = 0,
      options: {
        signal?: AbortSignal;
        minimumCursor?: string;
        presenceStatuses?: PresenceStatus[];
      } = {}
    ): Promise<MemberDirectoryPage> {
      const response = await rooms.listMembers(
        {
          roomId,
          search,
          page: { limit, offset },
          ...(options.presenceStatuses ? { presenceStatuses: options.presenceStatuses } : {})
        },
        {
          headers: minimumCursorHeaders(options.minimumCursor),
          ...(options.minimumCursor || options.presenceStatuses ? { timeoutMs: 10_000 } : {}),
          ...(options.signal ? { signal: options.signal } : {})
        }
      );
      options.signal?.throwIfAborted();
      const members = await loadUsers(response.userIds, options.minimumCursor);
      options.signal?.throwIfAborted();
      return {
        members,
        memberIds: response.userIds,
        consumedCount: response.userIds.length,
        totalCount: Number(response.page?.totalCount ?? 0),
        hasMore: response.page?.hasMore ?? false
      };
    },

    /** Load one presence group independently of the full directory scan. */
    async listOnlineRoomMembers(
      roomId: string,
      status: PresenceStatus,
      limit = 250,
      offset = 0,
      options: { minimumCursor?: string } = {}
    ): Promise<MemberDirectoryPage> {
      return this.listRoomMembers(roomId, '', limit, offset, {
        ...options,
        presenceStatuses: [status]
      });
    },

    async getRoomMember(roomId: string, userId: string): Promise<DirectoryMember | null> {
      try {
        const members = await readProfiles(async () => {
          const response = await rooms.getMember({ roomId, userId });
          return response.member ? [response.member] : [];
        });
        return members[0] ?? null;
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
      return readProfiles(async () => {
        const response = await rooms.batchGetMembers(
          { roomId, userIds },
          { ...(options.signal ? { signal: options.signal } : {}) }
        );
        return response.members;
      });
    }
  };
}

export type MemberDirectoryAPI = ReturnType<typeof createMemberDirectoryAPI>;
