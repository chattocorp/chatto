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

const api = { listAdminRoles: vi.fn(), moveRole: vi.fn() };
let server: TestServerScope;

function role(name: string): ServerRole {
  return {
    name,
    displayName: name.charAt(0).toUpperCase() + name.slice(1),
    description: '',
    permissions: [],
    isSystem: ['owner', 'admin', 'everyone'].includes(name),
    pingable: false
  };
}

function roles(...names: string[]): ServerRole[] {
  return names.map(role);
}

/** The server lists roles highest first. */
const ROLES = roles('owner', 'admin', 'moderator', 'helper', 'everyone');

/** A role that someone else creates while the viewer drags. */
const NEWCOMER = role('newcomer');

const ALL_ROLES = [...ROLES, NEWCOMER];

const SWAPPED = roles('owner', 'admin', 'helper', 'moderator', 'everyone');

function catalog(list: ServerRole[] = ROLES): RoleCatalog {
  return { roles: list, viewerCanManageRoles: true, viewerCanAssignRoles: true };
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
  movedId: string,
  source: 'pointer' | 'keyboard',
  trigger: string
) {
  const items = names.map((name) => ({ ...ALL_ROLES.find((r) => r.name === name)!, id: name }));
  container
    .querySelector('[data-testid="role-order-dropzone"]')!
    .dispatchEvent(
      new CustomEvent(type, { detail: { items, info: { id: movedId, source, trigger } } })
    );
}

/** Simulates the drop that svelte-dnd-action reports after a pointer drag of `movedId`. */
function drop(container: HTMLElement, names: string[], movedId: string) {
  dndEvent(container, 'finalize', names, movedId, 'pointer', 'droppedIntoZone');
}

/**
 * Simulates a keyboard drag of `movedId`. svelte-dnd-action reports each
 * arrow key move as `finalize` while the drag continues, and the drop as
 * `consider` with the `dragStopped` trigger.
 */
function keyboardDrag(container: HTMLElement, movedId: string, start: string[], moves: string[][]) {
  dndEvent(container, 'consider', start, movedId, 'keyboard', 'dragStarted');
  for (const order of moves) {
    dndEvent(container, 'finalize', order, movedId, 'keyboard', 'droppedIntoZone');
  }
  dndEvent(container, 'consider', moves.at(-1) ?? start, movedId, 'keyboard', 'dragStopped');
}

