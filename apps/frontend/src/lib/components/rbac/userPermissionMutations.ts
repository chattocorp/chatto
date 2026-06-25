/**
 * User-level permission mutation dispatch used by `UserPermissionsMatrix`.
 *
 * A user-level override can be configured at three scopes:
 *
 *   - server: no group/room context.
 *   - group:  a room group's scope.
 *   - room:   a specific room's scope.
 *
 * The backend exposes a single trio of mutations (`grantUserPermission`,
 * `denyUserPermission`, `clearUserPermissionState`) that route on
 * `roomId` vs `groupId` (mutually exclusive; with neither, server scope).
 */

import type { Client } from '@urql/svelte';
import { graphql } from '$lib/gql';

export type UserPermissionState = 'allow' | 'deny' | 'neutral';

export type UserMutationScope =
  | { tier: 'server' }
  | { tier: 'group'; groupId: string }
  | { tier: 'room'; roomId: string };

export async function setUserPermission(
  client: Client,
  userId: string,
  scope: UserMutationScope,
  permission: string,
  newState: UserPermissionState
): Promise<{ error?: string }> {
  const input: {
    userId: string;
    permission: string;
    roomId?: string;
    groupId?: string;
  } = { userId, permission };
  if (scope.tier === 'group') input.groupId = scope.groupId;
  if (scope.tier === 'room') input.roomId = scope.roomId;

  if (newState === 'allow') {
    const r = await client.mutation(
      graphql(`
        mutation MatrixGrantUserPerm($input: GrantUserPermissionInput!) {
          grantUserPermission(input: $input)
        }
      `),
      { input }
    );
    return { error: r.error?.message };
  }
  if (newState === 'deny') {
    const r = await client.mutation(
      graphql(`
        mutation MatrixDenyUserPerm($input: DenyUserPermissionInput!) {
          denyUserPermission(input: $input)
        }
      `),
      { input }
    );
    return { error: r.error?.message };
  }
  const r = await client.mutation(
    graphql(`
      mutation MatrixClearUserPerm($input: ClearUserPermissionStateInput!) {
        clearUserPermissionState(input: $input)
      }
    `),
    { input }
  );
  return { error: r.error?.message };
}
