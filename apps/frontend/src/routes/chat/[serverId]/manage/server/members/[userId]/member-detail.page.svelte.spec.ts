import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import type {
  AdminManagedUser,
  AdminMember,
  AdminMemberDetails,
  AdminRoleMutationResult,
  AdminUserManagementAPI
} from '$lib/api-client/adminUsers';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import { adminQueryKeys } from '$lib/query/admin';
import { removeRegisteredAdminUserQueries } from '$lib/query/cacheRegistry';
import { queryClient } from '$lib/query/client';
import {
  memberDetailPageTestState,
  memberDetailTestPage
} from './MemberDetailPageTestState.svelte';

const mocks = vi.hoisted(() => ({
  getMember: vi.fn(),
  updateUser: vi.fn(),
  clearUsernameCooldown: vi.fn(),
  updateUserPassword: vi.fn(),
  assignRole: vi.fn(),
  revokeRole: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
  scopeCurrent: true,
  canAdminManageAccounts: true
}));

vi.mock('$app/state', () => ({ page: memberDetailTestPage }));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    get serverId() {
      return memberDetailPageTestState.serverId;
    },
    get connection() {
      return {
        queryScope: memberDetailPageTestState.sessionId,
        getAPI: () =>
          ({
            getMember: mocks.getMember,
            updateUser: mocks.updateUser,
            clearUsernameCooldown: mocks.clearUsernameCooldown,
            updateUserPassword: mocks.updateUserPassword,
            assignRole: mocks.assignRole,
            revokeRole: mocks.revokeRole
          }) as unknown as AdminUserManagementAPI
      };
    },
    get store() {
      return {
        currentUser: { user: { id: 'viewer', settings: null } },
        permissions: {
          canAdminViewUsers: true,
          canAdminManageAccounts: mocks.canAdminManageAccounts
        }
      };
    },
    isCurrent: () => mocks.scopeCurrent
  })
}));

vi.mock('$lib/components/rbac', async () => ({
  UserPermissionsMatrix: (await import('./MemberPermissionsMatrixMock.svelte')).default
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveBio: () => null,
  getLiveTimezone: () => null,
  getLiveLogin: (_userId: string, login: string) => login,
  getLiveAvatarUrl: (_userId: string, avatarUrl: string | null) => avatarUrl,
  getLiveCustomStatus: () => null
}));

vi.mock('$lib/state/presenceCache.svelte', () => ({
  getPresenceCache: () => ({ get: (_key: unknown, fallback: unknown) => fallback })
}));

vi.mock('$lib/ui/toast', () => ({
  toast: { success: mocks.toastSuccess, error: mocks.toastError }
}));

import MemberDetailPage from './+page.svelte';

