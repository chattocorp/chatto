import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { adminQueryKeys } from '$lib/query/admin';
import { queryClient } from '$lib/query/client';

const mocks = vi.hoisted(() => ({
  listAdminRoles: vi.fn(),
  createRole: vi.fn(),
  goto: vi.fn()
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
      queryScope: 'role-create-test',
      getAPI: () => ({
        listAdminRoles: mocks.listAdminRoles,
        createRole: mocks.createRole
      })
    },
    isCurrent: () => true
  })
}));
vi.mock('$lib/api-client/roles', () => ({ createRoleAPI: vi.fn() }));
vi.mock('$lib/components/admin', async () => ({
  Panel: (await import('../[name]/RolePageSnippetMock.svelte')).default
}));
vi.mock('$lib/ui', async () => ({
  PaneContent: (await import('../[name]/RolePageSnippetMock.svelte')).default
}));
vi.mock('$lib/components/rbac', async () => ({
  RoleForm: (await import('./RoleCreateFormMock.svelte')).default
}));
vi.mock('$lib/ui/PaneHeader.svelte', async () => ({
  default: (await import('../[name]/RolePageSnippetMock.svelte')).default
}));
vi.mock('$lib/ui/PageTitle.svelte', async () => ({
  default: (await import('../[name]/RolePageSnippetMock.svelte')).default
}));

import RoleCreatePage from './+page.svelte';

describe('role creation query invalidation', () => {
  beforeEach(() => {
    queryClient.clear();
    vi.clearAllMocks();
    mocks.listAdminRoles.mockResolvedValue({ roles: [], viewerCanManageRoles: true });
    mocks.createRole.mockResolvedValue({ name: 'moderator' });
  });

  it('invalidates the cached permission tier before navigating to the new role', async () => {
    const connection = { queryScope: 'role-create-test' };
    const tierKey = adminQueryKeys.permissionTiers('origin', connection);
    queryClient.setQueryData(tierKey, { roles: [] });
    const { container } = render(RoleCreatePage);
    await vi.waitFor(() =>
      expect(container.querySelector('[data-testid="create-role"]')).not.toBeNull()
    );

    (container.querySelector('[data-testid="create-role"]') as HTMLButtonElement).click();

    await vi.waitFor(() => expect(mocks.createRole).toHaveBeenCalledOnce());
    expect(queryClient.getQueryState(tierKey)?.isInvalidated).toBe(true);
    expect(mocks.goto).toHaveBeenCalledWith('/chat/origin/manage/server/permissions/moderator');
  });

  it('reuses the cached role catalog capability snapshot', async () => {
    const connection = { queryScope: 'role-create-test' };
    queryClient.setQueryData(adminQueryKeys.roleCatalog('origin', connection), {
      roles: [],
      viewerCanManageRoles: true,
      viewerCanAssignRoles: false
    });

    const { container } = render(RoleCreatePage);
    await vi.waitFor(() =>
      expect(container.querySelector('[data-testid="create-role"]')).not.toBeNull()
    );

    expect(mocks.listAdminRoles).not.toHaveBeenCalled();
  });
});
