import { updateMask } from './updateMask';
import { authHeaders, createChattoClient, type ConnectAPIConfig } from './connect.js';
import { createAdminMemberLoader, type AdminMemberBatch } from '$lib/query/adminMembers';
import { AdminUserService } from '@chatto/api-types/admin/v1/members_connect';
import type { AdminMember as APIAdminMember } from '@chatto/api-types/admin/v1/members_pb';
import type { AdminRole as APIAdminRole } from '@chatto/api-types/admin/v1/roles_pb';
import type { Role as APIRole } from '@chatto/api-types/api/v1/roles_pb';
import type { User as APIUser } from '@chatto/api-types/api/v1/users_pb';

export type AdminUserManagementAPIConfig = ConnectAPIConfig;

export type AdminManagedUser = {
  id: string;
  login: string;
  displayName: string;
  avatarUrl?: string | null;
  isBot?: boolean;
};

export type AdminMember = AdminManagedUser & {
  roles: string[];
  createdAt?: string | null;
  deleted: boolean;
  hasVerifiedEmail: boolean;
  verifiedEmails: string[];
  primaryVerifiedEmail: string | null;
  viewerCanDeleteAccount: boolean;
  lastLoginChange?: string | null;
};

export type AdminRoleSummary = {
  name: string;
  displayName: string;
};

export type AdminRoleDetails = AdminRoleSummary & {
  position: number;
  permissions: string[];
  permissionDenials: string[];
};

export type AdminMemberList = {
  users: AdminMember[];
  roles: AdminRoleSummary[];
  totalCount: number;
  hasMore: boolean;
  /** Server IDs consumed, including members omitted during hydration. */
  consumedCount: number;
};

export type AdminMemberDetails = {
  member: AdminMember | null;
  roles: AdminRoleDetails[];
  availablePermissions: string[];
  viewerCanAssignRoles: boolean;
  viewerCanManageRoles: boolean;
  viewerCanManageUserPermissions: boolean;
  assignableRoleNames: string[] | null;
  revocableRoleNames: string[] | null;
};

export type AdminUpdateUserInput = {
  userId: string;
  login?: string;
  displayName?: string;
};

export type AdminDeleteUserInput = {
  userId: string;
};

export type AdminListMembersInput = {
  search?: string | null;
  limit: number;
  offset: number;
};

export type AdminMemberTarget =
  | {
      userId: string;
    }
  | {
      login: string;
    };

export type AdminRoleMutationResult = {
  changed: boolean;
  member: AdminMember | null;
};

