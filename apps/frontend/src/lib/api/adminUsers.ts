import { createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { AdminUserManagementService } from '$lib/pb/chatto/api/v1/admin_user_management_connect';
import type {
  AdminMember as APIAdminMember,
  AdminMemberRole as APIAdminMemberRole,
  AdminMemberRoleSummary as APIAdminMemberRoleSummary
} from '$lib/pb/chatto/api/v1/admin_user_management_pb';
import type { AccountUser as APIAccountUser } from '$lib/pb/chatto/api/v1/account_pb';

export type AdminUserManagementAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
};

export type AdminManagedUser = {
  id: string;
  login: string;
  displayName: string;
  avatarUrl?: string | null;
};

export type AdminMember = AdminManagedUser & {
  roles: string[];
  createdAt?: string | null;
  deleted: boolean;
  hasVerifiedEmail: boolean;
  verifiedEmails: string[];
  viewerCanDeleteAccount: boolean;
  lastLoginChange?: string | null;
};

export type AdminMemberRoleSummary = {
  name: string;
  displayName: string;
};

export type AdminMemberRole = AdminMemberRoleSummary & {
  position: number;
  permissions: string[];
  permissionDenials: string[];
};

export type AdminMemberList = {
  users: AdminMember[];
  roles: AdminMemberRoleSummary[];
  totalCount: number;
  hasMore: boolean;
};

export type AdminMemberDetails = {
  member: AdminMember | null;
  roles: AdminMemberRole[];
  availablePermissions: string[];
  viewerCanAssignRoles: boolean;
  viewerCanManageRoles: boolean;
  viewerCanManageUserPermissions: boolean;
};

export type AdminUpdateUserInput = {
  userId: string;
  login?: string;
  displayName?: string;
};

export type AdminListMembersInput = {
  search?: string | null;
  limit: number;
  offset: number;
};

export function createAdminUserManagementAPI(config: AdminUserManagementAPIConfig) {
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true
  });
  const client = createClient(AdminUserManagementService, transport);
  const headers = () =>
    config.bearerToken ? { Authorization: `Bearer ${config.bearerToken}` } : undefined;

  return {
    async listMembers(input: AdminListMembersInput): Promise<AdminMemberList> {
      const response = await client.listMembers(
        {
          search: input.search || undefined,
          limit: input.limit,
          offset: input.offset
        },
        { headers: headers() }
      );
      return {
        users: response.users.map(adminMember),
        roles: response.roles.map(adminMemberRoleSummary),
        totalCount: response.totalCount,
        hasMore: response.hasMore
      };
    },

    async getMember(userId: string): Promise<AdminMemberDetails> {
      const response = await client.getMember({ userId }, { headers: headers() });
      return {
        member: response.member ? adminMember(response.member) : null,
        roles: response.roles.map(adminMemberRole),
        availablePermissions: [...response.availablePermissions],
        viewerCanAssignRoles: response.viewerCanAssignRoles,
        viewerCanManageRoles: response.viewerCanManageRoles,
        viewerCanManageUserPermissions: response.viewerCanManageUserPermissions
      };
    },

    async assignRole(userId: string, roleName: string): Promise<boolean> {
      const response = await client.assignRole({ userId, roleName }, { headers: headers() });
      return response.assigned;
    },

    async revokeRole(userId: string, roleName: string): Promise<boolean> {
      const response = await client.revokeRole({ userId, roleName }, { headers: headers() });
      return response.revoked;
    },

    async updateUser(input: AdminUpdateUserInput): Promise<AdminManagedUser> {
      const response = await client.updateUser(input, { headers: headers() });
      return adminManagedUser(response.user);
    },

    async clearUsernameCooldown(userId: string): Promise<boolean> {
      const response = await client.clearUsernameCooldown({ userId }, { headers: headers() });
      return response.cleared;
    }
  };
}

export type AdminUserManagementAPI = ReturnType<typeof createAdminUserManagementAPI>;

function adminManagedUser(user: APIAccountUser | undefined): AdminManagedUser {
  if (!user) {
    throw new Error('admin user response did not include a user');
  }
  return {
    id: user.id,
    login: user.login,
    displayName: user.displayName,
    avatarUrl: user.avatarUrl ?? null
  };
}

function adminMember(user: APIAdminMember): AdminMember {
  return {
    id: user.id,
    login: user.login,
    displayName: user.displayName,
    avatarUrl: user.avatarUrl ?? null,
    roles: [...user.roles],
    createdAt: user.createdAt?.toDate().toISOString() ?? null,
    deleted: user.deleted,
    hasVerifiedEmail: user.hasVerifiedEmail,
    verifiedEmails: [...user.verifiedEmails],
    viewerCanDeleteAccount: user.viewerCanDeleteAccount,
    lastLoginChange: user.lastLoginChange?.toDate().toISOString() ?? null
  };
}

function adminMemberRoleSummary(role: APIAdminMemberRoleSummary): AdminMemberRoleSummary {
  return {
    name: role.name,
    displayName: role.displayName
  };
}

function adminMemberRole(role: APIAdminMemberRole): AdminMemberRole {
  return {
    ...adminMemberRoleSummary(role),
    position: role.position,
    permissions: [...role.permissions],
    permissionDenials: [...role.permissionDenials]
  };
}
