<!--
@component

Per-user permission matrix loader for human and bot accounts. Owns the
canonical ConnectRPC query and mutation dispatch for cell clicks; delegates
rendering to `SubjectPermissionsMatrix`.
-->
<script lang="ts">
  import { onDestroy } from 'svelte';
  import { Hint } from '$lib/ui';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { loadAccountMemberships } from './accountMemberships';
  import { createRoomCommandAPI } from '$lib/api-client/rooms';
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
    type CellState,
    type DecisionMode
  } from './SubjectPermissionsMatrix.svelte';
  import { createQuery } from '@tanstack/svelte-query';
  import { adminQueryKeys } from '$lib/query/admin';
  import { queryClient } from '$lib/query/client';

  type Matrix = MatrixData & { userId: string };

  let {
    userId,
    subjectKind = 'user',
    ownerCapped = false,
    decisionMode = 'tri-state'
  }: {
    userId: string;
    subjectKind?: string;
    ownerCapped?: boolean;
    decisionMode?: DecisionMode;
  } = $props();

  const serverScope = useServerScope();

  const matrixQuery = createQuery(
    () => {
      const serverId = serverScope.serverId;
      const activeConnection = serverScope.connection;
      const activeUserId = userId;
      const isBot = ownerCapped;
      const canManageAccounts = serverScope.store.permissions.canAdminManageAccounts;
      return {
        queryKey: adminQueryKeys.userPermissions(serverId, activeConnection, activeUserId),
        queryFn: async ({ signal }) => {
          const matrix = await activeConnection
            .getAPI(createPermissionAPI)
            .getUserPermissionMatrix(activeUserId, { signal });
          if (!matrix) return null;
          return loadAccountMemberships(activeConnection, matrix, isBot, canManageAccounts, signal);
        }
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
    if (scope.kind === 'DM') return { tier: 'dm' };
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

  // Fence mutation feedback by server session and account identity. Reconcile after
  // both success and failure because a failed response can follow a committed write.
  async function handleMembershipChange(scope: MatrixScope, joined: boolean) {
    if (!data || visibleUpdatingKey || scope.kind !== 'ROOM' || !scope.membership) return;
    if (joined ? !scope.membership.canJoin : !scope.membership.canLeave) return;
    const generation = ++mutationGeneration;
    const serverId = serverScope.serverId;
    const activeConnection = serverScope.connection;
    const activeUserId = data.userId;
    const context = JSON.stringify([serverId, activeConnection.queryScope, activeUserId]);
    const queryKey = adminQueryKeys.userPermissions(serverId, activeConnection, activeUserId);
    updatingKey = `${scope.id}::$membership`;
    mutationContext = context;
    mutationError = null;
    const current = () =>
      mutationGeneration === generation &&
      serverScope.isCurrent() &&
      context === activeMutationContext;
    try {
      await queryClient.cancelQueries({ queryKey, exact: true });
      const api = activeConnection.getAPI(createRoomCommandAPI);
      const input = { roomId: scope.id.slice('room:'.length), userId: activeUserId };
      if (joined) await api.addMember(input);
      else await api.removeMember(input);
      if (current()) toast.success(m('common.saved'));
    } catch {
      if (current()) {
        const message = m(joined ? 'room.join.failed' : 'room.leave.failed');
        mutationError = { context, message };
        toast.error(message);
      }
    } finally {
      if (serverScope.isCurrent()) await queryClient.invalidateQueries({ queryKey, exact: true });
      if (mutationGeneration === generation) updatingKey = null;
    }
  }

  async function handleCycle(scope: MatrixScope, permission: string, next: CellState) {
    if (!data || visibleUpdatingKey) return;
    const refreshMembership = data.scopes.some((scope) => scope.membership);
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

    if (result.update) {
      const decision = result.update.decision;
      queryClient.setQueryData<Matrix | null>(queryKey, (current) =>
        current
          ? {
              ...current,
              cells: current.cells.map((cell) =>
                cell.scopeId === scope.id && cell.permission === permission
                  ? { ...cell, override: decision }
                  : cell
              )
            }
          : current
      );
    }
    void queryClient.invalidateQueries({
      queryKey,
      exact: true,
      // Binary permissions derive inheritance locally. Bot membership actions
      // also depend on effective permissions, so refresh those from the server.
      refetchType: decisionMode === 'binary' && !refreshMembership ? 'none' : 'active'
    });
    if (!serverScope.isCurrent()) return;
    if (mutationGeneration === generation) updatingKey = null;
  }
</script>

{#if ownerCapped}
  <Hint tone="info">{m('settings.bots.permissions.owner_ceiling')}</Hint>
{/if}

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
    {subjectKind}
    readOnly={visibleUpdatingKey !== null &&
      (decisionMode === 'tri-state' || visibleUpdatingKey.endsWith('::$membership'))}
    onMembershipChange={handleMembershipChange}
    {decisionMode}
  />
{/if}
