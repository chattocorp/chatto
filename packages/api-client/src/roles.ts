import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { AdminRoleService } from "@chatto/api-types/admin/v1/roles_connect";
import type { AdminRole as APIAdminRole } from "@chatto/api-types/admin/v1/roles_pb";
import { RoleService } from "@chatto/api-types/api/v1/roles_connect";
import type { Role as APIRole } from "@chatto/api-types/api/v1/roles_pb";
import type { User as APIUser } from "@chatto/api-types/api/v1/users_pb";

export type RoleAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export type ServerRole = {
  name: string;
  displayName: string;
  description: string;
  permissions: string[];
  permissionDenials: string[];
  isSystem: boolean;
  position: number;
  pingable: boolean;
};

export type RoleUser = {
  id: string;
  login: string;
  displayName: string;
};

export type RoleCatalog = {
  roles: ServerRole[];
  viewerCanManageRoles: boolean;
  viewerCanAssignRoles: boolean;
};

export type RoleDetails = RoleCatalog & {
  role: ServerRole | null;
  users: RoleUser[];
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

export function createRoleAPI(config: RoleAPIConfig) {
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true,
  });
  const client = createClient(RoleService, transport);
  const adminClient = createClient(AdminRoleService, transport);
  const headers = () =>
    config.bearerToken
      ? { Authorization: `Bearer ${config.bearerToken}` }
      : undefined;

  return {
    async listRoles(): Promise<RoleCatalog> {
      const response = await client.listRoles({}, { headers: headers() });
      return {
        roles: response.roles.map((role) => serverRoleFromPublic(role)),
        viewerCanManageRoles: false,
        viewerCanAssignRoles: false,
      };
    },

    async listAdminRoles(): Promise<RoleCatalog> {
      const response = await adminClient.listRoles({}, { headers: headers() });
      return {
        roles: response.roles.map(serverRoleFromAdmin),
        viewerCanManageRoles: response.viewerCanManageRoles,
        viewerCanAssignRoles: response.viewerCanAssignRoles,
      };
    },

    async getRole(name: string): Promise<RoleDetails> {
      const response = await adminClient.getRole({ name }, { headers: headers() });
      return {
        roles: [],
        role: response.role ? serverRoleFromAdmin(response.role) : null,
        users: response.users.map(roleUser),
        viewerCanManageRoles: response.viewerCanManageRoles,
        viewerCanAssignRoles: response.viewerCanAssignRoles,
      };
    },

    async createRole(input: CreateRoleInput): Promise<ServerRole> {
      const response = await adminClient.createRole(input, { headers: headers() });
      return requiredAdminRole(response.role);
    },

    async updateRole(input: UpdateRoleInput): Promise<ServerRole> {
      const response = await adminClient.updateRole(input, { headers: headers() });
      return requiredAdminRole(response.role);
    },

    async deleteRole(name: string): Promise<boolean> {
      const response = await adminClient.deleteRole(
        { name },
        { headers: headers() },
      );
      return response.deleted;
    },
  };
}

export type RoleAPI = ReturnType<typeof createRoleAPI>;

function requiredAdminRole(role: APIAdminRole | undefined): ServerRole {
  if (!role) {
    throw new Error("role response did not include a role");
  }
  return serverRoleFromAdmin(role);
}

function serverRoleFromAdmin(role: APIAdminRole): ServerRole {
  if (!role.role) {
    throw new Error("admin role response did not include public role metadata");
  }
  return serverRoleFromPublic(role.role, role.permissions, role.permissionDenials);
}

function serverRoleFromPublic(
  role: APIRole,
  permissions: string[] = [],
  permissionDenials: string[] = [],
): ServerRole {
  return {
    name: role.name,
    displayName: role.displayName,
    description: role.description,
    permissions: [...permissions],
    permissionDenials: [...permissionDenials],
    isSystem: role.isSystem,
    position: role.position,
    pingable: role.pingable,
  };
}

function roleUser(user: APIUser): RoleUser {
  return {
    id: user.id,
    login: user.login,
    displayName: user.displayName,
  };
}
