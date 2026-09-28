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
  import { adminQueryKeys } from '$lib/query/admin';
  import { createInfiniteQuery, queryClient } from '$lib/query/client';
  import { invalidateRolePermissionDependents } from '$lib/query/adminInvalidation';

  import { mergePermissionPages } from './permissionPages';
  import type { PermissionScopePage } from '$lib/api-client/permissions';

  type Matrix = MatrixData & { page: PermissionScopePage; roleName: string };

  let { roleName }: { roleName: string } = $props();

  const serverScope = useServerScope();

  const matrixQuery = createInfiniteQuery(() => {
    const serverId = serverScope.serverId;
    const activeConnection = serverScope.connection;
    const activeRoleName = roleName;
    return {
      queryKey: adminQueryKeys.rolePermissions(serverId, activeConnection, activeRoleName),
      initialPageParam: 0,
      getNextPageParam: (last: Matrix | null, pages: (Matrix | null)[]) =>
        last?.page.hasMore
          ? pages.reduce((count, page) => count + (page?.scopes.length ?? 0), 0)
          : undefined,
      queryFn: ({ signal, pageParam }) =>
        activeConnection.getAPI(createPermissionAPI).getRolePermissionMatrix(activeRoleName, {
          signal,
          page: { limit: 20, offset: pageParam }
        })
    };
  });

  const data = $derived<Matrix | null>(
    mergePermissionPages(
      (matrixQuery.data?.pages ?? []).filter((page): page is Matrix => page !== null)
    )
  );
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

{#if matrixQuery.isFetchNextPageError}
  <Button variant="secondary" onclick={() => matrixQuery.fetchNextPage()}
    >{m('common.retry')}</Button
  >
{/if}

{#if !loading && !data}
  <Hint tone="info">{m('admin.permissions.role_not_found')}</Hint>
{:else}
  <SubjectPermissionsMatrix
    data={data ?? { applicablePermissions: [], scopes: [], cells: [] }}
    {loading}
    hasMore={matrixQuery.hasNextPage && !matrixQuery.isFetchNextPageError}
    loadingMore={matrixQuery.isFetching}
    onLoadMore={() => matrixQuery.fetchNextPage()}
    updatingKey={visibleUpdatingKey}
    onCycle={handleCycle}
    subjectKind={m('rbac.permissions.cell.role_subject')}
    forceAllow={isOwnerRole}
    readOnly={isOwnerRole || visibleUpdatingKey !== null}
  />
{/if}
