<!--
@component

Per-role permission matrix loader. Owns the ConnectRPC query for the
role's matrix and the mutation dispatch for cell clicks; delegates
rendering to `SubjectPermissionsMatrix` (shared with the user variant).

  Mutations go through the admin permission API via `setRolePermission`.
-->
<script lang="ts">
  import { errorMessage } from '$lib/utils/errorMessage';
  import { Button } from '$lib/ui/form';
  import { Hint } from '$lib/ui';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createSessionGuard } from '$lib/state/server/sessionGuard.svelte';
  import { createPermissionAPI } from '@chatto/client/api/permissions';
  import { toast } from '$lib/ui/toast';
  import { m } from '$lib/i18n/messages';
  import {
    setRolePermission,
    type MutationScope as RoleMutationScope,
    type PermissionState
  } from './permissionMutations';
  import SubjectPermissionsMatrix, {
    type MatrixScope,
    type CellState
  } from './SubjectPermissionsMatrix.svelte';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createQuery, queryClient } from '$lib/query/client';
  import { invalidateRolePermissionDependents } from '$lib/query/adminInvalidation';

  let { roleName }: { roleName: string } = $props();

  const serverScope = useServerScope();

  const matrixQuery = createQuery(() => {
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
  });

  const data = $derived(matrixQuery.data ?? null);
  const loading = $derived(matrixQuery.isPending);
  const loadError = $derived(matrixQuery.error ? errorMessage(matrixQuery.error) : null);
  const session = createSessionGuard(serverScope);
  // The page keeps this matrix mounted when only the role changes, so tag
  // mutation state with the role it belongs to.
  let pending = $state.raw<{ roleName: string; cellKey: string } | null>(null);
  let failure = $state.raw<{ roleName: string; message: string } | null>(null);
  const isOwnerRole = $derived(roleName === 'owner');
  const visibleMutationError = $derived(failure?.roleName === roleName ? failure.message : null);
  const visibleUpdatingKey = $derived(pending?.roleName === roleName ? pending.cellKey : null);

  function mutationScopeFor(scope: MatrixScope, name: string): RoleMutationScope {
    if (scope.kind === 'DM') return { tier: 'dm', roleName: name };
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
    const snapshot = session.snapshot();
    const activeRoleName = data.roleName;
    const mutation = { roleName: activeRoleName, cellKey: `${scope.id}::${permission}` };
    pending = mutation;
    failure = null;
    const result = await setRolePermission(
      snapshot.connection.getAPI(createPermissionAPI),
      mutationScopeFor(scope, activeRoleName),
      permission,
      next as PermissionState
    );
    if (session.isCurrent(snapshot)) {
      if (result.error) {
        if (activeRoleName === roleName) {
          failure = { roleName: activeRoleName, message: result.error };
          toast.error(result.error);
        }
      } else {
        await queryClient.invalidateQueries({
          queryKey: adminQueryKeys.rolePermissions(
            snapshot.serverId,
            snapshot.connection,
            activeRoleName
          ),
          exact: true
        });
        if (session.isCurrent(snapshot)) {
          invalidateRolePermissionDependents(
            snapshot.serverId,
            snapshot.connection,
            activeRoleName
          );
        }
      }
    }
    if (pending === mutation) pending = null;
  }
</script>

{#if visibleMutationError || loadError}
  <Hint tone="danger">{visibleMutationError ?? loadError}</Hint>
{/if}

{#if matrixQuery.isError}
  <Button variant="secondary" onclick={() => matrixQuery.refetch()}>{m('common.retry')}</Button>
{/if}

{#if !loading && !data}
  <Hint tone="info">{m('admin.permissions.role_not_found')}</Hint>
{:else}
  <SubjectPermissionsMatrix
    data={data ?? { applicablePermissions: [], scopes: [], cells: [] }}
    {loading}
    updatingKey={visibleUpdatingKey}
    onCycle={handleCycle}
    subjectKind={m('rbac.permissions.cell.role_subject')}
    forceAllow={isOwnerRole}
    readOnly={isOwnerRole || visibleUpdatingKey !== null}
  />
{/if}
