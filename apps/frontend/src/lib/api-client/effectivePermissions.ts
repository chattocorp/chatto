import { PermissionService } from '@chatto/api-types/api/v1/permissions_connect';
import { EffectivePermissionScopeKind } from '@chatto/api-types/api/v1/permissions_pb';
import { createChattoClient, type ConnectAPIConfig } from './connect';

/** One effective grant. Child coverage includes scopes hidden from the viewer. */
export type EffectivePermission = {
  permission: string;
  scope: 'server' | 'group' | 'room' | 'dm';
  scopeId: string;
  scopeName: string;
  parentGroupId: string;
  coversDescendants: boolean;
};

/** Read allowed authority through api.v1; configuration remains in the admin API. */
export function createEffectivePermissionAPI(config: ConnectAPIConfig) {
  const client = createChattoClient(PermissionService, config);
  return {
    async listEffectivePermissions(userId: string, signal?: AbortSignal) {
      const response = await client.listEffectivePermissions({ userId }, { signal });
      const permissions: EffectivePermission[] = response.permissions.map((entry) => {
        const scope = entry.scope;
        if (!scope) throw new Error('Missing effective permission scope');
        const kind =
          scope.kind === EffectivePermissionScopeKind.SERVER
            ? 'server'
            : scope.kind === EffectivePermissionScopeKind.DM
              ? 'dm'
              : scope.kind === EffectivePermissionScopeKind.GROUP
                ? 'group'
                : scope.kind === EffectivePermissionScopeKind.ROOM
                  ? 'room'
                  : null;
        if (!kind) throw new Error('Unsupported effective permission scope');
        return {
          permission: entry.permission,
          scope: kind,
          scopeId: scope.id,
          scopeName: scope.name,
          parentGroupId: scope.parentGroupId,
          coversDescendants: entry.coversDescendants
        };
      });
      return permissions;
    }
  };
}
