<script lang="ts">
  import { startDMWith } from '$lib/dm/startDM';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import UserContextMenu from '$lib/components/menus/UserContextMenu.svelte';
  import type { PresenceStatus } from '$lib/gql/graphql';
  import {
    getRoomMembersState,
    getMemberPresence,
    type RoomMember
  } from '$lib/state/room';
  import { getLiveDisplayName, getLiveLogin } from '$lib/state/userProfiles.svelte';
  import { getServerPermissions } from '$lib/state/instance/permissions.svelte';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import ResizeHandle from '$lib/components/ResizeHandle.svelte';
  import { roomInfoWidth } from '$lib/state/roomInfoWidth.svelte';
  import { ROOM_INFO_MAX_WIDTH, ROOM_INFO_MIN_WIDTH } from '$lib/storage/roomInfoWidth';

  const getInstanceId = getActiveInstance();

  let { loading = false }: { loading?: boolean } = $props();

  // Get members from shared store (populated by Room.svelte)
  const membersState = $derived(getRoomMembersState());
  const members = $derived(membersState.members);

  // Check if user can write DMs (from centralized instance permissions)
  const instancePerms = getServerPermissions();
  let canWriteDMs = $derived(instancePerms.current.canWriteDMs);

  // Track which member's popover is open
  let popoverMemberId = $state<string | null>(null);
  let popoverAnchorRect = $state<DOMRect | null>(null);

  function togglePopover(memberId: string, e: MouseEvent) {
    if (popoverMemberId === memberId) {
      popoverMemberId = null;
      popoverAnchorRect = null;
    } else {
      popoverMemberId = memberId;
      const button = (e.target as HTMLElement).closest('button');
      popoverAnchorRect = button?.getBoundingClientRect() ?? null;
    }
  }

  function closePopover() {
    popoverMemberId = null;
    popoverAnchorRect = null;
  }

  // Get effective presence for a member (live update or fall back to initial value)
  function getPresence(member: RoomMember): PresenceStatus {
    return getMemberPresence(member);
  }

  // Check if a presence status counts as "online" (connected to the system)
  function isOnlineStatus(status: PresenceStatus): boolean {
    return status !== 'OFFLINE';
  }

  // Sorted members: online first, then offline, alphabetically by display name within each group
  // Note: We read presenceVersion to ensure $derived re-runs on any presence change.
  // SvelteMap.size only changes when keys are added/removed, not when values change,
  // so it missed updates like OFFLINE→ONLINE for users already in the map.
  const sortedMembers = $derived(
    (membersState.presenceVersion,
    [...members].sort((a, b) => {
      const aOnline = isOnlineStatus(getPresence(a));
      const bOnline = isOnlineStatus(getPresence(b));
      if (aOnline !== bOnline) return aOnline ? -1 : 1;
      return getLiveDisplayName(a.id, a.displayName).localeCompare(
        getLiveDisplayName(b.id, b.displayName)
      );
    }))
  );

  // Count online/offline members (dependency on sortedMembers handles livePresence tracking)
  const onlineCount = $derived(sortedMembers.filter((m) => isOnlineStatus(getPresence(m))).length);
  const offlineCount = $derived(sortedMembers.length - onlineCount);

  // Look up the selected member for the popover (rendered outside the {#each} loop
  // to avoid Svelte reactivity cycles between the popover's $effect and sortedMembers' $derived)
  const popoverMember = $derived(
    popoverMemberId ? (members.find((m) => m.id === popoverMemberId) ?? null) : null
  );
</script>

<aside
  class="relative flex flex-col border-l border-border"
  style:width="{roomInfoWidth.value}px"
  aria-label="Room members"
>
  <ResizeHandle
    width={roomInfoWidth.value}
    min={ROOM_INFO_MIN_WIDTH}
    max={ROOM_INFO_MAX_WIDTH}
    onResize={(w) => roomInfoWidth.set(w)}
    onReset={() => roomInfoWidth.reset()}
    edge="left"
    label="Resize members pane"
  />
  <PaneHeader title="Members ({members.length})" {loading} skeletonButtons={0} />

  <nav class="flex flex-1 flex-col overflow-y-auto p-2" aria-label="Member list">
    {#if loading}
      <ul role="list">
        {#each Array(8) as _, i (i)}
          <li class="flex items-center gap-2 rounded-md px-2 py-1.5">
            <div class="skeleton h-8 w-8 shrink-0 rounded-full"></div>
            <div class="min-w-0 flex-1 space-y-1">
              <div class="skeleton h-3.5 w-24 rounded"></div>
              <div class="skeleton h-3 w-16 rounded"></div>
            </div>
          </li>
        {/each}
      </ul>
    {:else}
      <ul role="list">
        {#each sortedMembers as member, i (member.id)}
          {@const isOnline = isOnlineStatus(getPresence(member))}
          {@const prevMember = sortedMembers[i - 1]}
          {@const isFirstOnline = isOnline && (i === 0 || !isOnlineStatus(getPresence(prevMember)))}
          {@const isFirstOffline =
            !isOnline && (i === 0 || isOnlineStatus(getPresence(prevMember)))}

          {#if isFirstOnline && onlineCount > 0}
            <li
              class="px-2 pb-1 text-xs font-medium tracking-wide text-muted uppercase"
              role="presentation"
            >
              Online ({onlineCount})
            </li>
          {/if}

          {#if isFirstOffline && offlineCount > 0}
            <li
              class={[
                'px-2 pb-1 text-xs font-medium tracking-wide text-muted uppercase',
                i > 0 && 'pt-4'
              ]}
              role="presentation"
            >
              Offline ({offlineCount})
            </li>
          {/if}

          <li>
            <button
              type="button"
              class={['sidebar-item w-full cursor-pointer text-left', !isOnline && 'opacity-50']}
              onclick={(e: MouseEvent) => togglePopover(member.id, e)}
              oncontextmenu={(e: MouseEvent) => {
                e.preventDefault();
                togglePopover(member.id, e);
              }}
              title={`View profile of ${getLiveDisplayName(member.id, member.displayName)}`}
            >
              <UserAvatar user={member} size="sm" />
              <div class="min-w-0 flex-1">
                <div class="truncate">{getLiveDisplayName(member.id, member.displayName)}</div>
                <div class="truncate text-xs text-muted">
                  @{getLiveLogin(member.id, member.login)}
                </div>
              </div>
            </button>
          </li>
        {/each}
      </ul>
    {/if}

    {#if popoverMember && popoverAnchorRect}
      <UserContextMenu
        user={popoverMember}
        anchorRect={popoverAnchorRect}
        canSendMessage={canWriteDMs}
        onSendMessage={() => startDMWith(getInstanceId(), popoverMember!.id)}
        onClose={closePopover}
      />
    {/if}
  </nav>
</aside>
