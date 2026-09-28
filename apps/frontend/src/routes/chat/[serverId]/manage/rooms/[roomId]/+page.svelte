<script lang="ts">
  import { errorMessage } from '$lib/utils/errorMessage';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createSessionGuard, type SessionSnapshot } from '$lib/state/server/sessionGuard.svelte';
  import { onDestroy } from 'svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { createMutation, createQuery } from '@tanstack/svelte-query';
  import { serverIdToSegment } from '$lib/navigation';
  import { createAdminRoomLayoutAPI, type AdminManagedRoom } from '$lib/api-client/adminRoomLayout';
  import { createRoomCommandAPI } from '$lib/api-client/rooms';
  import { useProjectionEvent } from '$lib/hooks';
  import { Button } from '$lib/ui/form';
  import { AccessDenied, EmptyState, PaneContent, PaneHeader, PageTitle, Hint } from '$lib/ui';
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';
  import { toast } from '$lib/ui/toast';
  import { classifyManagementLoadError } from '$lib/utils/managementLoadError';
  import { adminQueryKeys } from '$lib/query/admin';
  import { queryClient } from '$lib/query/client';
  import {
    invalidateAdminRoomLayoutQueries,
    purgeAdminRoomQuery
  } from '$lib/query/adminInvalidation';
  import { invalidateRoomMemberQueries } from '$lib/query/roomMembers';
  import type { buildRoomSettingsUpdate } from './roomSettings';
  import RoomGeneralSettingsPanel from './RoomGeneralSettingsPanel.svelte';
  import RoomMembersPanel from './RoomMembersPanel.svelte';
  import { m } from '$lib/i18n/messages';

  const serverScope = useServerScope();

  const roomId = $derived(page.params.roomId!);
  const activeServerId = serverScope.serverId;
  const serverSegment = $derived(serverIdToSegment(activeServerId));

  let scrollContainer = $state<HTMLDivElement>();
  const session = createSessionGuard(serverScope);
  /** Increases when the room snapshot changes, so an older save does not overwrite it. */
  let snapshotGeneration = 0;
  let pendingMemberRevalidation: { roomId: string } | null = null;
  let formRevision = $state(0);

  onDestroy(() => {
    pendingMemberRevalidation = null;
  });

  type RoomMutationScope = SessionSnapshot & {
    roomId: string;
    queryKey: ReturnType<typeof adminQueryKeys.room>;
    api: ReturnType<typeof createRoomCommandAPI>;
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
            revalidation.roomId === targetRoomId
          ) {
            pendingMemberRevalidation = null;
            void invalidateRoomMemberQueries(serverId, connection, targetRoomId);
          }
          return room;
        },
        refetchOnMount: 'always' as const
      };
    },
    () => queryClient
  );

  const room = $derived(roomQuery.data ?? null);
  const canManageRoom = $derived(room?.canManageRoom ?? false);
  const canManagePermissions = $derived(room?.canManagePermissions ?? false);
  const backHref = $derived(
    serverScope.store.permissions.canManageRooms
      ? resolve('/chat/[serverId]/manage/rooms', { serverId: serverSegment })
      : resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId })
  );
  const loading = $derived(roomQuery.isPending);
  const classifiedLoadError = $derived(
    roomQuery.error ? classifyManagementLoadError(roomQuery.error) : null
  );
  const accessDenied = $derived(
    classifiedLoadError?.kind === 'access-denied' || (!loading && !room)
  );
  const loadFailure = $derived(
    classifiedLoadError?.kind === 'failure' ? classifiedLoadError.message : null
  );

  function isCurrentRoom(variables: RoomMutationScope | undefined): boolean {
    return session.isCurrent(variables) && variables.roomId === roomId;
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
                  slowModeSeconds: updated.slowModeSeconds,
                  threadingMode: updated.threadingMode,
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
        toast.success(m('admin.rooms_admin.room_updated'));
      },
      onError: (error, variables) => {
        if (!isCurrentRoom(variables)) return;
        toast.error(
          m('admin.rooms_admin.update_room_failed', {
            error: errorMessage(error)
          })
        );
      }
    }),
    () => queryClient
  );

  function saveGeneralSettings(input: ReturnType<typeof buildRoomSettingsUpdate>): void {
    if (!canManageRoom || updateRoomMutation.isPending) return;
    const snapshot = session.snapshot();
    updateRoomMutation.mutate({
      ...snapshot,
      roomId,
      queryKey: adminQueryKeys.room(snapshot.serverId, snapshot.connection, roomId),
      api: snapshot.connection.getAPI(createRoomCommandAPI),
      snapshotGeneration,
      input
    });
  }

  useProjectionEvent((event) => {
    if (event.resource?.case === 'rooms') {
      if (event.resource.value.rooms.some((room) => room.room?.id === roomId)) {
        snapshotGeneration += 1;
        invalidateAdminRoomLayoutQueries(activeServerId, serverScope.connection, roomId);
        return;
      }
      snapshotGeneration += 1;
      session.invalidate();
      purgeAdminRoomQuery(activeServerId, serverScope.connection, roomId);
      return;
    }
    if (event.event?.event.case === 'roomDeleted' && event.event.event.value.roomId === roomId) {
      snapshotGeneration += 1;
      session.invalidate();
      pendingMemberRevalidation = { roomId };
      purgeAdminRoomQuery(activeServerId, serverScope.connection, roomId);
      return;
    }
  });

  const saving = $derived(
    updateRoomMutation.isPending && isCurrentRoom(updateRoomMutation.variables)
  );

  const pageTitle = $derived(
    room ? `#${room.name} · ${m('room_list.room_settings')}` : m('room_list.room_settings')
  );
</script>

<PageTitle title={m('admin.common.server_admin_page_title', { title: pageTitle })} />

{#if !loading && loadFailure}
  <EmptyState icon="icon-[uil--exclamation-triangle]" title={m('common.error.generic')}>
    <div class="flex flex-col items-center gap-4">
      <p>{loadFailure}</p>
      <Button variant="secondary" onclick={() => void roomQuery.refetch()}>
        {m('common.retry')}
      </Button>
    </div>
  </EmptyState>
{:else if !loading && (accessDenied || !room || !canManagePermissions)}
  <AccessDenied
    message={m('ui.access_denied.message')}
    backHref={resolve('/chat/[serverId]', { serverId: serverSegment })}
    backLabel={m('admin.nav.back_to_server')}
  />
{:else}
  <div class="pane-page">
    <PaneHeader
      title={room ? `#${room.name}` : m('room_list.room_settings')}
      subtitle={m('room_list.room_settings')}
      {backHref}
    />

    <PaneContent bind:scrollContainer>
      <div class="flex flex-col gap-6">
        {#if room && canManageRoom}
          {#key `${room.id}:${formRevision}`}
            <RoomGeneralSettingsPanel {room} {saving} onSave={saveGeneralSettings} />
          {/key}
        {/if}

        {#if room}
          {#key roomId}
            <RoomMembersPanel
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
          <Hint>{m('admin.rooms_admin.room_permissions_hint')}</Hint>
          <Hint>{m('admin.permissions.resolution_hint')}</Hint>
          <PermissionMatrix
            {roomId}
            subtitle={m('admin.rooms_admin.room_permissions_subtitle')}
            scrollContents={false}
          />
        </div>
      </div>
    </PaneContent>
  </div>
{/if}
