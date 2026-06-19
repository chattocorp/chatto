<!--
@component

Per-role permission matrix loader. Owns the wire request for the
role's matrix and the mutation dispatch for cell clicks; delegates
rendering to `SubjectPermissionsMatrix` (shared with the user variant).

Mutations reuse the existing per-tier role mutations
(`grantPermission` / `grantGroupPermission` / `grantRoomPermission`
and the deny/clear variants) via `setRolePermission`.
-->
<script lang="ts">
  import { afterNavigate } from '$app/navigation';
  import { onMount } from 'svelte';
  import { Hint } from '$lib/ui';
  import {
    GetRolePermissionMatrixRequest,
    PermissionMatrixDecision,
    PermissionMatrixScopeKind,
    type PermissionMatrixCellView,
    type PermissionMatrixScopeView
  } from '$lib/pb/chatto/api/v1/chat_pb';
  import { toast } from '$lib/ui/toast';
  import { withActiveServerWireClient } from '$lib/wire/activeServerClient';
  import {
    setRolePermission,
    type MutationScope as RoleMutationScope,
    type PermissionState
  } from './permissionMutations';
  import SubjectPermissionsMatrix, {
    type MatrixData,
    type MatrixScope,
    type CellState,
    type MatrixDecision,
    type MatrixScopeKind
  } from './SubjectPermissionsMatrix.svelte';

  type Matrix = MatrixData & { roleName: string };

  let { roleName }: { roleName: string } = $props();

  let data = $state<Matrix | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let updatingKey = $state<string | null>(null);
  let currentLoadName = '';
  const isOwnerRole = $derived(roleName === 'owner');

  onMount(scheduleLoad);
  afterNavigate(scheduleLoad);

  function scheduleLoad() {
    if (roleName === currentLoadName) return;
    currentLoadName = roleName;
    void load(roleName);
  }

  async function load(name: string) {
    const current = data;
    if (!current || current.roleName !== name) loading = true;
    error = null;

    let matrix;
    try {
      const resp = await withActiveServerWireClient((client) =>
        client.getRolePermissionMatrix(new GetRolePermissionMatrixRequest({ roleName: name }))
      );
      matrix = resp.matrix;
    } catch (loadError: unknown) {
      if (name !== roleName) return;
      loading = false;
      error = errorMessage(loadError);
      return;
    }

    if (name !== roleName) return;

    loading = false;
    if (!matrix) {
      error = 'Role not found.';
      return;
    }
    const m = matrix;
    data = {
      roleName: m.roleName,
      applicablePermissions: [...m.applicablePermissions],
      scopes: m.scopes.map(scopeFromWire),
      cells: m.cells.map(cellFromWire)
    };
  }

  function scopeFromWire(scope: PermissionMatrixScopeView): MatrixScope {
    return {
      id: scope.id,
      label: scope.label,
      kind: scopeKindFromWire(scope.kind),
      parentGroupId: scope.parentGroupId
    };
  }

  function cellFromWire(cell: PermissionMatrixCellView): MatrixData['cells'][number] {
    return {
      permission: cell.permission,
      scopeId: cell.scopeId,
      override: decisionFromWire(cell.override),
      effective: decisionFromWire(cell.effective)
    };
  }

  function scopeKindFromWire(kind: PermissionMatrixScopeKind): MatrixScopeKind {
    if (kind === PermissionMatrixScopeKind.GROUP) return 'GROUP';
    if (kind === PermissionMatrixScopeKind.ROOM) return 'ROOM';
    return 'SERVER';
  }

  function decisionFromWire(decision: PermissionMatrixDecision): MatrixDecision {
    if (decision === PermissionMatrixDecision.ALLOW) return 'ALLOW';
    if (decision === PermissionMatrixDecision.DENY) return 'DENY';
    return 'NONE';
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

    const result = await withActiveServerWireClient((client) =>
      setRolePermission(client, mutationScopeFor(scope, data!.roleName), permission, next as PermissionState)
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

  function errorMessage(value: unknown): string {
    return value instanceof Error ? value.message : String(value);
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
