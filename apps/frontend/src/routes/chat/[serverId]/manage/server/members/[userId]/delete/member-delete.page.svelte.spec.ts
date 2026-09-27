import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import type { AdminMember, AdminMemberDetails } from '$lib/api-client/adminUsers';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import { adminQueryKeys } from '$lib/query/admin';
import {
  removeRegisteredAdminQueries,
  removeRegisteredAdminUserQueries
} from '$lib/query/cacheRegistry';
import { queryClient } from '$lib/query/client';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';

const mocks = vi.hoisted(() => ({
  toastSuccess: vi.fn(),
  goto: vi.fn()
}));

// Page titles are tested separately from this page's partial route/server fixtures.
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

vi.mock('$app/state', () => ({
  page: {
    get params() {
      return { userId: routeUserId };
    }
  }
}));

vi.mock('$app/navigation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$app/navigation')>()),
  goto: mocks.goto
}));

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

const api = { getMember: vi.fn(), deleteUser: vi.fn() };
let server: TestServerScope;
let routeUserId = $state('alice');

vi.mock('$lib/ui/toast', () => ({
  toast: { success: mocks.toastSuccess, error: vi.fn() }
}));

import DeletePage from './+page.svelte';

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
    roles: [],
    availablePermissions: [],
    viewerCanAssignRoles: false,
    viewerCanManageRoles: false,
    viewerCanManageUserPermissions: false,
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
  await vi.waitFor(() => expect(queryClient.isFetching()).toBe(0));
  flushSync();
}

describe('server member delete page', () => {
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
    api.deleteUser.mockResolvedValue(true);
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  function renderPage() {
    return render(DeletePage);
  }

  it('stops the flow when a realtime removal of the member arrives', async () => {
    const rendered = renderPage();
    await settle();
    expect(rendered.container.textContent).toContain('Danger Zone');

    removeRegisteredAdminUserQueries('server-1', 'alice');
    flushSync();
    await settle();

    expect(rendered.container.textContent).toContain('Member not found');
    expect(rendered.container.textContent).not.toContain('Danger Zone');
    expect(api.deleteUser).not.toHaveBeenCalled();
  });

  it('discards a delete result when a session cache purge arrives mid-flight', async () => {
    const deletion = deferred<void>();
    api.deleteUser.mockReturnValueOnce(deletion.promise);
    const rendered = renderPage();
    await settle();

    const input = rendered.container.querySelector('#member-delete-confirm') as HTMLInputElement;
    input.value = 'alice';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    const submit = [...rendered.container.querySelectorAll('button')].find(
      (candidate) => candidate.getAttribute('type') === 'submit'
    ) as HTMLButtonElement;
    submit.click();
    await vi.waitFor(() => expect(api.deleteUser).toHaveBeenCalledOnce());

    // Simulates an authentication/visibility purge between request and response.
    removeRegisteredAdminQueries('server-1');
    flushSync();
    deletion.resolve();
    await settle();

    expect(mocks.toastSuccess).not.toHaveBeenCalled();
    expect(mocks.goto).not.toHaveBeenCalled();
  });

  it('discards a delete result after the route target changes', async () => {
    const deletion = deferred<void>();
    api.deleteUser.mockReturnValueOnce(deletion.promise);
    const rendered = renderPage();
    await settle();

    const input = rendered.container.querySelector('#member-delete-confirm') as HTMLInputElement;
    input.value = 'alice';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    const submit = [...rendered.container.querySelectorAll('button')].find(
      (candidate) => candidate.getAttribute('type') === 'submit'
    ) as HTMLButtonElement;
    submit.click();
    await vi.waitFor(() => expect(api.deleteUser).toHaveBeenCalledOnce());

    routeUserId = 'bob';
    flushSync();
    await vi.waitFor(() => expect(api.getMember).toHaveBeenCalledWith('bob', expect.anything()));
    await settle();

    const bobKey = adminQueryKeys.member('server-1', server.scope.connection, 'bob');
    expect(queryClient.getQueryData(bobKey)).toEqual(details(member('bob')));

    deletion.resolve();
    await settle();

    expect(api.deleteUser).toHaveBeenCalledWith({ userId: 'alice' });
    expect(mocks.toastSuccess).not.toHaveBeenCalled();
    expect(mocks.goto).not.toHaveBeenCalled();
    expect(queryClient.getQueryData(bobKey)).toEqual(details(member('bob')));
  });

  it('blocks deleting the viewer account through this page', async () => {
    routeUserId = 'viewer';
    const rendered = renderPage();
    await settle();

    expect(rendered.container.textContent).toContain('You cannot delete this account.');
    expect(api.deleteUser).not.toHaveBeenCalled();
  });

  it('blocks deletion when the viewer cannot delete the account', async () => {
    api.getMember.mockResolvedValueOnce(
      details(member('alice', { viewerCanDeleteAccount: false }))
    );
    const rendered = renderPage();
    await settle();

    expect(rendered.container.textContent).toContain('You cannot delete this account.');
    expect(rendered.container.querySelector('#member-delete-confirm')).toBeNull();
    expect(api.deleteUser).not.toHaveBeenCalled();
  });

  it('keeps the submit button disabled until the login matches', async () => {
    const rendered = renderPage();
    await settle();

    const form = rendered.container.querySelector('form') as HTMLFormElement;
    const input = rendered.container.querySelector('#member-delete-confirm') as HTMLInputElement;
    input.value = 'ali';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    const submit = [...form.querySelectorAll('button')].find(
      (candidate) => candidate.getAttribute('type') === 'submit'
    ) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);

    input.value = 'alice';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    expect(submit.disabled).toBe(false);
  });

  it('deletes the member and returns to the members list', async () => {
    const rendered = renderPage();
    await settle();

    const membersKey = adminQueryKeys.member('server-1', server.scope.connection, 'alice');
    queryClient.setQueryData(membersKey, details(member('alice')));

    const input = rendered.container.querySelector('#member-delete-confirm') as HTMLInputElement;
    input.value = 'alice';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    expect(rendered.container.querySelector('#member-delete-password')).toBeNull();

    const submit = [...rendered.container.querySelectorAll('button')].find(
      (candidate) => candidate.getAttribute('type') === 'submit'
    ) as HTMLButtonElement;
    submit.click();
    await settle();

    expect(api.deleteUser).toHaveBeenCalledWith({ userId: 'alice' });
    expect(mocks.toastSuccess).toHaveBeenCalledOnce();
    expect(queryClient.getQueryData(membersKey)).toBeUndefined();
    expect(mocks.goto).toHaveBeenCalledOnce();
  });

  it('shows a failure without navigating away', async () => {
    api.deleteUser.mockRejectedValueOnce(new Error('permission denied'));
    const rendered = renderPage();
    await settle();

    const input = rendered.container.querySelector('#member-delete-confirm') as HTMLInputElement;
    input.value = 'alice';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    const submit = [...rendered.container.querySelectorAll('button')].find(
      (candidate) => candidate.getAttribute('type') === 'submit'
    ) as HTMLButtonElement;
    submit.click();
    await vi.waitFor(() => expect(rendered.container.textContent).toContain('permission denied'));
    // The form stays for retry; no navigation happened.
    expect(rendered.container.textContent).toContain('Danger Zone');
    expect(mocks.goto).not.toHaveBeenCalled();
  });
});
