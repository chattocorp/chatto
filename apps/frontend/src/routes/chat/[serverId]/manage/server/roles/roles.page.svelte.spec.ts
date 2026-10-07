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
  toastError: vi.fn(),
  goto: vi.fn()
}));

// Shared UI imports the other navigation functions too.
vi.mock('$app/navigation', () => ({
  goto: mocks.goto,
  pushState: vi.fn(),
  replaceState: vi.fn(),
  afterNavigate: vi.fn(),
  beforeNavigate: vi.fn(),
  invalidate: vi.fn(),
  invalidateAll: vi.fn()
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

import RolesPage from './+page.svelte';

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

function dndEvent(
  container: HTMLElement,
  type: 'consider' | 'finalize',
  names: string[],
  source: 'pointer' | 'keyboard',
  trigger: string
) {
  const items = names.map((name) => ({ ...ROLES.find((r) => r.name === name)!, id: name }));
  container
    .querySelector('[data-testid="role-order-dropzone"]')!
    .dispatchEvent(
      new CustomEvent(type, { detail: { items, info: { id: names[0], source, trigger } } })
    );
}

/** Simulates the drop that svelte-dnd-action reports after a pointer drag. */
function drop(container: HTMLElement, names: string[]) {
  dndEvent(container, 'finalize', names, 'pointer', 'droppedIntoZone');
}

/**
 * Simulates a keyboard drag. svelte-dnd-action reports each arrow key move as
 * `finalize` while the drag continues, and the drop as `consider` with the
 * `dragStopped` trigger.
 */
function keyboardDrag(container: HTMLElement, start: string[], moves: string[][]) {
  dndEvent(container, 'consider', start, 'keyboard', 'dragStarted');
  for (const order of moves) dndEvent(container, 'finalize', order, 'keyboard', 'droppedIntoZone');
  dndEvent(container, 'consider', moves.at(-1) ?? start, 'keyboard', 'dragStopped');
}

/** Replaces the cached catalogue, as a realtime refresh does. */
function setCachedOrder(roles: ServerRole[]) {
  queryClient.setQueryData(
    adminQueryKeys.roleCatalog('server-1', server.scope.connection),
    catalog(roles)
  );
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

const SWAPPED = [
  role('everyone', 0, true),
  role('moderator', 10, true),
  role('helper', 20, true),
  role('admin', 30, false),
  role('owner', 1000, false)
];

describe('roles page', () => {
  beforeEach(async () => {
    queryClient.clear();
    vi.clearAllMocks();
    server = createTestServerScope({ api, permissions: { canAdminManageRoles: true } });
    api.listAdminRoles.mockResolvedValue(catalog());
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('lists roles highest first and locks the roles that the viewer cannot move', async () => {
    const { container } = render(RolesPage);
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

  it('titles the page Roles and links to role creation', async () => {
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(renderedOrder(container)).toHaveLength(5));

    expect(container.querySelector('h1')?.textContent).toContain('Roles');
    const create = [...container.querySelectorAll('a')].find(
      (link) => link.textContent?.trim() === 'Create role'
    );
    expect(create?.getAttribute('href')).toBe('/chat/-/manage/server/roles/new');
  });

  it('opens every role: roles below the viewer to edit, the others read-only', async () => {
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(renderedOrder(container)).toHaveLength(5));

    const action = (label: string) =>
      container
        .querySelector<HTMLElement>(`[role="img"][aria-label="${label}"]`)
        ?.closest('button');
    expect(action('View Owner')).toBeTruthy();
    expect(action('View Admin')).toBeTruthy();
    expect(action('Edit Moderator')).toBeTruthy();
    expect(action('Edit Helper')).toBeTruthy();
    expect(action('Edit Everyone')).toBeTruthy();
    expect(action('Edit Admin')).toBeFalsy();

    action('Edit Moderator')!.click();
    expect(mocks.goto).toHaveBeenLastCalledWith('/chat/-/manage/server/roles/moderator');
    action('View Admin')!.click();
    expect(mocks.goto).toHaveBeenLastCalledWith('/chat/-/manage/server/roles/admin');
    expect(api.reorderRoles).not.toHaveBeenCalled();
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
    const { container } = render(RolesPage);
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
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(['moderator', 'helper']));

    drop(container, ['moderator', 'helper']);

    expect(api.reorderRoles).not.toHaveBeenCalled();
    expect(movableRoles(container)).toEqual(['moderator', 'helper']);
  });

  it('restores the saved order and reports a failed save', async () => {
    api.reorderRoles.mockRejectedValue(new ConnectError('backend failure', Code.Internal));
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(['moderator', 'helper']));

    drop(container, ['helper', 'moderator']);

    await vi.waitFor(() =>
      expect(mocks.toastError).toHaveBeenCalledWith('Could not save the role order')
    );
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(['moderator', 'helper']));
    expect(mocks.toastSuccess).not.toHaveBeenCalled();
  });

  it('saves a keyboard drag once when it stops, not on each arrow key', async () => {
    api.reorderRoles.mockImplementation(async () => {
      api.listAdminRoles.mockResolvedValue(catalog(SWAPPED));
      return SWAPPED;
    });
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(['moderator', 'helper']));

    dndEvent(container, 'consider', ['moderator', 'helper'], 'keyboard', 'dragStarted');
    dndEvent(container, 'finalize', ['helper', 'moderator'], 'keyboard', 'droppedIntoZone');
    dndEvent(container, 'finalize', ['moderator', 'helper'], 'keyboard', 'droppedIntoZone');
    dndEvent(container, 'finalize', ['helper', 'moderator'], 'keyboard', 'droppedIntoZone');
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(['helper', 'moderator']));
    expect(api.reorderRoles).not.toHaveBeenCalled();

    dndEvent(container, 'consider', ['helper', 'moderator'], 'keyboard', 'dragStopped');

    await vi.waitFor(() => expect(mocks.toastSuccess).toHaveBeenCalledOnce());
    expect(api.reorderRoles).toHaveBeenCalledExactlyOnceWith(['moderator', 'helper', 'admin']);
    // The draft is gone: the list follows the cached order again.
    setCachedOrder(ROLES);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(['moderator', 'helper']));
  });

  it('sends nothing after a keyboard pick-up and drop without a move', async () => {
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(['moderator', 'helper']));

    keyboardDrag(container, ['moderator', 'helper'], []);

    expect(api.reorderRoles).not.toHaveBeenCalled();
    // The list stays live and shows a later order from the server.
    setCachedOrder(SWAPPED);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(['helper', 'moderator']));
  });

  it('sends nothing when a keyboard drag returns the role to its place', async () => {
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(['moderator', 'helper']));

    keyboardDrag(
      container,
      ['moderator', 'helper'],
      [
        ['helper', 'moderator'],
        ['moderator', 'helper']
      ]
    );

    expect(api.reorderRoles).not.toHaveBeenCalled();
    setCachedOrder(SWAPPED);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(['helper', 'moderator']));
  });

  it('lets an owner move every role except owner and everyone', async () => {
    api.listAdminRoles.mockResolvedValue(
      catalog(ROLES.map((r) => ({ ...r, ranksBelowViewer: true })))
    );
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(renderedOrder(container)).toHaveLength(5));

    expect(movableRoles(container)).toEqual(['admin', 'moderator', 'helper']);
    expect(
      [...container.querySelectorAll<HTMLElement>('[data-locked]')].map((row) => row.dataset.role)
    ).toEqual(['owner', 'everyone']);
    expect(container.textContent).not.toContain('Locked');
  });

  it('explains when no role is below the viewer', async () => {
    api.listAdminRoles.mockResolvedValue(
      catalog(ROLES.map((r) => (r.name === 'everyone' ? r : { ...r, ranksBelowViewer: false })))
    );
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(renderedOrder(container)).toHaveLength(5));

    expect(movableRoles(container)).toEqual([]);
    expect(container.textContent).toContain('No roles are below your highest role.');
  });

  it('labels the list and its rows for screen readers and keeps them out of sidebar swipes', async () => {
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(['moderator', 'helper']));

    const zone = container.querySelector('[data-testid="role-order-dropzone"]')!;
    expect(zone.getAttribute('aria-label')).toBe('Roles that you can move');
    const row = zone.querySelector('[data-role="moderator"]')!;
    expect(row.getAttribute('aria-label')).toBe('Moderator');
    expect(row.hasAttribute('data-sidebar-swipe-ignore')).toBe(true);
    expect(
      row.querySelector('[aria-label="Move Moderator"]')?.hasAttribute('data-sidebar-swipe-ignore')
    ).toBe(true);
  });

  it('disables dragging while a save runs', async () => {
    const save = deferred<ServerRole[]>();
    api.reorderRoles.mockReturnValue(save.promise);
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(['moderator', 'helper']));
    const handle = () => container.querySelector<HTMLElement>('[aria-label="Move Moderator"]')!;
    expect(handle().tabIndex).toBe(0);

    drop(container, ['helper', 'moderator']);

    await vi.waitFor(() => expect(handle().tabIndex).toBe(-1));
    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull();
    api.listAdminRoles.mockResolvedValue(catalog(SWAPPED));
    save.resolve(SWAPPED);
    await vi.waitFor(() => expect(handle().tabIndex).toBe(0));
    expect(container.querySelector('[aria-busy="true"]')).toBeNull();
  });

  it('explains the missing permission without loading roles', async () => {
    server.permissions.canAdminManageRoles = false;
    const { container } = render(RolesPage);

    await vi.waitFor(() =>
      expect(container.textContent).toContain('You need the roles.manage permission')
    );
    expect(container.querySelector('[data-testid="role-order-dropzone"]')).toBeNull();
    expect(api.listAdminRoles).not.toHaveBeenCalled();
  });
});
