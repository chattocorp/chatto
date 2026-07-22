<!--
@component

Loads and edits a subject permission matrix. Callers provide resource-specific
load and mutation callbacks; this component owns request fencing, mutation
state, refreshes, and the shared matrix presentation.
-->
<script lang="ts">
  import { untrack } from 'svelte';
  import { Hint } from '$lib/ui';
  import { toast } from '$lib/ui/toast';
  import * as m from '$lib/i18n/messages';
  import SubjectPermissionsMatrix, {
    type CellState,
    type MatrixCellData,
    type MatrixData,
    type MatrixScope
  } from './SubjectPermissionsMatrix.svelte';

  let {
    resourceKey,
    load,
    mutate,
    subjectKind = 'subject',
    forceAllow = false,
    readOnly = false,
    containedScroll = true,
    emptyMessage = m['rbac.permissions.no_data'](),
    toastErrors = false,
    canGrant
  }: {
    resourceKey: string;
    load: (resourceKey: string) => Promise<MatrixData | null>;
    mutate: (
      resourceKey: string,
      scope: MatrixScope,
      permission: string,
      next: CellState
    ) => Promise<void>;
    subjectKind?: string;
    forceAllow?: boolean;
    readOnly?: boolean;
    containedScroll?: boolean;
    emptyMessage?: string;
    toastErrors?: boolean;
    /** Optional subject-specific ceiling for direct grants. */
    canGrant?: (cell: MatrixCellData) => boolean;
  } = $props();

  let data = $state<MatrixData | null>(null);
  let loadedKey = $state<string | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let updating = $state<{ resourceKey: string; cellKey: string } | null>(null);
  let generation = 0;

  const updatingKey = $derived(updating?.resourceKey === resourceKey ? updating.cellKey : null);

  function grantAllowed(cell: MatrixCellData): boolean {
    return cell.canAllow !== false && (canGrant?.(cell) ?? true);
  }

  const presentedData = $derived.by<MatrixData | null>(() =>
    data
      ? {
          ...data,
          cells: data.cells.map((cell) => ({ ...cell, canAllow: grantAllowed(cell) }))
        }
      : null
  );

  function errorMessage(cause: unknown): string {
    return cause instanceof Error ? cause.message : String(cause);
  }

  async function refresh(key: string, initial: boolean) {
    const requestGeneration = ++generation;
    if (initial && loadedKey !== key) {
      data = null;
      loading = true;
    }
    error = null;

    try {
      const next = await load(key);
      if (requestGeneration !== generation || key !== resourceKey) return;
      data = next;
      loadedKey = key;
    } catch (cause) {
      if (requestGeneration !== generation || key !== resourceKey) return;
      error = errorMessage(cause);
    } finally {
      if (requestGeneration === generation && key === resourceKey) loading = false;
    }
  }

  async function cycle(scope: MatrixScope, permission: string, next: CellState) {
    const key = resourceKey;
    const cell = data?.cells.find(
      (candidate) => candidate.scopeId === scope.id && candidate.permission === permission
    );
    if (next === 'allow' && cell && !grantAllowed(cell)) return;

    const pending = { resourceKey: key, cellKey: `${scope.id}::${permission}` };
    updating = pending;
    error = null;
    try {
      await mutate(key, scope, permission, next);
      if (key === resourceKey) await refresh(key, false);
    } catch (cause) {
      if (key !== resourceKey) return;
      error = errorMessage(cause);
      if (toastErrors) toast.error(error);
    } finally {
      if (updating === pending) updating = null;
    }
  }

  $effect(() => {
    const key = resourceKey;
    untrack(() => {
      void refresh(key, true);
    });
  });
</script>

{#if error}
  <Hint tone="danger">{error}</Hint>
{/if}

{#if loading && !data}
  <div class="text-muted">{m['rbac.permissions.loading']()}</div>
{:else if !presentedData}
  <Hint tone="info">{emptyMessage}</Hint>
{:else}
  <SubjectPermissionsMatrix
    data={presentedData}
    {updatingKey}
    {subjectKind}
    {forceAllow}
    {readOnly}
    {containedScroll}
    onCycle={cycle}
  />
{/if}
