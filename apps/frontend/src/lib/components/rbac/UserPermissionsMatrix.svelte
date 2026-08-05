<!--
@component

Per-user permission matrix loader. Owns the ConnectRPC query for the user's
matrix and the mutation dispatch for cell clicks; delegates rendering to
`SubjectPermissionsMatrix`.
-->
<script lang="ts">
  import { onDestroy } from 'svelte';
  import { Hint } from '$lib/ui';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createPermissionAPI } from '$lib/api-client/permissions';
  import { toast } from '$lib/ui/toast';
  import { m } from '$lib/i18n/messages';
  import {
    setUserPermission,
    type UserMutationScope,
    type UserPermissionState
  } from './userPermissionMutations';
  import SubjectPermissionsMatrix, {
    type MatrixData,
    type MatrixScope,
    type CellState
  } from './SubjectPermissionsMatrix.svelte';
  import { createQuery } from '@tanstack/svelte-query';
  import { adminQueryKeys } from '$lib/query/admin';
  import { queryClient } from '$lib/query/client';

  type Matrix = MatrixData & { userId: string };

  let { userId }: { userId: string } = $props();

  const serverScope = useServerScope();

  const matrixQuery = createQuery(
    () => {
      const serverId = serverScope.serverId;
      const activeConnection = serverScope.connection;
      const activeUserId = userId;
      return {
        queryKey: adminQueryKeys.userPermissions(serverId, activeConnection, activeUserId),
        queryFn: ({ signal }) =>
          activeConnection
            .getAPI(createPermissionAPI)
            .getUserPermissionMatrix(activeUserId, { signal })
      };
    },
    () => queryClient
  );

  const data = $derived<Matrix | null>(matrixQuery.data ?? null);
  const loading = $derived(matrixQuery.isPending);
  const loadError = $derived(matrixQuery.error instanceof Error ? matrixQuery.error.message : null);
  let mutationError = $state<{ context: string; message: string } | null>(null);
  let updatingKey = $state<string | null>(null);
  let mutationContext = $state<string | null>(null);
  let mutationGeneration = 0;
  const activeMutationContext = $derived(
    JSON.stringify([serverScope.serverId, serverScope.connection.queryScope, userId])
  );
  const visibleMutationError = $derived(
    mutationError?.context === activeMutationContext ? mutationError.message : null
  );
  const visibleUpdatingKey = $derived(
    mutationContext === activeMutationContext ? updatingKey : null
  );
  onDestroy(() => {
    mutationGeneration += 1;
  });

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
    if (!data || visibleUpdatingKey) return;
    const generation = ++mutationGeneration;
    const serverId = serverScope.serverId;
    const activeConnection = serverScope.connection;
    const activeUserId = data.userId;
    const context = JSON.stringify([serverId, activeConnection.queryScope, activeUserId]);
    const queryKey = adminQueryKeys.userPermissions(serverId, activeConnection, activeUserId);
    const cellKey = `${scope.id}::${permission}`;
    updatingKey = cellKey;
    mutationContext = context;
    mutationError = null;

    const result = await setUserPermission(
      activeConnection.getAPI(createPermissionAPI),
      activeUserId,
      mutationScopeFor(scope),
      permission,
      next as UserPermissionState
    );
    if (mutationGeneration !== generation || !serverScope.isCurrent()) return;
    if (result.error) {
      if (mutationGeneration === generation && context === activeMutationContext) {
        mutationError = { context, message: result.error };
        toast.error(result.error);
      }
      if (mutationGeneration === generation) {
        updatingKey = null;
      }
      return;
    }

    // Reload the matrix so both the override AND effective decisions stay
    // consistent — a server-scope grant flows into rooms via inheritance.
    await queryClient.invalidateQueries({ queryKey, exact: true });
    if (!serverScope.isCurrent()) return;
    if (mutationGeneration === generation) updatingKey = null;
  }
</script>

{#if visibleMutationError || loadError}
  <Hint tone="danger">{visibleMutationError ?? loadError}</Hint>
{/if}

{#if loading}
  <div class="text-muted">{m('rbac.permissions.loading')}</div>
{:else if !data}
  <Hint tone="info">{m('rbac.permissions.no_data')}</Hint>
{:else}
  <SubjectPermissionsMatrix
    {data}
    updatingKey={visibleUpdatingKey}
    onCycle={handleCycle}
    subjectKind="user"
    readOnly={visibleUpdatingKey !== null}
  />
{/if}
