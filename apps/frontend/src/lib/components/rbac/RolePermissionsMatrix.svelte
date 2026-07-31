<!--
@component

Per-role permission matrix loader. Owns the ConnectRPC query for the
role's matrix and the mutation dispatch for cell clicks; delegates
rendering to `SubjectPermissionsMatrix` (shared with the user variant).

  Mutations go through the admin permission API via `setRolePermission`.
-->
<script lang="ts">
  import { untrack } from 'svelte';
  import { Hint } from '$lib/ui';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createPermissionAPI } from '$lib/api-client/permissions';
  import { toast } from '$lib/ui/toast';
  import * as m from '$lib/i18n/messages';
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

  const serverScope = useServerScope();

  function permissionAPI() {
    return serverScope.connection.getAPI(createPermissionAPI);
  }

  let data = $state<Matrix | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let updatingKey = $state<string | null>(null);
  let resourceGeneration = 0;
  const isOwnerRole = $derived(roleName === 'owner');

  $effect(() => {
    const name = roleName;
    const generation = ++resourceGeneration;
    updatingKey = null;
    void load(name, generation);
  });

  async function load(name: string, generation: number) {
    const current = untrack(() => data);
    if (!current || current.roleName !== name) loading = true;
    error = null;

    let matrix: Matrix | null = null;
    try {
      matrix = await permissionAPI().getRolePermissionMatrix(name);
    } catch (err) {
      if (!serverScope.isCurrent() || generation !== resourceGeneration || name !== roleName)
        return;
      loading = false;
      error = err instanceof Error ? err.message : String(err);
      return;
    }

    if (!serverScope.isCurrent() || generation !== resourceGeneration || name !== roleName) return;

    loading = false;
    if (!matrix) {
      error = m['admin.permissions.role_not_found']();
      return;
    }
    const loadedMatrix = matrix;
    data = {
      roleName: loadedMatrix.roleName,
      applicablePermissions: [...loadedMatrix.applicablePermissions],
      scopes: loadedMatrix.scopes.map((s) => ({ ...s })),
      cells: loadedMatrix.cells.map((c) => ({ ...c }))
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
    if (!data || updatingKey) return;
    const targetRoleName = data.roleName;
    const generation = resourceGeneration;
    const cellKey = `${scope.id}::${permission}`;
    updatingKey = cellKey;
    error = null;

    const result = await setRolePermission(
      permissionAPI(),
      mutationScopeFor(scope, targetRoleName),
      permission,
      next as PermissionState
    );
    if (
      !serverScope.isCurrent() ||
      generation !== resourceGeneration ||
      targetRoleName !== roleName ||
      data?.roleName !== targetRoleName
    )
      return;
    if (result.error) {
      error = result.error;
      toast.error(result.error);
      updatingKey = null;
      return;
    }

    await load(targetRoleName, generation);
    if (serverScope.isCurrent() && generation === resourceGeneration && targetRoleName === roleName)
      updatingKey = null;
  }
</script>

{#if error}
  <Hint tone="danger">{error}</Hint>
{/if}

{#if loading}
  <div class="text-muted">{m['rbac.permissions.loading']()}</div>
{:else if !data}
  <Hint tone="info">{m['rbac.permissions.no_data']()}</Hint>
{:else}
  <SubjectPermissionsMatrix
    {data}
    {updatingKey}
    onCycle={handleCycle}
    subjectKind="role"
    forceAllow={isOwnerRole}
    readOnly={isOwnerRole || updatingKey !== null}
  />
{/if}
