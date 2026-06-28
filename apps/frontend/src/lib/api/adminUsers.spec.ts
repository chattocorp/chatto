import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createAdminUserManagementAPI } from './adminUsers';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  listMembers: vi.fn(),
  getMember: vi.fn(),
  assignRole: vi.fn(),
  revokeRole: vi.fn(),
  updateUser: vi.fn(),
  clearUsernameCooldown: vi.fn()
}));

vi.mock('@connectrpc/connect', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@connectrpc/connect')>();
  return {
    ...actual,
    createClient: mocks.createClient
  };
});

vi.mock('@connectrpc/connect-web', () => ({
  createConnectTransport: mocks.createConnectTransport
}));

describe('createAdminUserManagementAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.listMembers.mockReset();
    mocks.getMember.mockReset();
    mocks.assignRole.mockReset();
    mocks.revokeRole.mockReset();
    mocks.updateUser.mockReset();
    mocks.clearUsernameCooldown.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      listMembers: mocks.listMembers,
      getMember: mocks.getMember,
      assignRole: mocks.assignRole,
      revokeRole: mocks.revokeRole,
      updateUser: mocks.updateUser,
      clearUsernameCooldown: mocks.clearUsernameCooldown
    });
  });

  it('lists admin members and maps timestamps and roles', async () => {
    const createdAt = new Date('2026-01-02T03:04:05.000Z');
    mocks.listMembers.mockResolvedValue({
      users: [
        {
          id: 'user-1',
          login: 'alice',
          displayName: 'Alice',
          avatarUrl: undefined,
          roles: ['admin'],
          createdAt: { toDate: () => createdAt },
          deleted: false,
          hasVerifiedEmail: true,
          verifiedEmails: ['alice@example.test'],
          viewerCanDeleteAccount: true,
          lastLoginChange: undefined
        }
      ],
      roles: [{ name: 'admin', displayName: 'Admin' }],
      totalCount: 1,
      hasMore: false
    });
    const api = createAdminUserManagementAPI({
      baseUrl: '/api/connect',
      bearerToken: 'token'
    });

    const result = await api.listMembers({ search: 'alice', limit: 20, offset: 0 });

    expect(mocks.listMembers).toHaveBeenCalledWith(
      {
        search: 'alice',
        limit: 20,
        offset: 0
      },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(result).toEqual({
      users: [
        {
          id: 'user-1',
          login: 'alice',
          displayName: 'Alice',
          avatarUrl: null,
          roles: ['admin'],
          createdAt: '2026-01-02T03:04:05.000Z',
          deleted: false,
          hasVerifiedEmail: true,
          verifiedEmails: ['alice@example.test'],
          viewerCanDeleteAccount: true,
          lastLoginChange: null
        }
      ],
      roles: [{ name: 'admin', displayName: 'Admin' }],
      totalCount: 1,
      hasMore: false
    });
  });

  it('gets admin member details and maps permission metadata', async () => {
    const lastLoginChange = new Date('2026-02-03T04:05:06.000Z');
    mocks.getMember.mockResolvedValue({
      member: {
        id: 'user-2',
        login: 'bob',
        displayName: 'Bob',
        avatarUrl: '/assets/bob.png',
        roles: ['moderator'],
        createdAt: undefined,
        deleted: false,
        hasVerifiedEmail: false,
        verifiedEmails: [],
        viewerCanDeleteAccount: false,
        lastLoginChange: { toDate: () => lastLoginChange }
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
      viewerCanManageUserPermissions: true
    });
    const api = createAdminUserManagementAPI({ baseUrl: '/api/connect', bearerToken: null });

    const result = await api.getMember('user-2');

    expect(mocks.getMember).toHaveBeenCalledWith({ userId: 'user-2' }, { headers: undefined });
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
      viewerCanManageUserPermissions: true
    });
  });

  it('assigns and revokes roles with auth headers', async () => {
    mocks.assignRole.mockResolvedValue({ assigned: true });
    mocks.revokeRole.mockResolvedValue({ revoked: true });
    const api = createAdminUserManagementAPI({
      baseUrl: '/api/connect',
      bearerToken: 'token'
    });

    await expect(api.assignRole('user-1', 'moderator')).resolves.toBe(true);
    await expect(api.revokeRole('user-1', 'moderator')).resolves.toBe(true);

    expect(mocks.assignRole).toHaveBeenCalledWith(
      { userId: 'user-1', roleName: 'moderator' },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(mocks.revokeRole).toHaveBeenCalledWith(
      { userId: 'user-1', roleName: 'moderator' },
      { headers: { Authorization: 'Bearer token' } }
    );
  });

  it('updates a user with auth headers and maps the returned profile', async () => {
    mocks.updateUser.mockResolvedValue({
      user: {
        id: 'user-1',
        login: 'renamed',
        displayName: 'Renamed User',
        avatarUrl: '/assets/avatar.png'
      }
    });
    const api = createAdminUserManagementAPI({
      baseUrl: 'https://chat.example.test/api/connect',
      bearerToken: 'token'
    });

    const user = await api.updateUser({
      userId: 'user-1',
      login: 'renamed',
      displayName: 'Renamed User'
    });

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://chat.example.test/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.updateUser).toHaveBeenCalledWith(
      {
        userId: 'user-1',
        login: 'renamed',
        displayName: 'Renamed User'
      },
      { headers: { Authorization: 'Bearer token' } }
    );
    expect(user).toEqual({
      id: 'user-1',
      login: 'renamed',
      displayName: 'Renamed User',
      avatarUrl: '/assets/avatar.png'
    });
  });

  it('clears username cooldown without auth headers when no token is available', async () => {
    mocks.clearUsernameCooldown.mockResolvedValue({ cleared: true });
    const api = createAdminUserManagementAPI({ baseUrl: '/api/connect', bearerToken: null });

    await expect(api.clearUsernameCooldown('user-1')).resolves.toBe(true);

    expect(mocks.clearUsernameCooldown).toHaveBeenCalledWith(
      { userId: 'user-1' },
      { headers: undefined }
    );
  });
});
