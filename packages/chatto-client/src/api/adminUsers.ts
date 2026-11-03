/** Administrative account reads without catalogues or screen bootstrap data.
 * Resources remain protobuf messages; this module owns older-server fallback. */
import { Code, ConnectError, createChattoClient, type ConnectAPIConfig } from './connect.js';
import { AdminUserService } from '@chatto/api-types/admin/v1/members_connect';
import type { AdminMember } from '@chatto/api-types/admin/v1/members_pb';
import { AdminUser, AdminUsernameCooldown } from '@chatto/api-types/admin/v1/users_pb';

/** Resolve an account by exactly one stable identifier. */
export type AdminUserTarget = { userId: string } | { login: string };
type ReadOptions = { signal?: AbortSignal };

/** Create stateless admin account reads. Only UNIMPLEMENTED triggers fallback;
 * access denials, cancellation, and hydration failures always reach the host. */
export function createAdminUserAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(AdminUserService, config);
  return {
    /** Select one live page of account IDs; offsets count IDs, not hydration results. */
    async listUsers(
      input: { search?: string; page?: { limit?: number; offset?: number } },
      options: ReadOptions = {}
    ) {
      try {
        return await client.listUsers(input, options);
      } catch (error) {
        if (!unimplemented(error)) throw error;
        return client.listMembers(input, options);
      }
    },
    /** Read one canonical account. Missing accounts retain NOT_FOUND. */
    async getUser(target: AdminUserTarget, options: ReadOptions = {}): Promise<AdminUser> {
      const request = {
        target:
          'userId' in target
            ? { case: 'userId' as const, value: target.userId }
            : { case: 'login' as const, value: target.login }
      };
      try {
        const response = await client.getUser(request, options);
        if (!response.user) throw new Error('admin user response did not include an account');
        return response.user;
      } catch (error) {
        if (!unimplemented(error)) throw error;
        const response = await client.getMember(request, options);
        if (!response.member)
          throw new Error('admin member response did not include an account', { cause: error });
        return legacyAdminUser(response.member);
      }
    },
    /** Read up to 100 accounts. The server omits missing IDs and removes duplicates. */
    async batchGetUsers(userIds: string[], options: ReadOptions = {}): Promise<AdminUser[]> {
      try {
        return (await client.batchGetUsers({ userIds }, options)).users;
      } catch (error) {
        if (!unimplemented(error)) throw error;
        return (await client.batchGetMembers({ userIds }, options)).members.map(legacyAdminUser);
      }
    }
  };
}

function unimplemented(error: unknown): boolean {
  return error instanceof ConnectError && error.code === Code.Unimplemented;
}

function legacyAdminUser(member: AdminMember): AdminUser {
  if (!member.user) throw new Error('admin member response did not include a profile');
  return new AdminUser({
    profile: member.user,
    roleNames: member.roles,
    createdAt: member.createdAt,
    verifiedEmails: member.verifiedEmails,
    primaryVerifiedEmail: member.primaryVerifiedEmail,
    usernameCooldown: member.lastLoginChange
      ? new AdminUsernameCooldown({ lastLoginChange: member.lastLoginChange })
      : undefined
  });
}
