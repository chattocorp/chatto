<!--
@component

Renders the room list in the server sidebar. When a room layout is configured,
rooms are organized into collapsible sections. Otherwise, rooms display alphabetically.
-->
<script lang="ts">
  import { goto, pushState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverIdToSegment } from '$lib/navigation';
  import * as m from '$lib/i18n/messages';
  import {
    sidebarLinkAnchorAttributes,
    sidebarLinkTarget
  } from '$lib/navigation/sidebarLinkTarget';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import CollapsibleGroup from '$lib/ui/CollapsibleGroup.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import {
    roomSidebarPanelStorageSuffix,
    setPendingRoomSidebarPanel,
    setRoomSidebarPanel
  } from '$lib/storage/roomSidebarPanel';
  import { serverStorageKey } from '$lib/storage/serverStorage';
  import { PresenceStatus, RoomType, type UserAvatarUserView } from '$lib/render/types';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import NotificationBadge from '$lib/ui/NotificationBadge.svelte';
  import UnreadDot from '$lib/ui/UnreadDot.svelte';
  import { notificationTarget } from '$lib/state/server/notifications.svelte';
  import { prepareUiForNotificationTarget } from '$lib/notifications/notificationNavigationUi';
  import { getAppUiState } from '$lib/state/appUi.svelte';
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

  // No props — RoomList reads everything from the active server's stores.
  // All store references go through `stores` ($derived), so when the active
  // server changes (URL [serverId] param changes), every derived read in the
  // template re-evaluates against the new server's state automatically.

  const activeServerId = $derived(getActiveServer());
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  const activeServer = $derived(serverRegistry.getServer(activeServerId));
  const activeServerBaseURL = $derived(activeServer?.url ?? null);
  const stores = $derived(serverRegistry.getStore(activeServerId));
  const notificationStore = $derived(stores.notifications);
  const notificationLevelStore = $derived(stores.notificationLevels);
  const activeCallRooms = $derived(stores.activeCallRooms);
  const appUi = getAppUiState();

  const roomsStore = $derived(stores.rooms);
  const roomUnreadStore = $derived(stores.roomUnread);

  let activeRoomId = $derived(page.params.roomId);
  let roomContextMenu = $state<
    (ContextMenuTriggerDetails & { room: RoomsListItem }) | null
  >(null);

  function roomMenuTrigger(room: RoomsListItem) {
    return contextMenuTrigger((details) => {
      roomContextMenu = { ...details, room };
    });
  }

  function closeRoomContextMenu(): void {
    roomContextMenu = null;
  }

  function handleMarkRoomRead(room: RoomsListItem): void {
    closeRoomContextMenu();
    void markNavigationRoomAsRead(activeServerId, room.id);
  }

  function handleLeaveRoom(room: RoomsListItem): void {
    closeRoomContextMenu();
    pushState('', {
      modal: {
        type: 'leaveRoom',
        roomId: room.id,
        roomName: room.name
      }
    });
  }

  async function handleJoinRoom(room: RoomsListItem): Promise<void> {
    closeRoomContextMenu();
    const result = await stores.roomDirectory.joinRoom(room.id);
    if (result.ok) {
      toast.success(m['room.join.success']({ room: room.name }));
      return;
    }

    toast.error(m['room.join.failed']());
    console.error('Error joining room:', result.error);
  }

  // --- Derived layout helpers ---

  // Channels and DMs are stored together, but rendered as separate groups.
  // Room sets only apply to channels — DM rooms always render in their
  // own group below.
  let channels = $derived(roomsStore.rooms.filter((r) => r.type === RoomType.Channel));
  let dmRooms = $derived(
    roomsStore.rooms.filter((r) => r.type === RoomType.Dm && isNavigationVisibleRoom(r))
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

  // Sets that have at least one channel the viewer is a member of
  let visibleSets = $derived.by(() => {
    const sets = roomsStore.roomGroups;
    if (!sets) return [];
    return sets.filter((s) => getSetItems(s).length > 0);
  });

  const hasSidebarItems = $derived(visibleSets.some((set) => getSetItems(set).length > 0));

  // When no layout exists, display channels alphabetically
  let sortedRooms = $derived([...channels].sort((a, b) => a.name.localeCompare(b.name)));

  // DM display name: comma-joined participants other than the current user
  // (or "You" for self-DMs).
  //
  // `meId` comes from `roomsStore.currentUserId`, which is captured from the
  // same `viewer { user { id, rooms { members } } }` query that produced `room.members`.
  // Reading the viewer ID from a global auth context here is unsafe — the
  // [serverId] layout intentionally renders children while the per-instance
  // CurrentUserState is still loading, so `currentUserState.user?.id` can be
  // undefined for the first render and the filter would include self in the
  // label/avatars (e.g. a 1:1 with Teal rendering as "Teal, hmans").
  function dmDisplayName(room: RoomsListItem): string {
    const meId = roomsStore.currentUserId;
    const others = room.members.filter((m) => m.id !== meId);
    if (others.length === 0) return m['common.you']();
    return others.map((m) => getLiveDisplayName(m.id, m.displayName || m.login)).join(', ');
  }

  function dmAvatarParticipants(room: RoomsListItem) {
    const meId = roomsStore.currentUserId;
    const others = room.members.filter((m) => m.id !== meId);
    if (others.length === 0) {
      // Self-DM: show own avatar
      return room.members.slice(0, 1);
    }
    return others.slice(0, 3);
  }

  function callParticipantAvatarUser(participant: CallRoomParticipant): UserAvatarUserView {
    return {
      id: participant.userId,
      login: participant.login,
      displayName: participant.displayName,
      deleted: false,
      avatarUrl: participant.avatarUrl,
      presenceStatus: PresenceStatus.Offline
    };
  }

  // Whether a room should remain visible while its sidebar group is
  // collapsed. Active room + any unread / mention / pending notification
  // anchor the row so the user can always reach what's calling for
  // attention. Channels and DMs only differ in the notification accessor —
  // hasRoomNotification deliberately excludes DMs.
  function isHighlighted(room: RoomsListItem): boolean {
    if (room.id === activeRoomId) return true;
    if (activeCallRooms.has(room.id)) return true;
    if (roomUnreadStore.roomIsUnread(room.id)) return true;
    if (room.type === RoomType.Dm) {
      return room.viewerNotificationCount > 0;
    }
    return room.viewerNotificationCount > 0;
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
    setRoomSidebarPanel(activeServerId, roomId, 'call');
    setPendingRoomSidebarPanel(activeServerId, roomId, 'call');
    window.dispatchEvent(
      new StorageEvent('storage', {
        key: serverStorageKey(activeServerId, roomSidebarPanelStorageSuffix(roomId)),
        newValue: 'call'
      })
    );
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
    const notification = lookup.notification;

    if (!notification) {
      if (lookup.ok && lookup.totalCount === 0) {
        roomsStore.clearUnreadNotifications(roomId);
      } else {
        await goto(resolve('/chat/notifications'));
      }
      return;
    }

    const target = notificationTarget(notification);
    prepareUiForNotificationTarget(appUi, activeServerId, target);
    if (target.eventId && target.roomId) {
      stores.pendingHighlights.set(target.roomId, target.threadRootId, target.eventId);
    }
    roomsStore.decrementUnreadNotification(roomId);
    void notificationStore.dismiss(notification.id).then((dismissed) => {
      if (!dismissed) {
        roomsStore.incrementUnreadNotification(roomId);
        return;
      }
      void roomsStore.refreshNotificationCounts();
    });

    const path = notificationStore.getCleanPath(getActiveServer(), notification);
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- path from getCleanPath() is already resolved
    await goto(path);
  }
</script>

{#snippet activeCallIcon()}
  <span
    class="relative sidebar-icon text-action"
    aria-label={m['room_list.active_call']()}
    data-testid="room-call-icon"
  >
    <span class="relative inline-flex">
      <span
        class="absolute inset-0 pane-header-icon-glyph animate-ping opacity-45 uil--phone"
        aria-hidden="true"
        data-testid="active-call-pulse-icon"
      ></span>
      <span class="relative pane-header-icon-glyph text-action uil--phone" aria-hidden="true"
      ></span>
    </span>
  </span>
{/snippet}

{#snippet activeCallParticipants(roomId: string)}
  {@const participants = activeCallRooms.getParticipants(roomId)}
  {#if participants.length > 0}
    <div
      class="hidden shrink-0 items-center -space-x-1 @min-[220px]:flex"
      aria-label={m['room_list.call_participants']({ count: participants.length })}
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

{#snippet roomLink(room: RoomsListItem)}
  {@const hasActiveCall = activeCallRooms.has(room.id)}
  {@const hasUnread = roomUnreadStore.roomIsUnread(room.id)}
  {@const isJoined = room.viewerIsMember}
  {@const hasUnreadAttention =
    isJoined &&
    hasUnread &&
    room.id !== activeRoomId &&
    !notificationLevelStore.isRoomMuted(room.id)}
  {@const rowClass = [
    '@container sidebar-item group/badges',
    room.id === activeRoomId ? 'bg-surface' : '',
    hasUnreadAttention ? 'sidebar-item-attention' : '',
    !isJoined ? 'opacity-60 hover:opacity-85' : ''
  ]}
  <a
    href={resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId: room.id })}
    class={rowClass}
    aria-current={room.id === activeRoomId ? 'page' : undefined}
    onclick={(e) => handleRoomLinkClick(e, room)}
    onkeydown={(e) => handleRoomLinkKeydown(e, room)}
    {@attach roomMenuTrigger(room)}
  >
    {#if isJoined}
      <span class={['sidebar-icon', hasUnreadAttention ? 'text-text-top' : 'text-muted']}>#</span>
    {:else if room.viewerCanJoinRoom}
      <span class="sidebar-icon text-muted">+</span>
    {:else}
      <span class="sidebar-icon iconify text-muted uil--lock"></span>
    {/if}
    <span class="flex-1 truncate">{room.name}</span>
    {#if isJoined && hasActiveCall}
      {@render activeCallParticipants(room.id)}
      {@render activeCallIcon()}
    {/if}

    <!-- Notification Indicator (warning color for mentions and thread replies) -->
    {#if isJoined && room.viewerNotificationCount > 0}
      <button
        type="button"
        onclick={(e) => handleNotificationBadgeClick(e, room.id, false)}
        class="flex h-6 min-w-6 cursor-pointer items-center justify-center notification-dot"
        aria-label={m['room_list.go_to_notifications']({
          count: room.viewerNotificationCount
        })}
      >
        <NotificationBadge count={room.viewerNotificationCount} testid="room-notification-badge" />
      </button>
      <span class="sr-only">
        {m['room_list.notifications']({ count: room.viewerNotificationCount })}
      </span>
      <!-- Unread Indicator (subtle) -->
    {:else if isJoined && hasUnread && !notificationLevelStore.isRoomMuted(room.id)}
      <UnreadDot color="neutral" testid="room-unread-dot" />
      <span class="sr-only">{m['room_list.unread_messages']()}</span>
    {/if}
  </a>
{/snippet}

{#snippet dmLink(room: RoomsListItem)}
  {@const hasActiveCall = activeCallRooms.has(room.id)}
  {@const hasUnread = roomUnreadStore.roomIsUnread(room.id)}
  {@const hasUnreadAttention = hasUnread && room.id !== activeRoomId}
  <a
    href={resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId: room.id })}
    class={[
      'group/badges @container sidebar-item',
      room.id === activeRoomId ? 'bg-surface' : '',
      hasUnreadAttention ? 'sidebar-item-attention' : ''
    ]}
    aria-current={room.id === activeRoomId ? 'page' : undefined}
    onclick={(e) => handleRoomLinkClick(e, room)}
    onkeydown={(e) => handleRoomLinkKeydown(e, room)}
    {@attach roomMenuTrigger(room)}
  >
    <div class="flex shrink-0 -space-x-1">
      {#each dmAvatarParticipants(room) as participant (participant.id)}
        <UserAvatar user={participant} size="xs" />
      {/each}
    </div>
    <span class="flex-1 truncate">{dmDisplayName(room)}</span>
    {#if hasActiveCall}
      {@render activeCallParticipants(room.id)}
      {@render activeCallIcon()}
    {/if}

    {#if room.viewerNotificationCount > 0}
      <button
        type="button"
        onclick={(e) => handleNotificationBadgeClick(e, room.id, true)}
        class="flex h-6 min-w-6 cursor-pointer items-center justify-center notification-dot"
        aria-label={m['room_list.go_to_dm_notifications']({
          count: room.viewerNotificationCount
        })}
      >
        <NotificationBadge count={room.viewerNotificationCount} testid="dm-notification-badge" />
      </button>
      <span class="sr-only">
        {m['room_list.new_direct_messages']({ count: room.viewerNotificationCount })}
      </span>
    {:else if hasUnread}
      <UnreadDot color="neutral" testid="dm-unread-dot" />
      <span class="sr-only">{m['room_list.unread_messages']()}</span>
    {/if}
  </a>
{/snippet}

{#snippet sidebarLink(item: RoomsListGroupItem)}
  {#if item.type === 'room'}
    {@const room = channelMap.get(item.roomId)}
    {#if room}
      {@render roomLink(room)}
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
      <span class="sidebar-icon iconify text-muted uil--external-link-alt"></span>
      <span class="flex-1 truncate">{item.link.label}</span>
    </a>
  {/if}
{/snippet}

{#if channels.length === 0 && dmRooms.length === 0 && !hasSidebarItems && !roomsStore.isInitialLoading}
  <EmptyState icon="uil--comments" title={m['room_list.empty_title']()}>
    {m['room_list.empty_prefix']()}
    <a href={resolve('/chat/[serverId]/overview', { serverId: serverSegment })} class="link"
      >{m['room_list.empty_overview']()}</a
    >
    {m['room_list.empty_suffix']()}
  </EmptyState>
{:else}
  <nav class="room-list sidebar-nav p-2 md:w-full">
    {#if roomsStore.roomGroups && roomsStore.roomGroups.length > 0}
      <!-- Room-set layout -->
      {#each visibleSets as set, i (set.id)}
        <CollapsibleGroup
          label={set.name}
          items={getSetItems(set)}
          item={sidebarLink}
          persistKey={serverStorageKey(getActiveServer(), `collapsible:set:${set.id}`)}
          keepVisibleWhenCollapsed={isGroupItemHighlighted}
          class={i === 0 ? 'mt-4 first:mt-0' : 'mt-4'}
        />
      {/each}
    {:else if sortedRooms.length > 0}
      <!-- No layout configured yet — alphabetical fallback. -->
      <CollapsibleGroup
        label={m['common.rooms']()}
        items={sortedRooms}
        item={roomLink}
        persistKey={serverStorageKey(getActiveServer(), 'collapsible:rooms')}
        keepVisibleWhenCollapsed={isHighlighted}
        class="mt-4 first:mt-0"
      />
    {/if}

    {#if dmRooms.length > 0}
      <CollapsibleGroup
        label={m['room_list.direct_messages']()}
        items={dmRooms}
        item={dmLink}
        persistKey={serverStorageKey(getActiveServer(), 'collapsible:dms')}
        keepVisibleWhenCollapsed={isHighlighted}
        class="mt-4"
      />
    {/if}
  </nav>
{/if}

{#if roomContextMenu}
  {@const contextRoom = roomContextMenu.room}
  <ContextMenu
    position={roomContextMenu.position}
    presentation={roomContextMenu.presentation}
    ariaLabel={m['room_list.room_actions']({ room: contextRoom.name })}
    onclose={closeRoomContextMenu}
  >
    <NavigationContextMenu
      kind="room"
      isRoomMember={contextRoom.viewerIsMember}
      canJoin={contextRoom.viewerCanJoinRoom}
      canMarkRead={roomUnreadStore.roomIsUnread(contextRoom.id) ||
        contextRoom.viewerNotificationCount > 0}
      canLeave={!contextRoom.isUniversal && contextRoom.type !== RoomType.Dm}
      onJoin={() => void handleJoinRoom(contextRoom)}
      onMarkRead={() => handleMarkRoomRead(contextRoom)}
      onLeave={() => handleLeaveRoom(contextRoom)}
    />
  </ContextMenu>
{/if}
