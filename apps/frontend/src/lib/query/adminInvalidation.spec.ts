import { beforeEach, describe, expect, it } from 'vitest';
import { adminQueryKeys } from './admin';
import {
  invalidatePermissionTiers,
  invalidateRolePermissionDependents,
  removeDeletedRoleQueries
} from './adminInvalidation';
import { queryClient } from './client';

const connection = { queryScope: 'admin-invalidation-test' };
const serverId = 'server-1';
const tierKey = adminQueryKeys.permissionTiers(serverId, connection);
const roleKey = adminQueryKeys.rolePermissions(serverId, connection, 'moderator');
const userKey = adminQueryKeys.userPermissions(serverId, connection, 'user-1');

beforeEach(() => {
  queryClient.clear();
  queryClient.setQueryData(tierKey, { roles: [] });
  queryClient.setQueryData(roleKey, { roleName: 'moderator' });
  queryClient.setQueryData(userKey, { userId: 'user-1' });
});

describe('admin role query invalidation', () => {
  it('invalidates only permission tiers after role metadata changes', () => {
    invalidatePermissionTiers(serverId, connection);

    expect(queryClient.getQueryState(tierKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(roleKey)?.isInvalidated).toBe(false);
    expect(queryClient.getQueryState(userKey)?.isInvalidated).toBe(false);
  });

  it('invalidates tier and derived user matrices after role permission changes', () => {
    invalidateRolePermissionDependents(serverId, connection);

    expect(queryClient.getQueryState(tierKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(userKey)?.isInvalidated).toBe(true);
  });

  it('removes a deleted role matrix and invalidates every derived cache', () => {
    removeDeletedRoleQueries(serverId, connection, 'moderator');

    expect(queryClient.getQueryData(roleKey)).toBeUndefined();
    expect(queryClient.getQueryState(tierKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(userKey)?.isInvalidated).toBe(true);
  });
});
