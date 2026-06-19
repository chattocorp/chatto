<!--
@component

Per-user permission matrix loader. Owns the wire request for the user's
matrix and the mutation dispatch for cell clicks; delegates rendering to
`SubjectPermissionsMatrix`.
-->
<script lang="ts">
  import { afterNavigate } from '$app/navigation';
  import { onMount } from 'svelte';
  import { Hint } from '$lib/ui';
  import {
    GetUserPermissionMatrixRequest,
    PermissionMatrixDecision,
    PermissionMatrixScopeKind,
    type PermissionMatrixCellView,
    type PermissionMatrixScopeView
  } from '$lib/pb/chatto/api/v1/chat_pb';
  import { toast } from '$lib/ui/toast';
  import { withActiveServerWireClient } from '$lib/wire/activeServerClient';
  import {
    setUserPermission,
    type UserMutationScope,
    type UserPermissionState
  } from './userPermissionMutations';
  import SubjectPermissionsMatrix, {
    type MatrixData,
    type MatrixScope,
    type CellState,
    type MatrixDecision,
    type MatrixScopeKind
  } from './SubjectPermissionsMatrix.svelte';

  type Matrix = MatrixData & { userId: string };

  let { userId }: { userId: string } = $props();

  let data = $state<Matrix | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let updatingKey = $state<string | null>(null);
  let currentLoadUserId = '';

  onMount(scheduleLoad);
  afterNavigate(scheduleLoad);

  function scheduleLoad() {
    if (userId === currentLoadUserId) return;
    currentLoadUserId = userId;
    void load(userId);
  }

  async function load(uid: string) {
    // Only show the loading state on the initial load; refreshes after a
    // mutation keep the existing matrix visible so the page doesn't flash
    // a blank panel between request and response.
    //
    // Route-driven reloads call this directly, so reading `data` here won't
    // subscribe an async effect or create a reload loop.
    const current = data;
    if (!current || current.userId !== uid) loading = true;
    error = null;

    let matrix;
    try {
      const resp = await withActiveServerWireClient((client) =>
        client.getUserPermissionMatrix(new GetUserPermissionMatrixRequest({ userId: uid }))
      );
      matrix = resp.matrix;
    } catch (loadError: unknown) {
      if (uid !== userId) return;
      loading = false;
      error = errorMessage(loadError);
      return;
    }

    if (uid !== userId) return;

    loading = false;
    if (!matrix) {
      error = 'No data returned';
      return;
    }
    const m = matrix;
    data = {
      userId: m.userId,
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

  function mutationScopeFor(scope: MatrixScope): UserMutationScope {
    if (scope.kind === 'GROUP') {
      const groupId = scope.id.startsWith('group:') ? scope.id.slice('group:'.length) : '';
      return { tier: 'group', groupId };
    }
    if (scope.kind === 'ROOM') {
      const roomId = scope.id.startsWith('room:') ? scope.id.slice('room:'.length) : '';
      return { tier: 'room', roomId };
    }
    return { tier: 'server' };
  }

  async function handleCycle(scope: MatrixScope, permission: string, next: CellState) {
    if (!data) return;
    const cellKey = `${scope.id}::${permission}`;
    updatingKey = cellKey;
    error = null;

    const result = await withActiveServerWireClient((client) =>
      setUserPermission(client, data!.userId, mutationScopeFor(scope), permission, next as UserPermissionState)
    );
    if (result.error) {
      error = result.error;
      toast.error(result.error);
      updatingKey = null;
      return;
    }

    // Reload the matrix so both the override AND effective decisions stay
    // consistent — a server-scope grant flows into rooms via inheritance.
    await load(data.userId);
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
    subjectKind="user"
  />
{/if}
