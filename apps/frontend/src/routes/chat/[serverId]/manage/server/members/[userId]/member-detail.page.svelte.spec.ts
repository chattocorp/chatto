import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import type {
  AdminManagedUser,
  AdminMember,
  AdminMemberDetails,
  AdminRoleMutationResult
} from '$lib/api-client/adminUsers';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import { adminQueryKeys } from '$lib/query/admin';
import { removeRegisteredAdminUserQueries } from '$lib/query/cacheRegistry';
import { queryClient } from '$lib/query/client';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';

const mocks = vi.hoisted(() => ({
  toastSuccess: vi.fn(),
  toastError: vi.fn()
}));

vi.mock('$app/state', () => ({
  page: {
    get params() {
      return { userId: routeUserId };
    }
  }
}));

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

// Page titles are tested separately from this page's partial route/server fixtures.
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

const api = {
  getMember: vi.fn(),
  updateUser: vi.fn(),
  clearUsernameCooldown: vi.fn(),
  changeUserPassword: vi.fn(),
  assignRole: vi.fn(),
  revokeRole: vi.fn(),
  uploadAvatar: vi.fn(),
  deleteAvatar: vi.fn()
};
let server: TestServerScope;
let routeUserId = $state('alice');

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
    primaryVerifiedEmail: null,
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
    routeUserId = 'alice';
    server = createTestServerScope({
      viewer: { id: 'viewer' },
      api,
      permissions: { canAdminViewUsers: true, canAdminManageAccounts: true }
    });
    api.getMember.mockImplementation((userId: string) => Promise.resolve(details(member(userId))));
    api.updateUser.mockImplementation(({ userId, login, displayName }) =>
      Promise.resolve({
        id: userId,
        login: login ?? userId,
        displayName: displayName ?? userId.toUpperCase(),
        avatarUrl: null
      } satisfies AdminManagedUser)
    );
    api.clearUsernameCooldown.mockResolvedValue(true);
    api.changeUserPassword.mockImplementation((userId: string) => Promise.resolve(member(userId)));
    api.assignRole.mockImplementation((userId: string) =>
      Promise.resolve({
        changed: true,
        member: member(userId, { roles: ['everyone', 'admin'] })
      } satisfies AdminRoleMutationResult)
    );
    api.revokeRole.mockResolvedValue({ changed: true, member: null });
    api.uploadAvatar.mockImplementation((userId: string) =>
      Promise.resolve({ id: userId, avatarUrl: '/avatar.webp' })
    );
    api.deleteAvatar.mockImplementation((userId: string) =>
      Promise.resolve({ id: userId, avatarUrl: null })
    );
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('marks a bot account in the member overview', async () => {
    api.getMember.mockResolvedValueOnce(details(member('helper_bot', { isBot: true })));

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

    routeUserId = 'bob';
    flushSync();
    await settle();
    expect(rendered.container.textContent).toContain('BOB');

    routeUserId = 'alice';
    flushSync();
    await settle();

    expect(api.getMember).toHaveBeenCalledTimes(2);
    expect(rendered.container.textContent).toContain('ALICE');
  });

  it('ignores an older member response after the route changes', async () => {
    const alice = deferred<AdminMemberDetails>();
    api.getMember.mockReturnValueOnce(alice.promise).mockResolvedValueOnce(details(member('bob')));
    const rendered = render(MemberDetailPage);
    await vi.waitFor(() => expect(api.getMember).toHaveBeenCalledOnce());

    routeUserId = 'bob';
    flushSync();
    await settle();
    alice.resolve(details(member('alice')));
    await settle();

    expect(rendered.container.textContent).toContain('BOB');
    expect(rendered.container.textContent).not.toContain('ALICE');
  });

  it('reloads the same user when the server session changes', async () => {
    api.getMember
      .mockResolvedValueOnce(details(member('shared', { displayName: 'Server One' })))
      .mockResolvedValueOnce(details(member('shared', { displayName: 'Server Two' })));
    routeUserId = 'shared';
    const rendered = render(MemberDetailPage);
    await settle();
    expect(rendered.container.textContent).toContain('Server One');

    server.queryScope = 'session-2';
    flushSync();
    await settle();

    expect(api.getMember).toHaveBeenCalledTimes(2);
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
    expect(api.getMember).toHaveBeenCalledOnce();
  });

  it('offers account deletion only to authorised viewers of other human members', async () => {
    // user.delete-any is independent from user.manage-accounts. The backend
    // expresses the former through viewerCanDeleteAccount.
    server.permissions.canAdminManageAccounts = false;
    const rendered = render(MemberDetailPage);
    await settle();
    expect(rendered.container.textContent).toContain('Danger Zone');
    expect(rendered.container.textContent).toContain('Delete account');
    expect(rendered.container.textContent).not.toContain('Identity Settings');

    api.getMember.mockResolvedValueOnce(details(member('helper_bot', { isBot: true })));
    routeUserId = 'helper_bot';
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

    expect(api.updateUser).toHaveBeenCalledWith({ userId: 'alice', login: 'renamed' });
    const cached = queryClient.getQueryData<AdminMemberDetails>(
      adminQueryKeys.member('server-1', server.scope.connection, 'alice')
    );
    expect(cached?.member?.login).toBe('renamed');
  });

  it('uploads the selected member avatar and updates the detail cache', async () => {
    const rendered = render(MemberDetailPage);
    await settle();
    const file = new File([new Uint8Array([137, 80, 78, 71])], 'member.png', {
      type: 'image/png'
    });
    const input = rendered.container.querySelector('input[type="file"]') as HTMLInputElement;
    Object.defineProperty(input, 'files', { configurable: true, value: [file] });
    input.dispatchEvent(new Event('change', { bubbles: true }));

    await vi.waitFor(() => expect(api.uploadAvatar).toHaveBeenCalledWith('alice', file));
    await settle();
    const cached = queryClient.getQueryData<AdminMemberDetails>(
      adminQueryKeys.member('server-1', server.scope.connection, 'alice')
    );
    expect(cached?.member?.avatarUrl).toBe('/avatar.webp');
  });

  it('sets a password and clears the username cooldown through mutations', async () => {
    api.getMember.mockResolvedValueOnce(
      details(member('alice', { lastLoginChange: new Date().toISOString() }))
    );
    const rendered = render(MemberDetailPage);
    await settle();

    buttonByText(rendered.container, 'Reset cooldown').click();
    await settle();
    expect(api.clearUsernameCooldown).toHaveBeenCalledWith('alice');

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

    expect(api.changeUserPassword).toHaveBeenCalledWith('alice', 'new-password');
  });

  it('updates roles and invalidates the related permission snapshots', async () => {
    const userPermissionsKey = adminQueryKeys.userPermissions(
      'server-1',
      server.scope.connection,
      'alice'
    );
    queryClient.setQueryData(userPermissionsKey, { marker: true });
    const rendered = render(MemberDetailPage);
    await settle();

    (rendered.container.querySelector('#role-assignment-admin') as HTMLInputElement).click();
    await settle();

    expect(api.assignRole).toHaveBeenCalledWith('alice', 'admin');
    expect(queryClient.getQueryState(userPermissionsKey)?.isInvalidated).toBe(true);
    expect(rendered.container.textContent).toContain('Admin');
  });

  it('allows a new member role change while the previous member mutation is pending', async () => {
    const aliceRole = deferred<AdminRoleMutationResult>();
    api.assignRole.mockReturnValueOnce(aliceRole.promise).mockResolvedValueOnce({
      changed: true,
      member: member('bob', { roles: ['everyone', 'admin'] })
    });
    const rendered = render(MemberDetailPage);
    await settle();

    (rendered.container.querySelector('#role-assignment-admin') as HTMLInputElement).click();
    await vi.waitFor(() => expect(api.assignRole).toHaveBeenCalledOnce());

    routeUserId = 'bob';
    flushSync();
    await vi.waitFor(() => expect(queryClient.isFetching()).toBe(0));
    flushSync();
    (rendered.container.querySelector('#role-assignment-admin') as HTMLInputElement).click();

    await vi.waitFor(() => expect(api.assignRole).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => {
      const bob = queryClient.getQueryData<AdminMemberDetails>(
        adminQueryKeys.member('server-1', server.scope.connection, 'bob')
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
    api.updateUser.mockReturnValueOnce(update.promise);
    const rendered = render(MemberDetailPage);
    await settle();
    setInput(rendered.container.querySelector('#member-login') as HTMLInputElement, 'renamed');
    buttonByText(rendered.container, 'Save').click();
    await vi.waitFor(() => expect(api.updateUser).toHaveBeenCalledOnce());

    routeUserId = 'bob';
    flushSync();
    await vi.waitFor(() => expect(queryClient.isFetching()).toBe(0));
    flushSync();
    expect(rendered.container.textContent).toContain('BOB');
    update.resolve({ id: 'alice', login: 'renamed', displayName: 'ALICE', avatarUrl: null });
    await settle();

    const bob = queryClient.getQueryData<AdminMemberDetails>(
      adminQueryKeys.member('server-1', server.scope.connection, 'bob')
    );
    expect(bob?.member?.login).toBe('bob');
    expect(rendered.container.textContent).toContain('BOB');
    expect(rendered.container.textContent).not.toContain('renamed');
  });
});
