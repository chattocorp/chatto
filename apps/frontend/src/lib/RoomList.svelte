<!--
@component

Renders the room list in the server sidebar. When a room layout is configured,
rooms are organized into collapsible sections. Otherwise, rooms display alphabetically.
-->
<script lang="ts">
  import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { goto, pushState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverIdToSegment } from '$lib/navigation';
  import { m } from '$lib/i18n/messages';
  import {
    sidebarLinkAnchorAttributes,
    sidebarLinkTarget
  } from '$lib/navigation/sidebarLinkTarget';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import CollapsibleGroup from '$lib/ui/CollapsibleGroup.svelte';
  import RoomGroupSection from '$lib/components/chat/RoomGroupSection.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import { serverStorageKey } from '$lib/storage/serverStorage';
  import { buildDirectMessagePresentation, type UserAvatarUserView } from '$lib/render/users';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import NotificationBadge from '$lib/ui/NotificationBadge.svelte';
  import UnreadDot from '$lib/ui/UnreadDot.svelte';
  import { notificationTarget } from '$lib/state/server/notifications.svelte';
  import { prepareUiForNotificationTarget } from '$lib/notifications/notificationNavigationUi';
  import { getAppUiState, getRoomSidebarPresentation } from '$lib/state/appUi.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import {
    isNavigationVisibleRoom,
    type RoomsListItem,
    type RoomsListGroup,
    type RoomsListGroupItem
  } from '$lib/state/server/rooms.svelte';
  import type { CallRoomParticipant } from '$lib/state/server/activeCallRooms.svelte';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import NavigationContextMenu from '$lib/components/menus/NavigationContextMenu.svelte';
  import {
    contextMenuTrigger,
    type ContextMenuTriggerDetails
  } from '$lib/ui/contextMenuTrigger.svelte';
  import { markNavigationRoomAsRead } from '$lib/navigation/readActions';
  import { toast } from '$lib/ui/toast';

  // No props — RoomList reads everything from the route's server scope.
  // All store references go through `stores` ($derived), so when the URL
  // [serverId] param changes, every derived read in the template re-evaluates
  // against the new server's state automatically.

  const serverScope = useServerScope();
  const activeServerId = $derived(serverScope.serverId);
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  const activeServer = $derived(serverRegistry.getServer(activeServerId));
  const activeServerBaseURL = $derived(activeServer?.url ?? null);
  const stores = $derived(serverScope.store);
  const notificationStore = $derived(stores.notifications);
  const notificationLevelStore = $derived(stores.notificationLevels);
  const activeCallRooms = $derived(stores.activeCallRooms);
  const appUi = getAppUiState();

  const navigation = $derived(stores.navigation);
  const roomUnreadStore = $derived(stores.roomUnread);

  let activeRoomId = $derived(page.params.roomId);
  let roomContextMenu = $state<(ContextMenuTriggerDetails & { room: RoomsListItem }) | null>(null);
  let groupContextMenu = $state<(ContextMenuTriggerDetails & { group: RoomsListGroup }) | null>(
    null
  );

  function roomMenuTrigger(room: RoomsListItem) {
    return contextMenuTrigger((details) => {
      roomContextMenu = { ...details, room };
    });
  }

  function groupMenuTrigger(group: RoomsListGroup) {
    if (!group.viewerCanManageGroup) return undefined;
    return contextMenuTrigger((details) => {
      groupContextMenu = { ...details, group };
    });
  }

  function handleConfigureGroup(group: RoomsListGroup): void {
    groupContextMenu = null;
    void goto(
      resolve('/chat/[serverId]/manage/room-groups/[groupId]', {
        serverId: serverSegment,
        groupId: group.id
      })
    );
  }

  function handleMarkRoomRead(room: RoomsListItem): void {
    roomContextMenu = null;
    void markNavigationRoomAsRead(activeServerId, room.id);
  }

  function handleLeaveRoom(room: RoomsListItem): void {
    roomContextMenu = null;
    pushState('', {
      modal: {
        type: 'leaveRoom',
        serverId: activeServerId,
        roomId: room.id,
        roomName: room.name
      }
    });
  }

  function handleConfigureRoom(room: RoomsListItem): void {
    roomContextMenu = null;
    void goto(
      resolve('/chat/[serverId]/manage/rooms/[roomId]', {
        serverId: serverSegment,
        roomId: room.id
      })
    );
  }

  async function handleJoinRoom(room: RoomsListItem): Promise<void> {
    roomContextMenu = null;
    const result = await stores.roomDirectory.joinRoom(room.id);
    if (!serverScope.isCurrent()) return;
    if (result.ok) {
      toast.success(m('room.join.success', { room: room.name }));
      return;
    }

    toast.error(m('room.join.failed'));
    console.error('Error joining room:', result.error);
  }

  // --- Derived layout helpers ---

  // Channels and DMs are stored together, but rendered as separate groups.
  // Room sets only apply to channels — DM rooms always render in their
  // own group below.
  let channels = $derived(navigation.rooms.filter((r) => r.type === RoomKind.CHANNEL));
  let dmRooms = $derived(
    navigation.rooms.filter((r) => r.type === RoomKind.DM && isNavigationVisibleRoom(r))
  );

  let channelMap = $derived(new Map(channels.map((r) => [r.id, r])));

  function getSetItems(set: RoomsListGroup): RoomsListGroupItem[] {
    const items =
      set.items ??
      set.roomIds.map((roomId) => ({
        id: `room:${roomId}`,
        type: 'room' as const,
        roomId
      }));
    return items.filter((item) => item.type === 'link' || channelMap.has(item.roomId));
  }

  // Keep manageable groups discoverable even when none of their rooms are
  // visible to the viewer.
  let visibleSets = $derived.by(() => {
    const sets = navigation.roomGroups;
    return sets.filter((s) => s.viewerCanManageGroup || getSetItems(s).length > 0);
  });

  // When no layout exists, display channels alphabetically
  let sortedRooms = $derived([...channels].sort((a, b) => a.name.localeCompare(b.name)));

  // The viewer ID and DM members must come from the same server projection.
  // Reading the viewer ID from a global auth context here is unsafe — the
  // [serverId] layout intentionally renders children while the per-instance
  // CurrentUserState is still loading.
  function dmPresentation(room: RoomsListItem) {
    return buildDirectMessagePresentation(
      room.members,
      navigation.currentUserId,
      m('common.you'),
      getLiveDisplayName
    );
  }

  function callParticipantAvatarUser(participant: CallRoomParticipant): UserAvatarUserView {
    return {
      id: participant.userId,
      login: participant.login,
      displayName: participant.displayName,
      deleted: false,
      avatarUrl: participant.avatarUrl,
      presenceStatus: PresenceStatus.OFFLINE
    };
  }

  // Keep active rooms and rooms needing attention visible when their group is collapsed.
  function isHighlighted(room: RoomsListItem): boolean {
    return (
      room.id === activeRoomId ||
      activeCallRooms.has(room.id) ||
      roomUnreadStore.roomIsUnread(room.id) ||
      room.viewerNotificationCount > 0
    );
  }

  function isGroupItemHighlighted(item: RoomsListGroupItem): boolean {
    if (item.type === 'link') return false;
    const room = channelMap.get(item.roomId);
    return room ? isHighlighted(room) : false;
  }

  function wasCallIconClick(event: MouseEvent): boolean {
    const target = event.target;
    return target instanceof Element && target.closest('[data-testid="room-call-icon"]') !== null;
  }

  async function openRoomCallPanel(roomId: string): Promise<void> {
    appUi.requestRoomSidebarPanel(activeServerId, roomId, 'call', getRoomSidebarPresentation());
    await goto(resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId }));
  }

  function handleRoomLinkClick(event: MouseEvent, room: RoomsListItem): void {
    if (room.viewerIsMember && activeCallRooms.has(room.id) && wasCallIconClick(event)) {
      event.preventDefault();
      void openRoomCallPanel(room.id);
    }
  }

  function handleRoomLinkKeydown(event: KeyboardEvent, room: RoomsListItem): void {
    if (event.target !== event.currentTarget) return;
    if (!room.viewerIsMember) return;
    if (!activeCallRooms.has(room.id)) return;
    if (event.key !== 'Enter' && event.key !== ' ') return;

    event.preventDefault();
    void openRoomCallPanel(room.id);
  }

  async function handleNotificationBadgeClick(event: MouseEvent, roomId: string, isDM: boolean) {
    event.preventDefault();
    event.stopPropagation();

    const lookup = await notificationStore.resolveRoomNotification(roomId, { isDM });
    if (!serverScope.isCurrent()) return;
    const notification = lookup.notification;

    if (!notification) {
      if (!lookup.ok || lookup.totalCount !== 0) {
        await goto(resolve('/chat/notifications'));
      }
      return;
    }

    const target = notificationTarget(notification);
    prepareUiForNotificationTarget(appUi, activeServerId, target);
    if (target.eventId && target.roomId) {
      stores.pendingHighlights.set(target.roomId, target.threadRootId, target.eventId);
    }
    void notificationStore.dismiss(notification.id);

    const path = notificationStore.getCleanPath(activeServerId, notification);
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- getCleanPath() returns a resolved app path
    await goto(path);
  }
