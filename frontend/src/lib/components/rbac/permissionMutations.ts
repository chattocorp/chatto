/**
 * Permission mutation dispatch used by `PermissionMatrix`. After Phase 5 of
 * #330 there's only one tier of roles (server-wide); after ADR-031 there
 * are three scopes the matrix can edit:
 *
 *   - server: the role's default. {@link MutationScope} with `tier: 'server'`.
 *   - set:    a room group's grants/denials (ADR-031, top-level for channel
 *             rooms). `tier: 'group'` carries `groupId`.
 *   - room:   a per-room override on top of the room's set. `tier: 'room'`
 *             carries `roomId`.
 */

import type { Client } from '@urql/svelte';
import { graphql } from '$lib/gql';

export type PermissionState = 'allow' | 'deny' | 'neutral';

export type MutationScope =
  | { tier: 'server'; roleName: string }
  | { tier: 'group'; roleName: string; groupId: string }
  | { tier: 'room'; roleName: string; roomId: string };

export async function setRolePermission(
  client: Client,
  scope: MutationScope,
  permission: string,
  newState: PermissionState
): Promise<{ error?: string }> {
  if (scope.tier === 'group') {
    const input = { groupId: scope.groupId, subject: scope.roleName, permission };
    if (newState === 'allow') {
      const r = await client.mutation(
        graphql(`
          mutation MatrixGrantGroupPerm($input: GroupPermissionInput!) {
            grantGroupPermission(input: $input)
          }
        `),
        { input }
      );
      return { error: r.error?.message };
    }
    if (newState === 'deny') {
      const r = await client.mutation(
        graphql(`
          mutation MatrixDenyGroupPerm($input: GroupPermissionInput!) {
            denyGroupPermission(input: $input)
          }
        `),
        { input }
      );
      return { error: r.error?.message };
    }
    const r = await client.mutation(
      graphql(`
        mutation MatrixClearGroupPerm($input: GroupPermissionInput!) {
          clearGroupPermissionState(input: $input)
        }
      `),
      { input }
    );
    return { error: r.error?.message };
  }

  if (scope.tier === 'room') {
    const input = {
      roomId: scope.roomId,
      role: scope.roleName,
      permission
    };
    if (newState === 'allow') {
      const r = await client.mutation(
        graphql(`
          mutation MatrixGrantRoomPerm($input: GrantRoomPermissionInput!) {
            grantRoomPermission(input: $input)
          }
        `),
        { input }
      );
      return { error: r.error?.message };
    }
    if (newState === 'deny') {
      const r = await client.mutation(
        graphql(`
          mutation MatrixDenyRoomPerm($input: DenyRoomPermissionInput!) {
            denyRoomPermission(input: $input)
          }
        `),
        { input }
      );
      return { error: r.error?.message };
    }
    const r = await client.mutation(
      graphql(`
        mutation MatrixClearRoomPerm($input: ClearRoomPermissionInput!) {
          clearRoomPermission(input: $input)
        }
      `),
      { input }
    );
    return { error: r.error?.message };
  }

  // Server scope.
  const input = { role: scope.roleName, permission };
  if (newState === 'allow') {
    const r = await client.mutation(
      graphql(`
        mutation MatrixGrantServerPerm($input: GrantPermissionInput!) {
          grantPermission(input: $input)
        }
      `),
      { input }
    );
    return { error: r.error?.message };
  }
  if (newState === 'deny') {
    const r = await client.mutation(
      graphql(`
        mutation MatrixDenyServerPerm($input: DenyPermissionInput!) {
          denyPermission(input: $input)
        }
      `),
      { input }
    );
    return { error: r.error?.message };
  }
  const r = await client.mutation(
    graphql(`
      mutation MatrixClearServerPerm($input: ClearPermissionStateInput!) {
        clearPermissionState(input: $input)
      }
    `),
    { input }
  );
  return { error: r.error?.message };
}
