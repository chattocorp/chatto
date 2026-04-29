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
  import { untrack } from 'svelte';
  import { slide } from 'svelte/transition';
  import { useConnection } from '$lib/state/instance/connection.svelte';
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
  import { graphql, useFragment } from './gql';
  import type { PresenceStatus } from '$lib/gql/graphql';
  import UserAvatar, { UserAvatarFragment } from '$lib/components/UserAvatar.svelte';
  import UnreadDot from '$lib/ui/UnreadDot.svelte';
  import { notificationTarget } from '$lib/state/instance/notifications.svelte';

  let {
    spaceId
  }: {
    spaceId: string;
  } = $props();

  const connection = useConnection();
  const getInstanceId = getActiveInstance();
  const instanceSegment = $derived(instanceIdToSegment(getInstanceId()));
  const currentUserState = getCurrentUser();
  const stores = instanceRegistry.getStore(getInstanceId());
  const notificationStore = stores.notifications;
  const notificationLevelStore = stores.notificationLevels;
  const roomUnreadStore = stores.roomUnread;
  const activeCallRooms = stores.activeCallRooms;
  const voiceCallState = stores.voiceCall;
  const instanceState = stores.instance;

  let activeRoomId = $derived(page.params.roomId);

  // --- Room data ---

  type RoomItem = { id: string; name: string; hasUnread: boolean; hasMention: boolean };

  let rooms = $state<RoomItem[]>([]);
  let lastLoadedSpaceId = $state<string | null>(null);

  // --- Layout data ---

  type LayoutSection = { id: string; name: string; roomIds: string[] };
  let layoutSections = $state<LayoutSection[] | null>(null);
  let unsectionedRoomIds = $state<string[]>([]);

  // Collapsed section state (persisted to localStorage)
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

  // --- Data fetching ---

  async function loadRooms() {
    const result = await connection().client
      .query(
        graphql(`
          query GetMyRoomsInSpace($spaceId: ID!) {
            me {
              rooms(spaceId: $spaceId) {
                id
                name
                hasUnread
                hasMention
                archived
                viewerNotificationPreference {
                  level
                  effectiveLevel
                }
              }
            }
            space(id: $spaceId) {
              roomLayout {
                sections {
                  id
                  name
                  rooms {
                    id
                  }
                }
                unsectionedRoomIds
              }
            }
          }
        `),
        { spaceId }
      )
      .toPromise();

    if (result.data?.me) {
      const newRooms = result.data.me.rooms.filter((r) => !r.archived);

      // Keep notification preference store current for this space's rooms
      for (const room of result.data.me.rooms) {
        const pref = room.viewerNotificationPreference;
        if (pref) {
          notificationLevelStore.setRoomPreference(
            spaceId,
            room.id,
            pref.level,
            pref.effectiveLevel
          );
        }
      }

      // Always trust server state for hasUnread/hasMention. The previous OR-merge
      // (server || local) caused permanently stuck unread dots: once a live event
      // set hasUnread to true locally, every loadRooms() call perpetuated it even
      // when the server said false. The brief flash risk (dot disappears during a
      // fetch race with a live event) is negligible since loadRooms() only runs on
      // membership/room-update events, not on every message.
      rooms = newRooms;

      // Sync per-room unread state to the centralized store so space-level
      // dots also reflect the latest server state after room refresh events.
      roomUnreadStore.initSpaceRooms(spaceId, newRooms);
    }

    // Update layout sections and unsorted order
    if (result.data?.space?.roomLayout) {
      layoutSections = result.data.space.roomLayout.sections.map((s) => ({
        id: s.id,
        name: s.name,
        roomIds: s.rooms.map((r) => r.id)
      }));
      unsectionedRoomIds = result.data.space.roomLayout.unsectionedRoomIds;
    } else {
      layoutSections = null;
      unsectionedRoomIds = [];
    }
  }

  // Load rooms on mount and when spaceId changes
  $effect(() => {
    if (spaceId !== lastLoadedSpaceId) {
      lastLoadedSpaceId = spaceId;
      loadCollapsedFromStorage(spaceId);
      loadRooms();

      // Load active call room IDs if LiveKit is configured
      if (instanceState.livekitUrl) {
        activeCallRooms.load(spaceId);
      }
    }
  });

  // Refresh active call state when tab resumes (catches missed live events)
  useTabResumeCallback(() => {
    if (instanceState.livekitUrl && lastLoadedSpaceId) {
      activeCallRooms.load(lastLoadedSpaceId);
    }
  });

  // --- Derived layout helpers ---

  let roomMap = $derived(new Map(rooms.map((r) => [r.id, r])));

  function getSectionRooms(section: LayoutSection): RoomItem[] {
    return section.roomIds.map((id) => roomMap.get(id)).filter((r): r is RoomItem => r != null);
  }

  // Sections that have at least one room the viewer is a member of
  let visibleSections = $derived.by(() => {
    if (!layoutSections) return [];
    return layoutSections.filter((s) => getSectionRooms(s).length > 0);
  });

  // Rooms not assigned to any section, respecting stored order when available
  let unsectionedRooms = $derived.by(() => {
    if (!layoutSections) return [];
    const sectionedIds = new Set(layoutSections.flatMap((s) => s.roomIds));
    const unsectioned = rooms.filter((r) => !sectionedIds.has(r.id));

    if (unsectionedRoomIds.length > 0) {
      const roomMap = new Map(unsectioned.map((r) => [r.id, r]));
      const ordered: RoomItem[] = [];
      // eslint-disable-next-line svelte/prefer-svelte-reactivity -- local computation, not reactive state
      const seen = new Set<string>();
      for (const id of unsectionedRoomIds) {
        const room = roomMap.get(id);
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

  // When no layout exists, display all rooms alphabetically
  let sortedRooms = $derived([...rooms].sort((a, b) => a.name.localeCompare(b.name)));

  // --- Real-time event handlers ---

  // Clear unread and mention status when entering a room
  // (notification dismissal is handled by Room.svelte when it mounts)
  $effect(() => {
    if (activeRoomId) {
      // Use untrack to read rooms without establishing a dependency.
      // Otherwise, writing to rooms[roomIndex] would re-trigger this effect,
      // causing an infinite loop.
      untrack(() => {
        const roomIndex = rooms.findIndex((r) => r.id === activeRoomId);
        if (roomIndex !== -1) {
          // Create new object to trigger Svelte reactivity
          rooms[roomIndex] = { ...rooms[roomIndex], hasUnread: false, hasMention: false };
        }
      });
    }
  });

  // Handle space events for real-time updates
  useSpaceEvent((spaceEvent) => {
    const event = spaceEvent.event;

    // Handle membership changes
    if (event.__typename === 'UserJoinedRoomEvent') {
      loadRooms();
    } else if (event.__typename === 'UserLeftRoomEvent') {
      loadRooms();
      // Navigate away if we left the currently active room
      if (event.roomId === activeRoomId) {
        goto(resolve('/chat/[instanceId]/[spaceId]', { instanceId: instanceSegment, spaceId }));
      }
    }
    // Handle room updates (name/description changes) and archive/unarchive
    else if (
      event.__typename === 'RoomUpdatedEvent' ||
      event.__typename === 'RoomArchivedEvent' ||
      event.__typename === 'RoomUnarchivedEvent'
    ) {
      loadRooms();
    }
    // Handle voice call events — update active call indicators with participant data
    else if (event.__typename === 'CallParticipantJoinedEvent') {
      const actor = spaceEvent.actor ? useFragment(UserAvatarFragment, spaceEvent.actor) : null;
      activeCallRooms.handleJoin(event.spaceId, event.roomId, actor);
    } else if (event.__typename === 'CallParticipantLeftEvent') {
      activeCallRooms.handleLeave(event.spaceId, event.roomId, spaceEvent.actorId);
    }
  });

  // Handle mention notifications - mark room as having mention
  useMention((notification) => {
    // Only update if this mention is in the current space
    if (notification.spaceId !== spaceId) return;
    // Don't update if we're already in the room
    if (notification.roomId === activeRoomId) return;

    const roomIndex = rooms.findIndex((r) => r.id === notification.roomId);
    if (roomIndex !== -1) {
      // Create new object to trigger Svelte reactivity
      rooms[roomIndex] = { ...rooms[roomIndex], hasMention: true };
    }
  });

  // Handle room marked as read from other tabs/devices
  useRoomMarkedAsRead(({ spaceId: eventSpaceId, roomId }) => {
    // Only update if this event is in the current space
    if (eventSpaceId !== spaceId) return;

    const roomIndex = rooms.findIndex((r) => r.id === roomId);
    if (roomIndex !== -1) {
      // Create new object to trigger Svelte reactivity
      rooms[roomIndex] = { ...rooms[roomIndex], hasUnread: false, hasMention: false };
    }
  });

  // Handle new messages via instance events — mark room as having unread.
  // Uses the instance event bus (NewMessageInSpaceEvent) rather than the space event bus
  // (MessagePostedEvent) because it's more reliable for cross-room delivery.
  useInstanceEvent((instanceEvent) => {
    const event = instanceEvent.event;
    if (!event) return;

    if (event.__typename === 'NewMessageInSpaceEvent') {
      if (event.spaceId !== spaceId) return;
      if (event.roomId === activeRoomId) return;
      if (instanceEvent.actorId === currentUserState.user?.id) return;
      if (notificationLevelStore.isRoomMuted(event.spaceId, event.roomId)) return;

      const roomIndex = rooms.findIndex((r) => r.id === event.roomId);
      if (roomIndex !== -1) {
        rooms[roomIndex] = { ...rooms[roomIndex], hasUnread: true };
      }
    }
  });

  // Handle room layout updates from other users/tabs
  useRoomLayoutUpdated(({ spaceId: eventSpaceId }) => {
    if (eventSpaceId === spaceId) {
      loadRooms();
    }
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

    goto(resolve('/chat/[instanceId]/[spaceId]/[roomId]', { instanceId: instanceSegment, spaceId, roomId }));
  }

  // Handle click on room notification dot - navigate to notification source and dismiss
  async function handleRoomNotificationClick(event: MouseEvent, roomId: string) {
    event.preventDefault();
    event.stopPropagation();

    const notification = notificationStore.getRoomNotification(roomId);
    if (!notification) {
      // Clear stuck hasMention state — the dot was visible but no notification
      // exists in the store to dismiss. Clear the local flag so the dot disappears.
      const roomIndex = rooms.findIndex((r) => r.id === roomId);
      if (roomIndex !== -1) {
        rooms[roomIndex] = { ...rooms[roomIndex], hasMention: false };
      }
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
</script>

{#snippet roomLink(room: RoomItem)}
  {@const callParticipants = activeCallRooms.has(room.id) ? activeCallRooms.getParticipants(room.id) : []}
  <a
    href={resolve('/chat/[instanceId]/[spaceId]/[roomId]', { instanceId: instanceSegment, spaceId, roomId: room.id })}
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

<nav class="room-list sidebar-nav p-2 md:w-64">
  {#if layoutSections && layoutSections.length > 0}
    <!-- Sectioned layout -->
    {#each visibleSections as section (section.id)}
      {@const sectionRooms = getSectionRooms(section)}
      {@const isCollapsed = collapsedSections.has(section.id)}
      <div class="mt-4 first:mt-0">
        <button
          type="button"
          onclick={() => toggleSection(section.id)}
          class="hover:text-foreground flex w-full cursor-pointer items-center gap-1 px-2 py-1 text-xs font-semibold tracking-wider text-muted uppercase"
        >
          <span
            class={[
              'iconify text-[10px] transition-transform',
              isCollapsed ? 'uil--angle-right' : 'uil--angle-down'
            ]}
          ></span>
          {section.name}
        </button>
        {#if isCollapsed}
          {@const activeRoom = sectionRooms.find((r) => r.id === activeRoomId)}
          {#if activeRoom}
            {@render roomLink(activeRoom)}
          {/if}
        {:else}
          <div class="sidebar-nav" transition:slide={{ duration: 150 }}>
            {#each sectionRooms as room (room.id)}
              {@render roomLink(room)}
            {/each}
          </div>
        {/if}
      </div>
    {/each}

    <!-- Unsectioned rooms (not in any section) -->
    {#if unsectionedRooms.length > 0}
      {@const isCollapsed = collapsedSections.has('__unsorted__')}
      <div class="mt-4">
        <button
          type="button"
          onclick={() => toggleSection('__unsorted__')}
          class="hover:text-foreground flex w-full cursor-pointer items-center gap-1 px-2 py-1 text-xs font-semibold tracking-wider text-muted uppercase"
        >
          <span
            class={[
              'iconify text-[10px] transition-transform',
              isCollapsed ? 'uil--angle-right' : 'uil--angle-down'
            ]}
          ></span>
          Other
        </button>
        {#if !isCollapsed}
          <div class="sidebar-nav" transition:slide={{ duration: 150 }}>
            {#each unsectionedRooms as room (room.id)}
              {@render roomLink(room)}
            {/each}
          </div>
        {:else}
          {@const activeRoom = unsectionedRooms.find((r) => r.id === activeRoomId)}
          {#if activeRoom}
            <div transition:slide={{ duration: 150 }}>
              {@render roomLink(activeRoom)}
            </div>
          {/if}
        {/if}
      </div>
    {/if}
  {:else}
    <!-- No layout configured — alphabetical flat list -->
    {#each sortedRooms as room (room.id)}
      {@render roomLink(room)}
    {/each}
  {/if}
</nav>
