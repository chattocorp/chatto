<!--
@component

Bot permission editor. A bot has only direct decisions; the server intersects
its delegated result with the owner's live effective permission at every scope.
-->
<script lang="ts">
  import { onDestroy } from 'svelte';
  import { createQuery } from '@tanstack/svelte-query';
  import { createBotAPI, type BotPermissionScope } from '$lib/api-client/bots';
  import { m } from '$lib/i18n/messages';
  import { queryClient } from '$lib/query/client';
  import { settingsQueryKeys } from '$lib/query/settings';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Hint } from '$lib/ui';
  import { toast } from '$lib/ui/toast';
  import SubjectPermissionsMatrix, {
    type CellState,
    type MatrixData,
    type MatrixScope
  } from './SubjectPermissionsMatrix.svelte';

  let { botUserId }: { botUserId: string } = $props();
  const serverScope = useServerScope();

  const matrixQuery = createQuery(
    () => {
      const serverId = serverScope.serverId;
      const connection = serverScope.connection;
      const activeBotUserId = botUserId;
      return {
        queryKey: settingsQueryKeys.botPermissions(serverId, connection, activeBotUserId),
        queryFn: ({ signal }) =>
          connection.getAPI(createBotAPI).getPermissionMatrix(activeBotUserId, { signal })
      };
    },
    () => queryClient
  );

  const data = $derived.by<MatrixData | null>(() => {
    if (!matrixQuery.data) return null;
    return {
      applicablePermissions: matrixQuery.data.applicablePermissions,
      scopes: matrixQuery.data.scopes,
      cells: matrixQuery.data.cells.map((cell) => ({
        permission: cell.permission,
        scopeId: cell.scopeId,
        override: cell.configured,
        effective: cell.effectiveGranted ? 'ALLOW' : 'DENY',
        delegated: cell.delegated,
        ownerGranted: cell.ownerGranted
      }))
    };
  });
  let updatingKey = $state<string | null>(null);
  let mutationGeneration = 0;
  onDestroy(() => {
    mutationGeneration += 1;
  });

  function apiScope(scope: MatrixScope): BotPermissionScope {
    if (scope.kind === 'GROUP') return { tier: 'group', groupId: scope.id.replace(/^group:/, '') };
    if (scope.kind === 'ROOM') return { tier: 'room', roomId: scope.id.replace(/^room:/, '') };
    return { tier: 'server' };
  }

  async function handleCycle(scope: MatrixScope, permission: string, next: CellState) {
    if (updatingKey) return;
    const generation = ++mutationGeneration;
    const key = `${scope.id}::${permission}`;
    updatingKey = key;
    try {
      await serverScope.connection.getAPI(createBotAPI).setPermission({
        botUserId,
        permission,
        scope: apiScope(scope),
        decision: next === 'allow' ? 'ALLOW' : next === 'deny' ? 'DENY' : 'NONE'
      });
      if (generation !== mutationGeneration || !serverScope.isCurrent()) return;
      await queryClient.invalidateQueries({
        queryKey: settingsQueryKeys.botPermissions(
          serverScope.serverId,
          serverScope.connection,
          botUserId
        ),
        exact: true
      });
    } catch (error) {
      if (generation === mutationGeneration && serverScope.isCurrent()) {
        toast.error(
          error instanceof Error ? error.message : m('settings.bots.permissions.save_failed')
        );
      }
    } finally {
      if (generation === mutationGeneration) updatingKey = null;
    }
  }
</script>

<Hint tone="info">{m('settings.bots.permissions.owner_ceiling')}</Hint>
{#if matrixQuery.isPending}
  <div class="text-muted">{m('rbac.permissions.loading')}</div>
{:else if matrixQuery.error}
  <Hint tone="danger">{matrixQuery.error.message}</Hint>
{:else if data}
  <SubjectPermissionsMatrix
    {data}
    {updatingKey}
    onCycle={handleCycle}
    subjectKind={m('settings.bots.singular')}
    readOnly={updatingKey !== null}
  />
{:else}
  <Hint tone="info">{m('rbac.permissions.no_data')}</Hint>
{/if}
