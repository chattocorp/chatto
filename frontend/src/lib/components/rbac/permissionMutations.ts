/**
 * Permission mutation dispatch used by `PermissionMatrix`. The wire API
 * exposes one role permission state setter, and this helper maps UI scopes
 * onto that protobuf request:
 *
 *   - server: the role's default. {@link MutationScope} with `tier: 'server'`.
 *   - set:    a room group's grants/denials (ADR-031, top-level for channel
 *             rooms). `tier: 'group'` carries `groupId`.
 *   - room:   a per-room override on top of the room's set. `tier: 'room'`
 *             carries `roomId`.
 */

import {
  PermissionEditState,
  SetRolePermissionStateRequest
} from '$lib/pb/chatto/api/v1/chat_pb';
import type { WireClient } from '$lib/wire/client';

export type PermissionState = 'allow' | 'deny' | 'neutral';

export type MutationScope =
  | { tier: 'server'; roleName: string }
  | { tier: 'group'; roleName: string; groupId: string }
  | { tier: 'room'; roleName: string; roomId: string };

export async function setRolePermission(
  client: WireClient,
  scope: MutationScope,
  permission: string,
  newState: PermissionState
): Promise<{ error?: string }> {
  const request = new SetRolePermissionStateRequest({
    roleName: scope.roleName,
    permission,
    state: permissionStateToWire(newState)
  });
  if (scope.tier === 'group') request.groupId = scope.groupId;
  if (scope.tier === 'room') request.roomId = scope.roomId;

  try {
    await client.setRolePermissionState(request);
    return {};
  } catch (error: unknown) {
    return { error: errorMessage(error) };
  }
}

function permissionStateToWire(state: PermissionState): PermissionEditState {
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