</script>

{#snippet activeCallIcon()}
  <span
    class="relative sidebar-icon text-action"
    aria-label={m('room_list.active_call')}
    data-testid="room-call-icon"
  >
    <span class="relative inline-flex">
      <span
        class="absolute inset-0 icon-[uil--phone] pane-header-icon-glyph animate-ping opacity-45"
        aria-hidden="true"
        data-testid="active-call-pulse-icon"
      ></span>
      <span class="relative icon-[uil--phone] pane-header-icon-glyph text-action" aria-hidden="true"
      ></span>
    </span>
  </span>
{/snippet}

{#snippet activeCallParticipants(roomId: string)}
  {@const participants = activeCallRooms.getParticipants(roomId)}
  {#if participants.length > 0}
    <div
      class="hidden shrink-0 items-center -space-x-1 @min-[220px]:flex"
      aria-label={m('room_list.call_participants', { count: participants.length })}
      data-testid="room-call-participants"
    >
      {#each participants.slice(0, 4) as participant, i (participant.userId)}
        <span
          class={[
            'inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full ring-1 ring-background',
            i === 2 ? 'hidden @min-[280px]:inline-flex' : '',
            i === 3 ? 'hidden @min-[340px]:inline-flex' : ''
          ]}
          data-testid="room-call-participant-avatar"
        >
          <UserAvatar user={callParticipantAvatarUser(participant)} size="xs" />
        </span>
      {/each}
      {#if participants.length > 4}
        <span
          class="hidden h-5 min-w-5 shrink-0 items-center justify-center rounded-full bg-surface-emphasized px-1 text-[10px] leading-none font-medium text-muted ring-1 ring-background @min-[380px]:inline-flex"
          data-testid="room-call-overflow"
        >
          +{participants.length - 4}
        </span>
      {/if}
    </div>
  {/if}
{/snippet}

{#snippet navigationRoomLink(room: RoomsListItem)}
  {@const isDM = room.type === RoomKind.DM}
  {@const hasActiveCall = activeCallRooms.has(room.id)}
  {@const hasUnread = roomUnreadStore.roomIsUnread(room.id)}
  {@const isJoined = room.viewerIsMember}
  {@const isMuted = !isDM && notificationLevelStore.isRoomMuted(room.id)}
  {@const showUnread = hasUnread && (isDM || (isJoined && !isMuted))}
  {@const showActiveCall = hasActiveCall && (isDM || isJoined)}
  {@const presentation = isDM ? dmPresentation(room) : null}
  <a
    href={resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId: room.id })}
    class={[
      'group/badges @container sidebar-item',
      room.id === activeRoomId ? 'bg-surface' : '',
      showUnread && room.id !== activeRoomId ? 'sidebar-item-attention' : '',
      !isDM && !isJoined ? 'opacity-60 hover:opacity-85' : ''
    ]}
    aria-current={room.id === activeRoomId ? 'page' : undefined}
    onclick={(e) => handleRoomLinkClick(e, room)}
    onkeydown={(e) => handleRoomLinkKeydown(e, room)}
    {@attach roomMenuTrigger(room)}
  >
    {#if presentation}
      <div class="flex shrink-0 -space-x-1">
        {#each presentation.visibleParticipants.slice(0, 3) as participant (participant.id)}
          <UserAvatar user={participant} size="xs" />
        {/each}
      </div>
      <span class="flex-1 truncate">{presentation.label}</span>
    {:else}
      {#if isJoined}
        {#if room.isUniversal}
          <span
            class={[
              'iconify sidebar-icon icon-[uil--globe]',
              showUnread && room.id !== activeRoomId ? 'text-text-top' : 'text-muted'
            ]}
            role="img"
            aria-label={m('room.directory.universal')}
            title={m('room.directory.universal_title')}
          ></span>
        {:else}
          <span
            class={[
              'sidebar-icon',
              showUnread && room.id !== activeRoomId ? 'text-text-top' : 'text-muted'
            ]}>#</span
          >
        {/if}
      {:else if room.viewerCanJoinRoom}
        <span class="sidebar-icon text-muted">+</span>
      {:else}
        <span class="iconify sidebar-icon icon-[uil--lock] text-muted"></span>
      {/if}
      <span class="flex-1 truncate">{room.name}</span>
    {/if}
    {#if showActiveCall}
      {@render activeCallParticipants(room.id)}
      {@render activeCallIcon()}
    {/if}

    {#if (isDM || isJoined) && room.viewerNotificationCount > 0}
      <button
        type="button"
        onclick={(e) => handleNotificationBadgeClick(e, room.id, isDM)}
        class="flex h-6 min-w-6 cursor-pointer items-center justify-center notification-dot"
        aria-label={isDM
          ? m('room_list.go_to_dm_notifications', { count: room.viewerNotificationCount })
          : m('room_list.go_to_notifications', { count: room.viewerNotificationCount })}
      >
        <NotificationBadge
          count={room.viewerNotificationCount}
          testid={isDM ? 'dm-notification-badge' : 'room-notification-badge'}
        />
      </button>
      <span class="sr-only">
        {isDM
          ? m('room_list.new_direct_messages', { count: room.viewerNotificationCount })
          : m('room_list.notifications', { count: room.viewerNotificationCount })}
      </span>
    {:else if showUnread}
      <UnreadDot color="neutral" testid={isDM ? 'dm-unread-dot' : 'room-unread-dot'} />
      <span class="sr-only">{m('room_list.unread_messages')}</span>
    {/if}
  </a>
{/snippet}

{#snippet sidebarLink(item: RoomsListGroupItem)}
  {#if item.type === 'room'}
    {@const room = channelMap.get(item.roomId)}
    {#if room}
      {@render navigationRoomLink(room)}
    {/if}
  {:else}
    {@const target = sidebarLinkTarget(item.link.url, activeServerBaseURL)}
    <a
      {...sidebarLinkAnchorAttributes(target)}
      aria-disabled={!target.valid}
      class={['sidebar-item w-full text-left', !target.valid && 'cursor-not-allowed opacity-60']}
      onclick={(event) => {
        if (!target.valid) event.preventDefault();
      }}
    >
      <span class="iconify sidebar-icon icon-[uil--external-link-alt] text-muted"></span>
      <span class="flex-1 truncate">{item.link.label}</span>
    </a>
  {/if}
{/snippet}

{#if channels.length === 0 && dmRooms.length === 0 && visibleSets.length === 0 && !navigation.isInitialLoading}
  <EmptyState icon="icon-[uil--comments]" title={m('room_list.empty_title')}>
    {m('room_list.empty_prefix')}
    <a href={resolve('/chat/[serverId]/overview', { serverId: serverSegment })} class="link"
      >{m('room_list.empty_overview')}</a
    >
    {m('room_list.empty_suffix')}
  </EmptyState>
{:else}
  <nav class="room-list md:w-full">
    {#if navigation.roomGroups.length > 0}
      {#each visibleSets as set, i (set.id)}
        <RoomGroupSection
          label={set.name}
          items={getSetItems(set)}
          item={sidebarLink}
          persistKey={serverStorageKey(activeServerId, `collapsible:set:${set.id}`)}
          keepVisibleWhenCollapsed={isGroupItemHighlighted}
          contextMenuTrigger={groupMenuTrigger(set)}
          separated={i > 0}
        />
      {/each}
    {:else if sortedRooms.length > 0}
      <!-- No layout configured yet — alphabetical fallback. -->
      <div class="p-2">
        <CollapsibleGroup
          label={m('common.rooms')}
          items={sortedRooms}
          item={navigationRoomLink}
          persistKey={serverStorageKey(activeServerId, 'collapsible:rooms')}
          keepVisibleWhenCollapsed={isHighlighted}
        />
      </div>
    {/if}

    {#if dmRooms.length > 0}
      <div class="px-2 pb-2">
        <CollapsibleGroup
          label={m('room_list.direct_messages')}
          items={dmRooms}
          item={navigationRoomLink}
          persistKey={serverStorageKey(activeServerId, 'collapsible:dms')}
          keepVisibleWhenCollapsed={isHighlighted}
          class={visibleSets.length > 0 || sortedRooms.length > 0 ? 'mt-4' : ''}
        />
      </div>
    {/if}
  </nav>
{/if}

{#if groupContextMenu}
  {@const contextGroup = groupContextMenu.group}
  <ContextMenu
    position={groupContextMenu.position}
    presentation={groupContextMenu.presentation}
    ariaLabel={m('room_list.group_settings', { group: contextGroup.name })}
    onclose={() => (groupContextMenu = null)}
  >
    <div class="menu-section">
      <nav class="sidebar-nav">
        <button
          type="button"
          class="sidebar-item"
          onclick={() => handleConfigureGroup(contextGroup)}
          role="menuitem"
        >
          <span class="iconify sidebar-icon icon-[uil--setting]" aria-hidden="true"></span>
          {m('room_list.group_settings', { group: contextGroup.name })}
        </button>
      </nav>
    </div>
  </ContextMenu>
{/if}

{#if roomContextMenu}
  {@const contextRoom = roomContextMenu.room}
  <ContextMenu
    position={roomContextMenu.position}
    presentation={roomContextMenu.presentation}
    ariaLabel={m('room_list.room_actions', { room: contextRoom.name })}
    onclose={() => (roomContextMenu = null)}
  >
    <NavigationContextMenu
      kind="room"
      isRoomMember={contextRoom.viewerIsMember}
      canJoin={contextRoom.viewerCanJoinRoom}
      canMarkRead={roomUnreadStore.roomIsUnread(contextRoom.id) ||
        contextRoom.viewerNotificationCount > 0}
      canConfigure={contextRoom.viewerCanManageRoom && contextRoom.type !== RoomKind.DM}
      canLeave={!contextRoom.isUniversal && contextRoom.type !== RoomKind.DM}
      onJoin={() => void handleJoinRoom(contextRoom)}
      onMarkRead={() => handleMarkRoomRead(contextRoom)}
      onConfigure={() => handleConfigureRoom(contextRoom)}
      onLeave={() => handleLeaveRoom(contextRoom)}
    />
  </ContextMenu>
{/if}
