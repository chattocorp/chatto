/**
 * Permission mutation dispatch used by `PermissionMatrix`. Picks the right
 * grant/deny/clear triple based on the (tier, role kind) combination — at
 * room scope every role uses the room mutations; at space scope instance
 * roles use the dedicated instance-role-space mutations and space roles use
 * the plain space ones; at instance scope only instance roles are
 * meaningful.
 */

import type { Client } from '@urql/svelte';
import { graphql } from '$lib/gql';

export type PermissionState = 'allow' | 'deny' | 'neutral';

export type MutationScope =
  | { tier: 'instance'; roleName: string; isInstanceRole: true }
  | { tier: 'space'; roleName: string; isInstanceRole: boolean }
  | {
      tier: 'room';
      roleName: string;
      isInstanceRole: boolean;
      roomId: string;
    };

export async function setRolePermission(
  client: Client,
  scope: MutationScope,
  permission: string,
  newState: PermissionState
): Promise<{ error?: string }> {
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

  // Post-#330 PR(a) the InstanceRole-Space configurability collapsed:
  // configuring an instance role's permission at the (single) server space
  // is the same as configuring it at the instance, so we route through the
  // instance-permission mutations regardless of the role kind for the
  // 'space' + isInstanceRole case. Full RBAC unification follows in PR(c).
  if (scope.tier === 'space' && scope.isInstanceRole) {
    return setInstanceRolePermission(client, scope.roleName, permission, newState);
  }

  if (scope.tier === 'space') {
    const input = { role: scope.roleName, permission };
    if (newState === 'allow') {
      const r = await client.mutation(
        graphql(`
          mutation MatrixGrantSpacePerm($input: GrantSpacePermissionInput!) {
            grantSpacePermission(input: $input)
          }
        `),
        { input }
      );
      return { error: r.error?.message };
    }
    if (newState === 'deny') {
      const r = await client.mutation(
        graphql(`
          mutation MatrixDenySpacePerm($input: DenySpacePermissionInput!) {
            denySpacePermission(input: $input)
          }
        `),
        { input }
      );
      return { error: r.error?.message };
    }
    const r = await client.mutation(
      graphql(`
        mutation MatrixClearSpacePerm($input: ClearSpacePermissionStateInput!) {
          clearSpacePermissionState(input: $input)
        }
      `),
      { input }
    );
    return { error: r.error?.message };
  }

  // Instance scope.
  return setInstanceRolePermission(client, scope.roleName, permission, newState);
}

async function setInstanceRolePermission(
  client: Client,
  roleName: string,
  permission: string,
  newState: PermissionState
): Promise<{ error?: string }> {
  const input = { role: roleName, permission };
  if (newState === 'allow') {
    const r = await client.mutation(
      graphql(`
        mutation MatrixGrantInstancePerm($input: GrantInstancePermissionInput!) {
          grantInstancePermission(input: $input)
        }
      `),
      { input }
    );
    return { error: r.error?.message };
  }
  if (newState === 'deny') {
    const r = await client.mutation(
      graphql(`
        mutation MatrixDenyInstancePerm($input: DenyInstancePermissionInput!) {
          denyInstancePermission(input: $input)
        }
      `),
      { input }
    );
    return { error: r.error?.message };
  }
  const r = await client.mutation(
    graphql(`
      mutation MatrixClearInstancePerm($input: ClearInstancePermissionStateInput!) {
        clearInstancePermissionState(input: $input)
      }
    `),
    { input }
  );
  return { error: r.error?.message };
}
