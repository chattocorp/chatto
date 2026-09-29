<!--
@component

Shared frame of the room management sections. It loads the room, follows the
realtime events that change or remove it, renders the header and the section
tabs, and provides the room to the General, Members, and Permissions pages
through `roomManagementContext`.
-->
<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createSessionGuard } from '$lib/state/server/sessionGuard.svelte';
  import { onDestroy, type Snippet } from 'svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { createAdminRoomLayoutAPI } from '$lib/api-client/adminRoomLayout';
  import { useProjectionEvent } from '$lib/hooks';
  import { Button } from '$lib/ui/form';
  import {
    AccessDenied,
    EmptyState,
    LoadingFog,
    PaneContent,
    PaneHeader,
    PageTitle,
    TabNav,
    type TabNavItem
  } from '$lib/ui';
  import { classifyManagementLoadError } from '$lib/utils/managementLoadError';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createQuery } from '$lib/query/client';
  import {
    invalidateAdminRoomLayoutQueries,
    purgeAdminRoomQuery
  } from '$lib/query/adminInvalidation';
  import { invalidateRoomMemberQueries } from '$lib/query/roomMembers';
  import { m } from '$lib/i18n/messages';
  import { provideRoomManagement, type RoomMutationScope } from './roomManagementContext';

  let { children }: { children: Snippet } = $props();

  const serverScope = useServerScope();

  const roomId = $derived(page.params.roomId!);
  const activeServerId = serverScope.serverId;
  const serverSegment = serverIdToSegment(activeServerId);

  let scrollContainer = $state<HTMLDivElement>();
  const session = createSessionGuard(serverScope);
  /** Increases when the room snapshot changes, so an older save does not overwrite it. */
  let snapshotGeneration = 0;
  let pendingMemberRevalidation: { roomId: string } | null = null;

  onDestroy(() => {
    pendingMemberRevalidation = null;
  });

  const roomQuery = createQuery(() => {
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
  });

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

  function isCurrentRoom(target: RoomMutationScope | undefined): target is RoomMutationScope {
    return session.isCurrent(target) && target.roomId === roomId;
  }

  provideRoomManagement({
    get roomId() {
      return roomId;
    },
    get room() {
      return room!;
    },
    get canManageRoom() {
      return canManageRoom;
    },
    get scrollContainer() {
      return scrollContainer;
    },
    mutationScope() {
      return { ...session.snapshot(), roomId, snapshotGeneration };
    },
    isCurrentRoom,
    canApplyRoomSnapshot(target) {
      return isCurrentRoom(target) && target.snapshotGeneration === snapshotGeneration;
    }
  });

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

  const sectionTabs = $derived.by<TabNavItem[]>(() => {
    if (!room) return [];
    const params = { serverId: serverSegment, roomId };
    const routeId = page.route.id;
    const base = '/chat/[serverId]/manage/rooms/[roomId]';
    const items: TabNavItem[] = [];
    if (canManageRoom) {
      items.push({
        href: resolve('/chat/[serverId]/manage/rooms/[roomId]', params),
        label: m('admin.rooms_admin.tabs.general'),
        icon: 'icon-[uil--sliders-v-alt]',
        current: routeId === base
      });
    }
    items.push(
      {
        href: resolve('/chat/[serverId]/manage/rooms/[roomId]/members', params),
        label: m('admin.rooms_admin.tabs.members'),
        icon: 'icon-[uil--users-alt]',
        current: routeId === `${base}/members`
      },
      {
        href: resolve('/chat/[serverId]/manage/rooms/[roomId]/permissions', params),
        label: m('admin.rooms_admin.tabs.permissions'),
        icon: 'icon-[uil--shield-check]',
        current: routeId === `${base}/permissions`
      }
    );
    return items;
  });

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
    >
      {#snippet tabs()}
        <TabNav label={m('admin.rooms_admin.tabs.label')} items={sectionTabs} />
      {/snippet}
    </PaneHeader>

    <PaneContent bind:scrollContainer>
      <div class="flex flex-col gap-6">
        {#if room}
          {@render children()}
        {:else}
          <LoadingFog class="h-40 w-full" />
        {/if}
      </div>
    </PaneContent>
  </div>
{/if}
