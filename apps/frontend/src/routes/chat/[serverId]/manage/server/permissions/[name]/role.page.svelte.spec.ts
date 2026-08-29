import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import type { RoleDetails, ServerRole } from '$lib/api-client/roles';
import { adminQueryKeys } from '$lib/query/admin';
import { queryClient } from '$lib/query/client';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    getRole: vi.fn(),
    updateRole: vi.fn(),
    deleteRole: vi.fn(),
    goto: vi.fn()
  }
}));

vi.mock('$app/state', () => ({
  page: {
    params: {
      get name() {
        return activeRoleName;
      }
    }
  }
}));

vi.mock('$app/navigation', () => ({ goto: mocks.goto }));
vi.mock('$app/paths', () => ({
  resolve: (path: string, params?: Record<string, string>) =>
    Object.entries(params ?? {}).reduce(
      (resolved, [key, value]) => resolved.replace(`[${key}]`, value),
      path
    )
}));
vi.mock('$lib/navigation', () => ({ serverIdToSegment: (serverId: string) => serverId }));
vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'origin',
    store: {},
    connection: {
      queryScope: 'role-page-test',
      getAPI: () => ({
        getRole: mocks.getRole,
        updateRole: mocks.updateRole,
        deleteRole: mocks.deleteRole
      })
    },
    isCurrent: () => true
  })
}));
vi.mock('$lib/api-client/roles', () => ({ createRoleAPI: vi.fn() }));
vi.mock('$lib/components/admin', async () => ({
  UserList: (await import('./RolePageUserListMock.svelte')).default
}));
vi.mock('$lib/ui/Panel.svelte', async () => ({
  default: (await import('./RolePageSnippetMock.svelte')).default
}));
vi.mock('$lib/ui', async () => ({
  Hint: (await import('./RolePageSnippetMock.svelte')).default,
  PaneContent: (await import('./RolePageSnippetMock.svelte')).default
}));
vi.mock('$lib/components/rbac', async () => ({
  DeleteRoleModal: (await import('./RolePageDeleteMock.svelte')).default,
  RolePermissionsMatrix: (await import('./RolePagePermissionMatrixMock.svelte')).default
}));
vi.mock('$lib/ui/PaneHeader.svelte', async () => ({
  default: (await import('./RolePageSnippetMock.svelte')).default
}));
vi.mock('$lib/ui/PageTitle.svelte', async () => ({
  default: (await import('./RolePageSnippetMock.svelte')).default
}));
vi.mock('$lib/ui/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() }
}));

let activeRoleName = $state('role-a');

import RolePage from './+page.svelte';

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function role(name: string, displayName: string, description: string): ServerRole {
  return {
    name,
    displayName,
    description,
    permissions: [],
    permissionDenials: [],
    isSystem: false,
    position: 1,
    pingable: false
  };
}

function details(name: string, displayName: string, description: string): RoleDetails {
  return {
    roles: [],
    role: role(name, displayName, description),
    users: [{ id: `${name}-user`, login: `${name}-user`, displayName: `${displayName} User` }],
    viewerCanManageRoles: true,
    viewerCanAssignRoles: true
  };
}

