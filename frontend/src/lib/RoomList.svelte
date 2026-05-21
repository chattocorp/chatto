<!--
@component

Renders the room list in the space sidebar. When a room layout is configured,
rooms are organized into collapsible sections. Otherwise, rooms display alphabetically.

**Props:**
- `spaceId` - The ID of the space to show rooms for
-->
<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { type Snippet } from 'svelte';
  import { slide } from 'svelte/transition';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import CollapsibleGroup from '$lib/ui/CollapsibleGroup.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import type { CallRoomParticipant } from '$lib/state/server/activeCallRooms.svelte';
  import {
    useEvent,
    useTabResumeCallback,
    useMention,
    useRoomMarkedAsRead
  } from '$lib/hooks';
  import { serverStorageKey } from '$lib/storage/serverStorage';
  import { useFragment } from './gql';
  import { RoomType, type PresenceStatus } from '$lib/gql/graphql';
  import UserAvatar, { UserAvatarFragment } from '$lib/components/UserAvatar.svelte';
  import UnreadDot from '$lib/ui/UnreadDot.svelte';
  import { notificationTarget } from '$lib/state/server/notifications.svelte';
  import { appState } from '$lib/state/globals.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import { type RoomsListItem, type RoomsListGroup } from '$lib/state/space';

  // No props — RoomList reads everything from the active server's stores.
  // All store references go through `stores` ($derived), so when the active
  // server changes (URL [serverId] param changes), every derived read in the
  // template re-evaluates against the new server's state automatically.

  const serverSegment = $derived(serverIdToSegment(getActiveServer()));
  const stores = $derived(serverRegistry.getStore(getActiveServer()));
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

  function getSetRooms(set: RoomsListGroup): RoomsListItem[] {
    return set.roomIds.map((id) => channelMap.get(id)).filter((r): r is RoomsListItem => r != null);
  }

  // Sets that have at least one channel the viewer is a member of
  let visibleSets = $derived.by(() => {
    const sets = roomsStore.roomGroups;
    if (!sets) return [];
    return sets.filter((s) => getSetRooms(s).length > 0);
  });

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
    if (room.hasUnread) return true;
    if (room.hasMention) return true;
    if (room.type === RoomType.Dm) {
      return notificationStore.hasDMRoomNotification(room.id);
    }
    return notificationStore.hasRoomNotification(room.id);
  }

  // --- Real-time event handlers ---

  // Clear unread/mention when entering a room while present. Navigation
  // alone (e.g. clicking a notification while the tab is hidden) isn't
  // enough — we wait until the user can actually see the room. The
  // cross-tab `useRoomMarkedAsRead` handler below also clears the dot when
  // useRoomUnread fires its mutation on presence-true.
  $effect(() => {
    if (activeRoomId && appState.isPresent) roomsStore.markRead(activeRoomId);
  });

  // Handle space events that this component cares about beyond the store
  // refresh (which happens in ServerEventProvider): navigate away on leave,
  // and update voice-call indicators.
  useEvent((spaceEvent) => {
    const event = spaceEvent.event;

    if (event.__typename === 'UserLeftRoomEvent' && event.roomId === activeRoomId) {
      // Only navigate away when *the viewer* leaves the active room.
      // Without the actor check, any other member's leave (including the
      // cascade of UserLeftRoomEvents fired when a peer deletes their
      // account) would yank the viewer out of the room they're in.
      if (spaceEvent.actorId && spaceEvent.actorId === roomsStore.currentUserId) {
        goto(resolve('/chat/[serverId]', { serverId: serverSegment }));
      }
    } else if (event.__typename === 'CallParticipantJoinedEvent') {
      const actor = spaceEvent.actor ? useFragment(UserAvatarFragment, spaceEvent.actor) : null;
      activeCallRooms.handleJoin(event.roomId, actor);
    } else if (event.__typename === 'CallParticipantLeftEvent') {
      activeCallRooms.handleLeave(event.roomId, spaceEvent.actorId);
    }
  });

  // Mention notifications — mark room as having mention
  useMention((notification) => {
    if (notification.roomId === activeRoomId) return;
    roomsStore.setMention(notification.roomId);
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

  function toAvatarUser(p: CallRoomParticipant) {
    return {
      id: p.userId,
      login: p.login,
      displayName: p.displayName,
      avatarUrl: p.avatarUrl,
      presenceStatus: 'ONLINE' as PresenceStatus
    };
  }

  // Handle click on call participant badge — navigate to room and join the call
  function handleCallBadgeClick(e: Event, roomId: string) {
    e.preventDefault();
    e.stopPropagation();

    const livekitUrl = serverInfo.livekitUrl;
    if (livekitUrl) {
      voiceCallState.join(livekitUrl, roomId).catch(() => {
        // Silently catch — VoiceCallPanel provides fallback Join button
      });
    }

    goto(resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId }));
  }

  // Handle click on room notification dot - navigate to notification source and dismiss
  async function handleRoomNotificationClick(event: MouseEvent, roomId: string) {
    event.preventDefault();
    event.stopPropagation();

    const notification = notificationStore.getRoomNotification(roomId);
    if (!notification) {
      // Clear stuck hasMention state — the dot was visible but no notification
      // exists in the store to dismiss. Clear the local flag so the dot disappears.
      roomsStore.clearMention(roomId);
      return;
    }

    const target = notificationTarget(notification);
    if (target.eventId && target.roomId) {
      stores.pendingHighlights.set(target.roomId, target.threadRootId, target.eventId);
    }
    void notificationStore.dismiss(notification.id);

    const path = notificationStore.getCleanPath(getActiveServer(), notification);
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- path from getCleanPath() is already resolved
    await goto(path);
  }

  // Handle click on a DM notification dot. Mirrors handleRoomNotificationClick
  // but uses the DM-flavoured store accessors — `getRoomNotification` /
  // `hasRoomNotification` deliberately exclude DMs.
  async function handleDMNotificationClick(event: MouseEvent, roomId: string) {
    event.preventDefault();
    event.stopPropagation();

    const notification = notificationStore.getDMRoomNotification(roomId);
    if (!notification) return;

    void notificationStore.dismiss(notification.id);

    const path = notificationStore.getCleanPath(getActiveServer(), notification);
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- path from getCleanPath() is already resolved
    await goto(path);
  }
