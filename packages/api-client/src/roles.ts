import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { AdminRoleService } from "@chatto/api-types/admin/v1/roles_connect";
import type { Role as APIRole } from "@chatto/api-types/admin/v1/roles_pb";
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
  const client = createClient(AdminRoleService, transport);
  const headers = () =>
    config.bearerToken
      ? { Authorization: `Bearer ${config.bearerToken}` }
      : undefined;

  return {
    async listRoles(): Promise<RoleCatalog> {
      const response = await client.listRoles({}, { headers: headers() });
      return {
        roles: response.roles.map(serverRole),
        viewerCanManageRoles: response.viewerCanManageRoles,
        viewerCanAssignRoles: response.viewerCanAssignRoles,
      };
    },

    async getRole(name: string): Promise<RoleDetails> {
      const response = await client.getRole({ name }, { headers: headers() });
      return {
        roles: [],
        role: response.role ? serverRole(response.role) : null,
        users: response.users.map(roleUser),
        viewerCanManageRoles: response.viewerCanManageRoles,
        viewerCanAssignRoles: response.viewerCanAssignRoles,
      };
    },

    async createRole(input: CreateRoleInput): Promise<ServerRole> {
      const response = await client.createRole(input, { headers: headers() });
      return requiredRole(response.role);
    },

    async updateRole(input: UpdateRoleInput): Promise<ServerRole> {
      const response = await client.updateRole(input, { headers: headers() });
      return requiredRole(response.role);
    },

    async deleteRole(name: string): Promise<boolean> {
      const response = await client.deleteRole(
        { name },
        { headers: headers() },
      );
      return response.deleted;
    },
  };
}

export type RoleAPI = ReturnType<typeof createRoleAPI>;

function requiredRole(role: APIRole | undefined): ServerRole {
  if (!role) {
    throw new Error("role response did not include a role");
  }
  return serverRole(role);
}

function serverRole(role: APIRole): ServerRole {
  return {
    name: role.name,
    displayName: role.displayName,
    description: role.description,
    permissions: [...role.permissions],
    permissionDenials: [...role.permissionDenials],
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
