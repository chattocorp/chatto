import { createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { AdminPermissionService } from '$lib/pb/chatto/admin/v1/permissions_connect';
import {
  PermissionDecision,
  PermissionScopeKind,
  type PermissionMatrixCell as APIPermissionMatrixCell,
  type PermissionMatrixScope as APIPermissionMatrixScope,
  type RolePermissionMatrix as APIRolePermissionMatrix,
  type TierRole as APITierRole,
  type TierRoles as APITierRoles,
  type UserPermissionMatrix as APIUserPermissionMatrix
} from '$lib/pb/chatto/admin/v1/permissions_pb';

export type PermissionAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
};

export type PermissionState = 'allow' | 'deny' | 'neutral';
export type MatrixDecision = 'ALLOW' | 'DENY' | 'NONE';
export type MatrixScopeKind = 'SERVER' | 'GROUP' | 'ROOM';

export type PermissionScope =
  | { tier: 'server' }
  | { tier: 'group'; groupId: string }
  | { tier: 'room'; roomId: string };

export type TierPermissions = {
  permissions: string[];
  permissionDenials: string[];
};

export type TierRole = {
  roleName: string;
  displayName: string;
  description: string;
  isSystem: boolean;
  position: number;
  override: TierPermissions;
  inheritedAllows: string[];
  inheritedDenials: string[];
};

export type TierRoles = {
  applicablePermissions: string[];
  roles: TierRole[];
};

export type MatrixScope = {
  id: string;
  label: string;
  kind: MatrixScopeKind;
  parentGroupId: string;
};

export type MatrixCell = {
  permission: string;
  scopeId: string;
  override: MatrixDecision;
  effective: MatrixDecision;
};

export type MatrixData = {
  applicablePermissions: string[];
  scopes: MatrixScope[];
  cells: MatrixCell[];
};

export type RolePermissionMatrix = MatrixData & {
  roleName: string;
};

export type UserPermissionMatrix = MatrixData & {
  userId: string;
};

export function createPermissionAPI(config: PermissionAPIConfig) {
  const transport = createConnectTransport({
    baseUrl: config.baseUrl,
    useBinaryFormat: true
  });
  const client = createClient(AdminPermissionService, transport);
  const headers = () =>
    config.bearerToken ? { Authorization: `Bearer ${config.bearerToken}` } : undefined;

  return {
    async getRolePermissionTierMatrix(input: {
      roomId?: string | null;
      groupId?: string | null;
    }): Promise<TierRoles | null> {
      const response = await client.getRolePermissionTierMatrix(
        {
          scope: apiTierMatrixScope(input)
        },
        { headers: headers() }
      );
      return response.matrix ? tierRoles(response.matrix) : null;
    },

    async getRolePermissionMatrix(roleName: string): Promise<RolePermissionMatrix | null> {
      const response = await client.getRolePermissionMatrix({ roleName }, { headers: headers() });
      return response.matrix ? rolePermissionMatrix(response.matrix) : null;
    },

    async getUserPermissionMatrix(userId: string): Promise<UserPermissionMatrix | null> {
      const response = await client.getUserPermissionMatrix({ userId }, { headers: headers() });
      return response.matrix ? userPermissionMatrix(response.matrix) : null;
    },

    async setRolePermission(input: {
      roleName: string;
      scope: PermissionScope;
      permission: string;
      state: PermissionState;
    }): Promise<boolean> {
      const response = await client.setRolePermission(
        {
          roleName: input.roleName,
          permission: input.permission,
          decision: apiDecision(input.state),
          scope: apiScope(input.scope)
        },
        { headers: headers() }
      );
      return response.ok;
    },

    async setUserPermission(input: {
      userId: string;
      scope: PermissionScope;
      permission: string;
      state: PermissionState;
    }): Promise<boolean> {
      const response = await client.setUserPermission(
        {
          userId: input.userId,
          permission: input.permission,
          decision: apiDecision(input.state),
          scope: apiScope(input.scope)
        },
        { headers: headers() }
      );
      return response.ok;
    }
  };
}

export type PermissionAPI = ReturnType<typeof createPermissionAPI>;

function tierRoles(matrix: APITierRoles): TierRoles {
  return {
    applicablePermissions: [...matrix.applicablePermissions],
    roles: matrix.roles.map(tierRole)
  };
}

function tierRole(role: APITierRole): TierRole {
  return {
    roleName: role.roleName,
    displayName: role.displayName,
    description: role.description,
    isSystem: role.isSystem,
    position: role.position,
    override: {
      permissions: [...(role.override?.permissions ?? [])],
      permissionDenials: [...(role.override?.permissionDenials ?? [])]
    },
    inheritedAllows: [...role.inheritedAllows],
    inheritedDenials: [...role.inheritedDenials]
  };
}

function rolePermissionMatrix(matrix: APIRolePermissionMatrix): RolePermissionMatrix {
  return {
    roleName: matrix.roleName,
    applicablePermissions: [...matrix.applicablePermissions],
    scopes: matrix.scopes.map(matrixScope),
    cells: matrix.cells.map(matrixCell)
  };
}

function userPermissionMatrix(matrix: APIUserPermissionMatrix): UserPermissionMatrix {
  return {
    userId: matrix.userId,
    applicablePermissions: [...matrix.applicablePermissions],
    scopes: matrix.scopes.map(matrixScope),
    cells: matrix.cells.map(matrixCell)
  };
}

function matrixScope(scope: APIPermissionMatrixScope): MatrixScope {
  return {
    id: scope.id,
    label: scope.label,
    kind: scopeKind(scope.kind),
    parentGroupId: scope.parentGroupId
  };
}

function matrixCell(cell: APIPermissionMatrixCell): MatrixCell {
  return {
    permission: cell.permission,
    scopeId: cell.scopeId,
    override: matrixDecision(cell.override),
    effective: matrixDecision(cell.effective)
  };
}

function scopeKind(kind: PermissionScopeKind): MatrixScopeKind {
  if (kind === PermissionScopeKind.GROUP) return 'GROUP';
  if (kind === PermissionScopeKind.ROOM) return 'ROOM';
  return 'SERVER';
}

function matrixDecision(decision: PermissionDecision): MatrixDecision {
  if (decision === PermissionDecision.ALLOW) return 'ALLOW';
  if (decision === PermissionDecision.DENY) return 'DENY';
  return 'NONE';
}

function apiDecision(state: PermissionState): PermissionDecision {
  if (state === 'allow') return PermissionDecision.ALLOW;
  if (state === 'deny') return PermissionDecision.DENY;
  return PermissionDecision.NONE;
}

function apiScope(scope: PermissionScope): { kind: PermissionScopeKind; id: string } {
  if (scope.tier === 'group') {
    return { kind: PermissionScopeKind.GROUP, id: scope.groupId };
  }
  if (scope.tier === 'room') {
    return { kind: PermissionScopeKind.ROOM, id: scope.roomId };
  }
  return { kind: PermissionScopeKind.SERVER, id: '' };
}

function apiTierMatrixScope(input: {
  roomId?: string | null;
  groupId?: string | null;
}): { kind: PermissionScopeKind; id: string } {
  if (input.roomId) {
    return { kind: PermissionScopeKind.ROOM, id: input.roomId };
  }
  if (input.groupId) {
    return { kind: PermissionScopeKind.GROUP, id: input.groupId };
  }
  return { kind: PermissionScopeKind.SERVER, id: '' };
}
