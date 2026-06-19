/**
 * User-level permission mutation dispatch used by `UserPermissionsMatrix`.
 *
 * A user-level override can be configured at three scopes:
 *
 *   - server: no group/room context.
 *   - group:  a room group's scope.
 *   - room:   a specific room's scope.
 *
 * The wire API exposes a single state setter that routes on `roomId` vs
 * `groupId` (mutually exclusive; with neither, server scope).
 */

import {
  PermissionEditState,
  SetUserPermissionStateRequest
} from '$lib/pb/chatto/api/v1/chat_pb';
import type { WireClient } from '$lib/wire/client';

export type UserPermissionState = 'allow' | 'deny' | 'neutral';

export type UserMutationScope =
  | { tier: 'server' }
  | { tier: 'group'; groupId: string }
  | { tier: 'room'; roomId: string };

export async function setUserPermission(
  client: WireClient,
  userId: string,
  scope: UserMutationScope,
  permission: string,
  newState: UserPermissionState
): Promise<{ error?: string }> {
  const request = new SetUserPermissionStateRequest({
    userId,
    permission,
    state: permissionStateToWire(newState)
  });
  if (scope.tier === 'group') request.groupId = scope.groupId;
  if (scope.tier === 'room') request.roomId = scope.roomId;

  try {
    await client.setUserPermissionState(request);
    return {};
  } catch (error: unknown) {
    return { error: errorMessage(error) };
  }
}

function permissionStateToWire(state: UserPermissionState): PermissionEditState {
  if (state === 'allow') return PermissionEditState.ALLOW;
  if (state === 'deny') return PermissionEditState.DENY;
  return PermissionEditState.NEUTRAL;
}

function errorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}
