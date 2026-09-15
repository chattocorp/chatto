import { PermissionService } from '@chatto/api-types/api/v1/permissions_connect';
import { EffectivePermissionScopeKind } from '@chatto/api-types/api/v1/permissions_pb';
import { authHeaders, createChattoClient } from './connect';
import type { PermissionAPIConfig } from './permissions';

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
export function createEffectivePermissionAPI(config: PermissionAPIConfig) {
  const client = createChattoClient(PermissionService, config);
  return {
    async listEffectivePermissions(userId: string, offset = 0, signal?: AbortSignal) {
      const response = await client.listEffectivePermissions(
        { userId, page: { limit: 100, offset } },
        { headers: authHeaders(config), signal }
      );
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
      const hasMore = response.page?.hasMore ?? false;
      if (hasMore && permissions.length === 0) throw new Error('Empty effective permission page');
      return {
        permissions,
        hasMore,
        nextOffset: hasMore ? offset + permissions.length : undefined
      };
    }
  };
}
