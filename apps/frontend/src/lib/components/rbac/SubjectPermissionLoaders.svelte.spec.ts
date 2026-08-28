import '../../../app.css';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import RolePermissionsMatrix from './RolePermissionsMatrix.svelte';
import UserPermissionsMatrix from './UserPermissionsMatrix.svelte';
import { queryClient } from '$lib/query/client';
import { adminQueryKeys } from '$lib/query/admin';
import {
  refreshRegisteredAdminQueries,
  removeRegisteredAdminUserQueries
} from '$lib/query/cacheRegistry';

const permissionMocks = vi.hoisted(() => ({
  getRolePermissionMatrix: vi.fn(),
  getUserPermissionMatrix: vi.fn(),
  setRolePermission: vi.fn(),
  setUserPermission: vi.fn()
}));

vi.mock('$lib/api-client/permissions', () => ({
  createPermissionAPI: () => permissionMocks
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'origin',
    store: {},
    connection: { queryScope: 'permission-loader-test', getAPI: () => permissionMocks },
    isCurrent: () => true
  })
}));

function matrix(subject: { roleName: string } | { userId: string }) {
  return {
    ...subject,
    applicablePermissions: ['message.post', 'room.manage'],
    scopes: [{ id: 'server', label: 'Server', kind: 'SERVER', parentGroupId: '' }],
    cells: [
      {
        permission: 'message.post',
        scopeId: 'server',
        override: 'NONE',
        effective: 'NONE'
      },
      {
        permission: 'room.manage',
        scopeId: 'server',
        override: 'NONE',
        effective: 'NONE'
      }
    ]
  };
}

function cellButton(container: HTMLElement, permission: string): HTMLButtonElement {
  return container.querySelector(`td[data-permission="${permission}"] button`)!;
}

function scopedCellButton(
  container: HTMLElement,
  scopeId: string,
  permission: string
): HTMLButtonElement {
  return container.querySelector(
    `td[data-scope="${scopeId}"][data-permission="${permission}"] button`
  )!;
}

