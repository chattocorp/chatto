<!--
@component

Self-contained editor for one role's permissions at a single scope. Handles
loading via the unified rolePermissions query, dispatches to the right
per-scope mutation when a row is toggled, surfaces inheritance from the
tier above, and notifies the parent of the resolved role metadata via
`onLoaded` (so the parent can use displayName, isInstanceRole, etc. in
its own header without a second query).

Scope is implied by which of spaceId / roomId are set:

  spaceId  | roomId  | what's edited
  ---------+---------+--------------------------------------------------
  ∅        | ∅       | role's instance-scope permissions
  set      | ∅       | role's space-scope permissions
                       (instance roles use the instance-role-space-config
                        mutations under the hood)
  set      | set     | role's per-room overrides

Inheritance is shown automatically when an upper tier exists for the role
(instance for room overrides on a space role; instance for space overrides
on an instance role; etc.).
-->
<script lang="ts">
  import { useConnection } from '$lib/state/instance/connection.svelte';
  import { graphql } from '$lib/gql';
  import { Hint } from '$lib/ui';
  import { toast } from '$lib/ui/toast';
  import PermissionGrid from './PermissionGrid.svelte';
  import type { PermissionState } from './types';

  type TierData = { permissions: string[]; permissionDenials: string[] } | null;
  type RoleAcrossTiers = {
    roleName: string;
    displayName: string;
    description: string;
    isInstanceRole: boolean;
    isSystem: boolean;
    position: number;
    applicablePermissions: string[];
    instance: TierData;
    space: TierData;
    room: TierData;
  };

  let {
    roleName,
    spaceId = null,
    roomId = null,
    categoryOrder,
    onLoaded
  }: {
    roleName: string;
    /** Set for space- or room-scope edits. */
    spaceId?: string | null;
    /** Set for room-scope edits. Requires spaceId. */
    roomId?: string | null;
    /** Optional override for the category ordering inside the grid. */
    categoryOrder?: string[];
    /**
     * Called once after the role's metadata + tier data has loaded, so the
     * parent can surface displayName, description, isInstanceRole, etc.
     */
    onLoaded?: (role: RoleAcrossTiers) => void;
  } = $props();

  const connection = useConnection();

  let role = $state<RoleAcrossTiers | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let updating = $state<string | null>(null);

  // Re-load whenever the (role, scope) tuple changes.
  $effect(() => {
    const r = roleName;
    const s = spaceId ?? null;
    const rm = roomId ?? null;
    if (!r) return;

    void load(r, s, rm);
  });

  async function load(r: string, s: string | null, rm: string | null) {
    loading = true;
    error = null;

    const resp = await connection().client.query(
      graphql(`
        query RolePermissionEditorData($roleName: String!, $spaceId: ID, $roomId: ID) {
          rolePermissions(roleName: $roleName, spaceId: $spaceId, roomId: $roomId) {
            roleName
            displayName
            description
            isInstanceRole
            isSystem
            position
            applicablePermissions
            instance {
              permissions
              permissionDenials
            }
            space {
              permissions
              permissionDenials
            }
            room {
              permissions
              permissionDenials
            }
          }
        }
      `),
      { roleName: r, spaceId: s, roomId: rm }
    );

    // Stale-response guard.
    if (r !== roleName || s !== (spaceId ?? null) || rm !== (roomId ?? null)) return;

    loading = false;
    if (resp.error) {
      error = resp.error.message;
      return;
    }
    if (!resp.data?.rolePermissions) {
      error = `Role "${r}" is not available at this scope`;
      return;
    }
    role = resp.data.rolePermissions as RoleAcrossTiers;
    onLoaded?.(role);
  }

  // ----- Tier accessors ---------------------------------------------------

  const editingTier = $derived.by((): 'instance' | 'space' | 'room' => {
    if (roomId) return 'room';
    if (spaceId) return 'space';
    return 'instance';
  });

  const inheritedFromLabel = $derived.by(() => {
    if (!role) return undefined;
    if (editingTier === 'room') return 'space';
    if (editingTier === 'space' && role.isInstanceRole) return 'instance';
    return undefined;
  });

  const currentTier = $derived(role ? role[editingTier] : null);
  const inheritedTier = $derived.by((): TierData => {
    if (!role) return null;
    if (editingTier === 'room') return role.space;
    if (editingTier === 'space' && role.isInstanceRole) return role.instance;
    return null;
  });

  // ----- Mutation dispatch ------------------------------------------------

  async function setPermissionState(permission: string, newState: PermissionState) {
    if (!role) return;
    updating = permission;
    error = null;

    const result = await dispatchMutation(permission, newState);
    if (result.error) {
      error = result.error;
      updating = null;
      return;
    }

    // Optimistic update on the relevant tier.
    if (currentTier) {
      currentTier.permissions = currentTier.permissions.filter((p) => p !== permission);
      currentTier.permissionDenials = currentTier.permissionDenials.filter((p) => p !== permission);
      if (newState === 'allow') {
        currentTier.permissions = [...currentTier.permissions, permission];
        toast.success(`Granted ${permission}`);
      } else if (newState === 'deny') {
        currentTier.permissionDenials = [...currentTier.permissionDenials, permission];
        toast.success(`Denied ${permission}`);
      } else {
        toast.success(`Cleared ${permission}`);
      }
    }
    updating = null;
  }

  async function dispatchMutation(
    permission: string,
    newState: PermissionState
  ): Promise<{ error?: string }> {
    if (!role) return { error: 'role not loaded' };

    const client = connection().client;

    // Room scope.
    if (editingTier === 'room' && spaceId && roomId) {
      const input = { spaceId, roomId, role: roleName, permission };
      switch (newState) {
        case 'allow': {
          const r = await client.mutation(
            graphql(`
              mutation EditorGrantRoomPerm($input: GrantRoomPermissionInput!) {
                grantRoomPermission(input: $input)
              }
            `),
            { input }
          );
          return { error: r.error?.message };
        }
        case 'deny': {
          const r = await client.mutation(
            graphql(`
              mutation EditorDenyRoomPerm($input: DenyRoomPermissionInput!) {
                denyRoomPermission(input: $input)
              }
            `),
            { input }
          );
          return { error: r.error?.message };
        }
        case 'neutral': {
          const r = await client.mutation(
            graphql(`
              mutation EditorClearRoomPerm($input: ClearRoomPermissionInput!) {
                clearRoomPermission(input: $input)
              }
            `),
            { input }
          );
          return { error: r.error?.message };
        }
      }
    }

    // Space scope, instance role.
    if (editingTier === 'space' && spaceId && role.isInstanceRole) {
      const input = { spaceId, instanceRole: roleName, permission };
      switch (newState) {
        case 'allow': {
          const r = await client.mutation(
            graphql(`
              mutation EditorGrantInstanceRoleSpacePerm(
                $input: GrantInstanceRoleSpacePermissionInput!
              ) {
                grantInstanceRoleSpacePermission(input: $input)
              }
            `),
            { input }
          );
          return { error: r.error?.message };
        }
        case 'deny': {
          const r = await client.mutation(
            graphql(`
              mutation EditorDenyInstanceRoleSpacePerm(
                $input: DenyInstanceRoleSpacePermissionInput!
              ) {
                denyInstanceRoleSpacePermission(input: $input)
              }
            `),
            { input }
          );
          return { error: r.error?.message };
        }
        case 'neutral': {
          const r = await client.mutation(
            graphql(`
              mutation EditorClearInstanceRoleSpacePerm(
                $input: ClearInstanceRoleSpacePermissionInput!
              ) {
                clearInstanceRoleSpacePermission(input: $input)
              }
            `),
            { input }
          );
          return { error: r.error?.message };
        }
      }
    }

    // Space scope, space role.
    if (editingTier === 'space' && spaceId) {
      const input = { spaceId, role: roleName, permission };
      switch (newState) {
        case 'allow': {
          const r = await client.mutation(
            graphql(`
              mutation EditorGrantSpacePerm($input: GrantSpacePermissionInput!) {
                grantSpacePermission(input: $input)
              }
            `),
            { input }
          );
          return { error: r.error?.message };
        }
        case 'deny': {
          const r = await client.mutation(
            graphql(`
              mutation EditorDenySpacePerm($input: DenySpacePermissionInput!) {
                denySpacePermission(input: $input)
              }
            `),
            { input }
          );
          return { error: r.error?.message };
        }
        case 'neutral': {
          const r = await client.mutation(
            graphql(`
              mutation EditorClearSpacePerm($input: ClearSpacePermissionStateInput!) {
                clearSpacePermissionState(input: $input)
              }
            `),
            { input }
          );
          return { error: r.error?.message };
        }
      }
    }

    // Instance scope.
    {
      const input = { role: roleName, permission };
      switch (newState) {
        case 'allow': {
          const r = await client.mutation(
            graphql(`
              mutation EditorGrantInstancePerm($input: GrantInstancePermissionInput!) {
                grantInstancePermission(input: $input)
              }
            `),
            { input }
          );
          return { error: r.error?.message };
        }
        case 'deny': {
          const r = await client.mutation(
            graphql(`
              mutation EditorDenyInstancePerm($input: DenyInstancePermissionInput!) {
                denyInstancePermission(input: $input)
              }
            `),
            { input }
          );
          return { error: r.error?.message };
        }
        case 'neutral': {
          const r = await client.mutation(
            graphql(`
              mutation EditorClearInstancePerm($input: ClearInstancePermissionStateInput!) {
                clearInstancePermissionState(input: $input)
              }
            `),
            { input }
          );
          return { error: r.error?.message };
        }
      }
    }
  }
</script>

{#if error}
  <Hint variant="danger">{error}</Hint>
{/if}

{#if loading}
  <div class="text-muted">Loading permissions...</div>
{:else if !role}
  <Hint variant="danger">Role not found</Hint>
{:else}
  <PermissionGrid
    permissions={role.applicablePermissions}
    grantedPermissions={currentTier?.permissions ?? []}
    deniedPermissions={currentTier?.permissionDenials ?? []}
    inheritedPermissions={inheritedTier?.permissions ?? []}
    inheritedDenials={inheritedTier?.permissionDenials ?? []}
    {inheritedFromLabel}
    {categoryOrder}
    updatingPermission={updating}
    onSetState={setPermissionState}
  />
{/if}
