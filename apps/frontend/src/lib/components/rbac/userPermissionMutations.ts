import type { PermissionAPI, PermissionState } from '@chatto/api-client/permissions';

export type UserPermissionState = PermissionState;

export type UserMutationScope =
  | { tier: 'server' }
  | { tier: 'group'; groupId: string }
  | { tier: 'room'; roomId: string };

export async function setUserPermission(
  api: PermissionAPI,
  userId: string,
  scope: UserMutationScope,
  permission: string,
  newState: UserPermissionState
): Promise<{ error?: string }> {
  try {
    await api.setUserPermission({
      userId,
      scope,
      permission,
      state: newState
    });
    return {};
  } catch (error) {
    return { error: error instanceof Error ? error.message : String(error) };
  }
}