export function createAdminUserManagementAPI(config: AdminUserManagementAPIConfig) {
  const client = createChattoClient(AdminUserService, config);
  const headers = () => authHeaders(config);
  const batchMembers = async (
    userIds: string[],
    signal?: AbortSignal
  ): Promise<AdminMemberBatch> => {
    const response = await client.batchGetMembers(
      { userIds },
      { headers: headers(), ...(signal ? { signal } : {}) }
    );
    return {
      users: response.members.map(adminMember),
      roles: response.roles.map(adminRoleSummary)
    };
  };
  const loadMembers =
    config.serverId && config.queryScope
      ? createAdminMemberLoader(config.serverId, config.queryScope, batchMembers)
      : async (ids: string[], signal?: AbortSignal): Promise<AdminMemberBatch> => {
          const result: AdminMemberBatch = { users: [], roles: [] };
          for (let offset = 0; offset < ids.length; offset += 100) {
            signal?.throwIfAborted();
            const batch = await batchMembers(ids.slice(offset, offset + 100), signal);
            result.users.push(...batch.users);
            result.roles = batch.roles;
          }
          return result;
        };

  return {
    async listMembers(
      input: AdminListMembersInput,
      options: { signal?: AbortSignal } = {}
    ): Promise<AdminMemberList> {
      const response = await client.listMembers(
        {
          search: input.search || undefined,
          page: {
            limit: input.limit,
            offset: input.offset
          }
        },
        { headers: headers(), ...(options.signal ? { signal: options.signal } : {}) }
      );
      options.signal?.throwIfAborted();
      const members = await loadMembers(response.userIds, options.signal);
      options.signal?.throwIfAborted();
      return {
        ...members,
        consumedCount: response.userIds.length,
        totalCount: Number(response.page?.totalCount ?? 0),
        hasMore: response.page?.hasMore ?? false
      };
    },

    async getMember(
      target: string | AdminMemberTarget,
      options: { signal?: AbortSignal } = {}
    ): Promise<AdminMemberDetails> {
      const response = await client.getMember(
        { target: adminMemberTarget(target) },
        { headers: headers(), ...(options.signal ? { signal: options.signal } : {}) }
      );
      return {
        member: response.member ? adminMember(response.member) : null,
        roles: response.roles.map(adminRoleDetails),
        availablePermissions: [...response.availablePermissions],
        viewerCanAssignRoles: response.viewerCanAssignRoles,
        viewerCanManageRoles: response.viewerCanManageRoles,
        viewerCanManageUserPermissions: response.viewerCanManageUserPermissions,
        assignableRoleNames: response.roleAssignmentLimitsEnforced
          ? [...response.assignableRoleNames]
          : null,
        revocableRoleNames: response.roleAssignmentLimitsEnforced
          ? [...response.revocableRoleNames]
          : null
      };
    },

    async assignRole(userId: string, roleName: string): Promise<AdminRoleMutationResult> {
      const response = await client.assignRole({ userId, roleName }, { headers: headers() });
      return {
        changed: true,
        member: response.member ? adminMember(response.member) : null
      };
    },

    async revokeRole(userId: string, roleName: string): Promise<AdminRoleMutationResult> {
      const response = await client.revokeRole({ userId, roleName }, { headers: headers() });
      return {
        changed: true,
        member: response.member ? adminMember(response.member) : null
      };
    },

    async updateUser(input: AdminUpdateUserInput): Promise<AdminManagedUser> {
      const response = await client.updateUser(
        { ...input, updateMask: updateMask(input, ['displayName', 'login']) },
        { headers: headers() }
      );
      return adminManagedUser(response.user);
    },

    async changeUserPassword(userId: string, password: string): Promise<AdminMember> {
      const response = await client.changeUserPassword(
        { userId, password },
        { headers: headers() }
      );
      if (!response.member) {
        throw new Error('admin password response did not include a member');
      }
      return adminMember(response.member);
    },

    async clearUsernameCooldown(userId: string): Promise<boolean> {
      await client.clearUsernameCooldown({ userId }, { headers: headers() });
      return true;
    },

    async deleteUser(input: AdminDeleteUserInput): Promise<boolean> {
      await client.deleteUser(input, { headers: headers() });
      return true;
    }
  };
}

export type AdminUserManagementAPI = ReturnType<typeof createAdminUserManagementAPI>;

function adminMemberTarget(
  target: string | AdminMemberTarget
): { case: 'userId'; value: string } | { case: 'login'; value: string } {
  if (typeof target === 'string') {
    return { case: 'userId', value: target };
  }
  if ('login' in target) {
    return { case: 'login', value: target.login };
  }
  return { case: 'userId', value: target.userId };
}

function adminManagedUser(user: APIUser | undefined): AdminManagedUser {
  if (!user) {
    throw new Error('admin user response did not include a user');
  }
  return {
    id: user.id,
    login: user.login,
    displayName: user.displayName,
    avatarUrl: user.avatarUrl ?? null,
    ...(user.bot ? { isBot: true } : {})
  };
}

function adminMember(member: APIAdminMember): AdminMember {
  const summary = member.user;
  if (!summary) {
    throw new Error('admin member response did not include a user summary');
  }
  return {
    id: summary.id,
    login: summary.login,
    displayName: summary.displayName,
    avatarUrl: summary.avatarUrl ?? null,
    ...(summary.bot ? { isBot: true } : {}),
    roles: [...member.roles],
    createdAt: member.createdAt?.toDate().toISOString() ?? null,
    deleted: summary.deleted,
    hasVerifiedEmail: member.hasVerifiedEmail,
    verifiedEmails: [...member.verifiedEmails],
    primaryVerifiedEmail: member.primaryVerifiedEmail ?? null,
    viewerCanDeleteAccount: member.viewerCanDeleteAccount,
    lastLoginChange: member.lastLoginChange?.toDate().toISOString() ?? null
  };
}

function adminRoleSummary(role: APIRole): AdminRoleSummary {
  return {
    name: role.name,
    displayName: role.displayName
  };
}

function adminRoleDetails(role: APIAdminRole): AdminRoleDetails {
  if (!role.role) {
    throw new Error('admin member role response did not include public role metadata');
  }
  return {
    ...adminRoleSummary(role.role),
    position: role.role.position,
    permissions: [...role.permissions],
    permissionDenials: [...role.permissionDenials]
  };
}