async function settle(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

describe('role management page identity', () => {
  beforeEach(() => {
    queryClient.clear();
    vi.clearAllMocks();
    activeRoleName = 'role-a';
  });

  it('does not let a delayed role response overwrite a reused route', async () => {
    const roleA = deferred<RoleDetails>();
    const roleB = deferred<RoleDetails>();
    mocks.getRole.mockImplementation((name: string) =>
      name === 'role-a' ? roleA.promise : roleB.promise
    );

    const { container } = render(RolePage);
    await vi.waitFor(() =>
      expect(mocks.getRole).toHaveBeenCalledWith(
        'role-a',
        expect.objectContaining({ signal: expect.any(AbortSignal) })
      )
    );

    activeRoleName = 'role-b';
    flushSync();
    await vi.waitFor(() =>
      expect(mocks.getRole).toHaveBeenCalledWith(
        'role-b',
        expect.objectContaining({ signal: expect.any(AbortSignal) })
      )
    );

    roleB.resolve(details('role-b', 'Role B', 'Role B description'));
    await settle();
    expect(container.querySelector('code')?.textContent).toBe('role-b');
    expect((container.querySelector('#displayName') as HTMLInputElement).value).toBe('Role B');
    expect((container.querySelector('#description') as HTMLTextAreaElement).value).toBe(
      'Role B description'
    );
    expect(container.querySelector('[data-testid="role-users"]')?.textContent).toContain(
      'Role B User'
    );

    roleA.resolve(details('role-a', 'Role A', 'Role A description'));
    await settle();
    expect(container.querySelector('code')?.textContent).toBe('role-b');
    expect((container.querySelector('#displayName') as HTMLInputElement).value).toBe('Role B');
    expect(container.querySelector('[data-testid="role-users"]')?.textContent).not.toContain(
      'Role A User'
    );
  });

  it('reuses a fresh cached role snapshot after remounting', async () => {
    const connection = { queryScope: 'role-page-test' };
    queryClient.setQueryData(
      adminQueryKeys.role('origin', connection, 'role-a'),
      details('role-a', 'Cached Role', 'Cached description')
    );

    const first = render(RolePage);
    await settle();
    expect(first.container.querySelector('code')?.textContent).toBe('role-a');
    expect((first.container.querySelector('#displayName') as HTMLInputElement).value).toBe(
      'Cached Role'
    );
    first.unmount();

    const second = render(RolePage);
    await settle();
    expect((second.container.querySelector('#description') as HTMLTextAreaElement).value).toBe(
      'Cached description'
    );
    expect(mocks.getRole).not.toHaveBeenCalled();
  });

  it('invalidates the cached permission tier after role metadata changes', async () => {
    const connection = { queryScope: 'role-page-test' };
    const tierKey = adminQueryKeys.permissionTiers('origin', connection);
    queryClient.setQueryData(tierKey, { roles: [] });
    mocks.getRole.mockResolvedValue(details('role-a', 'Role A', 'Original description'));
    mocks.updateRole.mockResolvedValue(role('role-a', 'Role A updated', 'Original description'));
    const { container } = render(RolePage);
    await vi.waitFor(() => expect(container.querySelector('#displayName')).not.toBeNull());

    const displayName = container.querySelector('#displayName') as HTMLInputElement;
    displayName.value = 'Role A updated';
    displayName.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    const form = container.querySelector('form')!;
    expect(form.querySelector('button[type="submit"]')).toHaveClass('btn-action');
    form.requestSubmit();

    await vi.waitFor(() => expect(mocks.updateRole).toHaveBeenCalledOnce());
    expect(queryClient.getQueryState(tierKey)?.isInvalidated).toBe(true);
    expect(
      queryClient.getQueryData<RoleDetails>(adminQueryKeys.role('origin', connection, 'role-a'))
        ?.role?.displayName
    ).toBe('Role A updated');
  });

  it('preserves dirty metadata drafts when pingable saves immediately', async () => {
    const pingSave = deferred<ServerRole>();
    mocks.getRole.mockResolvedValue(details('role-a', 'Role A', 'Original description'));
    mocks.updateRole.mockReturnValue(pingSave.promise);
    const { container } = render(RolePage);
    await vi.waitFor(() => expect(container.querySelector('#displayName')).not.toBeNull());

    const displayName = container.querySelector('#displayName') as HTMLInputElement;
    const description = container.querySelector('#description') as HTMLTextAreaElement;
    displayName.value = 'Unsaved display name';
    displayName.dispatchEvent(new Event('input', { bubbles: true }));
    description.value = 'Unsaved description';
    description.dispatchEvent(new Event('input', { bubbles: true }));
    const pingable = container.querySelector('#pingable') as HTMLInputElement;
    pingable.checked = true;
    pingable.dispatchEvent(new Event('change', { bubbles: true }));

    await vi.waitFor(() =>
      expect(mocks.updateRole).toHaveBeenCalledWith({
        name: 'role-a',
        displayName: 'Role A',
        description: 'Original description',
        pingable: true
      })
    );
    displayName.value = 'Newest display name';
    displayName.dispatchEvent(new Event('input', { bubbles: true }));
    description.value = 'Newest description';
    description.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    pingSave.resolve({
      ...role('role-a', 'Role A', 'Original description'),
      pingable: true
    });
    await settle();
    expect(displayName.value).toBe('Newest display name');
    expect(description.value).toBe('Newest description');
  });

  it('removes a deleted role query and invalidates its derived caches', async () => {
    const connection = { queryScope: 'role-page-test' };
    const tierKey = adminQueryKeys.permissionTiers('origin', connection);
    const roleKey = adminQueryKeys.rolePermissions('origin', connection, 'role-a');
    const roleDetailsKey = adminQueryKeys.role('origin', connection, 'role-a');
    const userKey = adminQueryKeys.userPermissions('origin', connection, 'user-a');
    queryClient.setQueryData(tierKey, { roles: [] });
    queryClient.setQueryData(roleKey, { roleName: 'role-a' });
    queryClient.setQueryData(roleDetailsKey, details('role-a', 'Role A', 'Description'));
    queryClient.setQueryData(userKey, { userId: 'user-a' });
    mocks.getRole.mockResolvedValue(details('role-a', 'Role A', 'Description'));
    mocks.deleteRole.mockResolvedValue(true);
    const { container } = render(RolePage);
    await vi.waitFor(() => expect(container.querySelector('#displayName')).not.toBeNull());

    const openDelete = [...container.querySelectorAll('button')].find(
      (button) => button.textContent?.trim() === 'Delete role'
    )!;
    openDelete.click();
    flushSync();
    (container.querySelector('[data-testid="confirm-role-delete"]') as HTMLButtonElement).click();

    await vi.waitFor(() => expect(mocks.deleteRole).toHaveBeenCalledWith('role-a'));
    expect(queryClient.getQueryData(roleKey)).toBeUndefined();
    expect(queryClient.getQueryData(roleDetailsKey)).toBeUndefined();
    expect(queryClient.getQueryState(tierKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(userKey)?.isInvalidated).toBe(true);
  });
});