function member(id: string, overrides: Partial<AdminMember> = {}): AdminMember {
  return {
    id,
    login: id,
    displayName: id.toUpperCase(),
    avatarUrl: null,
    roles: ['everyone'],
    createdAt: '2026-01-01T12:00:00Z',
    deleted: false,
    hasVerifiedEmail: false,
    verifiedEmails: [],
    viewerCanDeleteAccount: true,
    lastLoginChange: null,
    ...overrides
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

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

async function settle(): Promise<void> {
  await vi.waitFor(() => {
    expect(queryClient.isFetching()).toBe(0);
    expect(queryClient.isMutating()).toBe(0);
  });
  flushSync();
}

function setInput(input: HTMLInputElement, value: string): void {
  input.value = value;
  input.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
}

function buttonByText(root: ParentNode, text: string): HTMLButtonElement {
  const button = [...root.querySelectorAll('button')].find(
    (candidate) => candidate.textContent?.trim() === text
  );
  if (!(button instanceof HTMLButtonElement)) throw new Error(`Button not found: ${text}`);
  return button;
}

describe('server member detail queries', () => {
  beforeEach(async () => {
    queryClient.clear();
    vi.clearAllMocks();
    memberDetailPageTestState.reset();
    mocks.scopeCurrent = true;
    mocks.canAdminManageAccounts = true;
    mocks.getMember.mockImplementation((userId: string) =>
      Promise.resolve(details(member(userId)))
    );
    mocks.updateUser.mockImplementation(({ userId, login, displayName }) =>
      Promise.resolve({
        id: userId,
        login: login ?? userId,
        displayName: displayName ?? userId.toUpperCase(),
        avatarUrl: null
      } satisfies AdminManagedUser)
    );
    mocks.clearUsernameCooldown.mockResolvedValue(true);
    mocks.updateUserPassword.mockImplementation((userId: string) =>
      Promise.resolve(member(userId))
    );
    mocks.assignRole.mockImplementation((userId: string) =>
      Promise.resolve({
        changed: true,
        member: member(userId, { roles: ['everyone', 'admin'] })
      } satisfies AdminRoleMutationResult)
    );
    mocks.revokeRole.mockResolvedValue({ changed: true, member: null });
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('marks a bot account in the member overview', async () => {
    mocks.getMember.mockResolvedValueOnce(details(member('helper_bot', { isBot: true })));

    const rendered = render(MemberDetailPage);
    await settle();

    expect(rendered.container.querySelector('[data-testid="bot-badge"]')).toBeTruthy();
    expect(rendered.container.textContent).toContain('bot');
    expect(rendered.container.textContent).not.toContain('Email not verified');
    expect(rendered.container.textContent).not.toContain('Deletion allowed');
    expect(rendered.container.textContent).not.toContain('Set Password');
    expect(rendered.container.textContent).not.toContain('Role Assignments');
  });

  it('reuses cached member details when revisiting a user in the same session', async () => {
    const rendered = render(MemberDetailPage);
    await settle();
    expect(rendered.container.textContent).toContain('ALICE');

    memberDetailPageTestState.userId = 'bob';
    flushSync();
    await settle();
    expect(rendered.container.textContent).toContain('BOB');

    memberDetailPageTestState.userId = 'alice';
    flushSync();
    await settle();

    expect(mocks.getMember).toHaveBeenCalledTimes(2);
    expect(rendered.container.textContent).toContain('ALICE');
  });

  it('ignores an older member response after the route changes', async () => {
    const alice = deferred<AdminMemberDetails>();
    mocks.getMember
      .mockReturnValueOnce(alice.promise)
      .mockResolvedValueOnce(details(member('bob')));
    const rendered = render(MemberDetailPage);
    await vi.waitFor(() => expect(mocks.getMember).toHaveBeenCalledOnce());

    memberDetailPageTestState.userId = 'bob';
    flushSync();
    await settle();
    alice.resolve(details(member('alice')));
    await settle();

    expect(rendered.container.textContent).toContain('BOB');
    expect(rendered.container.textContent).not.toContain('ALICE');
  });

  it('reloads the same user when the server session changes', async () => {
    mocks.getMember
      .mockResolvedValueOnce(details(member('shared', { displayName: 'Server One' })))
      .mockResolvedValueOnce(details(member('shared', { displayName: 'Server Two' })));
    memberDetailPageTestState.userId = 'shared';
    const rendered = render(MemberDetailPage);
    await settle();
    expect(rendered.container.textContent).toContain('Server One');

    memberDetailPageTestState.sessionId = 'session-2';
    flushSync();
    await settle();

    expect(mocks.getMember).toHaveBeenCalledTimes(2);
    expect(rendered.container.textContent).toContain('Server Two');
  });

  it('keeps a realtime-removed member cleared without refetching', async () => {
    const rendered = render(MemberDetailPage);
    await settle();
    expect(rendered.container.textContent).toContain('ALICE');

    removeRegisteredAdminUserQueries('server-1', 'alice');
    flushSync();
    await settle();

    expect(rendered.container.textContent).toContain('Member not found');
    expect(rendered.container.textContent).not.toContain('ALICE');
    expect(mocks.getMember).toHaveBeenCalledOnce();
  });

  it('offers account deletion only to authorised viewers of other human members', async () => {
    // user.delete-any is independent from user.manage-accounts. The backend
    // expresses the former through viewerCanDeleteAccount.
    mocks.canAdminManageAccounts = false;
    const rendered = render(MemberDetailPage);
    await settle();
    expect(rendered.container.textContent).toContain('Danger Zone');
    expect(rendered.container.textContent).toContain('Delete account');
    expect(rendered.container.textContent).not.toContain('Identity Settings');

    mocks.getMember.mockResolvedValueOnce(details(member('helper_bot', { isBot: true })));
    memberDetailPageTestState.userId = 'helper_bot';
    flushSync();
    await settle();
    expect(rendered.container.textContent).not.toContain('Danger Zone');
  });

  it('updates identity and related cached member details', async () => {
    const rendered = render(MemberDetailPage);
    await settle();
    setInput(rendered.container.querySelector('#member-login') as HTMLInputElement, 'renamed');
    buttonByText(rendered.container, 'Save').click();
    await settle();

    expect(mocks.updateUser).toHaveBeenCalledWith({ userId: 'alice', login: 'renamed' });
    const cached = queryClient.getQueryData<AdminMemberDetails>(
      adminQueryKeys.member('server-1', { queryScope: 'session-1' }, 'alice')
    );
    expect(cached?.member?.login).toBe('renamed');
  });

  it('sets a password and clears the username cooldown through mutations', async () => {
    mocks.getMember.mockResolvedValueOnce(
      details(member('alice', { lastLoginChange: new Date().toISOString() }))
    );
    const rendered = render(MemberDetailPage);
    await settle();

    buttonByText(rendered.container, 'Reset cooldown').click();
    await settle();
    expect(mocks.clearUsernameCooldown).toHaveBeenCalledWith('alice');

    setInput(
      rendered.container.querySelector('#admin-member-password') as HTMLInputElement,
      'new-password'
    );
    setInput(
      rendered.container.querySelector('#admin-member-password-confirm') as HTMLInputElement,
      'new-password'
    );
    buttonByText(rendered.container, 'Set Password').click();
    await settle();

    expect(mocks.updateUserPassword).toHaveBeenCalledWith('alice', 'new-password');
  });

  it('updates roles and invalidates the related permission snapshots', async () => {
    const userPermissionsKey = adminQueryKeys.userPermissions(
      'server-1',
      { queryScope: 'session-1' },
      'alice'
    );
    queryClient.setQueryData(userPermissionsKey, { marker: true });
    const rendered = render(MemberDetailPage);
    await settle();

    (rendered.container.querySelector('#role-assignment-admin') as HTMLInputElement).click();
    await settle();

    expect(mocks.assignRole).toHaveBeenCalledWith('alice', 'admin');
    expect(queryClient.getQueryState(userPermissionsKey)?.isInvalidated).toBe(true);
    expect(rendered.container.textContent).toContain('Admin');
  });

  it('allows a new member role change while the previous member mutation is pending', async () => {
    const aliceRole = deferred<AdminRoleMutationResult>();
    mocks.assignRole.mockReturnValueOnce(aliceRole.promise).mockResolvedValueOnce({
      changed: true,
      member: member('bob', { roles: ['everyone', 'admin'] })
    });
    const rendered = render(MemberDetailPage);
    await settle();

    (rendered.container.querySelector('#role-assignment-admin') as HTMLInputElement).click();
    await vi.waitFor(() => expect(mocks.assignRole).toHaveBeenCalledOnce());

    memberDetailPageTestState.userId = 'bob';
    flushSync();
    await vi.waitFor(() => expect(queryClient.isFetching()).toBe(0));
    flushSync();
    (rendered.container.querySelector('#role-assignment-admin') as HTMLInputElement).click();

    await vi.waitFor(() => expect(mocks.assignRole).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => {
      const bob = queryClient.getQueryData<AdminMemberDetails>(
        adminQueryKeys.member('server-1', { queryScope: 'session-1' }, 'bob')
      );
      expect(bob?.member?.roles).toContain('admin');
    });

    aliceRole.resolve({
      changed: true,
      member: member('alice', { roles: ['everyone', 'admin'] })
    });
    await settle();
    expect(rendered.container.textContent).toContain('BOB');
  });

  it('does not apply a mutation result after navigating to another member', async () => {
    const update = deferred<AdminManagedUser>();
    mocks.updateUser.mockReturnValueOnce(update.promise);
    const rendered = render(MemberDetailPage);
    await settle();
    setInput(rendered.container.querySelector('#member-login') as HTMLInputElement, 'renamed');
    buttonByText(rendered.container, 'Save').click();
    await vi.waitFor(() => expect(mocks.updateUser).toHaveBeenCalledOnce());

    memberDetailPageTestState.userId = 'bob';
    flushSync();
    await vi.waitFor(() => expect(queryClient.isFetching()).toBe(0));
    flushSync();
    expect(rendered.container.textContent).toContain('BOB');
    update.resolve({ id: 'alice', login: 'renamed', displayName: 'ALICE', avatarUrl: null });
    await settle();

    const bob = queryClient.getQueryData<AdminMemberDetails>(
      adminQueryKeys.member('server-1', { queryScope: 'session-1' }, 'bob')
    );
    expect(bob?.member?.login).toBe('bob');
    expect(rendered.container.textContent).toContain('BOB');
    expect(rendered.container.textContent).not.toContain('renamed');
  });
});
