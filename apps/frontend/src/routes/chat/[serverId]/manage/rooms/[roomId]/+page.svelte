<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
  import { onDestroy } from 'svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { createMutation, createQuery } from '@tanstack/svelte-query';
  import { serverIdToSegment } from '$lib/navigation';
  import { createAdminRoomLayoutAPI, type AdminManagedRoom } from '$lib/api-client/adminRoomLayout';
  import { createRoomCommandAPI } from '$lib/api-client/rooms';
  import { getChromePermissions } from '$lib/state/server/chromePermissions.svelte';
  import { useProjectionEvent } from '$lib/hooks';
  import { Button } from '$lib/ui/form';
  import AccessDenied from '$lib/ui/AccessDenied.svelte';
  import { EmptyState, PaneContent } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import Hint from '$lib/ui/Hint.svelte';
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';
  import { toast } from '$lib/ui/toast';
  import { classifyManagementLoadError } from '$lib/utils/managementLoadError';
  import { adminQueryKeys } from '$lib/query/admin';
  import { queryClient } from '$lib/query/client';
  import {
    invalidateAdminRoomLayoutQueries,
    purgeAdminRoomQuery
  } from '$lib/query/adminInvalidation';
  import { registerQueryCacheRemovalListener } from '$lib/query/cacheRegistry';
  import { invalidateRoomMemberQueries } from '$lib/query/roomMembers';
  import type { buildRoomSettingsUpdate } from './roomSettings';
  import RoomGeneralSettingsPanel from './RoomGeneralSettingsPanel.svelte';
  import RoomMembersPanel from './RoomMembersPanel.svelte';
  import * as m from '$lib/i18n/messages';

  const serverScope = useServerScope();

  const roomId = $derived(page.params.roomId!);
  const activeServerId = $derived(serverScope.serverId);
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  const getChromePermissionsState = getChromePermissions();
  const chromePermissions = $derived(getChromePermissionsState());

  let scrollContainer = $state<HTMLDivElement>();
  let privacyGeneration = 0;
  let snapshotGeneration = 0;
  let pendingMemberRevalidation: {
    serverId: string;
    queryScope: string;
    roomId: string;
  } | null = null;
  let formRevision = $state(0);
  const supportsAdminAPI = $derived(serverScope.store.serverInfo.supportsFeature('adminApi'));

  const removeCacheRemovalListener = registerQueryCacheRemovalListener((serverId) => {
    if (serverId === serverScope.serverId) privacyGeneration += 1;
  });

  onDestroy(() => {
    privacyGeneration += 1;
    snapshotGeneration += 1;
    pendingMemberRevalidation = null;
    removeCacheRemovalListener();
  });

  type RoomMutationScope = {
    serverId: string;
    connection: ServerConnection;
    roomId: string;
    queryKey: ReturnType<typeof adminQueryKeys.room>;
    api: ReturnType<typeof createRoomCommandAPI>;
    privacyGeneration: number;
    snapshotGeneration: number;
    input: ReturnType<typeof buildRoomSettingsUpdate>;
  };

  const roomQuery = createQuery(
    () => {
      const serverId = activeServerId;
      const connection = serverScope.connection;
      const targetRoomId = roomId;
      return {
        queryKey: adminQueryKeys.room(serverId, connection, targetRoomId),
        queryFn: async ({ signal }) => {
          const revalidation = pendingMemberRevalidation;
          const room = await connection
            .getAPI(createAdminRoomLayoutAPI)
            .getRoom(targetRoomId, { signal });
          if (
            !signal.aborted &&
            room &&
            revalidation !== null &&
            pendingMemberRevalidation === revalidation &&
            revalidation.serverId === serverId &&
            revalidation.queryScope === connection.queryScope &&
            revalidation.roomId === targetRoomId
          ) {
            pendingMemberRevalidation = null;
            void invalidateRoomMemberQueries(serverId, connection, targetRoomId);
          }
          return room;
        },
        enabled: supportsAdminAPI,
        refetchOnMount: 'always' as const
      };
    },
    () => queryClient
  );

  const room = $derived(roomQuery.data ?? null);
  const canManageRoom = $derived(room?.canManageRoom ?? false);
  const canManagePermissions = $derived(room?.canManagePermissions ?? false);
  const supportsMemberManagement = $derived(
    serverScope.store.serverInfo.supportsFeature('roomManagement')
  );
  const backHref = $derived(
    chromePermissions?.canManageRooms
      ? resolve('/chat/[serverId]/manage/rooms', { serverId: serverSegment })
      : resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId })
  );
  const loading = $derived(supportsAdminAPI && roomQuery.isPending);
  const classifiedLoadError = $derived(
    roomQuery.error ? classifyManagementLoadError(roomQuery.error) : null
  );
  const accessDenied = $derived(
    !supportsAdminAPI || classifiedLoadError?.kind === 'access-denied' || (!loading && !room)
  );
  const loadFailure = $derived(
    classifiedLoadError?.kind === 'failure' ? classifiedLoadError.message : null
  );

  function isCurrentRoom(variables: RoomMutationScope | undefined): boolean {
    return (
      variables !== undefined &&
      serverScope.isCurrent() &&
      variables.serverId === activeServerId &&
      variables.connection.queryScope === serverScope.connection.queryScope &&
      variables.roomId === roomId &&
      variables.privacyGeneration === privacyGeneration
    );
  }

  function canApplyRoomSnapshot(variables: RoomMutationScope): boolean {
    return isCurrentRoom(variables) && variables.snapshotGeneration === snapshotGeneration;
  }

  const updateRoomMutation = createMutation(
    () => ({
      mutationFn: async ({ api, input }: RoomMutationScope) => {
        const updated = await api.updateRoom(input);
        if (!updated) throw new Error('Room update returned no room');
        return updated;
      },
      onSuccess: (updated, variables) => {
        if (!isCurrentRoom(variables)) return;
        if (canApplyRoomSnapshot(variables)) {
          queryClient.setQueryData<AdminManagedRoom | null>(variables.queryKey, (current) =>
            current
              ? {
                  ...current,
                  name: updated.name,
                  description: updated.description || null,
                  isUniversal: updated.universal,
                  archived: updated.archived
                }
              : current
          );
          formRevision += 1;
        }
        invalidateAdminRoomLayoutQueries(
          variables.serverId,
          variables.connection,
          variables.roomId
        );
        void serverScope.store.adminRoomLayout.refresh();
        toast.success(m['admin.rooms_admin.room_updated']());
      },
      onError: (error, variables) => {
        if (!isCurrentRoom(variables)) return;
        toast.error(
          m['admin.rooms_admin.update_room_failed']({
            error: error instanceof Error ? error.message : String(error)
          })
        );
      }
    }),
    () => queryClient
  );

  function saveGeneralSettings(input: ReturnType<typeof buildRoomSettingsUpdate>): void {
    if (!canManageRoom || updateRoomMutation.isPending) return;
    const connection = serverScope.connection;
    updateRoomMutation.mutate({
      serverId: activeServerId,
      connection,
      roomId,
      queryKey: adminQueryKeys.room(activeServerId, connection, roomId),
      api: connection.getAPI(createRoomCommandAPI),
      privacyGeneration,
      snapshotGeneration,
      input
    });
  }

  useProjectionEvent((event) => {
    for (const operation of event.operations) {
      switch (operation.operation.case) {
        case 'roomUpsert':
          if (operation.operation.value.room?.room?.id === roomId) {
            snapshotGeneration += 1;
            invalidateAdminRoomLayoutQueries(activeServerId, serverScope.connection, roomId);
            return;
          }
          break;
        case 'roomRemove':
          if (operation.operation.value.roomId === roomId) {
            snapshotGeneration += 1;
            privacyGeneration += 1;
            pendingMemberRevalidation = {
              serverId: activeServerId,
              queryScope: serverScope.connection.queryScope,
              roomId
            };
            purgeAdminRoomQuery(activeServerId, serverScope.connection, roomId);
            return;
          }
          break;
      }
    }
  });

  const saving = $derived(
    updateRoomMutation.isPending && isCurrentRoom(updateRoomMutation.variables)
  );

  const pageTitle = $derived(
    room ? `#${room.name} · ${m['room_list.room_settings']()}` : m['room_list.room_settings']()
  );