</script>

{#snippet roomLink(room: RoomsListItem)}
  {@const callParticipants = activeCallRooms.has(room.id) ? activeCallRooms.getParticipants(room.id) : []}
  <a
    href={resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId: room.id })}
    class={[
      'sidebar-item group/badges',
      callParticipants.length > 0 ? 'flex-wrap gap-y-1' : '',
      room.id === activeRoomId ? 'bg-surface-100' : '',
      room.hasUnread &&
      room.id !== activeRoomId &&
      !notificationLevelStore.isRoomMuted(room.id)
        ? 'font-semibold'
        : ''
    ]}
    aria-current={room.id === activeRoomId ? 'page' : undefined}
  >
    <span class="sidebar-icon text-muted">#</span>
    <span class="flex-1 truncate">{room.name}</span>

    <!-- Notification Indicator (warning color for mentions and thread replies) -->
    {#if room.hasMention || notificationStore.hasRoomNotification(room.id)}
      <button
        type="button"
        onclick={(e) => handleRoomNotificationClick(e, room.id)}
        class="-mr-2 flex h-6 w-6 cursor-pointer items-center justify-center notification-dot"
        aria-label="Go to notification"
      >
        <UnreadDot />
      </button>
      <span class="sr-only">{room.hasMention ? 'you were mentioned' : 'thread reply'}</span>
      <!-- Unread Indicator (subtle) -->
    {:else if room.hasUnread && !notificationLevelStore.isRoomMuted(room.id)}
      <UnreadDot color="primary" testid="room-unread-dot" />
      <span class="sr-only">unread messages</span>
    {/if}

    <!-- Call participant avatars (badge row, wraps below room name).
         Clicking the badge navigates to the room AND joins the call. -->
    {#if callParticipants.length > 0}
      <div
        class="basis-full pl-7 cursor-pointer"
        role="button"
        tabindex="0"
        onclick={(e) => handleCallBadgeClick(e, room.id)}
        onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); handleCallBadgeClick(e, room.id); } }}
      >
        <div class={["meta-badge border-transparent gap-1.5 px-1.5 py-0.5", room.id === activeRoomId ? 'bg-surface-200' : '']}>
          <span class="iconify animate-pulse text-accent uil--phone text-sm"></span>
          <div class="inline-flex -space-x-1.5">
            {#each callParticipants as p (p.userId)}
              <UserAvatar user={toAvatarUser(p)} size="xs" showPresence={false} />
            {/each}
          </div>
        </div>
      </div>
    {/if}
  </a>
{/snippet}

{#snippet dmLink(room: RoomsListItem)}
  <a
    href={resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId: room.id })}
    class={[
      'sidebar-item',
      room.id === activeRoomId ? 'bg-surface-100' : '',
      room.hasUnread && room.id !== activeRoomId ? 'font-semibold' : ''
    ]}
    aria-current={room.id === activeRoomId ? 'page' : undefined}
  >
    <div class="flex shrink-0 -space-x-1">
      {#each dmAvatarParticipants(room) as participant (participant.id)}
        <UserAvatar user={participant} size="xs" />
      {/each}
    </div>
    <span class="flex-1 truncate">{dmDisplayName(room)}</span>

    {#if notificationStore.hasDMRoomNotification(room.id)}
      <button
        type="button"
        onclick={(e) => handleDMNotificationClick(e, room.id)}
        class="-mr-2 flex h-6 w-6 cursor-pointer items-center justify-center notification-dot"
        aria-label="Go to notification"
      >
        <UnreadDot />
      </button>
      <span class="sr-only">new direct message</span>
    {:else if room.hasUnread}
      <UnreadDot color="primary" testid="dm-unread-dot" />
      <span class="sr-only">unread messages</span>
    {/if}
  </a>
{/snippet}

{#if channels.length === 0 && dmRooms.length === 0 && !roomsStore.isInitialLoading}
  <EmptyState icon="uil--comments" title="No rooms yet">
    You haven't joined any rooms on this server. Head to the
    <a
      href={resolve('/chat/[serverId]/overview', { serverId: serverSegment })}
      class="link">Overview</a
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
          items={getSetRooms(set)}
          item={roomLink}
          persistKey={serverStorageKey(getActiveServer(), `collapsible:set:${set.id}`)}
          keepVisibleWhenCollapsed={isHighlighted}
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