/** Replaces the cached catalogue, as a realtime refresh does. */
function setCachedOrder(list: ServerRole[]) {
  queryClient.setQueryData(
    adminQueryKeys.roleCatalog('server-1', server.scope.connection),
    catalog(list)
  );
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

const MOVABLE = ['admin', 'moderator', 'helper'];

describe('roles page', () => {
  beforeEach(async () => {
    queryClient.clear();
    vi.clearAllMocks();
    // The viewer is an admin: the role order puts moderator and helper below them.
    server = createTestServerScope({
      api,
      permissions: { canAdminManageRoles: true },
      roleCatalog: { roles: ['owner', 'admin', 'moderator', 'helper'], viewerHighestRole: 'admin' }
    });
    api.listAdminRoles.mockResolvedValue(catalog());
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('lists roles highest first and lets role managers move every role but owner and everyone', async () => {
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(renderedOrder(container)).toHaveLength(5));

    expect(renderedOrder(container)).toEqual(['owner', 'admin', 'moderator', 'helper', 'everyone']);
    expect(movableRoles(container)).toEqual(MOVABLE);
    expect(
      [...container.querySelectorAll<HTMLElement>('[data-locked]')].map((row) => row.dataset.role)
    ).toEqual(['owner', 'everyone']);
    // The viewer's own role and the roles above it move too (ADR-115).
    expect(container.querySelector('[aria-label="Move Admin"]')).not.toBeNull();
    expect(container.textContent).toContain('The role order decides who can manage whom.');
  });

  it('keeps the order of the server', async () => {
    api.listAdminRoles.mockResolvedValue(catalog(roles('owner', 'beta', 'alpha', 'everyone')));
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(renderedOrder(container)).toHaveLength(4));

    expect(renderedOrder(container)).toEqual(['owner', 'beta', 'alpha', 'everyone']);
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

  it('opens every role to edit except owner, which only owners edit', async () => {
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(renderedOrder(container)).toHaveLength(5));

    const link = (label: string) =>
      container.querySelector<HTMLAnchorElement>(`a[aria-label="${label}"]`);
    expect(link('View Owner')?.getAttribute('href')).toBe('/chat/-/manage/server/roles/owner');
    expect(link('Edit Admin')?.getAttribute('href')).toBe('/chat/-/manage/server/roles/admin');
    expect(link('Edit Moderator')?.getAttribute('href')).toBe(
      '/chat/-/manage/server/roles/moderator'
    );
    expect(link('Edit Helper')?.getAttribute('href')).toBe('/chat/-/manage/server/roles/helper');
    expect(link('Edit Everyone')?.getAttribute('href')).toBe(
      '/chat/-/manage/server/roles/everyone'
    );
    expect(link('Edit Owner')).toBeNull();
    // A real link, not a toggle: it works with a middle-click and in a new tab.
    expect(link('Edit Moderator')?.hasAttribute('aria-pressed')).toBe(false);
    expect(api.moveRole).not.toHaveBeenCalled();
  });

  it('opens every role to edit for an owner', async () => {
    server.setRoleOrder(['owner', 'admin', 'moderator', 'helper'], 'owner');
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(renderedOrder(container)).toHaveLength(5));

    expect(container.querySelector('a[aria-label="Edit Admin"]')).not.toBeNull();
    expect(container.querySelector('a[aria-label="Edit Owner"]')).not.toBeNull();
  });

  it('moves a role up with the role below it as the anchor', async () => {
    api.moveRole.mockImplementation(async () => {
      // The realtime refresh after the save reads the new order.
      api.listAdminRoles.mockResolvedValue(catalog(SWAPPED));
      return SWAPPED;
    });
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(MOVABLE));

    drop(container, ['admin', 'helper', 'moderator'], 'helper');

    await vi.waitFor(() => expect(mocks.toastSuccess).toHaveBeenCalledWith('Role order saved'));
    expect(api.moveRole).toHaveBeenCalledExactlyOnceWith('helper', 'moderator');
    expect(movableRoles(container)).toEqual(['admin', 'helper', 'moderator']);
    expect(
      queryClient
        .getQueryData<RoleCatalog>(adminQueryKeys.roleCatalog('server-1', server.scope.connection))
        ?.roles.map((r) => r.name)
    ).toEqual(SWAPPED.map((r) => r.name));
  });

  it('moves a role down with the role now below it as the anchor', async () => {
    api.moveRole.mockResolvedValue(ROLES);
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(MOVABLE));

    drop(container, ['moderator', 'admin', 'helper'], 'admin');

    await vi.waitFor(() => expect(api.moveRole).toHaveBeenCalledExactlyOnceWith('admin', 'helper'));
  });

  it('moves a role to the bottom without an anchor', async () => {
    api.moveRole.mockResolvedValue(ROLES);
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(MOVABLE));

    drop(container, ['moderator', 'helper', 'admin'], 'admin');

    await vi.waitFor(() =>
      expect(api.moveRole).toHaveBeenCalledExactlyOnceWith('admin', undefined)
    );
  });

  it('does not save a drop that keeps the order', async () => {
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(MOVABLE));

    drop(container, MOVABLE, 'moderator');

    expect(api.moveRole).not.toHaveBeenCalled();
    expect(movableRoles(container)).toEqual(MOVABLE);
  });

  it('restores the saved order and reports a failed save', async () => {
    api.moveRole.mockRejectedValue(new ConnectError('backend failure', Code.Internal));
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(MOVABLE));

    drop(container, ['admin', 'helper', 'moderator'], 'helper');

    await vi.waitFor(() =>
      expect(mocks.toastError).toHaveBeenCalledWith('Could not save the role order')
    );
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(MOVABLE));
    expect(api.moveRole).toHaveBeenCalledExactlyOnceWith('helper', 'moderator');
    expect(mocks.toastSuccess).not.toHaveBeenCalled();
  });

  it('saves a keyboard drag once when it stops, not on each arrow key', async () => {
    api.moveRole.mockImplementation(async () => {
      api.listAdminRoles.mockResolvedValue(catalog(SWAPPED));
      return SWAPPED;
    });
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(MOVABLE));

    const swapped = ['admin', 'helper', 'moderator'];
    dndEvent(container, 'consider', MOVABLE, 'helper', 'keyboard', 'dragStarted');
    for (const order of [swapped, MOVABLE, swapped]) {
      dndEvent(container, 'finalize', order, 'helper', 'keyboard', 'droppedIntoZone');
    }
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(swapped));
    expect(api.moveRole).not.toHaveBeenCalled();

    dndEvent(container, 'consider', swapped, 'helper', 'keyboard', 'dragStopped');

    await vi.waitFor(() => expect(mocks.toastSuccess).toHaveBeenCalledOnce());
    expect(api.moveRole).toHaveBeenCalledExactlyOnceWith('helper', 'moderator');
    // The draft is gone: the list follows the cached order again.
    setCachedOrder(ROLES);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(MOVABLE));
  });

  it('sends only the moved role and its anchor when the role list changes during a drag', async () => {
    api.moveRole.mockResolvedValue(SWAPPED);
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(MOVABLE));

    const swapped = ['admin', 'helper', 'moderator'];
    dndEvent(container, 'consider', MOVABLE, 'helper', 'keyboard', 'dragStarted');
    dndEvent(container, 'finalize', swapped, 'helper', 'keyboard', 'droppedIntoZone');
    // Someone else creates a role while the drag runs.
    setCachedOrder([...ROLES.slice(0, -1), NEWCOMER, ROLES.at(-1)!]);
    dndEvent(container, 'consider', swapped, 'helper', 'keyboard', 'dragStopped');

    await vi.waitFor(() =>
      expect(api.moveRole).toHaveBeenCalledExactlyOnceWith('helper', 'moderator')
    );
  });

  it('sends nothing after a keyboard pick-up and drop without a move', async () => {
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(MOVABLE));

    keyboardDrag(container, 'moderator', MOVABLE, []);

    expect(api.moveRole).not.toHaveBeenCalled();
    // The list stays live and shows a later order from the server.
    setCachedOrder(SWAPPED);
    await vi.waitFor(() =>
      expect(movableRoles(container)).toEqual(['admin', 'helper', 'moderator'])
    );
  });

  it('sends nothing when a keyboard drag returns the role to its place', async () => {
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(MOVABLE));

    keyboardDrag(container, 'moderator', MOVABLE, [['admin', 'helper', 'moderator'], MOVABLE]);

    expect(api.moveRole).not.toHaveBeenCalled();
    setCachedOrder(SWAPPED);
    await vi.waitFor(() =>
      expect(movableRoles(container)).toEqual(['admin', 'helper', 'moderator'])
    );
  });

  it('labels the list and its rows for screen readers and keeps them out of sidebar swipes', async () => {
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(MOVABLE));

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
    api.moveRole.mockReturnValue(save.promise);
    const { container } = render(RolesPage);
    await vi.waitFor(() => expect(movableRoles(container)).toEqual(MOVABLE));
    const handle = () => container.querySelector<HTMLElement>('[aria-label="Move Moderator"]')!;
    expect(handle().tabIndex).toBe(0);

    drop(container, ['admin', 'helper', 'moderator'], 'helper');

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
      expect(container.textContent).toContain('You need the role.manage permission')
    );
    expect(container.querySelector('[data-testid="role-order-dropzone"]')).toBeNull();
    expect(api.listAdminRoles).not.toHaveBeenCalled();
  });
});
