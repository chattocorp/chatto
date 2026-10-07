import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { Code, ConnectError } from '@connectrpc/connect';
import type { RoleCatalog, ServerRole } from '@chatto/client/api/roles';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import { adminQueryKeys } from '$lib/query/admin';
import { queryClient } from '$lib/query/client';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';

const mocks = vi.hoisted(() => ({
  toastSuccess: vi.fn(),
  toastError: vi.fn()
}));

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

// Page titles are tested separately from this page's partial route/server fixtures.
vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

vi.mock('$lib/ui/toast', () => ({
  toast: { success: mocks.toastSuccess, error: mocks.toastError }
}));

import RoleOrderPage from './+page.svelte';

const api = { listAdminRoles: vi.fn(), reorderRoles: vi.fn() };
let server: TestServerScope;

function role(name: string, position: number, ranksBelowViewer: boolean): ServerRole {
  return {
    name,
    displayName: name.charAt(0).toUpperCase() + name.slice(1),
    description: '',
    permissions: [],
    permissionDenials: [],
    isSystem: ['owner', 'admin', 'everyone'].includes(name),
    position,
    pingable: false,
    ranksBelowViewer
  };
}

const ROLES = [
  role('everyone', 0, true),
  role('helper', 10, true),
  role('moderator', 20, true),
  role('admin', 30, false),
  role('owner', 1000, false)
];

function catalog(roles: ServerRole[] = ROLES): RoleCatalog {
  return { roles, viewerCanManageRoles: true, viewerCanAssignRoles: true };
}

function renderedOrder(container: HTMLElement): string[] {
  return [...container.querySelectorAll<HTMLElement>('[data-role]')].map(
    (row) => row.dataset.role ?? ''
  );
}

function movableRoles(container: HTMLElement): string[] {
  return [
    ...container.querySelectorAll<HTMLElement>('[data-testid="role-order-dropzone"] [data-role]')
  ].map((row) => row.dataset.role ?? '');
}

/** Simulates the drop that svelte-dnd-action reports after a drag. */
function drop(container: HTMLElement, names: string[]) {
  const items = names.map((name) => ({ ...ROLES.find((r) => r.name === name)!, id: name }));
  container.querySelector('[data-testid="role-order-dropzone"]')!.dispatchEvent(
    new CustomEvent('finalize', {
      detail: { items, info: { id: names[0], source: 'pointer', trigger: 'droppedIntoZone' } }
    })
  );
}

describe('role order page', () => {
  beforeEach(async () => {
    queryClient.clear();
    vi.clearAllMocks();
    server = createTestServerScope({ api, permissions: { canAdminManageRoles: true } });
    api.listAdminRoles.mockResolvedValue(catalog());
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('lists roles highest first and locks the roles that the viewer cannot move', async () => {
    const { container } = render(RoleOrderPage);
    await vi.waitFor(() => expect(renderedOrder(container)).toHaveLength(5));

    expect(renderedOrder(container)).toEqual(['owner', 'admin', 'moderator', 'helper', 'everyone']);
    expect(movableRoles(container)).toEqual(['moderator', 'helper']);
    expect(
      [...container.querySelectorAll<HTMLElement>('[data-locked]')].map((row) => row.dataset.role)
    ).toEqual(['owner', 'admin', 'everyone']);
    expect(container.querySelector('[aria-label="Move Moderator"]')).not.toBeNull();
    expect(container.querySelector('[aria-label="Move Admin"]')).toBeNull();
    expect(container.textContent).toContain('The role order decides who can manage whom.');
  });

  it('saves the complete order, lowest first, after a drop', async () => {
    const reordered = [
      role('everyone', 0, true),
      role('moderator', 10, true),
      role('helper', 20, true),
      role('admin', 30, false),
      role('owner', 1000, false)
    ];
    api.reorderRoles.mockImplementation(async () => {
      // The realtime refresh after the save reads the new order.
      api.listAdminRoles.mockResolvedValue(catalog(reordered));
      return reordered;
    });
    const { container } = render(RoleOrderPage);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(['moderator', 'helper']));

    drop(container, ['helper', 'moderator']);

    await vi.waitFor(() =>
      expect(api.reorderRoles).toHaveBeenCalledWith(['moderator', 'helper', 'admin'])
    );
    await vi.waitFor(() => expect(mocks.toastSuccess).toHaveBeenCalledWith('Role order saved'));
    expect(movableRoles(container)).toEqual(['helper', 'moderator']);
    expect(
      queryClient
        .getQueryData<RoleCatalog>(adminQueryKeys.roleCatalog('server-1', server.scope.connection))
        ?.roles.map((r) => r.name)
    ).toEqual(reordered.map((r) => r.name));
  });

  it('does not save a drop that keeps the order', async () => {
    const { container } = render(RoleOrderPage);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(['moderator', 'helper']));

    drop(container, ['moderator', 'helper']);

    expect(api.reorderRoles).not.toHaveBeenCalled();
    expect(movableRoles(container)).toEqual(['moderator', 'helper']);
  });

  it('restores the saved order and reports a failed save', async () => {
    api.reorderRoles.mockRejectedValue(new ConnectError('backend failure', Code.Internal));
    const { container } = render(RoleOrderPage);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(['moderator', 'helper']));

    drop(container, ['helper', 'moderator']);

    await vi.waitFor(() =>
      expect(mocks.toastError).toHaveBeenCalledWith('Could not save the role order')
    );
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(['moderator', 'helper']));
    expect(mocks.toastSuccess).not.toHaveBeenCalled();
  });

  it('explains the missing permission without loading roles', async () => {
    server.permissions.canAdminManageRoles = false;
    const { container } = render(RoleOrderPage);

    await vi.waitFor(() =>
      expect(container.textContent).toContain('You need the roles.manage permission')
    );
    expect(container.querySelector('[data-testid="role-order-dropzone"]')).toBeNull();
    expect(api.listAdminRoles).not.toHaveBeenCalled();
  });
});
