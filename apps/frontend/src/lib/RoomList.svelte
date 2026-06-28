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
  import { useEvent, useTabResumeCallback, useRoomMarkedAsRead } from '$lib/hooks';
  import {
    roomSidebarPanelStorageSuffix,
    setPendingRoomSidebarPanel,
    setRoomSidebarPanel
  } from '$lib/storage/roomSidebarPanel';
  import { serverStorageKey } from '$lib/storage/serverStorage';
  import { useRenderData } from './render/data';
  import { PresenceStatus, RoomType, type UserAvatarUserView } from '$lib/render/types';
  import UserAvatar, { UserAvatarViewData } from '$lib/components/UserAvatar.svelte';
  import NotificationBadge from '$lib/ui/NotificationBadge.svelte';
  import UnreadDot from '$lib/ui/UnreadDot.svelte';
  import { notificationTarget } from '$lib/state/server/notifications.svelte';
  import { appState } from '$lib/state/globals.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import type { EventEnvelope } from '$lib/eventBus.svelte';
  import { isMessagePostedEvent, RoomEventKind, roomEventKind } from '$lib/render/eventKinds';
  import {
    type RoomsListItem,
    type RoomsListGroup,
    type RoomsListGroupItem
  } from '$lib/state/server/rooms.svelte';
  import type { CallRoomParticipant } from '$lib/state/server/activeCallRooms.svelte';

  // No props — RoomList reads everything from the active server's stores.
  // All store references go through `stores` ($derived), so when the active
  // server changes (URL [serverId] param changes), every derived read in the
  // template re-evaluates against the new server's state automatically.

  const activeServerId = $derived(getActiveServer());
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  const activeServer = $derived(serverRegistry.getServer(activeServerId));
  const activeServerBaseURL = $derived(activeServer?.url ?? null);
  const stores = $derived(serverRegistry.getStore(activeServerId));
  const currentUserState = $derived(stores.currentUser);
  const notificationStore = $derived(stores.notifications);
  const notificationLevelStore = $derived(stores.notificationLevels);
  const activeCallRooms = $derived(stores.activeCallRooms);
  const voiceCallState = $derived(stores.voiceCall);
  const serverInfo = $derived(stores.serverInfo);

  const roomsStore = $derived(stores.rooms);

  let activeRoomId = $derived(page.params.roomId);

  function eventRoomId(event: EventEnvelope['event']): string | null {
    if (!event || !('roomId' in event) || typeof event.roomId !== 'string') return null;
    return event.roomId;
  }

  function callEventPayload(
    event: EventEnvelope['event']
  ): { roomId: string; callId: string } | null {
    if (
      !event ||
      !('roomId' in event) ||
      typeof event.roomId !== 'string' ||
      !('callId' in event) ||
      typeof event.callId !== 'string'
    ) {
      return null;
    }
    return { roomId: event.roomId, callId: event.callId };
  }

  // Load active call room IDs whenever the active server has a LiveKit URL.
  // Re-runs on server switch so a server with LiveKit configured fetches its
  // own active calls instead of inheriting the previous server's snapshot.
  $effect(() => {
    if (serverInfo.livekitUrl) activeCallRooms.load();
  });

  // Refresh active call state when tab resumes (catches missed live events)
  useTabResumeCallback(() => {
    if (serverInfo.livekitUrl) activeCallRooms.load();
  });

  // --- Derived layout helpers ---

  // Channels and DMs are stored together, but rendered as separate groups.
  // Room sets only apply to channels — DM rooms always render in their
  // own group below.
  let channels = $derived(roomsStore.rooms.filter((r) => r.type === RoomType.Channel));
  let dmRooms = $derived(roomsStore.rooms.filter((r) => r.type === RoomType.Dm));

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
    if (others.length === 0) return 'You';
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
    if (room.hasUnread) return true;
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

  // --- Real-time event handlers ---

  // Clear unread when entering a room while present. Navigation
  // alone (e.g. clicking a notification while the tab is hidden) isn't
  // enough — we wait until the user can actually see the room. The
  // cross-tab `useRoomMarkedAsRead` handler below also clears the unread
  // marker when useRoomUnread fires its mutation on presence-true.
  $effect(() => {
    if (activeRoomId && appState.isPresent) roomsStore.markRead(activeRoomId);
  });

  // Handle server events that this component cares about beyond the store
  // refresh (which happens in ServerEventProvider): navigate away on leave,
  // and update voice-call indicators.
  useEvent((serverEvent) => {
    const event = serverEvent.event;
    if (!event) return;

    switch (roomEventKind(event)) {
      case RoomEventKind.UserLeftRoom:
        if (eventRoomId(event) === activeRoomId) {
          // Only navigate away when *the viewer* leaves the active room.
          // Without the actor check, any other member's leave (including the
          // cascade of UserLeftRoomEvents fired when a peer deletes their
          // account) would yank the viewer out of the room they're in.
          if (serverEvent.actorId === roomsStore.currentUserId) {
            goto(resolve('/chat/[serverId]', { serverId: serverSegment }));
          }
        }
        break;
      case RoomEventKind.CallParticipantJoined: {
        const call = callEventPayload(event);
        if (!call) break;
        const actor = serverEvent.actor
          ? useRenderData(UserAvatarViewData, serverEvent.actor)
          : null;
        void activeCallRooms.handleJoin(call.roomId, call.callId, actor);
        break;
      }
      case RoomEventKind.CallParticipantLeft: {
        const call = callEventPayload(event);
        if (!call) break;
        activeCallRooms.handleLeave(call.roomId, call.callId, serverEvent.actorId ?? null);
        voiceCallState.handleParticipantLeftEvent(
          call.roomId,
          call.callId,
          serverEvent.actorId ?? null,
          roomsStore.currentUserId
        );
        break;
      }
      case RoomEventKind.CallEnded: {
        const call = callEventPayload(event);
        if (!call) break;
        activeCallRooms.handleEnd(call.roomId, call.callId);
        voiceCallState.handleCallEndedEvent(call.roomId, call.callId);
        break;
      }
    }
  });

  // Marked-as-read from other tabs/devices.
  useRoomMarkedAsRead(({ roomId }) => {
    roomsStore.markRead(roomId);
  });

  // New root messages → bump DM rows to the top + mark unread when the
  // message lands in a room the viewer isn't currently looking at. Reads
  // MessagePostedEvent directly off the unified live.server.> stream
  // (every accepted server.> message is republished into it, so the
  // viewer sees one event per message in every room they're a member of).
  useEvent((serverEvent) => {
    const event = serverEvent.event;
    if (!event) return;
    if (!isMessagePostedEvent(event)) return;
    if (event.threadRootEventId) return; // root messages only

    // Bump DM rooms to the top of the Direct Messages section on ANY
    // root-message activity — including the viewer's own messages. The
    // store no-ops if the room isn't a DM.
    roomsStore.bumpRoom(event.roomId);

    // Unread bookkeeping is suppressed for the viewer's own messages and
    // for the room they're currently present on. "Present" requires the
    // window to be focused AND the tab visible — if the URL matches but
    // the user is on another app / tab, the dot should still light up so
    // they see the signal when they return.
    if (event.roomId === activeRoomId && appState.isPresent) return;
    if (serverEvent.actorId === currentUserState.user?.id) return;
    if (notificationLevelStore.isRoomMuted(event.roomId)) return;
    roomsStore.setUnread(event.roomId);
  });

  function openJoinRoomModal(room: RoomsListItem) {
    pushState('', {
      modal: {
        type: 'joinRoom',
        roomId: room.id,
        roomName: room.name,
        viewerCanJoinRoom: room.viewerCanJoinRoom
      }
    });
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
    if (!room.viewerIsMember) {
      event.preventDefault();
      openJoinRoomModal(room);
      return;
    }

    if (activeCallRooms.has(room.id) && wasCallIconClick(event)) {
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
    class="relative sidebar-icon text-accent"
    aria-label={m['room_list.active_call']()}
    data-testid="room-call-icon"
  >
    <span class="relative inline-flex">
      <span
        class="absolute inset-0 pane-header-icon-glyph animate-ping opacity-45 uil--phone"
        aria-hidden="true"
        data-testid="active-call-pulse-icon"
      ></span>
      <span class="relative pane-header-icon-glyph text-accent uil--phone" aria-hidden="true"
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
          class="hidden h-5 min-w-5 shrink-0 items-center justify-center rounded-full bg-surface-200 px-1 text-[10px] leading-none font-medium text-muted ring-1 ring-background @min-[380px]:inline-flex"
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
  {@const isJoined = room.viewerIsMember}
  {@const hasUnreadAttention =
    isJoined &&
    room.hasUnread &&
    room.id !== activeRoomId &&
    !notificationLevelStore.isRoomMuted(room.id)}
  {@const rowClass = [
    '@container sidebar-item group/badges',
    room.id === activeRoomId ? 'bg-surface-100' : '',
    hasUnreadAttention ? 'font-semibold text-text-top hover:!text-text-top' : '',
    !isJoined ? 'opacity-60 hover:opacity-85' : ''
  ]}
  <a
    href={resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId: room.id })}
    class={rowClass}
    aria-current={room.id === activeRoomId ? 'page' : undefined}
    onclick={(e) => handleRoomLinkClick(e, room)}
    onkeydown={(e) => handleRoomLinkKeydown(e, room)}
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
    {:else if isJoined && room.hasUnread && !notificationLevelStore.isRoomMuted(room.id)}
      <UnreadDot color="primary" testid="room-unread-dot" />
      <span class="sr-only">{m['room_list.unread_messages']()}</span>
    {/if}
  </a>
{/snippet}

{#snippet dmLink(room: RoomsListItem)}
  {@const hasActiveCall = activeCallRooms.has(room.id)}
  {@const hasUnreadAttention = room.hasUnread && room.id !== activeRoomId}
  <a
    href={resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId: room.id })}
    class={[
      'group/badges @container sidebar-item',
      room.id === activeRoomId ? 'bg-surface-100' : '',
      hasUnreadAttention ? 'font-semibold text-text-top hover:!text-text-top' : ''
    ]}
    aria-current={room.id === activeRoomId ? 'page' : undefined}
    onclick={(e) => handleRoomLinkClick(e, room)}
    onkeydown={(e) => handleRoomLinkKeydown(e, room)}
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
    {:else if room.hasUnread}
      <UnreadDot color="primary" testid="dm-unread-dot" />
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
        label="Direct Messages"
        items={dmRooms}
        item={dmLink}
        persistKey={serverStorageKey(getActiveServer(), 'collapsible:dms')}
        keepVisibleWhenCollapsed={isHighlighted}
        class="mt-4"
      />
    {/if}
  </nav>
{/if}
