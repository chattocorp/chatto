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
  import { useFragment } from './gql';
  import { RoomType } from '$lib/gql/graphql';
  import UserAvatar, { UserAvatarFragment } from '$lib/components/UserAvatar.svelte';
  import NotificationBadge from '$lib/ui/NotificationBadge.svelte';
  import UnreadDot from '$lib/ui/UnreadDot.svelte';
  import { notificationTarget } from '$lib/state/server/notifications.svelte';
  import { appState } from '$lib/state/globals.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import {
    type RoomsListItem,
    type RoomsListGroup,
    type RoomsListGroupItem
  } from '$lib/state/server/rooms.svelte';

  // No props — RoomList reads everything from the active server's stores.
  // All store references go through `stores` ($derived), so when the active
  // server changes (URL [serverId] param changes), every derived read in the
  // template re-evaluates against the new server's state automatically.

  const activeServerId = $derived(getActiveServer());
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  const stores = $derived(serverRegistry.getStore(activeServerId));
  const currentUserState = $derived(stores.currentUser);
  const notificationStore = $derived(stores.notifications);
  const notificationLevelStore = $derived(stores.notificationLevels);
  const activeCallRooms = $derived(stores.activeCallRooms);
  const voiceCallState = $derived(stores.voiceCall);
  const serverInfo = $derived(stores.serverInfo);

  const roomsStore = $derived(stores.rooms);

  let activeRoomId = $derived(page.params.roomId);

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

    if (event.__typename === 'UserLeftRoomEvent' && event.roomId === activeRoomId) {
      // Only navigate away when *the viewer* leaves the active room.
      // Without the actor check, any other member's leave (including the
      // cascade of UserLeftRoomEvents fired when a peer deletes their
      // account) would yank the viewer out of the room they're in.
      if (serverEvent.actorId === roomsStore.currentUserId) {
        goto(resolve('/chat/[serverId]', { serverId: serverSegment }));
      }
    } else if (event.__typename === 'CallParticipantJoinedEvent') {
      const actor = serverEvent.actor ? useFragment(UserAvatarFragment, serverEvent.actor) : null;
      activeCallRooms.handleJoin(event.roomId, event.callId, actor);
    } else if (event.__typename === 'CallParticipantLeftEvent') {
      activeCallRooms.handleLeave(event.roomId, event.callId, serverEvent.actorId ?? null);
      voiceCallState.handleParticipantLeftEvent(
        event.roomId,
        event.callId,
        serverEvent.actorId ?? null,
        roomsStore.currentUserId
      );
    } else if (event.__typename === 'CallEndedEvent') {
      activeCallRooms.handleEnd(event.roomId, event.callId);
      voiceCallState.handleCallEndedEvent(event.roomId, event.callId);
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
    if (event.__typename !== 'MessagePostedEvent') return;
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
      if (!dismissed) roomsStore.incrementUnreadNotification(roomId);
    });

    const path = notificationStore.getCleanPath(getActiveServer(), notification);
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- path from getCleanPath() is already resolved
    await goto(path);
  }
</script>

