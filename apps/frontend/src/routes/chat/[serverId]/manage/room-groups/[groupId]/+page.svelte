<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { createMutation, createQuery } from '@tanstack/svelte-query';
  import { onDestroy } from 'svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import {
    createAdminRoomLayoutAPI,
    type AdminManagedRoomGroup
  } from '$lib/api-client/adminRoomLayout';
  import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Button } from '$lib/ui/form';
  import AccessDenied from '$lib/ui/AccessDenied.svelte';
  import { EmptyState } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import Hint from '$lib/ui/Hint.svelte';
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';
  import { useProjectionEvent } from '$lib/hooks';
  import { toast } from '$lib/ui/toast';
  import { classifyManagementLoadError } from '$lib/utils/managementLoadError';
  import { adminQueryKeys } from '$lib/query/admin';
  import { queryClient } from '$lib/query/client';
  import {
    invalidateAdminRoomLayoutQueries,
    purgeAdminRoomGroupQuery
  } from '$lib/query/adminInvalidation';
  import { registerQueryCacheRemovalListener } from '$lib/query/cacheRegistry';
  import RoomGroupGeneralSettingsPanel from './RoomGroupGeneralSettingsPanel.svelte';
  import type { buildRoomGroupSettingsUpdate } from './roomGroupSettings';
  import * as m from '$lib/i18n/messages';

  const serverScope = useServerScope();
  const groupId = $derived(page.params.groupId!);
  const activeServerId = $derived(serverScope.serverId);
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  const backHref = $derived(resolve('/chat/[serverId]/manage/rooms', { serverId: serverSegment }));

  const supportsAdminAPI = $derived(serverScope.store.serverInfo.supportsFeature('adminApi'));
  let privacyGeneration = 0;
  let snapshotGeneration = 0;
  let formRevision = $state(0);
  const removeCacheRemovalListener = registerQueryCacheRemovalListener((serverId) => {
    if (serverId === serverScope.serverId) privacyGeneration += 1;
  });

  onDestroy(() => {
    privacyGeneration += 1;
    snapshotGeneration += 1;
    removeCacheRemovalListener();
  });

  type GroupMutationScope = {
    serverId: string;
    connection: ServerConnection;
    groupId: string;
    queryKey: ReturnType<typeof adminQueryKeys.roomGroup>;
    api: ReturnType<typeof createAdminRoomLayoutAPI>;
    privacyGeneration: number;
    snapshotGeneration: number;
    input: ReturnType<typeof buildRoomGroupSettingsUpdate>;
  };

  const groupQuery = createQuery(
    () => {
      const serverId = activeServerId;
      const connection = serverScope.connection;
      const targetGroupId = groupId;
      return {
        queryKey: adminQueryKeys.roomGroup(serverId, connection, targetGroupId),
        queryFn: ({ signal }) =>
          connection.getAPI(createAdminRoomLayoutAPI).getRoomGroup(targetGroupId, { signal }),
        enabled: supportsAdminAPI,
        refetchOnMount: 'always' as const
      };
    },
    () => queryClient
  );

  const groupDetails = $derived(groupQuery.data ?? null);
  const group = $derived(groupDetails?.group ?? null);
  const canManageGroup = $derived(groupDetails?.canManageGroup ?? false);
  const canManagePermissions = $derived(groupDetails?.canManagePermissions ?? false);
  const loading = $derived(supportsAdminAPI && groupQuery.isPending);
  const classifiedLoadError = $derived(
    groupQuery.error ? classifyManagementLoadError(groupQuery.error) : null
  );
  const accessDenied = $derived(
    !supportsAdminAPI || classifiedLoadError?.kind === 'access-denied' || (!loading && !group)
  );
  const loadFailure = $derived(
    classifiedLoadError?.kind === 'failure' ? classifiedLoadError.message : null
  );

  function isCurrentGroup(variables: GroupMutationScope | undefined): boolean {
    return (
      variables !== undefined &&
      serverScope.isCurrent() &&
      variables.serverId === activeServerId &&
      variables.connection.queryScope === serverScope.connection.queryScope &&
      variables.groupId === groupId &&
      variables.privacyGeneration === privacyGeneration
    );
  }

  function canApplyGroupSnapshot(variables: GroupMutationScope): boolean {
    return isCurrentGroup(variables) && variables.snapshotGeneration === snapshotGeneration;
  }

  const updateGroupMutation = createMutation(
    () => ({
      mutationFn: async ({ api, input }: GroupMutationScope) => {
        const updated = await api.updateRoomGroup(input);
        if (!updated) throw new Error('Room group update returned no group');
        return updated;
      },
      onSuccess: (updated, variables) => {
        if (!isCurrentGroup(variables)) return;
        if (canApplyGroupSnapshot(variables)) {
          queryClient.setQueryData<AdminManagedRoomGroup | null>(variables.queryKey, (current) =>
            current ? { ...current, group: updated } : current
          );
          formRevision += 1;
        }
        invalidateAdminRoomLayoutQueries(
          variables.serverId,
          variables.connection,
          undefined,
          variables.groupId
        );
        void serverScope.store.adminRoomLayout.refresh();
        toast.success(m['admin.rooms_admin.group_renamed']());
      },
      onError: (error, variables) => {
        if (!isCurrentGroup(variables)) return;
        toast.error(
          m['admin.rooms_admin.rename_group_failed']({
            error: error instanceof Error ? error.message : String(error)
          })
        );
      }
    }),
    () => queryClient
  );

  function saveGeneralSettings(input: ReturnType<typeof buildRoomGroupSettingsUpdate>): void {
    if (!canManageGroup || updateGroupMutation.isPending) return;
    const connection = serverScope.connection;
    updateGroupMutation.mutate({
      serverId: activeServerId,
      connection,
      groupId,
      queryKey: adminQueryKeys.roomGroup(activeServerId, connection, groupId),
      api: connection.getAPI(createAdminRoomLayoutAPI),
      privacyGeneration,
      snapshotGeneration,
      input
    });
  }

  useProjectionEvent((event) => {
    for (const operation of event.operations) {
      if (operation.operation.case !== 'roomGroupsReplace') continue;
      snapshotGeneration += 1;
      if (operation.operation.value.groups.some((candidate) => candidate.id === groupId)) {
        invalidateAdminRoomLayoutQueries(
          activeServerId,
          serverScope.connection,
          undefined,
          groupId
        );
      } else {
        privacyGeneration += 1;
        purgeAdminRoomGroupQuery(activeServerId, serverScope.connection, groupId);
      }
      return;
    }
  });

  const saving = $derived(
    updateGroupMutation.isPending && isCurrentGroup(updateGroupMutation.variables)
  );

  const pageTitle = $derived(
    group
      ? `${group.name} · ${m['admin.rooms_admin.rename_group']()}`
      : m['admin.rooms_admin.rename_group']()
  );
