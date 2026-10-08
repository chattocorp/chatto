import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import type { RoleDetails, RoleMemberPage, ServerRole } from '@chatto/client/api/roles';
import { adminQueryKeys } from '$lib/query/admin';
import { queryClient } from '$lib/query/client';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    getRole: vi.fn(),
    listMembers: vi.fn(),
    updateRole: vi.fn(),
    deleteRole: vi.fn(),
    goto: vi.fn(),
    toastSuccess: vi.fn()
  }
}));

vi.mock('$app/state', () => ({
  page: {
    params: {
      get name() {
        return activeRoleName;
      }
    },
    route: {
      get id() {
        return activeRouteId;
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
vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);
vi.mock('@chatto/client/api/roles', () => ({ createRoleAPI: vi.fn() }));
vi.mock('$lib/components/admin', async () => ({
  UserList: (await import('./RolePageUserListMock.svelte')).default
}));
vi.mock('$lib/ui/Panel.svelte', async () => ({
  default: (await import('./RolePageSnippetMock.svelte')).default
}));
vi.mock('$lib/ui', async () => ({
  PaneHeader: (await import('$lib/ui/PaneHeader.svelte')).default,
  PageTitle: (await import('$lib/ui/PageTitle.svelte')).default,
  LoadingFog: (await import('$lib/ui/LoadingFog.svelte')).default,
  Panel: (await import('$lib/ui/Panel.svelte')).default,
  Hint: (await import('./RolePageSnippetMock.svelte')).default,
  PaneContent: (await import('./RolePageSnippetMock.svelte')).default,
  TabNav: (await import('$lib/ui/TabNav.svelte')).default
}));
vi.mock('$lib/components/rbac', async () => ({
  DeleteRoleModal: (await import('./RolePageDeleteMock.svelte')).default,
  RolePermissionsMatrix: (await import('./RolePagePermissionMatrixMock.svelte')).default
}));
vi.mock('$lib/ui/PaneHeader.svelte', async () => ({
  default: (await import('./RolePageHeaderMock.svelte')).default
}));
vi.mock('$lib/ui/PageTitle.svelte', async () => ({
  default: (await import('./RolePageSnippetMock.svelte')).default
}));
vi.mock('$lib/ui/toast', () => ({
  toast: { success: mocks.toastSuccess, error: vi.fn() }
}));

const ROLE_ROUTE = '/chat/[serverId]/manage/server/roles/[name]';
let activeRoleName = $state('role-a');
let activeRouteId = $state(ROLE_ROUTE);
let server: TestServerScope;

import RoleDetailTestHarness from './RoleDetailTestHarness.svelte';

type Section = 'general' | 'permissions' | 'members';

/** Renders the layout with every section, like the former single role page. */
function renderRole(sections: Section[] = ['general', 'permissions', 'members']) {
  return render(RoleDetailTestHarness, { props: { sections } });
}

function tabs(container: HTMLElement) {
  return [...container.querySelectorAll('nav a')].map((link) => ({
    label: link.textContent?.trim(),
    href: link.getAttribute('href'),
    current: link.getAttribute('aria-current') === 'page'
  }));
}

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
    pingable: false
  };
}

function details(name: string, displayName: string, description: string): RoleDetails {
  return {
    roles: [],
    role: role(name, displayName, description),
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
    activeRouteId = ROLE_ROUTE;
    server = createTestServerScope({
      serverId: 'origin',
      api: {
        getRole: mocks.getRole,
        listMembers: mocks.listMembers,
        updateRole: mocks.updateRole,
        deleteRole: mocks.deleteRole
      }
    });
    mocks.listMembers.mockImplementation((name: string) =>
      Promise.resolve({
        users: [
          {
            id: `${name}-user`,
            login: `${name}-user`,
            displayName: `${name === 'role-a' ? 'Role A' : 'Role B'} User`,
            isBot: false
          }
        ],
        totalCount: 1,
        hasMore: false
      })
    );
  });

  it('loads member pages separately and fences a late page after a role switch', async () => {
    mocks.getRole.mockImplementation((name: string) => Promise.resolve(details(name, name, '')));
    const late = deferred<RoleMemberPage>();
    mocks.listMembers.mockImplementation((name: string, page: { offset: number }) => {
      if (name === 'role-a' && page.offset === 1) return late.promise;
      return Promise.resolve({
        users: [{ id: name, login: name, displayName: `${name} member`, isBot: false }],
        totalCount: name === 'role-a' ? 2 : 1,
        hasMore: name === 'role-a'
      });
    });
    const { container } = renderRole();
    await vi.waitFor(() => expect(container.textContent).toContain('role-a member'));
    (
      Array.from(container.querySelectorAll('button')).find(
        (button) => button.textContent === 'Load next test page'
      ) as HTMLButtonElement
    ).click();
    await vi.waitFor(() =>
      expect(mocks.listMembers).toHaveBeenCalledWith(
        'role-a',
        { limit: 20, offset: 1 },
        expect.objectContaining({ signal: expect.any(AbortSignal) })
      )
    );
    activeRoleName = 'role-b';
    flushSync();
    await vi.waitFor(() => expect(container.textContent).toContain('role-b member'));
    late.resolve({
      users: [{ id: 'late', login: 'late', displayName: 'Late A member', isBot: false }],
      totalCount: 2,
      hasMore: false
    });
    await settle();
    expect(container.textContent).not.toContain('Late A member');
    expect(container.textContent).not.toContain('role-a member');
  });

  it('shows a role at or above the viewer as read-only', async () => {
    server.setRoleOrder(['owner', 'role-a', 'role-b'], 'role-a');
    mocks.getRole.mockResolvedValue(details('role-a', 'Role A', 'Role A description'));
    const { container } = renderRole();
    await vi.waitFor(() => expect(container.querySelector('code')?.textContent).toBe('role-a'));

    expect(container.textContent).toContain('The role order does not let you change this role.');
    expect(container.querySelector('#displayName')).toBeNull();
    expect(container.textContent).toContain('Role A description');
    expect((container.querySelector('#pingable') as HTMLInputElement).disabled).toBe(true);
    expect(
      [...container.querySelectorAll('button')].some(
        (button) => button.textContent?.trim() === 'Delete role'
      )
    ).toBe(false);
    expect(
      container.querySelector('[data-testid="role-permissions"]')?.getAttribute('data-read-only')
    ).toBe('true');
  });

  it('lets the viewer edit a role below their highest role', async () => {
    mocks.getRole.mockResolvedValue(details('role-a', 'Role A', ''));
    const { container } = renderRole();
    await vi.waitFor(() => expect(container.querySelector('#displayName')).not.toBeNull());

    expect(container.textContent).not.toContain('The role order');
    expect(
      container.querySelector('[data-testid="role-permissions"]')?.getAttribute('data-read-only')
    ).toBe('false');
  });

  it('shows the General, Permissions, and Members tabs and marks the current one', async () => {
    mocks.getRole.mockResolvedValue(details('role-a', 'Role A', ''));
    activeRouteId = `${ROLE_ROUTE}/permissions`;
    const { container } = renderRole(['permissions']);
    await vi.waitFor(() => expect(tabs(container)).toHaveLength(3));

    expect(tabs(container)).toEqual([
      { label: 'General', href: '/chat/origin/manage/server/roles/role-a', current: false },
      {
        label: 'Permissions',
        href: '/chat/origin/manage/server/roles/role-a/permissions',
        current: true
      },
      {
        label: 'Members',
        href: '/chat/origin/manage/server/roles/role-a/members',
        current: false
      }
    ]);
    expect(
      container.querySelector('[data-testid="role-permissions"]')?.getAttribute('data-role-name')
    ).toBe('role-a');
  });

  it('explains the everyone role on its members address', async () => {
    activeRoleName = 'everyone';
    activeRouteId = `${ROLE_ROUTE}/members`;
    mocks.getRole.mockResolvedValue(details('everyone', 'Everyone', ''));
    const { container } = renderRole(['members']);
    await vi.waitFor(() =>
      expect(container.textContent).toContain(
        'All server members have the everyone role implicitly.'
      )
    );

    expect(container.textContent).not.toContain('You do not have permission');
    expect(mocks.listMembers).not.toHaveBeenCalled();
  });

  it('hides the Members tab for the everyone role and explains why', async () => {
    activeRoleName = 'everyone';
    mocks.getRole.mockResolvedValue(details('everyone', 'Everyone', ''));
    const { container } = renderRole(['general']);
    await vi.waitFor(() => expect(tabs(container)).toHaveLength(2));

    expect(tabs(container).map((tab) => tab.label)).toEqual(['General', 'Permissions']);
    expect(container.textContent).toContain(
      'All server members have the everyone role implicitly.'
    );
    expect(mocks.listMembers).not.toHaveBeenCalled();
  });

  it('hides the Members tab and denies the section without assignment authority', async () => {
    mocks.getRole.mockResolvedValue({
      ...details('role-a', 'Role A', ''),
      viewerCanAssignRoles: false
    });
    activeRouteId = `${ROLE_ROUTE}/members`;
    const { container } = renderRole(['members']);
    await vi.waitFor(() => expect(tabs(container)).toHaveLength(2));

    expect(tabs(container).map((tab) => tab.label)).toEqual(['General', 'Permissions']);
    expect(container.textContent).toContain('You do not have permission to access this page.');
    expect(mocks.listMembers).not.toHaveBeenCalled();
  });

  it('shows no tabs without role management', async () => {
    mocks.getRole.mockResolvedValue({
      ...details('role-a', 'Role A', ''),
      viewerCanManageRoles: false
    });
    const { container } = renderRole();
    await vi.waitFor(() =>
      expect(container.textContent).toContain('You need the role.manage permission')
    );

    expect(tabs(container)).toEqual([]);
    expect(container.querySelector('#displayName')).toBeNull();
  });

  it('does not request or render a roster without assignment authority', async () => {
    mocks.getRole.mockResolvedValue({
      ...details('role-a', 'Role A', ''),
      viewerCanAssignRoles: false
    });
    const { container } = renderRole();
    await vi.waitFor(() => expect(container.querySelector('#displayName')).not.toBeNull());
    expect(mocks.listMembers).not.toHaveBeenCalled();
    expect(container.querySelector('[data-testid="role-users"]')).toBeNull();
  });

  it('does not let a delayed role response overwrite a reused route', async () => {
    const roleA = deferred<RoleDetails>();
    const roleB = deferred<RoleDetails>();
    mocks.getRole.mockImplementation((name: string) =>
      name === 'role-a' ? roleA.promise : roleB.promise
    );

    const { container } = renderRole();
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
    const connection = server.scope.connection;
    queryClient.setQueryData(
      adminQueryKeys.role('origin', connection, 'role-a'),
      details('role-a', 'Cached Role', 'Cached description')
    );

    const first = renderRole();
    await settle();
    expect(first.container.querySelector('code')?.textContent).toBe('role-a');
    expect((first.container.querySelector('#displayName') as HTMLInputElement).value).toBe(
      'Cached Role'
    );
    first.unmount();

    const second = renderRole();
    await settle();
    expect((second.container.querySelector('#description') as HTMLTextAreaElement).value).toBe(
      'Cached description'
    );
    expect(mocks.getRole).not.toHaveBeenCalled();
  });

  it('invalidates the cached permission tier after role metadata changes', async () => {
    const connection = server.scope.connection;
    const tierKey = adminQueryKeys.permissionTiers('origin', connection);
    queryClient.setQueryData(tierKey, { roles: [] });
    mocks.getRole.mockResolvedValue(details('role-a', 'Role A', 'Original description'));
    mocks.updateRole.mockResolvedValue(role('role-a', 'Role A updated', 'Original description'));
    const { container } = renderRole();
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
    const { container } = renderRole();
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
    expect(mocks.toastSuccess).toHaveBeenCalledWith('Role pings turned on');
    expect(displayName.value).toBe('Newest display name');
    expect(description.value).toBe('Newest description');
  });

  it('confirms that role pings are off after the save', async () => {
    mocks.getRole.mockResolvedValue({
      ...details('role-a', 'Role A', ''),
      role: { ...role('role-a', 'Role A', ''), pingable: true }
    });
    mocks.updateRole.mockResolvedValue({ ...role('role-a', 'Role A', ''), pingable: false });
    const { container } = renderRole(['general']);
    await vi.waitFor(() => expect(container.querySelector('#pingable')).not.toBeNull());

    const pingable = container.querySelector('#pingable') as HTMLInputElement;
    pingable.checked = false;
    pingable.dispatchEvent(new Event('change', { bubbles: true }));

    await vi.waitFor(() =>
      expect(mocks.toastSuccess).toHaveBeenCalledWith('Role pings turned off')
    );
  });

  it('removes a deleted role query and invalidates its derived caches', async () => {
    const connection = server.scope.connection;
    const tierKey = adminQueryKeys.permissionTiers('origin', connection);
    const roleKey = adminQueryKeys.rolePermissions('origin', connection, 'role-a');
    const roleDetailsKey = adminQueryKeys.role('origin', connection, 'role-a');
    const userKey = adminQueryKeys.userPermissions('origin', connection, 'user-a');
    const catalogKey = adminQueryKeys.roleCatalog('origin', connection);
    queryClient.setQueryData(catalogKey, { roles: [] });
    queryClient.setQueryData(tierKey, { roles: [] });
    queryClient.setQueryData(roleKey, { roleName: 'role-a' });
    queryClient.setQueryData(roleDetailsKey, details('role-a', 'Role A', 'Description'));
    queryClient.setQueryData(userKey, { userId: 'user-a' });
    mocks.getRole.mockResolvedValue(details('role-a', 'Role A', 'Description'));
    mocks.deleteRole.mockResolvedValue(true);
    const { container } = renderRole();
    await vi.waitFor(() => expect(container.querySelector('#displayName')).not.toBeNull());

    const openDelete = [...container.querySelectorAll('button')].find(
      (button) => button.textContent?.trim() === 'Delete role'
    )!;
    openDelete.click();
    flushSync();
    (container.querySelector('[data-testid="confirm-role-delete"]') as HTMLButtonElement).click();

    await vi.waitFor(() => expect(mocks.deleteRole).toHaveBeenCalledWith('role-a'));
    await vi.waitFor(() =>
      expect(mocks.goto).toHaveBeenCalledWith('/chat/origin/manage/server/roles')
    );
    expect(queryClient.getQueryData(roleKey)).toBeUndefined();
    expect(queryClient.getQueryData(roleDetailsKey)).toBeUndefined();
    expect(queryClient.getQueryState(tierKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(userKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(catalogKey)?.isInvalidated).toBe(true);
  });
});
