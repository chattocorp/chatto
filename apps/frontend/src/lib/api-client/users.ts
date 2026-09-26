import {
  createChattoClient,
  StaleResponseError,
  type ConnectAPIConfig,
  minimumCursorHeaders
} from './connect.js';
import { getUserStore } from '$lib/state/server/users.svelte';
import { UserService } from '@chatto/api-types/api/v1/user_service_connect';
import { DirectoryMember as APIDirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import type { User } from '@chatto/api-types/api/v1/users_pb';

const REALTIME_RESOURCE_TIMEOUT_MS = 10_000;

export { mapUserSummary, mapOptionalUserSummary, type UserSummary } from './userSummary.js';
import { mapUserSummary, type UserSummary } from './userSummary.js';

export function createUserAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(UserService, config);
  const store = config.serverId ? getUserStore(config.serverId, config.queryScope) : undefined;
  const updateProfile = async (read: () => Promise<User>): Promise<UserSummary> => {
    if (!store) return mapUserSummary(await read());
    let acknowledged!: User;
    await store.readSnapshot(async () => {
      acknowledged = await read();
      return [new APIDirectoryMember({ ...store.get(acknowledged.id), user: acknowledged })];
    });
    if (store.isDeleted(acknowledged.id)) throw new StaleResponseError(false);
    // The command's own event can invalidate the profile before its response.
    // That does not turn a successful command into a failed acknowledgement.
    return mapUserSummary(store.get(acknowledged.id)?.user ?? acknowledged);
  };

  return {
    async batchGetUsers(userIds: string[], minimumCursor?: string): Promise<UserSummary[]> {
      const read = async (ids: string[]) =>
        (
          await client.batchGetUsers(
            { userIds: ids },
            {
              headers: minimumCursorHeaders(minimumCursor),
              ...(minimumCursor ? { timeoutMs: REALTIME_RESOURCE_TIMEOUT_MS } : {})
            }
          )
        ).users;
      const members = store
        ? await store.resolve(userIds, read, minimumCursor)
        : await read(userIds);
      return members.flatMap((member) => {
        const summary = member.user;
        return summary ? [mapUserSummary(summary)] : [];
      });
    },
    async uploadAvatar(userId: string, file: File): Promise<UserSummary> {
      return updateProfile(async () => {
        const response = await client.uploadAvatar({
          userId,
          image: {
            image: new Uint8Array(await file.arrayBuffer()),
            filename: file.name,
            contentType: file.type
          }
        });
        return requiredUser(response.user);
      });
    },
    async deleteAvatar(userId: string): Promise<UserSummary> {
      return updateProfile(async () => {
        const response = await client.deleteAvatar({ userId });
        return requiredUser(response.user);
      });
    }
  };
}

export type UserAPI = ReturnType<typeof createUserAPI>;

function requiredUser(user: Parameters<typeof mapUserSummary>[0] | undefined) {
  if (!user) throw new Error('avatar response did not include a user');
  return user;
}