{#snippet activeCallIcon()}
  <span
    class="sidebar-icon relative text-accent"
    aria-label="Active call"
    data-testid="room-call-icon"
  >
    <span class="relative inline-flex">
      <span
        class="pane-header-icon-glyph absolute inset-0 animate-ping opacity-45 uil--phone"
        aria-hidden="true"
        data-testid="active-call-pulse-icon"
      ></span>
      <span
        class="pane-header-icon-glyph relative text-accent uil--phone"
        aria-hidden="true"
      ></span>
    </span>
  </span>
{/snippet}

{#snippet roomLink(room: RoomsListItem)}
  {@const hasActiveCall = activeCallRooms.has(room.id)}
  {@const isJoined = room.viewerIsMember}
  {@const rowClass = [
    'sidebar-item group/badges',
    room.id === activeRoomId ? 'bg-surface-100' : '',
    room.hasUnread && room.id !== activeRoomId && !notificationLevelStore.isRoomMuted(room.id)
      ? 'font-semibold'
      : '',
    !isJoined ? 'opacity-60 hover:opacity-85' : ''
  ]}
  <a
    href={resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId: room.id })}
    class={rowClass}
    aria-current={room.id === activeRoomId ? 'page' : undefined}
    onclick={(e) => handleRoomLinkClick(e, room)}
    onkeydown={(e) => handleRoomLinkKeydown(e, room)}
  >
    {#if isJoined && hasActiveCall}
      {@render activeCallIcon()}
    {:else if isJoined}
      <span class="sidebar-icon text-muted">#</span>
    {:else if room.viewerCanJoinRoom}
      <span class="sidebar-icon text-muted">+</span>
    {:else}
      <span class="sidebar-icon iconify text-muted uil--lock"></span>
    {/if}
    <span class="flex-1 truncate">{room.name}</span>

    <!-- Notification Indicator (warning color for mentions and thread replies) -->
    {#if isJoined && room.viewerNotificationCount > 0}
      <button
        type="button"
        onclick={(e) => handleNotificationBadgeClick(e, room.id, false)}
        class="-mr-2 flex h-6 min-w-6 cursor-pointer items-center justify-center notification-dot"
        aria-label={`Go to ${room.viewerNotificationCount} notifications`}
      >
        <NotificationBadge count={room.viewerNotificationCount} testid="room-notification-badge" />
      </button>
      <span class="sr-only">{room.viewerNotificationCount} notifications</span>
      <!-- Unread Indicator (subtle) -->
    {:else if isJoined && room.hasUnread && !notificationLevelStore.isRoomMuted(room.id)}
      <UnreadDot color="primary" testid="room-unread-dot" />
      <span class="sr-only">unread messages</span>
    {/if}

  </a>
{/snippet}

{#snippet dmLink(room: RoomsListItem)}
  {@const hasActiveCall = activeCallRooms.has(room.id)}
  <a
    href={resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId: room.id })}
    class={[
      'group/badges sidebar-item',
      room.id === activeRoomId ? 'bg-surface-100' : '',
      room.hasUnread && room.id !== activeRoomId ? 'font-semibold' : ''
    ]}
    aria-current={room.id === activeRoomId ? 'page' : undefined}
    onclick={(e) => handleRoomLinkClick(e, room)}
    onkeydown={(e) => handleRoomLinkKeydown(e, room)}
  >
    {#if hasActiveCall}
      {@render activeCallIcon()}
    {:else}
      <div class="flex shrink-0 -space-x-1">
        {#each dmAvatarParticipants(room) as participant (participant.id)}
          <UserAvatar user={participant} size="xs" />
        {/each}
      </div>
    {/if}
    <span class="flex-1 truncate">{dmDisplayName(room)}</span>

    {#if room.viewerNotificationCount > 0}
      <button
        type="button"
        onclick={(e) => handleNotificationBadgeClick(e, room.id, true)}
        class="-mr-2 flex h-6 min-w-6 cursor-pointer items-center justify-center notification-dot"
        aria-label={`Go to ${room.viewerNotificationCount} direct message notifications`}
      >
        <NotificationBadge count={room.viewerNotificationCount} testid="dm-notification-badge" />
      </button>
      <span class="sr-only">{room.viewerNotificationCount} new direct messages</span>
    {:else if room.hasUnread}
      <UnreadDot color="primary" testid="dm-unread-dot" />
      <span class="sr-only">unread messages</span>
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
    <button
      type="button"
      class="sidebar-item w-full text-left"
      onclick={() => window.open(item.link.url, '_blank', 'noopener,noreferrer')}
    >
      <span class="sidebar-icon iconify text-muted uil--external-link-alt"></span>
      <span class="flex-1 truncate">{item.link.label}</span>
    </button>
  {/if}
{/snippet}

{#if channels.length === 0 && dmRooms.length === 0 && !hasSidebarItems && !roomsStore.isInitialLoading}
  <EmptyState icon="uil--comments" title="No rooms yet">
    You haven't joined any rooms on this server. Head to the
    <a href={resolve('/chat/[serverId]/overview', { serverId: serverSegment })} class="link"
      >Overview</a
    >
    to browse the directory and join the ones you're interested in.
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
        label="Rooms"
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