async function settle(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

beforeEach(() => {
  vi.clearAllMocks();
  permissionMocks.getRolePermissionMatrix.mockImplementation((roleName: string) =>
    Promise.resolve(matrix({ roleName }))
  );
  permissionMocks.getUserPermissionMatrix.mockImplementation((userId: string) => {
    if (userId === 'bot-a') {
      return Promise.resolve({
        userId,
        applicablePermissions: ['message.post'],
        scopes: [{ id: 'server', label: 'Server', kind: 'SERVER', parentGroupId: '' }],
        cells: [
          {
            permission: 'message.post',
            scopeId: 'server',
            override: 'ALLOW',
            effective: 'DENY',
            allowPermitted: false
          }
        ]
      });
    }
    return Promise.resolve(matrix({ userId }));
  });
  permissionMocks.setRolePermission.mockResolvedValue({});
  permissionMocks.setUserPermission.mockResolvedValue({});
});

afterEach(() => queryClient.clear());

describe('subject permission loaders', () => {
  it('keeps the same matrix elements mounted while refreshed data replaces their cells', async () => {
    let resolveRefresh: ((value: ReturnType<typeof matrix>) => void) | undefined;
    const rendered = render(RolePermissionsMatrix, { props: { roleName: 'role-a' } });
    await settle();
    const originalTable = rendered.container.querySelector('table');
    const originalCell = rendered.container.querySelector(
      'td[data-scope="server"][data-permission="message.post"]'
    );
    permissionMocks.getRolePermissionMatrix.mockImplementationOnce(
      () => new Promise((resolve) => (resolveRefresh = resolve))
    );

    refreshRegisteredAdminQueries('origin');
    await settle();

    expect(rendered.container.querySelector('table')).toBe(originalTable);
    expect(
      rendered.container.querySelector('td[data-scope="server"][data-permission="message.post"]')
    ).toBe(originalCell);

    const refreshed = matrix({ roleName: 'role-a' });
    refreshed.cells[0].override = 'ALLOW';
    resolveRefresh?.(refreshed);
    await vi.waitFor(() => {
      expect(cellButton(rendered.container, 'message.post').getAttribute('aria-label')).toContain(
        'Override allow'
      );
    });
    expect(rendered.container.querySelector('table')).toBe(originalTable);
    expect(
      rendered.container.querySelector('td[data-scope="server"][data-permission="message.post"]')
    ).toBe(originalCell);
  });

  it('isolates pending role mutation state after route reuse', async () => {
    const mutations: Array<{
      resolve: (value: object) => void;
      reject: (error: Error) => void;
    }> = [];
    permissionMocks.setRolePermission.mockImplementation(
      () =>
        new Promise<object>((resolve, reject) => {
          mutations.push({ resolve, reject });
        })
    );
    const rendered = render(RolePermissionsMatrix, { props: { roleName: 'role-a' } });
    await settle();

    cellButton(rendered.container, 'message.post').click();
    await rendered.rerender({ roleName: 'role-b' });
    await settle();

    const replacementButton = cellButton(rendered.container, 'message.post');
    expect(replacementButton.disabled).toBe(false);
    replacementButton.click();
    await settle();
    expect(cellButton(rendered.container, 'message.post').disabled).toBe(true);

    mutations[0].reject(new Error('stale role failure'));
    await settle();

    expect(permissionMocks.getRolePermissionMatrix).toHaveBeenCalledWith(
      'role-b',
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
    expect(rendered.container.textContent).not.toContain('stale role failure');
    expect(cellButton(rendered.container, 'message.post').disabled).toBe(true);

    mutations[1].resolve({});
    await vi.waitFor(() =>
      expect(cellButton(rendered.container, 'message.post').disabled).toBe(false)
    );
  });

  it('isolates pending user mutation state after route reuse', async () => {
    const mutations: Array<{
      resolve: (value: object) => void;
      reject: (error: Error) => void;
    }> = [];
    permissionMocks.setUserPermission.mockImplementation(
      () =>
        new Promise<object>((resolve, reject) => {
          mutations.push({ resolve, reject });
        })
    );
    const rendered = render(UserPermissionsMatrix, { props: { userId: 'user-a' } });
    await settle();

    cellButton(rendered.container, 'message.post').click();
    await rendered.rerender({ userId: 'user-b' });
    await settle();

    const replacementButton = cellButton(rendered.container, 'message.post');
    expect(replacementButton.disabled).toBe(false);
    replacementButton.click();
    await settle();
    expect(cellButton(rendered.container, 'message.post').disabled).toBe(true);

    mutations[0].reject(new Error('stale user failure'));
    await settle();

    expect(permissionMocks.getUserPermissionMatrix).toHaveBeenCalledWith(
      'user-b',
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
    expect(rendered.container.textContent).not.toContain('stale user failure');
    expect(cellButton(rendered.container, 'message.post').disabled).toBe(true);

    mutations[1].resolve({});
    await vi.waitFor(() =>
      expect(cellButton(rendered.container, 'message.post').disabled).toBe(false)
    );
  });

  it('scrubs a mounted user matrix without refetching after realtime user removal', async () => {
    const rendered = render(UserPermissionsMatrix, { props: { userId: 'user-a' } });
    await settle();
    expect(rendered.container.querySelector('table')).not.toBeNull();

    removeRegisteredAdminUserQueries('origin', 'user-a');
    await settle();

    expect(rendered.container.querySelector('table')).toBeNull();
    expect(permissionMocks.getUserPermissionMatrix).toHaveBeenCalledOnce();
  });

  it('updates a user permission without remounting the matrix or changing page scroll', async () => {
    let resolveMutation: ((value: object) => void) | undefined;
    permissionMocks.setUserPermission.mockImplementation(
      () => new Promise<object>((resolve) => (resolveMutation = resolve))
    );
    const rendered = render(UserPermissionsMatrix, { props: { userId: 'user-a' } });
    await settle();
    rendered.container.style.height = '80px';
    rendered.container.style.overflowY = 'auto';
    rendered.container.scrollTop = 60;
    const originalScrollTop = rendered.container.scrollTop;
    const originalTable = rendered.container.querySelector('table');

    cellButton(rendered.container, 'message.post').click();
    await settle();
    resolveMutation?.({ decision: 'ALLOW' });

    await vi.waitFor(() =>
      expect(cellButton(rendered.container, 'message.post').disabled).toBe(false)
    );
    expect(rendered.container.querySelector('table')).toBe(originalTable);
    expect(rendered.container.scrollTop).toBe(originalScrollTop);
  });

  it('updates only the active cell in a binary user matrix', async () => {
    let resolveMutation: ((value: object) => void) | undefined;
    permissionMocks.setUserPermission.mockImplementation(
      () => new Promise<object>((resolve) => (resolveMutation = resolve))
    );
    const rendered = render(UserPermissionsMatrix, {
      props: { userId: 'user-a', decisionMode: 'binary' }
    });
    await settle();
    const originalTable = rendered.container.querySelector('table');
    const originalTarget = cellButton(rendered.container, 'message.post');
    const originalOther = cellButton(rendered.container, 'room.manage');
    const otherClassName = originalOther.className;

    originalTarget.click();
    await settle();

    expect(cellButton(rendered.container, 'message.post')).toBe(originalTarget);
    expect(originalTarget.disabled).toBe(false);
    expect(originalTarget.getAttribute('aria-disabled')).toBe('true');
    expect(cellButton(rendered.container, 'room.manage')).toBe(originalOther);
    expect(originalOther.disabled).toBe(false);
    expect(originalOther.className).toBe(otherClassName);
    originalOther.click();
    expect(permissionMocks.setUserPermission).toHaveBeenCalledOnce();

    resolveMutation?.({ decision: 'ALLOW' });
    await vi.waitFor(() => expect(originalTarget.getAttribute('aria-disabled')).toBeNull());

    expect(rendered.container.querySelector('table')).toBe(originalTable);
    expect(cellButton(rendered.container, 'room.manage')).toBe(originalOther);
    expect(permissionMocks.getUserPermissionMatrix).toHaveBeenCalledOnce();
    expect(
      queryClient.getQueryState(
        adminQueryKeys.userPermissions('origin', { queryScope: 'permission-loader-test' }, 'user-a')
      )?.isInvalidated
    ).toBe(true);
  });

  it('keeps a ceiling-blocked inherited room inert without writing a denial', async () => {
    permissionMocks.getUserPermissionMatrix.mockResolvedValue({
      userId: 'bot-inheritance',
      applicablePermissions: ['message.post'],
      scopes: [
        { id: 'server', label: 'Server', kind: 'SERVER', parentGroupId: '' },
        { id: 'group:general', label: 'General', kind: 'GROUP', parentGroupId: '' },
        { id: 'room:lobby', label: 'Lobby', kind: 'ROOM', parentGroupId: 'general' }
      ],
      cells: [
        {
          permission: 'message.post',
          scopeId: 'server',
          override: 'NONE',
          effective: 'NONE'
        },
        {
          permission: 'message.post',
          scopeId: 'group:general',
          override: 'ALLOW',
          effective: 'ALLOW'
        },
        {
          permission: 'message.post',
          scopeId: 'room:lobby',
          override: 'NONE',
          effective: 'ALLOW',
          allowPermitted: false
        }
      ]
    });
    const rendered = render(UserPermissionsMatrix, {
      props: { userId: 'bot-inheritance', decisionMode: 'binary', ownerCapped: true }
    });
    await settle();
    const table = rendered.container.querySelector('table');
    const group = scopedCellButton(rendered.container, 'group:general', 'message.post');
    const room = scopedCellButton(rendered.container, 'room:lobby', 'message.post');
    const groupClassName = group.className;

    expect(room.title).toContain('Enabled · Inherited from a broader scope');
    expect(room.querySelector('[class~="bg-warning/20"]')).not.toBeNull();
    expect(room.querySelector('[class~="icon-[uil--lock]"]')).not.toBeNull();
    expect(room.disabled).toBe(true);

    room.click();
    await settle();

    expect(permissionMocks.setUserPermission).not.toHaveBeenCalled();
    expect(rendered.container.querySelector('table')).toBe(table);
    expect(scopedCellButton(rendered.container, 'group:general', 'message.post')).toBe(group);
    expect(group.className).toBe(groupClassName);
    expect(room.querySelector('[class~="bg-warning/20"]')).not.toBeNull();
    expect(room.querySelector('[class~="icon-[uil--lock]"]')).not.toBeNull();
    expect(room.disabled).toBe(true);
    expect(rendered.container.querySelector('table')).toBe(table);
    expect(permissionMocks.getUserPermissionMatrix).toHaveBeenCalledOnce();
  });

  it('shows a bot narrow permission as enabled when message.read includes it', async () => {
    permissionMocks.getUserPermissionMatrix.mockResolvedValue({
      userId: 'bot-read',
      applicablePermissions: ['message.read', 'message.read-interactions'],
      scopes: [{ id: 'server', label: 'Server', kind: 'SERVER', parentGroupId: '' }],
      cells: [
        {
          permission: 'message.read',
          scopeId: 'server',
          override: 'ALLOW',
          effective: 'ALLOW',
          allowPermitted: true
        },
        {
          permission: 'message.read-interactions',
          scopeId: 'server',
          override: 'NONE',
          effective: 'ALLOW',
          allowPermitted: true
        }
      ]
    });
    const rendered = render(UserPermissionsMatrix, {
      props: { userId: 'bot-read', decisionMode: 'binary', ownerCapped: true }
    });
    await settle();

    const child = scopedCellButton(rendered.container, 'server', 'message.read-interactions');
    expect(child.title).toContain('Included by message.read');
    expect(child.querySelector('[class~="icon-[uil--lock]"]')).not.toBeNull();
    expect(child.disabled).toBe(true);
  });

  it('serializes role mutations within one resource', async () => {
    let resolveMutation: ((value: object) => void) | undefined;
    permissionMocks.setRolePermission.mockImplementation(
      () => new Promise<object>((resolve) => (resolveMutation = resolve))
    );
    const rendered = render(RolePermissionsMatrix, { props: { roleName: 'role-a' } });
    await settle();

    cellButton(rendered.container, 'message.post').click();
    await settle();
    expect(cellButton(rendered.container, 'room.manage').disabled).toBe(true);

    cellButton(rendered.container, 'room.manage').click();
    cellButton(rendered.container, 'message.post').click();
    expect(permissionMocks.setRolePermission).toHaveBeenCalledOnce();

    resolveMutation?.({});
    await vi.waitFor(() => {
      expect(cellButton(rendered.container, 'message.post').disabled).toBe(false);
      expect(cellButton(rendered.container, 'room.manage').disabled).toBe(false);
    });
  });

  it('invalidates cached user matrices after a role permission changes', async () => {
    const connection = { queryScope: 'permission-loader-test' };
    const userPermissionKey = adminQueryKeys.userPermissions('origin', connection, 'user-a');
    queryClient.setQueryData(userPermissionKey, matrix({ userId: 'user-a' }));
    const rendered = render(RolePermissionsMatrix, { props: { roleName: 'role-a' } });
    await settle();

    cellButton(rendered.container, 'message.post').click();

    await vi.waitFor(() =>
      expect(queryClient.getQueryState(userPermissionKey)?.isInvalidated).toBe(true)
    );
  });

  it('serializes user mutations within one resource', async () => {
    let resolveMutation: ((value: object) => void) | undefined;
    permissionMocks.setUserPermission.mockImplementation(
      () => new Promise<object>((resolve) => (resolveMutation = resolve))
    );
    const rendered = render(UserPermissionsMatrix, { props: { userId: 'user-a' } });
    await settle();

    cellButton(rendered.container, 'message.post').click();
    await settle();
    expect(cellButton(rendered.container, 'room.manage').disabled).toBe(true);

    cellButton(rendered.container, 'room.manage').click();
    cellButton(rendered.container, 'message.post').click();
    expect(permissionMocks.setUserPermission).toHaveBeenCalledOnce();

    resolveMutation?.({});
    await vi.waitFor(() => {
      expect(cellButton(rendered.container, 'message.post').disabled).toBe(false);
      expect(cellButton(rendered.container, 'room.manage').disabled).toBe(false);
    });
  });

  it('shows the owner ceiling and writes bot decisions through the user permission API', async () => {
    const rendered = render(UserPermissionsMatrix, {
      props: { userId: 'bot-a', subjectKind: 'bot', ownerCapped: true }
    });
    await settle();

    const button = cellButton(rendered.container, 'message.post');
    expect(button.querySelector('[class~="icon-[uil--exclamation-triangle]"]')).not.toBeNull();
    expect(button.querySelector('[class~="icon-[uil--lock]"]')).toBeNull();
    expect(rendered.container.textContent).toContain('your bot');

    button.click();
    await settle();
    expect(permissionMocks.setUserPermission).toHaveBeenCalledWith({
      userId: 'bot-a',
      permission: 'message.post',
      scope: { tier: 'server' },
      state: 'deny'
    });
  });
});
