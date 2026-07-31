import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import type { RoleDetails, ServerRole } from '$lib/api-client/roles';

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
  Panel: (await import('./RolePageSnippetMock.svelte')).default,
  UserList: (await import('./RolePageUserListMock.svelte')).default
}));
vi.mock('$lib/ui', async () => ({
  Hint: (await import('./RolePageSnippetMock.svelte')).default,
  PaneContent: (await import('./RolePageSnippetMock.svelte')).default
}));
vi.mock('$lib/components/rbac', async () => ({
  DeleteRoleModal: (await import('./RolePageSnippetMock.svelte')).default,
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
    await vi.waitFor(() => expect(mocks.getRole).toHaveBeenCalledWith('role-a'));

    activeRoleName = 'role-b';
    flushSync();
    await vi.waitFor(() => expect(mocks.getRole).toHaveBeenCalledWith('role-b'));

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
});