</script>

<PageTitle title={m['admin.common.server_admin_page_title']({ title: pageTitle })} />

{#if loading}
  <!-- The management shell remains visible while the room capability loads. -->
{:else if loadFailure}
  <EmptyState icon="uil--exclamation-triangle" title={m['common.error.generic']()}>
    <div class="flex flex-col items-center gap-4">
      <p>{loadFailure}</p>
      <Button variant="secondary" onclick={() => void roomQuery.refetch()}>
        {m['common.retry']()}
      </Button>
    </div>
  </EmptyState>
{:else if accessDenied || !room || !canManagePermissions}
  <AccessDenied
    message={m['ui.access_denied.message']()}
    backHref={resolve('/chat/[serverId]', { serverId: serverSegment })}
    backLabel={m['admin.nav.back_to_server']()}
  />
{:else}
  <div class="pane-page">
    <PaneHeader
      title={`#${room.name}`}
      subtitle={m['room_list.room_settings']()}
      {backHref}
      showMobileNav
    />

    <PaneContent bind:scrollContainer>
      <div class="flex flex-col gap-6">
        {#if canManageRoom}
          {#key `${activeServerId}:${serverScope.connection.queryScope}:${room.id}:${formRevision}`}
            <RoomGeneralSettingsPanel {room} {saving} onSave={saveGeneralSettings} />
          {/key}
        {/if}

        {#if supportsMemberManagement}
          {#key `${activeServerId}:${serverScope.connection.queryScope}:${roomId}`}
            <RoomMembersPanel
              serverId={activeServerId}
              {roomId}
              roomName={room.name}
              isUniversal={room.isUniversal}
              archived={room.archived}
              canManageMembers={canManageRoom}
              scrollRoot={scrollContainer}
            />
          {/key}
        {/if}

        <div class="flex flex-col gap-4">
          <h2 class="text-lg font-semibold text-text-top">
            {m['admin.rooms_admin.room_permissions_title_fallback']()}
          </h2>
          <Hint>{m['admin.rooms_admin.room_permissions_hint']()}</Hint>
          <Hint>{m['admin.permissions.resolution_hint']()}</Hint>
          <PermissionMatrix {roomId} scrollContents={false} />
        </div>
      </div>
    </PaneContent>
  </div>
{/if}
