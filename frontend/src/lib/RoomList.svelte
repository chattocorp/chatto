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
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { untrack, type Snippet } from 'svelte';
  import { slide } from 'svelte/transition';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import type { CallRoomParticipant } from '$lib/state/instance/activeCallRooms.svelte';
  import {
    useSpaceEvent,
    useTabResumeCallback,
    useInstanceEvent,
    useMention,
    useRoomMarkedAsRead,
    useRoomLayoutUpdated
  } from '$lib/hooks';
  import { getCurrentUser } from '$lib/auth/currentUser.svelte';
  import { instanceStorageKey } from '$lib/storage/instanceStorage';
  import { SvelteSet } from 'svelte/reactivity';
  import { useFragment } from './gql';
  import { RoomType, type PresenceStatus } from '$lib/gql/graphql';
  import { DM_SPACE_ID } from '$lib/constants';
  import UserAvatar, { UserAvatarFragment } from '$lib/components/UserAvatar.svelte';
  import UnreadDot from '$lib/ui/UnreadDot.svelte';
  import { notificationTarget } from '$lib/state/instance/notifications.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import { getSpaceRoomsStore, type SpaceRoom, type SpaceLayoutSection } from '$lib/state/space';

  let {
    spaceId
  }: {
    spaceId: string;
  } = $props();

  const getInstanceId = getActiveInstance();
  const instanceSegment = $derived(instanceIdToSegment(getInstanceId()));
  const currentUserState = getCurrentUser();
  const stores = instanceRegistry.getStore(getInstanceId());
  const notificationStore = stores.notifications;
  const notificationLevelStore = stores.notificationLevels;
  const activeCallRooms = stores.activeCallRooms;
  const voiceCallState = stores.voiceCall;
  const instanceState = stores.instance;

  const roomsStore = getSpaceRoomsStore();

  let activeRoomId = $derived(page.params.roomId);

  // --- Collapsed-section UI state (persisted to localStorage) ---

  let collapsedSections = new SvelteSet<string>();

  function collapsedSectionsKey(sid: string): string {
    return instanceStorageKey(getInstanceId(), `space:${sid}:collapsed-sections`);
  }

  function loadCollapsedFromStorage(sid: string) {
    collapsedSections.clear();
    try {
      const key = collapsedSectionsKey(sid);
      let json = localStorage.getItem(key);

      // Lazy migration: try legacy key if namespaced key is absent
      if (!json) {
        const legacyKey = `space:${sid}:collapsed-sections`;
        json = localStorage.getItem(legacyKey);
        if (json) {
          localStorage.setItem(key, json);
          localStorage.removeItem(legacyKey);
        }
      }

      if (json) {
        for (const id of JSON.parse(json)) {
          collapsedSections.add(id);
        }
      }
    } catch {
      // ignore malformed localStorage data
    }
  }

  function saveCollapsedSections(sid: string) {
    localStorage.setItem(collapsedSectionsKey(sid), JSON.stringify([...collapsedSections]));
  }

  function toggleSection(sectionId: string) {
    if (collapsedSections.has(sectionId)) {
      collapsedSections.delete(sectionId);
    } else {
      collapsedSections.add(sectionId);
    }
    saveCollapsedSections(spaceId);
  }

  // Parent remounts via {#key spaceId}, so the initial prop is the only
  // value this instance will ever see — read once during init.
  const initialSpaceId = untrack(() => spaceId);
  loadCollapsedFromStorage(initialSpaceId);

  // Load active call room IDs once per spaceId mount.
  if (instanceState.livekitUrl) activeCallRooms.load(initialSpaceId);

  // Refresh active call state when tab resumes (catches missed live events)
  useTabResumeCallback(() => {
    if (instanceState.livekitUrl) activeCallRooms.load(spaceId);
  });

  // --- Derived layout helpers ---

  // Channels and DMs are stored together, but rendered as separate groups.
  // Layout sections (and the alphabetical fallback) only apply to channels —
  // DM rooms always render in their own group below.
  let channels = $derived(roomsStore.rooms.filter((r) => r.type === RoomType.Channel));
  let dmRooms = $derived(roomsStore.rooms.filter((r) => r.type === RoomType.Dm));

  let channelMap = $derived(new Map(channels.map((r) => [r.id, r])));

  function getSectionRooms(section: SpaceLayoutSection): SpaceRoom[] {
    return section.roomIds.map((id) => channelMap.get(id)).filter((r): r is SpaceRoom => r != null);
  }

  // Sections that have at least one channel the viewer is a member of
  let visibleSections = $derived.by(() => {
    const sections = roomsStore.layoutSections;
    if (!sections) return [];
    return sections.filter((s) => getSectionRooms(s).length > 0);
  });

  // Channels not assigned to any section, respecting stored order when available
  let unsectionedRooms = $derived.by(() => {
    const sections = roomsStore.layoutSections;
    if (!sections) return [];
    const sectionedIds = new Set(sections.flatMap((s) => s.roomIds));
    const unsectioned = channels.filter((r) => !sectionedIds.has(r.id));

    if (roomsStore.unsectionedRoomIds.length > 0) {
      const orderedMap = new Map(unsectioned.map((r) => [r.id, r]));
      const ordered: SpaceRoom[] = [];
      // eslint-disable-next-line svelte/prefer-svelte-reactivity -- local computation, not reactive state
      const seen = new Set<string>();
      for (const id of roomsStore.unsectionedRoomIds) {
        const room = orderedMap.get(id);
        if (room) {
          ordered.push(room);
          seen.add(id);
        }
      }
      // Append new rooms not in stored order, alphabetically
      const extra = unsectioned
        .filter((r) => !seen.has(r.id))
        .sort((a, b) => a.name.localeCompare(b.name));
      return [...ordered, ...extra];
    }

    return unsectioned.sort((a, b) => a.name.localeCompare(b.name));
  });

  // When no layout exists, display channels alphabetically
  let sortedRooms = $derived([...channels].sort((a, b) => a.name.localeCompare(b.name)));

  // DM display name: comma-joined participants other than the current user
  // (or "You" for self-DMs).
  function dmDisplayName(room: SpaceRoom): string {
    const meId = currentUserState.user?.id;
    const others = room.members.filter((m) => m.id !== meId);
    if (others.length === 0) return 'You';
    return others.map((m) => getLiveDisplayName(m.id, m.displayName || m.login)).join(', ');
  }

  function dmAvatarParticipants(room: SpaceRoom) {
    const meId = currentUserState.user?.id;
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
  function isHighlighted(room: SpaceRoom): boolean {
    if (room.id === activeRoomId) return true;
    if (room.hasUnread) return true;
    if (room.hasMention) return true;
    if (room.type === RoomType.Dm) {
      return notificationStore.hasDMRoomNotification(room.id);
    }
    return notificationStore.hasRoomNotification(room.id);
  }

  // --- Real-time event handlers ---

  // Clear unread/mention when entering a room. Notification dismissal is
  // handled by Room.svelte when it mounts.
  $effect(() => {
    if (activeRoomId) roomsStore.markRead(activeRoomId);
  });

  // Handle space events that this component cares about beyond the store
  // refresh (which happens in SpaceEventProvider): navigate away on leave,
  // and update voice-call indicators.
  useSpaceEvent((spaceEvent) => {
    const event = spaceEvent.event;

    if (event.__typename === 'UserLeftRoomEvent' && event.roomId === activeRoomId) {
      goto(resolve('/chat/[instanceId]', { instanceId: instanceSegment }));
    } else if (event.__typename === 'CallParticipantJoinedEvent') {
      const actor = spaceEvent.actor ? useFragment(UserAvatarFragment, spaceEvent.actor) : null;
      activeCallRooms.handleJoin(event.spaceId, event.roomId, actor);
    } else if (event.__typename === 'CallParticipantLeftEvent') {
      activeCallRooms.handleLeave(event.spaceId, event.roomId, spaceEvent.actorId);
    }
  });

  // Mention notifications — mark room as having mention
  useMention((notification) => {
    if (notification.spaceId !== spaceId) return;
    if (notification.roomId === activeRoomId) return;
    roomsStore.setMention(notification.roomId);
  });

  // Marked-as-read from other tabs/devices. Accept DM-space events too —
  // DM rooms now appear in this sidebar and need the same cross-tab sync.
  useRoomMarkedAsRead(({ spaceId: eventSpaceId, roomId }) => {
    if (eventSpaceId !== spaceId && eventSpaceId !== DM_SPACE_ID) return;
    roomsStore.markRead(roomId);
  });

  // New messages via instance events — mark room as having unread.
  // Uses the instance event bus (NewMessageInSpaceEvent) rather than the
  // space event bus (MessagePostedEvent) because it's more reliable for
  // cross-room delivery.
  //
  // Accepts events for both the active (primary) space and the DM space,
  // since DM rooms now appear in this sidebar alongside channels.
  useInstanceEvent((instanceEvent) => {
    const event = instanceEvent.event;
    if (!event) return;

    if (event.__typename === 'NewMessageInSpaceEvent') {
      if (event.spaceId !== spaceId && event.spaceId !== DM_SPACE_ID) return;

      // Bump DM rooms to the top of the Direct Messages section on ANY
      // root-message activity — including the viewer's own messages, even in
      // the room they're currently looking at. The sort matches what a
      // reload would produce; without this, posting in a not-at-the-top DM
      // leaves the row stranded until next refresh. Channels render
      // alphabetically so the bump is visually a no-op for them.
      if (event.spaceId === DM_SPACE_ID) {
        roomsStore.bumpRoom(event.roomId);
      }

      // Unread bookkeeping is suppressed for the viewer's own messages and
      // for the room they're currently in.
      if (event.roomId === activeRoomId) return;
      if (instanceEvent.actorId === currentUserState.user?.id) return;
      if (notificationLevelStore.isRoomMuted(event.spaceId, event.roomId)) return;
      roomsStore.setUnread(event.roomId);
    }
  });

  // Room layout updates from other users/tabs
  useRoomLayoutUpdated(({ spaceId: eventSpaceId }) => {
    if (eventSpaceId === spaceId) void roomsStore.refresh();
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

    const livekitUrl = instanceState.livekitUrl;
    if (livekitUrl) {
      voiceCallState.join(livekitUrl, spaceId, roomId).catch(() => {
        // Silently catch — VoiceCallPanel provides fallback Join button
      });
    }

    goto(resolve('/chat/[instanceId]/(chrome)/[roomId]', { instanceId: instanceSegment, roomId }));
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
    if (target.eventId && target.spaceId && target.roomId) {
      stores.pendingHighlights.set(target.spaceId, target.roomId, target.threadRootId, target.eventId);
    }
    void notificationStore.dismiss(notification.id);

    const path = notificationStore.getCleanPath(getInstanceId(), notification);
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

    const path = notificationStore.getCleanPath(getInstanceId(), notification);
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- path from getCleanPath() is already resolved
    await goto(path);
  }
</script>

{#snippet roomLink(room: SpaceRoom)}
  {@const callParticipants = activeCallRooms.has(room.id) ? activeCallRooms.getParticipants(room.id) : []}
  <a
    href={resolve('/chat/[instanceId]/(chrome)/[roomId]', { instanceId: instanceSegment, roomId: room.id })}
    class={[
      'sidebar-item group/badges',
      callParticipants.length > 0 ? 'flex-wrap gap-y-1' : '',
      room.id === activeRoomId ? 'bg-surface-100' : '',
      room.hasUnread &&
      room.id !== activeRoomId &&
      !notificationLevelStore.isRoomMuted(spaceId, room.id)
        ? 'font-semibold'
        : ''
    ]}
    aria-current={room.id === activeRoomId ? 'page' : undefined}
  >
    <span class="sidebar-icon text-lg text-muted">#</span>
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
    {:else if room.hasUnread && !notificationLevelStore.isRoomMuted(spaceId, room.id)}
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

{#snippet collapsibleGroup(
  groupId: string,
  label: string,
  rooms: SpaceRoom[],
  link: Snippet<[SpaceRoom]>,
  marginTopClass: string = 'mt-4'
)}
  {@const isCollapsed = collapsedSections.has(groupId)}
  <div class={marginTopClass}>
    <button
      type="button"
      onclick={() => toggleSection(groupId)}
      class="hover:text-foreground flex w-full cursor-pointer items-center gap-2 px-3 py-1 text-xs font-semibold tracking-wider text-muted uppercase"
    >
      <span class="sidebar-icon">
        <span
          class={[
            'iconify uil--angle-right-b transition-transform',
            isCollapsed ? '' : 'rotate-90'
          ]}
        ></span>
      </span>
      {label}
    </button>
    <div class="sidebar-nav">
      {#each rooms as room (room.id)}
        {#if !isCollapsed || isHighlighted(room)}
          <div transition:slide={{ duration: 150 }}>
            {@render link(room)}
          </div>
        {/if}
      {/each}
    </div>
  </div>
{/snippet}

{#snippet dmLink(room: SpaceRoom)}
  <a
    href={resolve('/chat/[instanceId]/(chrome)/[roomId]', { instanceId: instanceSegment, roomId: room.id })}
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

<nav class="room-list sidebar-nav p-2 md:w-64">
  {#if roomsStore.layoutSections && roomsStore.layoutSections.length > 0}
    <!-- Sectioned layout -->
    {#each visibleSections as section, i (section.id)}
      {@render collapsibleGroup(
        section.id,
        section.name,
        getSectionRooms(section),
        roomLink,
        i === 0 ? 'mt-4 first:mt-0' : 'mt-4'
      )}
    {/each}

    <!-- Unsectioned rooms (not in any section) -->
    {#if unsectionedRooms.length > 0}
      {@render collapsibleGroup('__unsorted__', 'Other', unsectionedRooms, roomLink)}
    {/if}
  {:else if sortedRooms.length > 0}
    <!-- No layout configured — single collapsible "Rooms" group -->
    {@render collapsibleGroup('__rooms__', 'Rooms', sortedRooms, roomLink, 'mt-4 first:mt-0')}
  {/if}

  {#if dmRooms.length > 0}
    {@render collapsibleGroup('__dms__', 'Direct Messages', dmRooms, dmLink)}
  {/if}
</nav>
