import { authHeaders, createChattoClient } from './connect.js';
import { AdminPermissionService } from '@chatto/api-types/admin/v1/permissions_connect';
import {
  PermissionDecision,
  PermissionScopeKind,
  type PermissionMatrixCell as APIPermissionMatrixCell,
  type PermissionMatrixScope as APIPermissionMatrixScope,
  type PermissionDecisionUpdate as APIPermissionDecisionUpdate,
  type RolePermissionMatrix as APIRolePermissionMatrix,
  type ScopedPermissionDecision as APIScopedPermissionDecision,
  type TierRole as APITierRole,
  type TierRoles as APITierRoles,
  type UserPermissionMatrix as APIUserPermissionMatrix
} from '@chatto/api-types/admin/v1/permissions_pb';

export type PermissionAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export type PermissionState = 'allow' | 'deny' | 'neutral';
export type MatrixDecision = 'ALLOW' | 'DENY' | 'NONE';
export type MatrixScopeKind = 'SERVER' | 'GROUP' | 'ROOM' | 'DM';

export type PermissionScope =
  | { tier: 'server' }
  | { tier: 'dm' }
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
  pingable: boolean;
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
  allowPermitted?: boolean;
};

export type MatrixData = {
  applicablePermissions: string[];
  scopes: MatrixScope[];
  cells: MatrixCell[];
};

/** Scope counts, rather than permission-cell counts. */
export type PermissionScopePage = { totalCount: number; hasMore: boolean };
export type PermissionReadOptions = {
  signal?: AbortSignal;
  page?: { limit?: number; offset?: number };
  scope?: PermissionScope;
};

export type RolePermissionMatrix = MatrixData & {
  roleName: string;
  page: PermissionScopePage;
};

export type UserPermissionMatrix = MatrixData & {
  userId: string;
  page: PermissionScopePage;
};

export type PermissionDecisionEntry = {
  permission: string;
  scope: PermissionScope;
  override: MatrixDecision;
  effective: MatrixDecision;
};

export type PermissionDecisionUpdate = {
  permission: string;
  scope: PermissionScope;
  decision: MatrixDecision;
};

export type RolePermissionDecisions = {
  roleName: string;
  decisions: PermissionDecisionEntry[];
  scopes: PermissionScope[];
  page: PermissionScopePage;
};

export type UserPermissionDecisions = {
  userId: string;
  decisions: PermissionDecisionEntry[];
  scopes: PermissionScope[];
  page: PermissionScopePage;
};

