import type {
  PermissionAPI,
  PermissionDecisionUpdate,
  PermissionState
} from '$lib/api-client/permissions';

export type UserPermissionState = PermissionState;

export type UserMutationScope =
  { tier: 'server' } | { tier: 'group'; groupId: string } | { tier: 'room'; roomId: string };

export async function setUserPermission(
  api: PermissionAPI,
  userId: string,
  scope: UserMutationScope,
  permission: string,
  newState: UserPermissionState
): Promise<{ update?: PermissionDecisionUpdate; error?: string }> {
  try {
    const update = await api.setUserPermission({
      userId,
      scope,
      permission,
      state: newState
    });
    return { update };
  } catch (error) {
    return { error: error instanceof Error ? error.message : String(error) };
  }
}