</script>

<PageTitle title={m['admin.common.server_admin_page_title']({ title: pageTitle })} />

{#if loading}
  <!-- The management shell remains visible while the room group loads. -->
{:else if loadFailure}
  <EmptyState icon="uil--exclamation-triangle" title={m['common.error.generic']()}>
    <div class="flex flex-col items-center gap-4">
      <p>{loadFailure}</p>
      <Button variant="secondary" onclick={() => void groupQuery.refetch()}>
        {m['common.retry']()}
      </Button>
    </div>
  </EmptyState>
{:else if accessDenied || !group || !canManagePermissions}
  <AccessDenied
    message={m['ui.access_denied.message']()}
    backHref={resolve('/chat/[serverId]', { serverId: serverSegment })}
    backLabel={m['admin.nav.back_to_server']()}
  />
{:else}
  <div class="pane-page">
    <PaneHeader
      title={group.name}
      subtitle={m['admin.rooms_admin.rename_group']()}
      {backHref}
      backLabel={m['admin.rooms_admin.back_to_rooms']()}
      showMobileNav
    />

    <div class="flex flex-col gap-6 overflow-y-auto p-6">
      {#if canManageGroup}
        {#key `${activeServerId}:${serverScope.connection.queryScope}:${group.id}:${formRevision}`}
          <RoomGroupGeneralSettingsPanel {group} {saving} onSave={saveGeneralSettings} />
        {/key}
      {/if}

      <div class="flex flex-col gap-4">
        <h2 class="text-lg font-semibold text-text-top">
          {m['admin.rooms_admin.group_permissions_title_fallback']()}
        </h2>
        <Hint>{m['admin.rooms_admin.group_permissions_hint']()}</Hint>
        <Hint>{m['admin.permissions.resolution_hint']()}</Hint>
        <PermissionMatrix {groupId} />
      </div>
    </div>
  </div>
{/if}
