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
    toastErrors = false
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
  } = $props();

  let data = $state<MatrixData | null>(null);
  let loadedKey = $state<string | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let updatingKey = $state<string | null>(null);
  let generation = 0;

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
    if (next === 'allow' && cell?.canAllow === false) return;

    updatingKey = `${scope.id}::${permission}`;
    error = null;
    try {
      await mutate(key, scope, permission, next);
      if (key === resourceKey) await refresh(key, false);
    } catch (cause) {
      if (key !== resourceKey) return;
      error = errorMessage(cause);
      if (toastErrors) toast.error(error);
    } finally {
      if (key === resourceKey) updatingKey = null;
    }
  }

  $effect(() => {
    const key = resourceKey;
    untrack(() => void refresh(key, true));
  });
</script>

{#if error}
  <Hint tone="danger">{error}</Hint>
{/if}

{#if loading && !data}
  <div class="text-muted">{m['rbac.permissions.loading']()}</div>
{:else if !data}
  <Hint tone="info">{emptyMessage}</Hint>
{:else}
  <SubjectPermissionsMatrix
    {data}
    {updatingKey}
    {subjectKind}
    {forceAllow}
    {readOnly}
    {containedScroll}
    onCycle={cycle}
  />
{/if}
