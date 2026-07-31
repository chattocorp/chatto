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
    applicablePermissions: ['message.post'],
    scopes: [{ id: 'server', label: 'Server', kind: 'SERVER', parentGroupId: '' }],
    cells: [
      {
        permission: 'message.post',
        scopeId: 'server',
        override: 'NONE',
        effective: 'NONE'
      }
    ]
  };
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
  it('does not publish a role mutation failure after route reuse', async () => {
    let rejectMutation: ((error: Error) => void) | undefined;
    permissionMocks.setRolePermission.mockImplementation(
      () =>
        new Promise((_resolve, reject) => {
          rejectMutation = reject;
        })
    );
    const rendered = render(RolePermissionsMatrix, { props: { roleName: 'role-a' } });
    await settle();

    (
      rendered.container.querySelector('button[aria-label*="message.post"]') as HTMLButtonElement
    ).click();
    await rendered.rerender({ roleName: 'role-b' });
    await settle();
    rejectMutation?.(new Error('stale role failure'));
    await settle();

    expect(permissionMocks.getRolePermissionMatrix).toHaveBeenCalledWith('role-b');
    expect(rendered.container.textContent).not.toContain('stale role failure');
  });

  it('does not publish a user mutation failure after route reuse', async () => {
    let rejectMutation: ((error: Error) => void) | undefined;
    permissionMocks.setUserPermission.mockImplementation(
      () =>
        new Promise((_resolve, reject) => {
          rejectMutation = reject;
        })
    );
    const rendered = render(UserPermissionsMatrix, { props: { userId: 'user-a' } });
    await settle();

    (
      rendered.container.querySelector('button[aria-label*="message.post"]') as HTMLButtonElement
    ).click();
    await rendered.rerender({ userId: 'user-b' });
    await settle();
    rejectMutation?.(new Error('stale user failure'));
    await settle();

    expect(permissionMocks.getUserPermissionMatrix).toHaveBeenCalledWith('user-b');
    expect(rendered.container.textContent).not.toContain('stale user failure');
  });
});
