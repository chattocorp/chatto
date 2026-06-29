import type { PermissionAPI, PermissionState } from '@chatto/api-client/permissions';

export type { PermissionState };

export type MutationScope =
  | { tier: 'server'; roleName: string }
  | { tier: 'group'; roleName: string; groupId: string }
  | { tier: 'room'; roleName: string; roomId: string };

export async function setRolePermission(
  api: PermissionAPI,
  scope: MutationScope,
  permission: string,
  newState: PermissionState
): Promise<{ error?: string }> {
  try {
    await api.setRolePermission({
      roleName: scope.roleName,
      scope,
      permission,
      state: newState
    });
    return {};
  } catch (error) {
    return { error: error instanceof Error ? error.message : String(error) };
  }
}
