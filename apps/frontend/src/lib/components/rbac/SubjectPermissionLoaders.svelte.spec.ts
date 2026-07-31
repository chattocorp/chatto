import '../../../app.css';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import RolePermissionsMatrix from './RolePermissionsMatrix.svelte';
import UserPermissionsMatrix from './UserPermissionsMatrix.svelte';

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
    connection: { getAPI: () => permissionMocks },
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
  permissionMocks.getUserPermissionMatrix.mockImplementation((userId: string) =>
    Promise.resolve(matrix({ userId }))
  );
  permissionMocks.setRolePermission.mockResolvedValue({});
  permissionMocks.setUserPermission.mockResolvedValue({});
});

describe('subject permission loaders', () => {
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

    expect(permissionMocks.getRolePermissionMatrix).toHaveBeenCalledWith('role-b');
    expect(rendered.container.textContent).not.toContain('stale role failure');
    expect(cellButton(rendered.container, 'message.post').disabled).toBe(true);

    mutations[1].resolve({});
    await settle();
    expect(cellButton(rendered.container, 'message.post').disabled).toBe(false);
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

    expect(permissionMocks.getUserPermissionMatrix).toHaveBeenCalledWith('user-b');
    expect(rendered.container.textContent).not.toContain('stale user failure');
    expect(cellButton(rendered.container, 'message.post').disabled).toBe(true);

    mutations[1].resolve({});
    await settle();
    expect(cellButton(rendered.container, 'message.post').disabled).toBe(false);
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
    await settle();
    expect(cellButton(rendered.container, 'message.post').disabled).toBe(false);
    expect(cellButton(rendered.container, 'room.manage').disabled).toBe(false);
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
    await settle();
    expect(cellButton(rendered.container, 'message.post').disabled).toBe(false);
    expect(cellButton(rendered.container, 'room.manage').disabled).toBe(false);
  });
});
