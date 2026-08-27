<!--
@component

Per-role permission matrix loader. Owns the ConnectRPC query for the
role's matrix and the mutation dispatch for cell clicks; delegates
rendering to `SubjectPermissionsMatrix` (shared with the user variant).

  Mutations go through the admin permission API via `setRolePermission`.
-->
<script lang="ts">
  import { onDestroy } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import { Hint } from '$lib/ui';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createPermissionAPI } from '$lib/api-client/permissions';
  import { toast } from '$lib/ui/toast';
  import { m } from '$lib/i18n/messages';
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
  import { createQuery } from '@tanstack/svelte-query';
  import { adminQueryKeys } from '$lib/query/admin';
  import { queryClient } from '$lib/query/client';
  import { invalidateRolePermissionDependents } from '$lib/query/adminInvalidation';
  import {
    cancelPermissionMutationScroll,
    capturePermissionMutationScroll,
    restorePermissionMutationScroll
  } from './permissionMutationScroll';

  type Matrix = MatrixData & { roleName: string };

  let { roleName }: { roleName: string } = $props();

  const serverScope = useServerScope();

  const matrixQuery = createQuery(
    () => {
      const serverId = serverScope.serverId;
      const activeConnection = serverScope.connection;
      const activeRoleName = roleName;
      return {
        queryKey: adminQueryKeys.rolePermissions(serverId, activeConnection, activeRoleName),
        queryFn: ({ signal }) =>
          activeConnection
            .getAPI(createPermissionAPI)
            .getRolePermissionMatrix(activeRoleName, { signal })
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
  let matrixElement = $state<HTMLDivElement>();
  let mutationGeneration = 0;
  const isOwnerRole = $derived(roleName === 'owner');
  const activeMutationContext = $derived(
    JSON.stringify([serverScope.serverId, serverScope.connection.queryScope, roleName])
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

  const trackMatrixElement: Attachment<HTMLDivElement> = (element) => {
    matrixElement = element;
    restorePermissionMutationScroll(`role:${activeMutationContext}`, element);
    return () => {
      if (matrixElement === element) matrixElement = undefined;
    };
  };

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
    if (!data || visibleUpdatingKey) return;
    const generation = ++mutationGeneration;
    const serverId = serverScope.serverId;
    const activeConnection = serverScope.connection;
    const activeRoleName = data.roleName;
    const context = JSON.stringify([serverId, activeConnection.queryScope, activeRoleName]);
    const queryKey = adminQueryKeys.rolePermissions(serverId, activeConnection, activeRoleName);
    const cellKey = `${scope.id}::${permission}`;
    updatingKey = cellKey;
    mutationContext = context;
    mutationError = null;
    capturePermissionMutationScroll(`role:${context}`, matrixElement);

    const result = await setRolePermission(
      activeConnection.getAPI(createPermissionAPI),
      mutationScopeFor(scope, activeRoleName),
      permission,
      next as PermissionState
    );
    if (mutationGeneration !== generation || !serverScope.isCurrent()) return;
    if (result.error) {
      cancelPermissionMutationScroll(`role:${context}`);
      if (mutationGeneration === generation && context === activeMutationContext) {
        mutationError = { context, message: result.error };
        toast.error(result.error);
      }
      if (mutationGeneration === generation) {
        updatingKey = null;
      }
      return;
    }

    await queryClient.invalidateQueries({ queryKey, exact: true });
    if (!serverScope.isCurrent()) return;
    invalidateRolePermissionDependents(serverId, activeConnection, activeRoleName);
    if (mutationGeneration === generation) updatingKey = null;
  }
</script>

{#if visibleMutationError || loadError}
  <Hint tone="danger">{visibleMutationError ?? loadError}</Hint>
{/if}

{#if loading}
  <div class="text-muted">{m('rbac.permissions.loading')}</div>
{:else if !data}
  <Hint tone="info">{m('admin.permissions.role_not_found')}</Hint>
{:else}
  <div {@attach trackMatrixElement}>
    <SubjectPermissionsMatrix
      {data}
      updatingKey={visibleUpdatingKey}
      onCycle={handleCycle}
      subjectKind="role"
      forceAllow={isOwnerRole}
      readOnly={isOwnerRole || visibleUpdatingKey !== null}
    />
  </div>
{/if}