export function createPermissionAPI(config: PermissionAPIConfig) {
  const client = createChattoClient(AdminPermissionService, config);
  const headers = () => authHeaders(config);

  return {
    async getRolePermissionTierMatrix(
      input: {
        roomId?: string | null;
        groupId?: string | null;
      },
      options: { signal?: AbortSignal } = {}
    ): Promise<TierRoles | null> {
      const response = await client.getRolePermissionTierMatrix(
        {
          scope: apiTierMatrixScope(input)
        },
        { headers: headers(), ...(options.signal ? { signal: options.signal } : {}) }
      );
      return response.matrix ? tierRoles(response.matrix) : null;
    },

    async getRolePermissionMatrix(
      roleName: string,
      options: PermissionReadOptions = {}
    ): Promise<RolePermissionMatrix | null> {
      const response = await client.getRolePermissionMatrix(
        {
          roleName,
          includeDirectMessageScope: true,
          page: options.page,
          scope: options.scope ? apiScope(options.scope) : undefined
        },
        { headers: headers(), ...(options.signal ? { signal: options.signal } : {}) }
      );
      return response.matrix
        ? { ...rolePermissionMatrix(response.matrix), page: scopePage(response.page) }
        : null;
    },

    async listRolePermissionDecisions(
      roleName: string,
      options: PermissionReadOptions = {}
    ): Promise<RolePermissionDecisions> {
      const response = await client.listRolePermissionDecisions(
        {
          roleName,
          includeDirectMessageScope: true,
          page: options.page,
          scope: options.scope ? apiScope(options.scope) : undefined
        },
        { headers: headers(), signal: options.signal }
      );
      return {
        roleName: response.roleName,
        decisions: response.decisions.map(permissionDecisionEntry),
        scopes: response.scopes.map(permissionScope),
        page: scopePage(response.page)
      };
    },

    async getUserPermissionMatrix(
      userId: string,
      options: PermissionReadOptions = {}
    ): Promise<UserPermissionMatrix | null> {
      const response = await client.getUserPermissionMatrix(
        {
          userId,
          includeDirectMessageScope: true,
          page: options.page,
          scope: options.scope ? apiScope(options.scope) : undefined
        },
        { headers: headers(), ...(options.signal ? { signal: options.signal } : {}) }
      );
      return response.matrix
        ? { ...userPermissionMatrix(response.matrix), page: scopePage(response.page) }
        : null;
    },

    async listUserPermissionDecisions(
      userId: string,
      options: PermissionReadOptions = {}
    ): Promise<UserPermissionDecisions> {
      const response = await client.listUserPermissionDecisions(
        {
          userId,
          includeDirectMessageScope: true,
          page: options.page,
          scope: options.scope ? apiScope(options.scope) : undefined
        },
        { headers: headers(), signal: options.signal }
      );
      return {
        userId: response.userId,
        decisions: response.decisions.map(permissionDecisionEntry),
        scopes: response.scopes.map(permissionScope),
        page: scopePage(response.page)
      };
    },

    async setRolePermission(input: {
      roleName: string;
      scope: PermissionScope;
      permission: string;
      state: PermissionState;
    }): Promise<PermissionDecisionUpdate> {
      const response = await client.setRolePermission(
        {
          roleName: input.roleName,
          permission: input.permission,
          decision: apiDecision(input.state),
          scope: apiScope(input.scope)
        },
        { headers: headers() }
      );
      return permissionDecisionUpdate(response.decision);
    },

    async setUserPermission(input: {
      userId: string;
      scope: PermissionScope;
      permission: string;
      state: PermissionState;
    }): Promise<PermissionDecisionUpdate> {
      const response = await client.setUserPermission(
        {
          userId: input.userId,
          permission: input.permission,
          decision: apiDecision(input.state),
          scope: apiScope(input.scope)
        },
        { headers: headers() }
      );
      return permissionDecisionUpdate(response.decision);
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
  const apiRole = role.role;
  if (!apiRole) {
    throw new Error('permission tier role response did not include role metadata');
  }
  return {
    roleName: apiRole.name,
    displayName: apiRole.displayName,
    description: apiRole.description,
    isSystem: apiRole.isSystem,
    position: apiRole.position,
    pingable: apiRole.pingable,
    override: {
      permissions: [...(role.override?.permissions ?? [])],
      permissionDenials: [...(role.override?.permissionDenials ?? [])]
    },
    inheritedAllows: [...role.inheritedAllows],
    inheritedDenials: [...role.inheritedDenials]
  };
}

function rolePermissionMatrix(matrix: APIRolePermissionMatrix): Omit<RolePermissionMatrix, 'page'> {
  return {
    roleName: matrix.roleName,
    applicablePermissions: [...matrix.applicablePermissions],
    scopes: matrix.scopes.map(matrixScope),
    cells: matrix.cells.map(matrixCell)
  };
}

function userPermissionMatrix(matrix: APIUserPermissionMatrix): Omit<UserPermissionMatrix, 'page'> {
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
    effective: matrixDecision(cell.effective),
    ...(cell.allowPermitted !== undefined ? { allowPermitted: cell.allowPermitted } : {})
  };
}

function permissionDecisionEntry(decision: APIScopedPermissionDecision): PermissionDecisionEntry {
  return {
    permission: decision.permission,
    scope: permissionScope(decision.scope),
    override: matrixDecision(decision.override),
    effective: matrixDecision(decision.effective)
  };
}

function permissionDecisionUpdate(
  decision: APIPermissionDecisionUpdate | undefined
): PermissionDecisionUpdate {
  if (!decision) {
    throw new Error('permission write response did not include a decision');
  }
  return {
    permission: decision.permission,
    scope: permissionScope(decision.scope),
    decision: matrixDecision(decision.decision)
  };
}

function permissionScope(
  scope: { kind: PermissionScopeKind; id: string } | undefined
): PermissionScope {
  if (scope?.kind === PermissionScopeKind.GROUP) {
    return { tier: 'group', groupId: scope.id };
  }
  if (scope?.kind === PermissionScopeKind.ROOM) {
    return { tier: 'room', roomId: scope.id };
  }
  if (scope?.kind === PermissionScopeKind.DM) return { tier: 'dm' };
  if (scope?.kind === PermissionScopeKind.SERVER) return { tier: 'server' };
  throw new Error(`unsupported permission scope kind: ${scope?.kind ?? 'missing'}`);
}

function scopeKind(kind: PermissionScopeKind): MatrixScopeKind {
  if (kind === PermissionScopeKind.GROUP) return 'GROUP';
  if (kind === PermissionScopeKind.ROOM) return 'ROOM';
  if (kind === PermissionScopeKind.DM) return 'DM';
  if (kind === PermissionScopeKind.SERVER) return 'SERVER';
  throw new Error(`unsupported permission matrix scope kind: ${kind}`);
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

function apiScope(scope: PermissionScope): {
  kind: PermissionScopeKind;
  id: string;
} {
  if (scope.tier === 'group') {
    return { kind: PermissionScopeKind.GROUP, id: scope.groupId };
  }
  if (scope.tier === 'room') {
    return { kind: PermissionScopeKind.ROOM, id: scope.roomId };
  }
  if (scope.tier === 'dm') {
    return { kind: PermissionScopeKind.DM, id: '' };
  }
  return { kind: PermissionScopeKind.SERVER, id: '' };
}

function apiTierMatrixScope(input: { roomId?: string | null; groupId?: string | null }): {
  kind: PermissionScopeKind;
  id: string;
} {
  if (input.roomId) {
    return { kind: PermissionScopeKind.ROOM, id: input.roomId };
  }
  if (input.groupId) {
    return { kind: PermissionScopeKind.GROUP, id: input.groupId };
  }
  return { kind: PermissionScopeKind.SERVER, id: '' };
}

function scopePage(
  page: { totalCount: bigint; hasMore: boolean } | undefined
): PermissionScopePage {
  if (!page) throw new Error('permission response did not include scope page metadata');
  return { totalCount: Number(page.totalCount), hasMore: page.hasMore };
}
