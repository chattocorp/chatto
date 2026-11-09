import { updateMask } from './updateMask.js';
import { Code, ConnectError, createChattoClient, type ConnectAPIConfig } from './connect.js';
import { AdminRoleService } from '@chatto/api-types/admin/v1/roles_connect';
import { AdminUserService } from '@chatto/api-types/admin/v1/members_connect';
import type { AdminRole as APIAdminRole } from '@chatto/api-types/admin/v1/roles_pb';
import { RoleService } from '@chatto/api-types/api/v1/roles_connect';
import type { Role as APIRole } from '@chatto/api-types/api/v1/roles_pb';
import type { User as APIUser } from '@chatto/api-types/api/v1/users_pb';

export type ServerRole = {
  name: string;
  displayName: string;
  description: string;
  isSystem: boolean;
  position: number;
  pingable: boolean;
  /** Active human members; absent when not reported or not authorized. */
  memberCount?: number;
  /** Explicit grants across all scopes, excluding virtual owner access. */
  permissionGrantCount?: number;
  /** Explicit denials across all scopes. */
  permissionDenialCount?: number;
};

export type RoleUser = {
  id: string;
  login: string;
  displayName: string;
  isBot: boolean;
};

export type RoleCatalog = {
  roles: ServerRole[];
  viewerCanManageRoles: boolean;
  viewerCanAssignRoles: boolean;
};

export type RoleDetails = RoleCatalog & {
  role: ServerRole | null;
};

export type RoleMemberPage = {
  users: RoleUser[];
  totalCount: number;
  hasMore: boolean;
};

/** Explicit human-account roles and the caller's current assignment limits.
 * Limits are advisory. Read again after assignments or permission changes. */
export type UserRoleAssignments = {
  userId: string;
  roleNames: string[];
  assignableRoleNames: string[];
  revocableRoleNames: string[];
};

export type CreateRoleInput = {
  name: string;
  displayName: string;
  description: string;
  pingable: boolean;
};

export type UpdateRoleInput = {
  name: string;
  displayName: string;
  description: string;
  pingable?: boolean;
};

export function createRoleAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(RoleService, config);
  const adminClient = createChattoClient(AdminRoleService, config);
  const adminUsers = createChattoClient(AdminUserService, config);

  return {
    /** Read role assignments with role.assign, without private account access.
     * Unknown users and bot targets retain the server's error codes. */
    async getUserRoleAssignments(
      userId: string,
      options: { signal?: AbortSignal } = {}
    ): Promise<UserRoleAssignments> {
      const response = await adminUsers.getUserRoleAssignments({ userId }, options);
      if (!response.assignments) {
        throw new Error('role assignment response did not include assignment state');
      }
      return {
        userId: response.assignments.userId,
        roleNames: [...response.assignments.roleNames],
        assignableRoleNames: [...response.assignments.assignableRoleNames],
        revocableRoleNames: [...response.assignments.revocableRoleNames]
      };
    },

    async listRoles(): Promise<RoleCatalog> {
      const response = await client.listRoles({});
      return {
        roles: response.roles.map((role) => serverRoleFromPublic(role)),
        viewerCanManageRoles: false,
        viewerCanAssignRoles: false
      };
    },

    async getPublicRole(name: string): Promise<ServerRole | null> {
      try {
        const response = await client.getRole({ name });
        return response.role ? serverRoleFromPublic(response.role) : null;
      } catch (err) {
        if (err instanceof ConnectError && err.code === Code.NotFound) {
          return null;
        }
        throw err;
      }
    },

    async batchGetPublicRoles(names: string[]): Promise<ServerRole[]> {
      const response = await client.batchGetRoles({ names });
      return response.roles.map((role) => serverRoleFromPublic(role));
    },

    async listAdminRoles(options: { signal?: AbortSignal } = {}): Promise<RoleCatalog> {
      const response = await adminClient.listRoles({}, { signal: options.signal });
      return {
        roles: response.roles.map(serverRoleFromAdmin),
        viewerCanManageRoles: response.viewerCanManageRoles,
        viewerCanAssignRoles: response.viewerCanAssignRoles
      };
    },

    async getRole(name: string, options: { signal?: AbortSignal } = {}): Promise<RoleDetails> {
      const response = await adminClient.getRole({ name }, { signal: options.signal });
      return {
        roles: [],
        role: response.role ? serverRoleFromAdmin(response.role) : null,
        viewerCanManageRoles: response.viewerCanManageRoles,
        viewerCanAssignRoles: response.viewerCanAssignRoles
      };
    },

    async listMembers(
      name: string,
      page: { limit: number; offset: number },
      options: { signal?: AbortSignal } = {}
    ): Promise<RoleMemberPage> {
      const response = await adminClient.listMembers({ name, page }, { signal: options.signal });
      return {
        users: response.members.map(roleUser),
        totalCount: Number(response.page?.totalCount ?? 0),
        hasMore: response.page?.hasMore ?? false
      };
    },

    async createRole(input: CreateRoleInput): Promise<ServerRole> {
      const response = await adminClient.createRole(input);
      return requiredAdminRole(response.role);
    },

    async updateRole(input: UpdateRoleInput): Promise<ServerRole> {
      const response = await adminClient.updateRole({
        ...input,
        updateMask: updateMask(input, ['displayName', 'description', 'pingable'])
      });
      return requiredAdminRole(response.role);
    },

    async deleteRole(name: string): Promise<boolean> {
      await adminClient.deleteRole({ name });
      return true;
    }
  };
}

export type RoleAPI = ReturnType<typeof createRoleAPI>;

function requiredAdminRole(role: APIAdminRole | undefined): ServerRole {
  if (!role) {
    throw new Error('role response did not include a role');
  }
  return serverRoleFromAdmin(role);
}

function serverRoleFromAdmin(role: APIAdminRole): ServerRole {
  if (!role.role) {
    throw new Error('admin role response did not include public role metadata');
  }
  return {
    ...serverRoleFromPublic(role.role),
    memberCount: role.memberCount === undefined ? undefined : Number(role.memberCount),
    permissionGrantCount:
      role.permissionGrantCount === undefined ? undefined : Number(role.permissionGrantCount),
    permissionDenialCount:
      role.permissionDenialCount === undefined ? undefined : Number(role.permissionDenialCount)
  };
}

function serverRoleFromPublic(role: APIRole): ServerRole {
  return {
    name: role.name,
    displayName: role.displayName,
    description: role.description,
    isSystem: role.isSystem,
    position: role.position,
    pingable: role.pingable
  };
}

function roleUser(user: APIUser): RoleUser {
  return {
    id: user.id,
    login: user.login,
    displayName: user.displayName,
    isBot: !!user.bot
  };
}
