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
import { queryClient } from './queryClient';

const connection = { queryScope: 'admin-invalidation-test' };
const serverId = 'server-1';
const scope = { serverId, connection };
const tierKey = adminQueryKeys.permissionTiers(scope);
const catalogKey = adminQueryKeys.roleCatalog(scope);
const roleKey = adminQueryKeys.rolePermissions(scope, 'moderator');
const membersKey = adminQueryKeys.roleMembers(scope, 'moderator');
const roleDetailsKey = adminQueryKeys.role(scope, 'moderator');
const userKey = adminQueryKeys.userPermissions(scope, 'user-1');
const roomKey = adminQueryKeys.room(scope, 'room-1');
const groupKey = adminQueryKeys.roomGroup(scope, 'group-1');
const roomPermissionsKey = adminQueryKeys.permissionTier(scope, 'room-1', null);
const groupPermissionsKey = adminQueryKeys.permissionTier(scope, null, 'group-1');

beforeEach(() => {
  queryClient.clear();
  queryClient.setQueryData(tierKey, { roles: [] });
  queryClient.setQueryData(catalogKey, { roles: [] });
  queryClient.setQueryData(roleKey, { roleName: 'moderator' });
  queryClient.setQueryData(roleDetailsKey, { role: { name: 'moderator' } });
  queryClient.setQueryData(membersKey, { pages: [{ users: [{ id: 'user-1' }] }], pageParams: [0] });
  queryClient.setQueryData(userKey, { userId: 'user-1' });
  queryClient.setQueryData(roomKey, { id: 'room-1', name: 'general' });
  queryClient.setQueryData(groupKey, { group: { id: 'group-1', name: 'Lobby' } });
  queryClient.setQueryData(roomPermissionsKey, { roles: ['private-room-role'] });
  queryClient.setQueryData(groupPermissionsKey, { roles: ['private-group-role'] });
});

describe('admin room-layout query invalidation', () => {
  it('invalidates only the named room and group snapshots', () => {
    invalidateAdminRoomLayoutQueries(scope, 'room-1', 'group-1');

    expect(queryClient.getQueryState(roomKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(groupKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(tierKey)?.isInvalidated).toBe(false);
  });

  it('synchronously purges removed room and group snapshots', () => {
    purgeAdminRoomQuery(scope, 'room-1');
    purgeAdminRoomGroupQuery(scope, 'group-1');

    expect(queryClient.getQueryData(roomKey)).toBeNull();
    expect(queryClient.getQueryData(groupKey)).toBeNull();
    expect(queryClient.getQueryData(roomPermissionsKey)).toBeUndefined();
    expect(queryClient.getQueryData(groupPermissionsKey)).toBeUndefined();
    expect(queryClient.getQueryState(roomKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(groupKey)?.isInvalidated).toBe(true);
  });
});

describe('admin role query invalidation', () => {
  it('invalidates role listings after metadata or membership changes', () => {
    invalidatePermissionTiers(scope);

    expect(queryClient.getQueryState(tierKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(catalogKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(roleKey)?.isInvalidated).toBe(false);
    expect(queryClient.getQueryState(userKey)?.isInvalidated).toBe(false);
  });

  it('invalidates role counts, details, and derived user matrices after permission changes', () => {
    invalidateRolePermissionDependents(scope, 'moderator');

    expect(queryClient.getQueryState(tierKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(catalogKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(roleDetailsKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(userKey)?.isInvalidated).toBe(true);
  });

  it('removes a deleted role matrix and invalidates every derived cache', () => {
    removeDeletedRoleQueries(scope, 'moderator');

    expect(queryClient.getQueryData(roleKey)).toBeUndefined();
    expect(queryClient.getQueryData(roleDetailsKey)).toBeUndefined();
    expect(queryClient.getQueryData(membersKey)).toBeUndefined();
    expect(queryClient.getQueryState(tierKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(userKey)?.isInvalidated).toBe(true);
  });
});
