import { beforeEach, describe, expect, it } from 'vitest';
import { adminQueryKeys } from './admin';
import {
  invalidateAdminRoomLayoutQueries,
  invalidatePermissionTiers,
  invalidateRolePermissionDependents,
  purgeAdminRoomGroupQuery,
  purgeAdminRoomQuery,
  removeDeletedRoleQueries
} from './adminInvalidation';
import { queryClient } from './client';

const connection = { queryScope: 'admin-invalidation-test' };
const serverId = 'server-1';
const tierKey = adminQueryKeys.permissionTiers(serverId, connection);
const catalogKey = adminQueryKeys.roleCatalog(serverId, connection);
const roleKey = adminQueryKeys.rolePermissions(serverId, connection, 'moderator');
const roleDetailsKey = adminQueryKeys.role(serverId, connection, 'moderator');
const userKey = adminQueryKeys.userPermissions(serverId, connection, 'user-1');
const roomKey = adminQueryKeys.room(serverId, connection, 'room-1');
const groupKey = adminQueryKeys.roomGroup(serverId, connection, 'group-1');
const roomPermissionsKey = adminQueryKeys.permissionTier(serverId, connection, 'room-1', null);
const groupPermissionsKey = adminQueryKeys.permissionTier(serverId, connection, null, 'group-1');

beforeEach(() => {
  queryClient.clear();
  queryClient.setQueryData(tierKey, { roles: [] });
  queryClient.setQueryData(catalogKey, { roles: [] });
  queryClient.setQueryData(roleKey, { roleName: 'moderator' });
  queryClient.setQueryData(roleDetailsKey, { role: { name: 'moderator' } });
  queryClient.setQueryData(userKey, { userId: 'user-1' });
  queryClient.setQueryData(roomKey, { id: 'room-1', name: 'general' });
  queryClient.setQueryData(groupKey, { group: { id: 'group-1', name: 'Lobby' } });
  queryClient.setQueryData(roomPermissionsKey, { roles: ['private-room-role'] });
  queryClient.setQueryData(groupPermissionsKey, { roles: ['private-group-role'] });
});

describe('admin room-layout query invalidation', () => {
  it('invalidates only the named room and group snapshots', () => {
    invalidateAdminRoomLayoutQueries(serverId, connection, 'room-1', 'group-1');

    expect(queryClient.getQueryState(roomKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(groupKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(tierKey)?.isInvalidated).toBe(false);
  });

  it('synchronously purges removed room and group snapshots', () => {
    purgeAdminRoomQuery(serverId, connection, 'room-1');
    purgeAdminRoomGroupQuery(serverId, connection, 'group-1');

    expect(queryClient.getQueryData(roomKey)).toBeNull();
    expect(queryClient.getQueryData(groupKey)).toBeNull();
    expect(queryClient.getQueryData(roomPermissionsKey)).toBeUndefined();
    expect(queryClient.getQueryData(groupPermissionsKey)).toBeUndefined();
    expect(queryClient.getQueryState(roomKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(groupKey)?.isInvalidated).toBe(true);
  });
});

describe('admin role query invalidation', () => {
  it('invalidates only permission tiers after role metadata changes', () => {
    invalidatePermissionTiers(serverId, connection);

    expect(queryClient.getQueryState(tierKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(catalogKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(roleKey)?.isInvalidated).toBe(false);
    expect(queryClient.getQueryState(userKey)?.isInvalidated).toBe(false);
  });

  it('invalidates tier, role details, and derived user matrices after role permission changes', () => {
    invalidateRolePermissionDependents(serverId, connection, 'moderator');

    expect(queryClient.getQueryState(tierKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(roleDetailsKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(userKey)?.isInvalidated).toBe(true);
  });

  it('removes a deleted role matrix and invalidates every derived cache', () => {
    removeDeletedRoleQueries(serverId, connection, 'moderator');

    expect(queryClient.getQueryData(roleKey)).toBeUndefined();
    expect(queryClient.getQueryData(roleDetailsKey)).toBeUndefined();
    expect(queryClient.getQueryState(tierKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(userKey)?.isInvalidated).toBe(true);
  });
});
