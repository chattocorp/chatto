import { describe, expect, it, vi } from 'vitest';
import type { PermissionAPI, PermissionState } from '$lib/api-client/permissions';
import { setRolePermission } from './permissionMutations';
import { setUserPermission } from './userPermissionMutations';

function mockAPI() {
  return {
    setRolePermission: vi.fn().mockResolvedValue(true),
    setUserPermission: vi.fn().mockResolvedValue(true)
  } as unknown as PermissionAPI;
}

describe('permission mutation dispatch', () => {
  const roleCases = [
    [{ tier: 'server', roleName: 'admin' } as const, 'allow'],
    [{ tier: 'group', roleName: 'admin', groupId: 'G1' } as const, 'deny'],
    [{ tier: 'room', roleName: 'admin', roomId: 'R1' } as const, 'neutral']
  ] satisfies Array<[Parameters<typeof setRolePermission>[1], PermissionState]>;

  it.each(roleCases)('sets a role permission via Connect for %o %s', async (scope, state) => {
    const api = mockAPI();

    await setRolePermission(api, scope, 'message.post', state);

    expect(api.setRolePermission).toHaveBeenCalledWith({
      roleName: 'admin',
      scope,
      permission: 'message.post',
      state
    });
  });

  it('returns the role permission error message when the Connect call fails', async () => {
    const api = mockAPI();
    vi.mocked(api.setRolePermission).mockRejectedValue(new Error('boom'));

    const result = await setRolePermission(
      api,
      { tier: 'server', roleName: 'admin' },
      'message.post',
      'allow'
    );

    expect(result.error).toBe('boom');
  });

  const userCases = [
    [{ tier: 'server' } as const, 'allow'],
    [{ tier: 'group', groupId: 'G1' } as const, 'deny'],
    [{ tier: 'room', roomId: 'R1' } as const, 'neutral']
  ] satisfies Array<[Parameters<typeof setUserPermission>[2], PermissionState]>;

  it.each(userCases)('sets a user permission via Connect for %o %s', async (scope, state) => {
    const api = mockAPI();

    await setUserPermission(api, 'U1', scope, 'admin.view-users', state);

    expect(api.setUserPermission).toHaveBeenCalledWith({
      userId: 'U1',
      scope,
      permission: 'admin.view-users',
      state
    });
  });

  it('returns the user permission error message when the Connect call fails', async () => {
    const api = mockAPI();
    vi.mocked(api.setUserPermission).mockRejectedValue(new Error('boom'));

    const result = await setUserPermission(
      api,
      'U1',
      { tier: 'server' },
      'admin.view-users',
      'allow'
    );

    expect(result.error).toBe('boom');
  });
});
