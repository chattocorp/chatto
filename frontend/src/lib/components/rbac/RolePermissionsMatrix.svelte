<!--
@component

Per-role permission matrix loader. Owns the GraphQL query for the
role's matrix and the mutation dispatch for cell clicks; delegates
rendering to `SubjectPermissionsMatrix` (shared with the user variant).

Mutations reuse the existing per-tier role mutations
(`grantPermission` / `grantGroupPermission` / `grantRoomPermission`
and the deny/clear variants) via `setRolePermission`.
-->
<script lang="ts">
  import { untrack } from 'svelte';
  import { Hint } from '$lib/ui';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { graphql } from '$lib/gql';
  import { toast } from '$lib/ui/toast';
  import {
    setRolePermission,
    type MutationScope as RoleMutationScope,
    type PermissionState
  } from './permissionMutations';
  import SubjectPermissionsMatrix, {
    type MatrixData,
    type MatrixScope,
    type CellState
  } from './SubjectPermissionsMatrix.svelte';

  type Matrix = MatrixData & { roleName: string };

  let { roleName }: { roleName: string } = $props();

  const connection = useConnection();

  let data = $state<Matrix | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let updatingKey = $state<string | null>(null);
  const isOwnerRole = $derived(roleName === 'owner');

  $effect(() => {
    void load(roleName);
  });

  async function load(name: string) {
    const current = untrack(() => data);
    if (!current || current.roleName !== name) loading = true;
    error = null;

    const resp = await connection().client.query(
      graphql(`
        query RolePermissionsMatrixQuery($roleName: String!) {
          admin {
            rbac {
              rolePermissionMatrix(roleName: $roleName) {
                roleName
                applicablePermissions
                scopes {
                  id
                  label
                  kind
                  parentGroupId
                }
                cells {
                  permission
                  scopeId
                  override
                  effective
                }
              }
            }
          }
        }
      `),
      { roleName: name },
      { requestPolicy: 'network-only' }
    );

    if (name !== roleName) return;

    loading = false;
    if (resp.error) {
      error = resp.error.message;
      return;
    }
    const matrix = resp.data?.admin?.rbac.rolePermissionMatrix;
    if (!matrix) {
      error = 'Role not found.';
      return;
    }
    const m = matrix;
    data = {
      roleName: m.roleName,
      applicablePermissions: [...m.applicablePermissions],
      scopes: m.scopes.map((s) => ({ ...s })),
      cells: m.cells.map((c) => ({ ...c }))
    };
  }

  function mutationScopeFor(scope: MatrixScope, name: string): RoleMutationScope {
    if (scope.kind === 'GROUP') {
      const groupId = scope.id.startsWith('group:') ? scope.id.slice('group:'.length) : '';
      return { tier: 'group', roleName: name, groupId };
    }
    if (scope.kind === 'ROOM') {
      const roomId = scope.id.startsWith('room:') ? scope.id.slice('room:'.length) : '';
      return { tier: 'room', roleName: name, roomId };
    }
    return { tier: 'server', roleName: name };
  }

  async function handleCycle(scope: MatrixScope, permission: string, next: CellState) {
    if (!data) return;
    const cellKey = `${scope.id}::${permission}`;
    updatingKey = cellKey;
    error = null;

    const result = await setRolePermission(
      connection().client,
      mutationScopeFor(scope, data.roleName),
      permission,
      next as PermissionState
    );
    if (result.error) {
      error = result.error;
      toast.error(result.error);
      updatingKey = null;
      return;
    }

    await load(data.roleName);
    updatingKey = null;
  }
</script>

{#if error}
  <Hint tone="danger">{error}</Hint>
{/if}

{#if loading}
  <div class="text-muted">Loading permissions…</div>
{:else if !data}
  <Hint tone="info">No data available.</Hint>
{:else}
  <SubjectPermissionsMatrix
    {data}
    {updatingKey}
    onCycle={handleCycle}
    subjectKind="role"
    forceAllow={isOwnerRole}
    readOnly={isOwnerRole}
  />
{/if}
