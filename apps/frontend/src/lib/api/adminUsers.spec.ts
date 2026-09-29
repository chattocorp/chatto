import { Timestamp } from '@bufbuild/protobuf';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AdminUserService } from '@chatto/api-types/admin/v1/members_connect';
import { createAdminUserManagementAPI } from './adminUsers';
import { fakeServer, mockService, receivedRequest } from '@chatto/client/testing/fakeServer';

const mocks = mockService(AdminUserService);

function adminUserAPI() {
  return createAdminUserManagementAPI(
    fakeServer((router) => router.service(AdminUserService, mocks))
  );
}

describe('createAdminUserManagementAPI', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('lists admin members and maps timestamps and roles', async () => {
    const createdAt = new Date('2026-01-02T03:04:05.000Z');
    mocks.listMembers.mockReturnValue({
      userIds: ['user-1'],
      page: { totalCount: 1n, hasMore: false }
    });
    mocks.batchGetMembers.mockReturnValue({
      members: [
        {
          user: {
            id: 'user-1',
            login: 'alice',
            displayName: 'Alice',
            avatarUrl: undefined,
            deleted: false,
            bot: { ownerUserId: 'owner' }
          },
          roles: ['admin'],
          createdAt: Timestamp.fromDate(createdAt),
          hasVerifiedEmail: true,
          verifiedEmails: ['first@example.test', 'alice@example.test'],
          primaryVerifiedEmail: 'alice@example.test',
          viewerCanDeleteAccount: true,
          lastLoginChange: undefined
        }
      ],
      roles: [{ name: 'admin', displayName: 'Admin' }]
    });
    const api = adminUserAPI();

    const result = await api.listMembers({ search: 'alice', limit: 20, offset: 0 });

    expect(receivedRequest(mocks.batchGetMembers)).toMatchObject({ userIds: ['user-1'] });
    expect(receivedRequest(mocks.listMembers)).toMatchObject({
      search: 'alice',
      page: { limit: 20, offset: 0 }
    });
    expect(result).toEqual({
      consumedCount: 1,
      users: [
        {
          id: 'user-1',
          login: 'alice',
          displayName: 'Alice',
          avatarUrl: null,
          isBot: true,
          roles: ['admin'],
          createdAt: '2026-01-02T03:04:05.000Z',
          deleted: false,
          hasVerifiedEmail: true,
          verifiedEmails: ['first@example.test', 'alice@example.test'],
          primaryVerifiedEmail: 'alice@example.test',
          viewerCanDeleteAccount: true,
          lastLoginChange: null
        }
      ],
      roles: [{ name: 'admin', displayName: 'Admin' }],
      totalCount: 1,
      hasMore: false
    });
  });

  it('counts IDs omitted during hydration and skips the batch for an empty page', async () => {
    mocks.listMembers
      .mockReturnValueOnce({ userIds: ['missing'], page: { totalCount: 2n, hasMore: true } })
      .mockReturnValueOnce({ userIds: [], page: { totalCount: 0n, hasMore: false } });
    mocks.batchGetMembers.mockReturnValue({ members: [], roles: [] });
    const api = adminUserAPI();
    expect(await api.listMembers({ limit: 20, offset: 0 })).toEqual({
      users: [],
      roles: [],
      consumedCount: 1,
      totalCount: 2,
      hasMore: true
    });
    expect(await api.listMembers({ limit: 20, offset: 1 })).toEqual({
      users: [],
      roles: [],
      consumedCount: 0,
      totalCount: 0,
      hasMore: false
    });
    expect(mocks.batchGetMembers).toHaveBeenCalledOnce();
  });

  it('gets admin member details and maps permission metadata', async () => {
    const lastLoginChange = new Date('2026-02-03T04:05:06.000Z');
    mocks.getMember.mockReturnValue({
      member: {
        user: {
          id: 'user-2',
          login: 'bob',
          displayName: 'Bob',
          avatarUrl: '/assets/bob.png',
          deleted: false
        },
        roles: ['moderator'],
        createdAt: undefined,
        hasVerifiedEmail: false,
        verifiedEmails: [],
        viewerCanDeleteAccount: false,
        lastLoginChange: Timestamp.fromDate(lastLoginChange)
      },
      roles: [
        {
          role: {
            name: 'moderator',
            displayName: 'Moderator',
            description: '',
            isSystem: true,
            position: 50,
            pingable: false
          },
          permissions: ['room.manage'],
          permissionDenials: ['message.post']
        }
      ],
      availablePermissions: ['room.manage', 'message.post'],
      viewerCanAssignRoles: true,
      viewerCanManageRoles: false,
      viewerCanManageUserPermissions: true,
      assignableRoleNames: ['moderator'],
      revocableRoleNames: ['moderator'],
      roleAssignmentLimitsEnforced: true
    });
    const api = adminUserAPI();

    const result = await api.getMember('user-2');

    expect(receivedRequest(mocks.getMember)).toMatchObject({
      target: { case: 'userId', value: 'user-2' }
    });
    expect(result).toEqual({
      member: {
        id: 'user-2',
        login: 'bob',
        displayName: 'Bob',
        avatarUrl: '/assets/bob.png',
        roles: ['moderator'],
        createdAt: null,
        deleted: false,
        hasVerifiedEmail: false,
        verifiedEmails: [],
        primaryVerifiedEmail: null,
        viewerCanDeleteAccount: false,
        lastLoginChange: '2026-02-03T04:05:06.000Z'
      },
      roles: [
        {
          name: 'moderator',
          displayName: 'Moderator',
          position: 50,
          permissions: ['room.manage'],
          permissionDenials: ['message.post']
        }
      ],
      availablePermissions: ['room.manage', 'message.post'],
      viewerCanAssignRoles: true,
      viewerCanManageRoles: false,
      viewerCanManageUserPermissions: true,
      assignableRoleNames: ['moderator'],
      revocableRoleNames: ['moderator']
    });
  });

  it('gets a member by login', async () => {
    mocks.getMember.mockReturnValue({
      member: undefined,
      roles: [],
      availablePermissions: [],
      viewerCanAssignRoles: false,
      viewerCanManageRoles: false,
      viewerCanManageUserPermissions: false,
      roleAssignmentLimitsEnforced: false
    });
    const api = adminUserAPI();

    await api.getMember({ login: 'alice' });

    expect(receivedRequest(mocks.getMember)).toMatchObject({
      target: { case: 'login', value: 'alice' }
    });
  });

  it('assigns and revokes roles', async () => {
    const member = {
      user: {
        id: 'user-1',
        login: 'alice',
        displayName: 'Alice',
        deleted: false
      },
      roles: ['moderator'],
      hasVerifiedEmail: false,
      verifiedEmails: [],
      viewerCanDeleteAccount: false
    };
    mocks.assignRole.mockReturnValue({ member });
    mocks.revokeRole.mockReturnValue({ member: { ...member, roles: [] } });
    const api = adminUserAPI();

    await expect(api.assignRole('user-1', 'moderator')).resolves.toMatchObject({
      changed: true,
      member: { id: 'user-1', roles: ['moderator'] }
    });
    await expect(api.revokeRole('user-1', 'moderator')).resolves.toMatchObject({
      changed: true,
      member: { id: 'user-1', roles: [] }
    });

    expect(receivedRequest(mocks.assignRole)).toMatchObject({
      userId: 'user-1',
      roleName: 'moderator'
    });
    expect(receivedRequest(mocks.revokeRole)).toMatchObject({
      userId: 'user-1',
      roleName: 'moderator'
    });
  });

  it('clears username cooldown', async () => {
    mocks.clearUsernameCooldown.mockReturnValue({});
    const api = adminUserAPI();

    await expect(api.clearUsernameCooldown('user-1')).resolves.toBe(true);

    expect(receivedRequest(mocks.clearUsernameCooldown)).toMatchObject({ userId: 'user-1' });
  });

  it('sets a user password', async () => {
    mocks.changeUserPassword.mockReturnValue({
      member: {
        user: {
          id: 'user-1',
          login: 'alice',
          displayName: 'Alice',
          avatarUrl: undefined,
          deleted: false
        },
        roles: ['admin'],
        createdAt: undefined,
        hasVerifiedEmail: false,
        verifiedEmails: [],
        viewerCanDeleteAccount: true,
        lastLoginChange: undefined
      }
    });
    const api = adminUserAPI();

    await expect(api.changeUserPassword('user-1', 'newpassword456')).resolves.toMatchObject({
      id: 'user-1',
      login: 'alice',
      displayName: 'Alice',
      roles: ['admin']
    });

    expect(receivedRequest(mocks.changeUserPassword)).toMatchObject({
      userId: 'user-1',
      password: 'newpassword456'
    });
  });

  it('deletes a user', async () => {
    mocks.deleteUser.mockReturnValue({});
    const api = adminUserAPI();

    await expect(api.deleteUser({ userId: 'user-1' })).resolves.toBe(true);

    expect(receivedRequest(mocks.deleteUser)).toMatchObject({ userId: 'user-1' });
  });
});
