import { authHeaders, createChattoClient } from "./connect.js";
import { AdminUserService } from "@chatto/api-types/admin/v1/members_connect";
import type { AdminMember as APIAdminMember } from "@chatto/api-types/admin/v1/members_pb";
import type { AdminRole as APIAdminRole } from "@chatto/api-types/admin/v1/roles_pb";
import type { Role as APIRole } from "@chatto/api-types/api/v1/roles_pb";
import type { User as APIUser } from "@chatto/api-types/api/v1/users_pb";

export type AdminUserManagementAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
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

export type AdminRoleReference = {
  name: string;
  displayName: string;
};

export type AdminMemberRole = AdminRoleReference & {
  position: number;
  permissions: string[];
  permissionDenials: string[];
};

export type AdminMemberList = {
  users: AdminMember[];
  roles: AdminRoleReference[];
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

export type AdminDeleteUserInput = {
  userId: string;
  currentPassword?: string;
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

export function createAdminUserManagementAPI(
  config: AdminUserManagementAPIConfig,
) {
  const client = createChattoClient(AdminUserService, config);
  const headers = () => authHeaders(config);

  return {
    async listMembers(input: AdminListMembersInput): Promise<AdminMemberList> {
      const response = await client.listMembers(
        {
          search: input.search || undefined,
          page: {
            limit: input.limit,
            offset: input.offset,
          },
        },
        { headers: headers() },
      );
      return {
        users: response.members.map(adminMember),
        roles: response.roles.map(adminRoleReference),
        totalCount: Number(response.page?.totalCount ?? 0),
        hasMore: response.page?.hasMore ?? false,
      };
    },

    async getMember(
      target: string | AdminMemberTarget,
    ): Promise<AdminMemberDetails> {
      const response = await client.getMember(
        { target: adminMemberTarget(target) },
        { headers: headers() },
      );
      return {
        member: response.member ? adminMember(response.member) : null,
        roles: response.roles.map(adminMemberRole),
        availablePermissions: [...response.availablePermissions],
        viewerCanAssignRoles: response.viewerCanAssignRoles,
        viewerCanManageRoles: response.viewerCanManageRoles,
        viewerCanManageUserPermissions: response.viewerCanManageUserPermissions,
      };
    },

    async assignRole(
      userId: string,
      roleName: string,
    ): Promise<AdminRoleMutationResult> {
      const response = await client.assignRole(
        { userId, roleName },
        { headers: headers() },
      );
      return {
        changed: response.assigned,
        member: response.member ? adminMember(response.member) : null,
      };
    },

    async revokeRole(
      userId: string,
      roleName: string,
    ): Promise<AdminRoleMutationResult> {
      const response = await client.revokeRole(
        { userId, roleName },
        { headers: headers() },
      );
      return {
        changed: response.revoked,
        member: response.member ? adminMember(response.member) : null,
      };
    },

    async updateUser(input: AdminUpdateUserInput): Promise<AdminManagedUser> {
      const response = await client.updateUser(input, { headers: headers() });
      return adminManagedUser(response.user);
    },

    async updateUserPassword(
      userId: string,
      password: string,
    ): Promise<AdminMember> {
      const response = await client.updateUserPassword(
        { userId, password },
        { headers: headers() },
      );
      if (!response.member) {
        throw new Error("admin password response did not include a member");
      }
      return adminMember(response.member);
    },

    async clearUsernameCooldown(userId: string): Promise<boolean> {
      const response = await client.clearUsernameCooldown(
        { userId },
        { headers: headers() },
      );
      return response.cleared;
    },

    async deleteUser(input: AdminDeleteUserInput): Promise<boolean> {
      const response = await client.deleteUser(input, { headers: headers() });
      return response.deleted;
    },
  };
}

export type AdminUserManagementAPI = ReturnType<
  typeof createAdminUserManagementAPI
>;

function adminMemberTarget(
  target: string | AdminMemberTarget,
): { case: "userId"; value: string } | { case: "login"; value: string } {
  if (typeof target === "string") {
    return { case: "userId", value: target };
  }
  if ("login" in target) {
    return { case: "login", value: target.login };
  }
  return { case: "userId", value: target.userId };
}

function adminManagedUser(user: APIUser | undefined): AdminManagedUser {
  if (!user) {
    throw new Error("admin user response did not include a user");
  }
  return {
    id: user.id,
    login: user.login,
    displayName: user.displayName,
    avatarUrl: user.avatarUrl ?? null,
  };
}

function adminMember(member: APIAdminMember): AdminMember {
  const summary = member.user;
  if (!summary) {
    throw new Error("admin member response did not include a user summary");
  }
  return {
    id: summary.id,
    login: summary.login,
    displayName: summary.displayName,
    avatarUrl: summary.avatarUrl ?? null,
    roles: [...member.roles],
    createdAt: member.createdAt?.toDate().toISOString() ?? null,
    deleted: summary.deleted,
    hasVerifiedEmail: member.hasVerifiedEmail,
    verifiedEmails: [...member.verifiedEmails],
    viewerCanDeleteAccount: member.viewerCanDeleteAccount,
    lastLoginChange: member.lastLoginChange?.toDate().toISOString() ?? null,
  };
}

function adminRoleReference(role: APIRole): AdminRoleReference {
  return {
    name: role.name,
    displayName: role.displayName,
  };
}

function adminMemberRole(role: APIAdminRole): AdminMemberRole {
  if (!role.role) {
    throw new Error(
      "admin member role response did not include public role metadata",
    );
  }
  return {
    ...adminRoleReference(role.role),
    position: role.role.position,
    permissions: [...role.permissions],
    permissionDenials: [...role.permissionDenials],
  };
}
