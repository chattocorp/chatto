import type {
  AdminMember,
  AdminMemberDetails,
  AdminUserManagementAPI
} from '$lib/api-client/adminUsers';
import { describe, expect, it, vi } from 'vitest';
import { MemberDetailStore } from './MemberDetailStore.svelte';

function member(id: string, roles = ['everyone']): AdminMember {
  return {
    id,
    login: id,
    displayName: id.toUpperCase(),
    avatarUrl: null,
    roles,
    createdAt: '2026-01-01T12:00:00Z',
    deleted: false,
    hasVerifiedEmail: false,
    verifiedEmails: [],
    viewerCanDeleteAccount: true,
    lastLoginChange: null
  };
}

function details(value: AdminMember): AdminMemberDetails {
  return {
    member: value,
    roles: [
      {
        name: 'everyone',
        displayName: 'Everyone',
        position: 0,
        permissions: [],
        permissionDenials: []
      },
      {
        name: 'admin',
        displayName: 'Admin',
        position: 1,
        permissions: [],
        permissionDenials: []
      }
    ],
    availablePermissions: [],
    viewerCanAssignRoles: true,
    viewerCanManageRoles: true,
    viewerCanManageUserPermissions: true,
    assignableRoleNames: null,
    revocableRoleNames: null
  };
}

function api(overrides: Partial<AdminUserManagementAPI> = {}): AdminUserManagementAPI {
  return {
    listMembers: vi.fn(),
    getMember: vi.fn().mockResolvedValue(details(member('default'))),
    assignRole: vi.fn(),
    revokeRole: vi.fn(),
    updateUser: vi.fn(),
    updateUserPassword: vi.fn(),
    clearUsernameCooldown: vi.fn(),
    deleteUser: vi.fn(),
    ...overrides
  } as AdminUserManagementAPI;
}

function deferred<T>(): {
  promise: Promise<T>;
  resolve: (value: T) => void;
} {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}

describe('MemberDetailStore', () => {
  it('ignores an older member response after the route changes', async () => {
    const aliceDetails = deferred<AdminMemberDetails>();
    const getMember = vi
      .fn()
      .mockReturnValueOnce(aliceDetails.promise)
      .mockResolvedValueOnce(details(member('bob')));
    const store = new MemberDetailStore(() => api({ getMember }));

    const staleLoad = store.setMember('server-1', 'alice');
    const currentLoad = store.setMember('server-1', 'bob');
    await currentLoad;

    aliceDetails.resolve(details(member('alice')));
    await staleLoad;

    expect(getMember).toHaveBeenNthCalledWith(1, 'alice');
    expect(getMember).toHaveBeenNthCalledWith(2, 'bob');
    expect(store.member).toEqual(member('bob'));
    expect(store.loading).toBe(false);
  });

  it('reloads and fences member details when the server changes with the same user ID', async () => {
    const firstServerDetails = deferred<AdminMemberDetails>();
    const getMember = vi
      .fn()
      .mockReturnValueOnce(firstServerDetails.promise)
      .mockResolvedValueOnce(details({ ...member('shared-user'), displayName: 'Server Two User' }));
    const store = new MemberDetailStore(() => api({ getMember }));

    const staleLoad = store.setMember('server-1', 'shared-user');
    await store.setMember('server-2', 'shared-user');
    firstServerDetails.resolve(
      details({ ...member('shared-user'), displayName: 'Server One User' })
    );
    await staleLoad;

    expect(getMember).toHaveBeenCalledTimes(2);
    expect(store.member?.displayName).toBe('Server Two User');
  });

  it('does not apply a completed role mutation to the next member', async () => {
    const assignment = deferred<{
      changed: boolean;
      member: AdminMember | null;
    }>();
    const getMember = vi
      .fn()
      .mockResolvedValueOnce(details(member('alice')))
      .mockResolvedValueOnce(details(member('bob')));
    const assignRole = vi.fn().mockReturnValue(assignment.promise);
    const store = new MemberDetailStore(() => api({ getMember, assignRole }));

    await store.setMember('server-1', 'alice');
    const staleAssignment = store.toggleRole('admin', false);
    await store.setMember('server-1', 'bob');
    assignment.resolve({
      changed: true,
      member: member('alice', ['everyone', 'admin'])
    });
    await staleAssignment;

    expect(assignRole).toHaveBeenCalledWith('alice', 'admin');
    expect(store.member).toEqual(member('bob'));
    expect(store.updatingRole).toBe(null);
  });

  it('keeps role-command success authoritative when the canonical reread fails', async () => {
    const getMember = vi
      .fn()
      .mockResolvedValueOnce(details(member('alice')))
      .mockRejectedValueOnce(new Error('projection temporarily unavailable'));
    const assignRole = vi.fn().mockResolvedValue({ changed: true, member: null });
    const store = new MemberDetailStore(() => api({ getMember, assignRole }));

    await store.setMember('server-1', 'alice');

    expect(await store.toggleRole('admin', false)).toBe(true);
    expect(store.error).toBe('projection temporarily unavailable');
    expect(store.updatingRole).toBe(null);
  });

  it('applies successful identity and password updates to the current member', async () => {
    const updateUser = vi.fn().mockResolvedValue({
      id: 'alice',
      login: 'alice-renamed',
      displayName: 'Alice Renamed',
      avatarUrl: null
    });
    const updatedMember = {
      ...member('alice'),
      login: 'alice-renamed',
      displayName: 'Alice Renamed'
    };
    const updateUserPassword = vi.fn().mockResolvedValue(updatedMember);
    const store = new MemberDetailStore(() =>
      api({
        getMember: vi.fn().mockResolvedValue(details(member('alice'))),
        updateUser,
        updateUserPassword
      })
    );

    await store.setMember('server-1', 'alice');
    await store.updateIdentity({
      login: 'alice-renamed',
      displayName: 'Alice Renamed'
    });
    await store.updatePassword('new-password');

    expect(updateUser).toHaveBeenCalledWith({
      userId: 'alice',
      login: 'alice-renamed',
      displayName: 'Alice Renamed'
    });
    expect(updateUserPassword).toHaveBeenCalledWith('alice', 'new-password');
    expect(store.member).toEqual(updatedMember);
  });
});
